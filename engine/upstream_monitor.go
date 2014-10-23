package engine

import (
	"encoding/json"
	"fmt"
	"github.com/dropbox/changes-client/client"
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

type JobStep struct {
	Status struct {
		ID string
	}
	Result struct {
		ID string
	}
}

type UpstreamMonitor struct {
	Config    *client.Config
}

func (um *UpstreamMonitor) WaitUntilAbort() error {
	var (
		err error
		js *JobStep
	)

	for {
		js, err = um.fetchJobStep()
		if err != nil {
			log.Printf("[upstream] %s", err)
		} else if js.Status.ID == STATUS_FINISHED {
			if js.Result.ID == RESULT_ABORTED {
				return nil
			} else {
				return fmt.Errorf("JobStep marked as finished, but not aborted.")
			}
		}

		time.Sleep(3 * time.Second)
	}

	return fmt.Errorf("How did we get here?")
}

func (um *UpstreamMonitor) fetchJobStep() (*JobStep, error) {
	var err error

	url := um.Config.Server + "/jobsteps/" + um.Config.JobstepID + "/"

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		err = fmt.Errorf("Request to fetch JobStep failed with status code: %d", resp.StatusCode)
		return nil, err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	r := &JobStep{}
	err = json.Unmarshal(body, r)
	if err != nil {
		return nil, err
	}

	return r, nil
}
