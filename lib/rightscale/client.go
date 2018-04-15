package rightscale

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"
	"github.com/pkg/errors"
)

type RequestParams struct {
	method string
	url    string
	body   interface{}
}

type Client struct {
	RefreshToken string
	EndPoint     string
	BearerToken  string
}
//todo, think about not exporting client - https://stackoverflow.com/questions/37135193/how-to-set-default-values-in-golang-structs
func New(refreshToken string, endpoint string) (c Client, e error) {
	c.EndPoint = endpoint
	c.RefreshToken = refreshToken
	bt,err := getBearerToken(refreshToken, endpoint)
	if err != nil {
		return Client{}, errors.Errorf("encountered issue building clinet %s",err)
	}
	c.BearerToken = bt
	//c.validate <- todo
	return
}

func (c Client) Request(RequestParams RequestParams) ([]byte,error) {
	client := http.Client{}
	url := strings.Join([]string{c.EndPoint, RequestParams.url}, "")
	req, err := http.NewRequest(RequestParams.method, url, nil)
	if RequestParams.body != nil {
		j, _ := json.Marshal(RequestParams.body)
		req, err = http.NewRequest(RequestParams.method, url, strings.NewReader(string(j)))
		if err != nil {
			return []byte{}, errors.Errorf("an error was encountered while building request with body %s",err)
		}
	}

	if err != nil {
		return []byte{}, errors.Errorf("an error was encountered while building request %s",err)
	}

	req.Header.Add("X_API_VERSION", "1.5")
	req.Header.Add("Authorization", c.BearerToken)
	req.Header.Add("Content-type", "application/json")
	response, err := client.Do(req)

	if err != nil {
		return []byte{}, errors.Errorf("An error was encountered while performing request to RS %s", err)
	}
	defer response.Body.Close()
	responseBody, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return []byte{}, errors.Errorf("an error was encountered reading response data from RS request %s", err)
	}
	//fmt.Printf("%v",string(responseBody)) Print raw response JSON
	return responseBody,nil
}
