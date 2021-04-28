package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	"github.com/fatih/color"
	"github.com/ilyakaznacheev/cleanenv"
	"github.com/skratchdot/open-golang/open"
)

var (
	conf   *oauth2.Config
	ctx    context.Context
	client *http.Client
)

type AppConfig struct {
	ClientID      string `yaml:"clientID" env:"CLIENT_ID"`
	ClientSecret  string `yaml:"clientSecret" env:"CLIENT_SECRET"`
	TokenFileName string `yaml:"tokenFileName" env:"TOKEN_FILE_NAME" env-default:"token.json"`
}

var cfg AppConfig

// Retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// Saves a token to a file path.
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

func callbackHandler(w http.ResponseWriter, r *http.Request) {
	queryParts, _ := url.ParseQuery(r.URL.RawQuery)

	// Use the authorization code that is pushed to the redirect
	// URL.
	code := queryParts["code"][0]
	log.Printf("code: %s\n", code)

	// Exchange will do the handshake to retrieve the initial access token.
	tok, err := conf.Exchange(ctx, code)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Token: %s", tok)

	saveToken(cfg.TokenFileName, tok)

	// The HTTP Client returned by conf.Client will refresh the token as necessary.
	client = conf.Client(ctx, tok)

	// show succes page
	msg := "<p><strong>Success!</strong></p>"
	msg = msg + "<p>You are authenticated and can now return to the CLI.</p>"
	msg = msg + "<script>close();</script>"

	fmt.Fprintf(w, msg)
}

// Retrieve a token, saves the token, then returns the generated client.
func getClient(scopes []string) *http.Client {
	ctx = context.Background()

	conf = &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Scopes:       scopes,
		Endpoint:     google.Endpoint,
		RedirectURL:  "http://127.0.0.1:9999/oauth/callback",
	}

	// add transport for self-signed certificate to context
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	sslcli := &http.Client{Transport: tr}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, sslcli)

	tok, err := tokenFromFile(cfg.TokenFileName)
	if err == nil {
		client = conf.Client(ctx, tok)
	} else {
		// Redirect user to consent page to ask for permission
		// for the scopes specified above.
		url := conf.AuthCodeURL("state", oauth2.AccessTypeOffline)

		log.Println(color.CyanString("You will now be taken to your browser for authentication"))
		time.Sleep(1 * time.Second)
		open.Run(url)
		time.Sleep(1 * time.Second)
		log.Printf("Authentication URL: %s\n", url)

		http.HandleFunc("/oauth/callback", callbackHandler)
		go func() {
			log.Fatal(http.ListenAndServe(":9999", nil))
		}()
	}

	for client == nil {
		time.Sleep(time.Second)
	}

	return client
}

func readPipedInput() string {
	bigString := ""
	r := bufio.NewReader(os.Stdin)
	buf := make([]byte, 0, 4*1024)

	for {

		n, err := r.Read(buf[:cap(buf)])
		buf = buf[:n]

		if n == 0 {
			if err == nil {
				continue
			}

			if err == io.EOF {
				break
			}

			log.Fatal(err)
		}

		bigString += string(buf)

		if err != nil && err != io.EOF {
			log.Fatalf("Failed reading piped input: %v", err)
		}
	}

	return bigString
}

func csvStringToArray(in string) [][]interface{} {
	out := [][]interface{}{}
	tmpRow := []interface{}{}

	for _, row := range strings.Split(in, "\n") {
		tmpRow = []interface{}{}

		for _, cell := range strings.Split(row, ",") {
			tmpRow = append(tmpRow, cell)
		}

		out = append(out, tmpRow)
	}

	return out
}

func main() {
	err := cleanenv.ReadConfig("config.yml", &cfg)
	if err != nil {
		log.Fatalf("Unable to load config: %v", err)
	}

	pipedInput := readPipedInput()
	rawToArray := csvStringToArray(pipedInput)

	client := getClient([]string{"https://www.googleapis.com/auth/spreadsheets"})

	srv, err := sheets.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		log.Fatalf("Unable to retrieve Sheets client: %v", err)
	}

	// Prints the names and majors of students in a sample spreadsheet:
	// https://docs.google.com/spreadsheets/d/1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgvE2upms/edit
	spreadsheetId := os.Getenv("SHEET_ID")
	sheetTitle := os.Getenv("SHEET_TITLE")
	if sheetTitle == "" {
		sheetTitle = "csv-to-google-sheet " + time.Now().String()
	}

	if spreadsheetId == "" {
		spreadsheet, err := srv.Spreadsheets.Create(&sheets.Spreadsheet{
			Properties: &sheets.SpreadsheetProperties{
				Title: sheetTitle,
			},
		}).Do()
		if err != nil {
			log.Fatalf("Unable to create sheet: %v", err)
		}
		spreadsheetId = spreadsheet.SpreadsheetId
	}

	writeRange := "A1"

	var vr sheets.ValueRange

	vr.Values = rawToArray

	_, err = srv.Spreadsheets.Values.Clear(spreadsheetId, "A1:Z10000", &sheets.ClearValuesRequest{}).Do()
	if err != nil {
		log.Fatalf("Unable to retrieve data from sheet. %v", err)
	}

	_, err = srv.Spreadsheets.Values.Update(spreadsheetId, writeRange, &vr).ValueInputOption("RAW").Do()
	if err != nil {
		log.Fatalf("Unable to retrieve data from sheet. %v", err)
	}
}
