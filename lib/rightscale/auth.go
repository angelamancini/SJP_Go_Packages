package rightscale

import (
	"bytes"
	"encoding/json"
	"github.com/pkg/errors"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
)

// Bearer is a container for Oauth response from authentication endpoint
type Bearer struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

func getBearerToken(refreshToken string, endPoint string) (string, error) {
	data := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refreshToken}}
	client := http.Client{}
	path := strings.Join([]string{endPoint, "/api/oauth2"}, "")
	req, err := http.NewRequest("POST", path, bytes.NewBufferString(data.Encode()))

	if err != nil {
		return "", errors.Errorf("an error was encountered while building bearer token request to RS %s", err)
	}
	req.Header.Add("X_API_VERSION", "1.5")
	req.Header.Add("accept", "json")
	response, err := client.Do(req)

	if err != nil {
		return "", errors.Errorf("An error was encountered retrieving bearer token from RS %s", err)
	}
	defer response.Body.Close()
	ResponseText, err := ioutil.ReadAll(response.Body)
	if err != nil {
		log.Fatalln("An error was encountered reading response data from bearer token request")
	}
	result := Bearer{}
	err = json.Unmarshal([]byte(ResponseText), &result)
	if err != nil {
		return "", errors.Errorf("Could not unmarshal json from oauth call %s", err)
	}
	token := strings.Join([]string{"Bearer", result.AccessToken}, " ")
	return token, nil
}
