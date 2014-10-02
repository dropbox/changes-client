package client

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

var (
	server    string
	jobstepID string
	workspace string
	debug     bool
)

type ConfigCmd struct {
	ID            string
	Script        string
	Env           map[string]string
	Cwd           string
	Artifacts     []string
	CaptureOutput bool
	Type          struct {
		ID string
	}
}

type Config struct {
	Server    string
	JobstepID string
	Workspace string
	Debug     bool
	Snapshot  struct {
		ID string
	}
	Source struct {
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
	var err error

	server = strings.TrimRight(server, "/")

	url := server + "/jobsteps/" + jobstepID + "/"
	conf, err := fetchConfig(url)
	if err != nil {
		return nil, err
	}

	if workspace == "" {
		cwd, err := os.Getwd()
		if err != nil {
			panic("Unable to find working directory")
		}
		workspace, err = filepath.Abs(cwd)
	} else {
		workspace, err = filepath.Abs(workspace)
	}
	if err != nil {
		panic("Unable to find working directory")
	}

	conf.Server = server
	conf.JobstepID = jobstepID
	conf.Workspace = workspace
	conf.Debug = debug

	return conf, err
}

func init() {
	flag.StringVar(&server, "server", "", "URL to get config from")
	flag.StringVar(&jobstepID, "jobstep_id", "", "Job ID whose commands are to be executed")
	flag.StringVar(&workspace, "workspace", "", "Workspace to checkout source into")
	flag.BoolVar(&debug, "debug", false, "Indicates that the client is running in debug mode and should not report results upstream")
}
