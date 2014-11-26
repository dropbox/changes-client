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
	Config *client.Config
}

type HeartbeatResponse struct {
	Finished bool
	Aborted  bool
}

func (um *UpstreamMonitor) WaitUntilAbort() error {
	var (
		err error
		hr  *HeartbeatResponse
	)

	client := &http.Client{}

	for {
		log.Printf("[upstream] sending heartbeat")

		hr, err = um.postHeartbeat(client)
		if err != nil {
			log.Printf("[upstream] %s", err)
		} else if hr.Finished {
			if hr.Aborted {
				log.Print("[upstream] JobStep was aborted")
				return nil
			} else {
				log.Print("[upstream] WARNING: JobStep marked as finished, but not aborted")
				return fmt.Errorf("JobStep marked as finished, but not aborted.")
			}
		}

		time.Sleep(3 * time.Second)
	}

	return fmt.Errorf("How did we get here?")
}

func (um *UpstreamMonitor) postHeartbeat(client *http.Client) (*HeartbeatResponse, error) {
	var err error

	url := um.Config.Server + "/jobsteps/" + um.Config.JobstepID + "/heartbeat/"

	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 410 {
		return &HeartbeatResponse{
			Finished: true,
			Aborted:  true,
		}, nil
	}

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

	hr := &HeartbeatResponse{
		Finished: r.Status.ID == STATUS_FINISHED,
		Aborted:  r.Result.ID == RESULT_ABORTED,
	}

	return hr, nil
}
