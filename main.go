// Package hue provides an API for interacting with Hue lights.
//
// It allows individual lights to be controlled, and can download scenes from a
// user's meethue.com account.

package hue

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
)

// Hub represents a Hue hub.
type Hub struct {
	Id         string `json:"id"`
	IpAddress  string `json:"internalipaddress"`
	MacAddress string `json:"macaddress"`
	Name       string `json:"name"`
}

// LightState describes the state of a light.
type LightState struct {
	On         bool      `json:"on"`
	Brightness int       `json:"bri,omitempty"`
	Hue        int       `json:"hue,omitempty"`
	Saturation int       `json:"sat,omitempty"`
	Xy         []float64 `json:"xy,omitempty"`
	Ct         int       `json:"ct,omitempty"`
	Alert      string    `json:"alert,omitempty"`
	Effect     string    `json:"effect,omitempty"`
}

// Light represents a light.
type Light struct {
	Id          string            `json:"-"`
	State       LightState        `json:"state"`
	Type        string            `json:"type"`
	Name        string            `json:"name"`
	Model       string            `json:"modelid"`
	SwVersion   string            `json:"swversion"`
	PointSymbol map[string]string `json:"pointsymbol"`
}

// Scene describes the states of a group of lights.
type Scene struct {
	id     string
	Name   string   `json:"name"`
	Lights []string `json:"lights"`
	Active bool     `json:"active"`
}

// Session is a handle used to interact with a specific hub.
type Session struct {
	ipAddress string
	username  string
}

// GetHubs returns a list of hubs.
// This function uses the meethue.com service for locating hubs.
func GetHubs() ([]Hub, error) {
	var hubs []Hub
	err := restGet("https://www.meethue.com/api/nupnp", &hubs)
	return hubs, err
}

// NewSession creates a new session for a hub.
// This involves registering a new username with the hub. If a username is
// provided it must be between 10 and 40 characters long. If the username is
// empty, the hub will generate a random username.
func NewSession(ipAddress, username string) (session Session, err error) {
	if username != "" && len(username) < 10 {
		return session, errors.New("Username must be at least 10 characters")
	}

	postData := map[string]string{"devicetype": "go-hue user"}
	if username != "" {
		postData["username"] = username
	}

	var data []byte
	if data, err = restPost("http://"+ipAddress+"/api/", postData); err != nil {
		return
	}

	log.Printf("Got from hub: %s", string(data))

	var responses []struct {
		Success struct {
			Username string `json:"username"`
		} `json:"success"`
		Error struct {
			Description string `json:"description"`
		} `json:"error"`
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	if err = dec.Decode(&responses); err != nil {
		return
	}

	response := responses[0]
	if response.Success.Username != "" {
		session = Session{
			ipAddress: ipAddress,
			username:  response.Success.Username,
		}
	} else {
		err = errors.New(response.Error.Description)
	}

	return
}

// OpenSession opens an existing session on a hub.
func OpenSession(ipAddress string, username string) Session {
	return Session{
		ipAddress: ipAddress,
		username:  username,
	}
}

// IpAddress returns the IP address of a session.
func (s *Session) IpAddress() string {
	return s.ipAddress
}

// Username returns the username of a session.
func (s *Session) Username() string {
	return s.username
}

// Url returns the URL a session uses to control a hub.
func (s *Session) Url() string {
	return "http://" + s.ipAddress + "/api/" + s.username
}

// Lights returns a map of the Lights available from session's hub.
func (s *Session) Lights() (map[string]Light, error) {
	var lights map[string]Light
	err := restGet(s.Url()+"/lights", &lights)
	if err == nil {
		for id, light := range lights {
			light.Id = id
			lights[id] = light
		}
	}
	return lights, err
}

// Scenes returns a map of the Scenes available from the session's hub.
func (s *Session) Scenes() (map[string]Scene, error) {
	var scenes map[string]Scene
	err := restGet(s.Url()+"/scenes", &scenes)
	return scenes, err
}

// SetScene sets the scene for group 0.
func (s *Session) SetScene(id string) error {
	data := map[string]string{
		"scene": id}
	resp, err := restPut(s.Url()+"/groups/0/action", &data)
	log.Printf("Response: %#v", resp)
	return err
}

// SetLightState sets the state of a specific light.
func (s *Session) SetLightState(id string, state LightState) error {
	log.Printf("Setting light state to: %#v", state)
	resp, err := restPut(s.Url()+"/lights/"+id+"/state", state)
	log.Printf("Response: %#v", resp)
	return err
}

// support functions ///////////////////////////////////////////////////

type restResponse struct {
	Success map[string]interface{} `json:"success"`
	Error   map[string]interface{} `json:"error"`
}

func restGet(url string, item interface{}) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)

	dec := json.NewDecoder(bytes.NewReader(body))
	err = dec.Decode(item)
	if err != nil {
		return err
	}

	return nil
}

func restSend(url string, data interface{}, method string) ([]byte, error) {
	var body []byte
	var err error

	if data != nil {
		body, err = json.Marshal(data)
		if err != nil {
			return nil, err
		}
	}

	log.Printf(method+"ing to URL %s: %s", url, body)
	req, err := http.NewRequest(method, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	body, _ = ioutil.ReadAll(resp.Body)
	return body, nil
}

func restPost(url string, data interface{}) ([]byte, error) {
	return restSend(url, data, "POST")
}

func restPut(url string, data interface{}) (restResponse, error) {
	body, err := restSend(url, data, "PUT")
	if err != nil {
		return restResponse{}, err
	}

	var messages []restResponse
	dec := json.NewDecoder(bytes.NewReader(body))
	err = dec.Decode(&messages)

	if len(messages) == 0 {
		return restResponse{}, err
	}

	message := messages[0]

	if err != nil {
		return message, err
	}

	if message.Error != nil {
		return message, errors.New(message.Error["description"].(string))
	}

	return message, nil
}
