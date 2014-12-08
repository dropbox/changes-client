package reporter

import (
	"log"
	"os"
	"strconv"
)

type JobStepReporter struct {
	jobstepID  string
	httpStream *HttpStream
	debug      bool
	hostname   string
}

func NewJobStepReporter(publishUri string, jobstepID string, debug bool) *JobStepReporter {
	log.Printf("[reporter] Constructing jobstep reporter with publish uri: %s", publishUri)

	hostname, err := os.Hostname()
	if err != nil {
		hostname = ""
	}

	r := &JobStepReporter{
		jobstepID:  jobstepID,
		httpStream: NewHttpStream(publishUri, debug),
		debug:      debug,
		hostname:   hostname,
	}

	return r
}

func (r *JobStepReporter) PushBuildStatus(status string, result string) {
	form := make(map[string]string)
	if status != "" {
		form["status"] = status
	}
	if result != "" {
		form["result"] = result
	}
	if r.hostname != "" {
		form["node"] = r.hostname
	}
	r.httpStream.Push(HttpPayload{"/jobsteps/" + r.jobstepID + "/", form, ""})
}

func (r *JobStepReporter) PushCommandStatus(cID string, status string, retCode int) {
	form := make(map[string]string)
	if status != "" {
		form["status"] = status
	}
	if retCode >= 0 {
		form["return_code"] = strconv.Itoa(retCode)
	}
	r.httpStream.Push(HttpPayload{"/commands/" + cID + "/", form, ""})
}

func (r *JobStepReporter) PushLogChunk(source string, payload []byte) {
	form := make(map[string]string)
	form["source"] = source
	form["text"] = string(payload)
	if r.debug {
		log.Print(string(payload))
	}
	r.httpStream.Push(HttpPayload{"/jobsteps/" + r.jobstepID + "/logappend/", form, ""})
}

func (r *JobStepReporter) PushCommandOutput(cID string, status string, retCode int, output []byte) {
	form := make(map[string]string)
	form["status"] = status
	form["output"] = string(output)
	if retCode >= 0 {
		form["return_code"] = strconv.Itoa(retCode)
	}
	r.httpStream.Push(HttpPayload{"/commands/" + cID + "/", form, ""})
}

func (r *JobStepReporter) PushArtifacts(artifacts []string) {
	// TODO: PushArtifacts is synchronous due to races with Adapter.Shutdown(), but
	// really what we'd want to do is just say "wait until channel empty, ok continue"
	for _, artifact := range artifacts {
		r.httpStream.PushSync(HttpPayload{"/jobsteps/" + r.jobstepID + "/artifacts/", nil, artifact})
	}
}

func (r *JobStepReporter) Shutdown() {
	r.httpStream.Shutdown()
}
