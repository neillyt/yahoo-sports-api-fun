package main

import (
	b64 "encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// the flow:
// - create a new app, you will be issued a client_id and a client_secret.
// - use the client_id and the client_secret to retrieve an oauth code. These are passed in a basic authorization header
//   like so: Authorization: Basic <base64 encoded client_id:client_secret>
// - use the client_id and the client_secret and the code to retrieve an access_token and a refresh_token
// - use the refresh_token to retrieve a new access_token every 1 hour
// - use the access_token to authorize use of the API. access token is passed like so:
//    Authorization Bearer: <access_token>



const (
	// GetTokenURL is a hard coded URL
	GetTokenURL = "https://api.login.yahoo.com/oauth2/get_token"
)

type GetTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// Write writes the token to a file
func (t *GetTokenResponse) Write(filePath string) error {
	// prevent writing an empty token to the file
	if t.RefreshToken == "" || t.AccessToken == "" || t.ExpiresIn == 0 || t.TokenType == "" {
		return fmt.Errorf("token is empty: not writing to file (%+v)", t)
	}

	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}

	// close the file afterwards
	defer f.Close()

	// pack up the token into bytes
	bytes, err := json.Marshal(t)
	if err != nil {
		return err
	}

	fmt.Printf("writing token to %s\n", filePath)

	// write the token response to a file
	_, err = f.Write(bytes)
	if err != nil {
		return err
	}

	return nil
}

// GetCode prints out a link for getting the initial code used for obtaining an oauth token.
// THIS MUST BE DONE VIA BROWSER! this cannot be done programmatically.
func GetCode(clientID string) string {
	if clientID == "" {
		fmt.Printf("please provide a client id, if you do not have one try create-app")
		os.Exit(2)
	}
	return fmt.Sprintf("Click this link to retrieve a code:\nhttps://api.login.yahoo.com/oauth2/request_auth?client_id=%s&redirect_uri=oob&response_type=code\n\n", clientID)
}

// GetToken uses a client id, secret, code and/or refresh token to fetch a new access token
func GetToken(clientID string, clientSecret string, values url.Values) (*GetTokenResponse, error) {
	var tokenResponse *GetTokenResponse
	httpClient := http.DefaultClient

	// create a new request
	request, err := http.NewRequest("POST", GetTokenURL, strings.NewReader(values.Encode()))
	if err != nil {
		return tokenResponse, err
	}

	// concatenate the client_id:client_secret, these will be used in the Authorization header
	// to authorize us to obtain a new oauth token.
	auth := []byte(clientID + ":" + clientSecret)

	// base64 encode the auth credentials
	encodedAuth := b64.StdEncoding.EncodeToString(auth)

	// Set up all the headers
	request.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Add("Authorization", "Basic "+encodedAuth)
	request.Header.Add("Content-Length", strconv.Itoa(len(values.Encode())))

	// send the request
	response, err := httpClient.Do(request)
	if err != nil {
		return tokenResponse, err
	}

	// always close the body
	defer response.Body.Close()

	// read the response body as bytes
	bytes, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return tokenResponse, err
	}

	// unmarshal the response to the GetTokenResponse type
	json.Unmarshal(bytes, &tokenResponse)

	// return the response and no errors
	return tokenResponse, nil
}

// LoadToken loads the token from the token file
func LoadToken(filePath string) (*GetTokenResponse, error) {
	var token *GetTokenResponse

	_, err := os.Stat(filePath)
	if err != nil {
		return token, fmt.Errorf("does the file (%s) exist? %v", filePath, err)
	}

	f, err := os.Open(filePath)
	if err != nil {
		return token, err
	}

	// make sure the file is closed afterwards
	defer f.Close()

	bytes, err := ioutil.ReadAll(f)
	if err != nil {
		return token, err
	}

	err = json.Unmarshal(bytes, &token)
	return token, err
}

func lintArgs(clientID string, clientSecret string, clientCode string, checkCode bool) {
	if clientID == "" || clientSecret == "" {
		fmt.Printf("please provide a client id and a client secret")
		os.Exit(2)
	}

	if checkCode && clientCode == "" {
		fmt.Printf("please provide a code to obtain the secret with (did you run get-code?)")
		os.Exit(2)
	}
}

// help spits out a help message and exits 2
func help() {
	help := `expected some command. available commands:
  create-app
  get-code
  get-token
  refresh-token
  show-token`
	fmt.Println(help)
	os.Exit(2)
}

// checkEnv checks if an environment variable is set and returns the value of it if it is
func checkEnv(env string) string {
	e, exists := os.LookupEnv(env)
	if exists {
		return e
	}

	return ""
}

// let's begin!
func main() {

	var (
		clientCode   string
		clientID     string
		clientSecret string
		tokenFile    string
	)

	// get-token flags
	getCmd := flag.NewFlagSet("get-token", flag.ExitOnError)
	getCmd.StringVar(&clientID, "id", checkEnv("YAHOO_APP_CLIENT_ID"), "client ID")
	getCmd.StringVar(&clientCode, "code", checkEnv("YAHOO_APP_CLIENT_CODE"), "client ID")
	getCmd.StringVar(&clientSecret, "secret", checkEnv("YAHOO_APP_CLIENT_SECRET"), "client secret to obtain oauth token")
	getCmd.StringVar(&tokenFile, "file", checkEnv("YAHOO_APP_TOKEN_FILE"), "file to write the token to")

	// refresh-token flags
	refreshCmd := flag.NewFlagSet("refresh-token", flag.ExitOnError)
	refreshCmd.StringVar(&clientID, "id", checkEnv("YAHOO_APP_CLIENT_ID"), "client ID")
	refreshCmd.StringVar(&clientSecret, "secret", checkEnv("YAHOO_APP_CLIENT_SECRET"), "client secret to obtain oauth token")
	refreshCmd.StringVar(&tokenFile, "file", checkEnv("YAHOO_APP_TOKEN_FILE"), "file to write the token to")

	// get-code flags
	getCodeCmd := flag.NewFlagSet("get-code", flag.ExitOnError)
	getCodeCmd.StringVar(&clientID, "id", checkEnv("YAHOO_APP_CLIENT_ID"), "client ID")

	// make sure we have at least a subcommand argument
	if len(os.Args) < 2 {
		help()
	}

	// switch through our subcommand argument
	switch os.Args[1] {
	case "create-app":
		fmt.Printf("To create a new Yahoo API app, visit: https://developer.yahoo.com/apps/create/\n\n")
	case "get-code":
		getCodeCmd.Parse(os.Args[2:])
		fmt.Printf(GetCode(clientID))
	case "get-token":
		getCmd.Parse(os.Args[2:])
		lintArgs(clientID, clientSecret, clientCode, true)
		// set our form values for obtaining a new token
		values := url.Values{}
		values.Set("client_id", clientID)
		values.Set("client_secret", clientSecret)
		values.Set("code", clientCode)
		values.Set("grant_type", "authorization_code")
		values.Set("redirect_uri", "oob")

		token, err := GetToken(clientID, clientSecret, values)
		if err != nil {
			fmt.Printf("error: %v", err)
			return
		}

		err = token.Write(tokenFile)
		if err != nil {
			fmt.Printf("error writing token: %v", err)
			return
		}

	case "refresh-token":
		refreshCmd.Parse(os.Args[2:])
		lintArgs(clientID, clientSecret, clientCode, false)
		oldToken, err := LoadToken(tokenFile)
		if err != nil {
			fmt.Printf( "error loading token: %v", err)
			return
		}

		// set our form values for obtaining a new token
		values := url.Values{}
		values.Set("client_id", clientID)
		values.Set("client_secret", clientSecret)
		values.Set("grant_type", "refresh_token")
		values.Set("redirect_uri", "oob")
		values.Set("refresh_token", oldToken.RefreshToken)

		token, err := GetToken(clientID, clientSecret, values)
		if err != nil {
			fmt.Printf("error obtaining token: %v", err)
			return
		}

		err = token.Write(tokenFile)
		if err != nil {
			fmt.Printf("error writing token: %v", err)
			return
		}
	case "show-token":
		token, err := LoadToken(tokenFile)
		if err != nil {
			fmt.Printf("error loading token: %v", err)
			return
		}

		fmt.Printf("%s", token.AccessToken)
	default:
		help()
	}
}
