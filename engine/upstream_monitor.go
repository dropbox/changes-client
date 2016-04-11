package engine

import (
	"encoding/json"
	"fmt"
	"github.com/dropbox/changes-client/client"
	"hash/fnv"
	"io/ioutil"
	"log"
	"math/rand"
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
	client := &http.Client{}

	h := fnv.New64()
	// seed with JobstepID so each jobstep hits Changes at slightly
	// different times
	h.Write([]byte(um.Config.JobstepID))
	// make our own random generator so that heartbeat variance is
	// unaffected by other interspersed calls to math/rand
	randGen := rand.New(rand.NewSource(int64(h.Sum64())))
	for {
		log.Printf("[upstream] sending heartbeat")

		hr, err := um.postHeartbeat(client)
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

		// vary sleep time by up to 10 seconds to avoid all shards sending
		// heartbeats at the same time
		time.Sleep(time.Duration(20+randGen.Intn(10)) * time.Second)
	}
}

func (um *UpstreamMonitor) postHeartbeat(client *http.Client) (*HeartbeatResponse, error) {
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
		return nil, fmt.Errorf("Request to fetch JobStep failed with status code: %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	r := &JobStep{}
	if err := json.Unmarshal(body, r); err != nil {
		return nil, err
	}

	hr := &HeartbeatResponse{
		Finished: r.Status.ID == STATUS_FINISHED,
		Aborted:  r.Result.ID == RESULT_ABORTED.String(),
	}

	return hr, nil
}
