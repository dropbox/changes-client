package runner

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
)

var (
	conf string
)

type Config struct {
	ApiUri string `json:"api-uri"`
	Cmds   []struct {
		Id        string            `json:"id"`
		Script    string            `json:"script"`
		Env       map[string]string `json:"env"`
		Cwd       string            `json:"cwd"`
		Artifacts []string          `json:"artifacts"`
	} `json:"cmds"`
}

func parseConfig(filename string) (*Config, error) {
	conf, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	r := &Config{}
	err = json.Unmarshal(conf, r)
	if err != nil {
		return nil, err
	}

	return r, nil
}

func fetchConfig(url string) (*Config, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		err := fmt.Errorf("Request to fetch config failed with status code: %d", resp.StatusCode)
		return nil, err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	r := &Config{}
	err = json.Unmarshal(body, r)
	if err != nil {
		return nil, err
	}

	return r, nil
}

func GetConfig() (*Config, error) {
	url, err := url.Parse(conf)
	if err != nil {
		return nil, err
	}

	if !url.IsAbs() || url.Scheme == "file" {
		return parseConfig(url.Path)
	} else if url.Scheme == "http" {
		return fetchConfig(conf)
	}

	err = fmt.Errorf("Unrecognized path: %s", conf)
	return nil, err
}

func init() {
	flag.StringVar(&conf, "conf", "", "URL to get config from")
}
