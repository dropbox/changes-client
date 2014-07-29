package runner

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
)

var (
	server    string
	jobstepID string
)

type ConfigCmd struct {
	Id        string
	Script    string
	Env       map[string]string
	Cwd       string
	Artifacts []string
}

type Config struct {
	Server    string
	JobstepID string
	Source    struct {
		Revision struct {
			Sha string
		}
		Patch struct {
			ID string
		}
	}
	Repository struct {
		URL     string
		Backend struct {
			ID string
		}
	}
	Cmds []ConfigCmd `json:"commands"`
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

	return LoadConfig(body)
}

func LoadConfig(content []byte) (*Config, error) {
	r := &Config{}
	err := json.Unmarshal(content, r)
	if err != nil {
		return nil, err
	}

	return r, nil
}

func GetConfig() (*Config, error) {
	url := server + "/jobsteps/" + jobstepID
	conf, err := fetchConfig(url)
	if err != nil {
		return nil, err
	}

	conf.Server = server
	conf.JobstepID = jobstepID
	return conf, err
}

func init() {
	flag.StringVar(&server, "server", "", "URL to get config from")
	flag.StringVar(&jobstepID, "jobstep_id", "", "Job ID whose commands are to be executed")
}
