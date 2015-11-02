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
	server             string
	artifactSearchPath string
	upstreamMonitor    bool
	debug              bool
	ignoreSnapshots    bool
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

// ResourceLimits describes all specified limits
// that should be applied while executing the JobStep.
type ResourceLimits struct {
	// Number of CPUs.
	Cpus *int
	// Memory limit in megabytes.
	Memory *int
}

type Config struct {
	Server             string
	JobstepID          string
	ArtifactSearchPath string
	UpstreamMonitor    bool
	Snapshot           struct {
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
	Cmds             []ConfigCmd `json:"commands"`
	ExpectedSnapshot struct {
		// If this build is expected to generate a snapshot, this is the snapshot ID.
		ID string
	}

	ResourceLimits ResourceLimits

	DebugConfig map[string]*json.RawMessage `json:"debugConfig"`
}

// GetDebugConfig parses the debug config JSON at the given key to dest, returning whether the key
// was present, and if it was, any error that occurred in trying to parse it to dest.
func (c *Config) GetDebugConfig(key string, dest interface{}) (present bool, err error) {
	data, ok := c.DebugConfig[key]
	if !ok || data == nil {
		return false, nil
	}
	e := json.Unmarshal([]byte(*data), dest)
	if e != nil {
		e = fmt.Errorf("Malformed JSON in debug config key %q: %s", key, e)
	}
	return true, e
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

func GetConfig(jobstepID string) (*Config, error) {
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
	conf.ArtifactSearchPath = artifactSearchPath
	conf.UpstreamMonitor = upstreamMonitor
	// deprecated flag
	if debug {
		conf.UpstreamMonitor = false
	}

	if ignoreSnapshots {
		conf.Snapshot.ID = ""
	}
	return conf, nil
}

func init() {
	flag.StringVar(&server, "server", "", "URL to get config from")
	flag.StringVar(&artifactSearchPath, "artifact-search-path", "", "Folder where artifacts will be searched for relative to adapter root")
	flag.BoolVar(&upstreamMonitor, "upstream-monitor", true, "Indicates whether the client should monitor upstream for aborts")
	flag.BoolVar(&debug, "debug", false, "DEPRECATED. debug=true is the same as upstreamMonitor=false.")
	flag.BoolVar(&ignoreSnapshots, "no-snapshots", false, "Ignore any existing snapshots, and build a fresh environment")
}
