package hue

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
)

// MeetHueSession represents a session with meethue.com.
type MeetHueSession struct {
	token string
}

// MeetHueLightState describes the state of a light.
type MeetHueLightState struct {
	LightState
	Id    string `json:"id"`
	Color *struct {
		Red   float64 `json:"red"`
		Green float64 `json:"green"`
		Blue  float64 `json:"blue"`
	} `json:"color"`
}

// MeetHueScene describes the states of a group of lights.
type MeetHueScene struct {
	Id       string              `json:"id"`
	Lights   []MeetHueLightState `json:"lights"`
	Name     string              `json:"name"`
	Version  string              `json:"version"`
	Recipe   string              `json:"lightrecipe"`
	Category string
}

// GetMeetHueToken logs into meethue.com using the provided username and
// password and returns the user's API token.
// The API token can be used to access the user's meethue.com account.
func GetMeetHueToken(username, password string) (string, error) {
	jar, _ := cookiejar.New(nil)
	client := http.Client{Jar: jar}
	resp, err := client.Get("https://www.meethue.com/en-us/api/gettoken?deviceid=Browser&devicename=Safari&appid=myhue")
	if err != nil {
		return "", err
	}

	values := url.Values{}
	values.Set("email", username)
	values.Set("password", password)

	resp, err = client.PostForm("https://www.meethue.com/en-us/api/getaccesstokengivepermission", values)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)

	text := string(body)
	start := strings.Index(text, "https://my.meethue.com/?token=")
	tokenUrl := text[start:]
	end := strings.Index(tokenUrl, "\"")
	tokenUrl = tokenUrl[:end]
	log.Printf("token url: %s\n", tokenUrl)

	req, err := http.NewRequest("GET", tokenUrl, nil)
	req.Header.Add("accept-language", "en-US,en;q=0.8")

	resp, err = client.Do(req)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()
	body, _ = ioutil.ReadAll(resp.Body)

	parsedUrl, _ := url.Parse(tokenUrl)
	cookies := jar.Cookies(parsedUrl)

	for _, cookie := range cookies {
		if cookie.Name == "myhueapi" {
			return cookie.Value, nil
		}
	}

	return "", fmt.Errorf("Did not receive token cookie")
}

// OpenMeetHueSession opens an existing session at meethue.com.
func OpenMeetHueSession(apiToken string) MeetHueSession {
	return MeetHueSession{apiToken}
}

// ToLightState returns an equivalent LightState for a MeetHueLightState.
func (l *MeetHueLightState) ToLightState() LightState {
	ls := l.LightState
	if l.Color != nil {
		r := l.Color.Red
		g := l.Color.Green
		b := l.Color.Blue
		x, y := toXy(r, g, b)
		ls.Xy = []float64{x, y}
	}

	return ls
}

// GetMeetHueScenes returns the scenes available in a user's meethue.com account.
func (s *MeetHueSession) GetMeetHueScenes() ([]MeetHueScene, error) {
	var page struct {
		Pagination struct {
			Amount  int  `json:"amountperpage"`
			HasNext bool `json:"hasnextpage"`
			Page    int  `json:"page"`
		} `json:"pagination"`
		TotalAmount string `json:"totalamount"`
		Scenes      []struct {
			Category string       `json:"category"`
			Json     MeetHueScene `json:"json"`
		} `json:"scenes"`
	}
	var scenes []MeetHueScene
	pageNum := 1

	for {
		sceneUrl := fmt.Sprintf("https://my.meethue.com/api/v3/myhue/scenes/?token=%s&page=%v", s.token, pageNum)
		resp, err := http.Get(sceneUrl)
		if err != nil {
			return nil, err
		}

		defer resp.Body.Close()
		body, _ := ioutil.ReadAll(resp.Body)

		dec := json.NewDecoder(bytes.NewReader(body))
		err = dec.Decode(&page)
		if err != nil {
			return nil, err
		}

		for _, scene := range page.Scenes {
			scene.Json.Category = scene.Category
			scenes = append(scenes, scene.Json)
		}

		if page.Pagination.HasNext {
			break
		}

		pageNum++
	}

	return scenes, nil
}
