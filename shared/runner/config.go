package runner

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
)

var (
	server          string
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
	// TODO(dcramer): the ID is overloaded and represents either a SnapshotID or a JobStepID
	ID string
	Server    string
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
	conf := &Config{}
	err := json.Unmarshal(content, conf)
	if err != nil {
		return nil, err
	}

	conf.Server = server
	conf.Workspace = workspace
	conf.Debug = debug

	if ignoreSnapshots {
		conf.Snapshot.ID = ""
	}

	return conf, nil
}

func GetJobStepConfig(jobstepID string) (*Config, error) {
	var err error

	if server == "" {
		return nil, fmt.Errorf("Missing required configuration: server")
	}

	server = strings.TrimRight(server, "/")

	url := server + "/jobsteps/" + jobstepID + "/"
	conf, err := fetchConfig(url)
	if err != nil {
		return nil, err
	}

	return conf, err
}

func GetSnapshotConfig(snapshotID string) (*Config, error) {
	var err error

	if server == "" {
		return nil, fmt.Errorf("Missing required configuration: server")
	}

	server = strings.TrimRight(server, "/")

	url := server + "/snapshotimages/" + snapshotID + "/"
	conf, err := fetchConfig(url)
	if err != nil {
		return nil, err
	}

	return conf, err
}

func init() {
	flag.StringVar(&server, "server", "", "URL to get config from")
	flag.StringVar(&workspace, "workspace", "", "Workspace to checkout source into")
	flag.BoolVar(&debug, "debug", false, "Indicates that the client is running in debug mode and should not report results upstream")
	flag.BoolVar(&ignoreSnapshots, "no-snapshots", false, "Ignore any existing snapshots, and build a fresh environment")
}
