# What is this?
A Golang CLI to convert CSV input to a Google Sheet

# Prerequisites
There's a `config.yml` file which will need a ClientID & ClientSecret which is enabled for Google Sheets API calls. The ClientID & ClientSecret can be setup by any user (and doesn't have to be associated to the org where the data resides).
Once these are put into the config file, the CLI uses your web browser and an OAuth flow to authenticate to wherever you wish to store the sheet data.
```
cp config.yml.sample config.yml
```

# Show me an example (creates a new sheet)
This will make you 
```
echo "Exported At:,$(date -u)
a2,b2,c2
a3,b3,c3" | go run main.go 
```

# Show me an example (clears and updates and existing sheet)
```
echo "Exported At:,$(date -u)
a2,b2,c2
a3,b3,c3" | SHEET_ID='deadbeef-some-big-long-sheet-id-goes-here' go run main.go 
```