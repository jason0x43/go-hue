// Package hue provides an API for interacting with Hue lights.
//
// It allows individual lights to be controlled, and can download scenes from a
// user's meethue.com account.
package hue

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

// Hub represents a Hue hub.
type Hub struct {
	ID         string `json:"id"`
	IPAddress  string `json:"internalipaddress"`
	MacAddress string `json:"macaddress"`
	Name       string `json:"name"`
}

func (h Hub) String() string {
	return h.IPAddress
}

// LightState describes the state of a light.
type LightState struct {
	On         bool       `json:"on"`
	Brightness int        `json:"bri,omitempty"`
	Hue        int        `json:"hue,omitempty"`
	Saturation int        `json:"sat,omitempty"`
	Xy         [2]float64 `json:"xy,omitempty"`
	Ct         int        `json:"ct,omitempty"`
	Alert      string     `json:"alert,omitempty"`
	Effect     string     `json:"effect,omitempty"`
	ColorMode  string     `json:"colormode,omitempty"`
}

// Light represents a light.
type Light struct {
	hueLight
	ID string
}

type hueLight struct {
	State     LightState `json:"state"`
	Type      string     `json:"type"`
	Name      string     `json:"name"`
	Model     string     `json:"modelid"`
	SwVersion string     `json:"swversion"`
}

func (l *Light) String() string {
	return fmt.Sprintf("[%s] %v", l.ID, l.Name)
}

// GetColorRGB returns a light's color as an RGB value
func (l *Light) GetColorRGB() (int, int, int) {
	gamut := GetGamut(l.Model)
	state := l.State
	r, g, b := gamut.ToRGB(state.Xy[0], state.Xy[1], float64(state.Brightness)/255.0)
	log.Printf("XyY(%f, %f, %f) -> RGB(%d, %d, %d)", state.Xy[0], state.Xy[1], float64(state.Brightness)/255.0, r, g, b)
	return r, g, b
}

// SetColorRGB sets a light's color from an RGB value
func (l *Light) SetColorRGB(r, g, b int) (err error) {
	gamut := GetGamut(l.Model)
	x, y, Y := gamut.ToXyY(r, g, b)
	l.State.Xy = [2]float64{x, y}
	l.State.Brightness = int(math.Ceil(Y*255.0 - 0.5))
	log.Printf("RGB(%d, %d, %d) -> XyY(%f, %f, %f) [%d]", r, g, b, x, y, Y, l.State.Brightness)
	return
}

// SetColorHex sets a light's color from an RGB hex string
func (l *Light) SetColorHex(hex string) (err error) {
	if matched, err := regexp.MatchString("#?[a-fA-F0-9]{6}", hex); !matched || err != nil {
		if err != nil {
			return err
		}
		return fmt.Errorf("Invalid color string '%s'", hex)
	}

	hex = strings.TrimPrefix(hex, "#")
	r, _ := strconv.ParseInt(hex[0:2], 16, 0)
	g, _ := strconv.ParseInt(hex[2:4], 16, 0)
	b, _ := strconv.ParseInt(hex[4:6], 16, 0)

	return l.SetColorRGB(int(r), int(g), int(b))
}

// GetColorHSL returns a light's color as an HSL value
func (l *Light) GetColorHSL() (float64, float64, float64) {
	gamut := GetGamut(l.Model)
	state := l.State
	return gamut.ToHSL(state.Xy[0], state.Xy[1], float64(state.Brightness)/255.0)
}

// SetColorHSL sets a light's color from an HSL value
func (l *Light) SetColorHSL(h, s, bri float64) (err error) {
	return
}

// ByID is a Light array used for sorting
type ByID []Light

func (b ByID) Len() int           { return len(b) }
func (b ByID) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b ByID) Less(i, j int) bool { return b[i].ID < b[j].ID }

// Scene describes the states of a group of lights.
type Scene struct {
	hueScene
	ID        string
	ShortName string
}

type hueScene struct {
	Name        string   `json:"name"`
	Owner       string   `json:"owner"`
	LastUpdated string   `json:"lastupdated"`
	Lights      []string `json:"lights"`
	Version     int      `json:"version"`
}

func (s Scene) String() string {
	return fmt.Sprintf("%s [%s]", s.Name, strings.Join(s.Lights, ", "))
}

// ByName is a Scene array used for sorting
type ByName []Scene

func (b ByName) Len() int           { return len(b) }
func (b ByName) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b ByName) Less(i, j int) bool { return b[i].Name < b[j].Name }

// Group represents a group of lights.
type Group struct {
	hueGroup
	ID string
}

type hueGroup struct {
	Name   string     `json:"name"`
	Lights []string   `json:"lights"`
	Type   string     `json:"type"`
	State  LightState `json:"action"`
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

// IPAddress returns the IP address of a session.
func (s *Session) IPAddress() string {
	return s.ipAddress
}

// Username returns the username of a session.
func (s *Session) Username() string {
	return s.username
}

// URL returns the URL a session uses to control a hub.
func (s *Session) URL() string {
	return "http://" + s.ipAddress + "/api/" + s.username
}

// Lights returns a map of the Lights available from session's hub.
func (s *Session) Lights() (lights map[string]Light, err error) {
	if err = restGet(s.URL()+"/lights", &lights); err != nil {
		return
	}
	for id, light := range lights {
		light.ID = id
		lights[id] = light
	}
	return
}

// Scenes returns a map of the Scenes available from the session's hub.
func (s *Session) Scenes() (scenes map[string]Scene, err error) {
	if err = restGet(s.URL()+"/scenes", &scenes); err != nil {
		return
	}
	re, _ := regexp.Compile("\\son\\s\\d+$")
	for id, scene := range scenes {
		scene.ShortName = re.ReplaceAllString(scene.Name, "")
		scene.ID = id
		scenes[id] = scene
	}
	return
}

// Groups returns a map of the Groups available from the session's hub.
func (s *Session) Groups() (groups map[string]Group, err error) {
	if err = restGet(s.URL()+"/groups", &groups); err != nil {
		return
	}
	for id, group := range groups {
		group.ID = id
		groups[id] = group
	}
	return
}

// SetScene sets the scene for group 0.
func (s *Session) SetScene(id string) error {
	data := map[string]string{"scene": id}
	resp, err := restPut(s.URL()+"/groups/0/action", &data)
	log.Printf("Response: %#v", resp)
	return err
}

// SetLightState sets the state of a specific light.
func (s *Session) SetLightState(id string, state LightState) error {
	// clear the colormode before posting
	state.ColorMode = ""
	log.Printf("Setting light state to: %#v", state)
	resp, err := restPut(s.URL()+"/lights/"+id+"/state", state)
	log.Printf("Response: %#v", resp)
	return err
}

// SetLightName sets the name of a specific light.
func (s *Session) SetLightName(id string, name string) error {
	log.Printf("Setting light name to: %#v", name)
	data := map[string]string{"name": name}
	resp, err := restPut(s.URL()+"/lights/"+id, &data)
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
