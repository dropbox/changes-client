package client

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"
)

var (
	server          string
	jobstepID       string
	workspace       string
	debug           bool
	ignoreSnapshots bool
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
	Project struct {
		Name string
		Slug string
	}
	Cmds []ConfigCmd `json:"commands"`
}

// Duration is in nanoseconds and is multiplied by 2 on each retry
//
// We need to retry because there is a race condition in interactions
// with Changes where the jenkins job is created before the jobstep
// in Changes. This probably only occurs when there is a long running
// transaction. We don't want to delay too much, so we start with a small
// delay in case the jenkins job just got started very quickly, but then we delay
// longer between each retry in case we have to wait for some long transaction
// to occur.
//
// NOTE: Due to the nature of this race condition we only retry on 404s.
func fetchConfig(url string, retries int, retryDelay time.Duration) (*Config, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		// The race condition ends up giving us a 404. If we got anything else, its
		// a real error and we shouldn't bother retrying.
		if retries == 0 || resp.StatusCode != 404 {
			err := fmt.Errorf("Request to fetch config failed with status code: %d", resp.StatusCode)
			return nil, err
		} else {
			log.Printf("Failed to fetch configuration (404). Retries left: %d", retries)
			time.Sleep(retryDelay)
			return fetchConfig(url, retries-1, retryDelay*2)
		}
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

	if server == "" {
		return nil, fmt.Errorf("Missing required configuration: server")
	}

	if jobstepID == "" {
		return nil, fmt.Errorf("Missing required configuration: jobstep_id")
	}

	server = strings.TrimRight(server, "/")

	url := server + "/jobsteps/" + jobstepID + "/"
	conf, err := fetchConfig(url, 8, 250*time.Millisecond)
	if err != nil {
		return nil, err
	}

	conf.Server = server
	conf.JobstepID = jobstepID
	conf.Workspace = workspace
	conf.Debug = debug

	if ignoreSnapshots {
		conf.Snapshot.ID = ""
	}

	return conf, err
}

func init() {
	flag.StringVar(&server, "server", "", "URL to get config from")
	flag.StringVar(&jobstepID, "jobstep_id", "", "Job ID whose commands are to be executed")
	flag.StringVar(&workspace, "workspace", "", "Workspace to checkout source into")
	flag.BoolVar(&debug, "debug", false, "Indicates that the client is running in debug mode and should not report results upstream")
	flag.BoolVar(&ignoreSnapshots, "no-snapshots", false, "Ignore any existing snapshots, and build a fresh environment")
}
