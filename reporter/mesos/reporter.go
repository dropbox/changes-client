package mesosreporter

import (
	"fmt"
	"log"
	"os/exec"
	"strconv"

	"github.com/dropbox/changes-client/client"
	"github.com/dropbox/changes-client/client/adapter"
	"github.com/dropbox/changes-client/client/reporter"
	"github.com/dropbox/changes-client/common/sentry"
)

// A reporter that connects and reports to a specific jobstep id.
// Each jobstep id has a number of endpoints associated with it that
// allows the reporter to update the status of logs, snapshots, etc.
type Reporter struct {
	reporter.DefaultReporter
	dontPushLogChunks bool
}

func (r *Reporter) Init(c *client.Config) {
	r.dontPushLogChunks = c.GetDebugConfigBool("mesosDontPushLogChunks", false)
	r.DefaultReporter.Init(c)
}

func (r *Reporter) PushJobstepStatus(status string, result string) {
	log.Printf("[reporter] Pushing status %s", status)
	form := make(map[string]string)
	form["status"] = status
	if len(result) > 0 {
		form["result"] = result
	}

	if out, err := exec.Command("/bin/hostname", "-f").Output(); err != nil {
		sentry.Message(fmt.Sprintf("[reporter] Unable to detect hostname: %v", err), map[string]string{})
	} else {
		form["node"] = string(out)
	}
	r.PublishChannel <- reporter.ReportPayload{Path: r.JobstepAPIPath(), Data: form, Filename: ""}
}

func (r *Reporter) PushCommandStatus(cID string, status string, retCode int) {
	form := make(map[string]string)
	form["status"] = status
	if retCode >= 0 {
		form["return_code"] = strconv.Itoa(retCode)
	}
	r.PublishChannel <- reporter.ReportPayload{Path: "/commands/" + cID + "/", Data: form, Filename: ""}
}

func (r *Reporter) PushLogChunk(source string, payload []byte) bool {
	if r.dontPushLogChunks {
		return true
	}
	// logappend endpoint only works for console logs
	if source != "console" {
		return true
	}
	form := make(map[string]string)
	form["source"] = source
	form["text"] = string(payload)
	r.PublishChannel <- reporter.ReportPayload{Path: r.JobstepAPIPath() + "logappend/", Data: form, Filename: ""}
	return true
}

func (r *Reporter) PushCommandOutput(cID string, status string, retCode int, output []byte) {
	form := make(map[string]string)
	form["status"] = status
	form["output"] = string(output)
	if retCode >= 0 {
		form["return_code"] = strconv.Itoa(retCode)
	}
	r.PublishChannel <- reporter.ReportPayload{Path: "/commands/" + cID + "/", Data: form, Filename: ""}
}

func (r *Reporter) PublishArtifacts(cmd client.ConfigCmd, a adapter.Adapter, clientLog *client.Log) error {
	// The artifactstore reporter should handle all artifact publishing, so this does nothing.
	return nil
}

func New() reporter.Reporter {
	return &Reporter{}
}

func init() {
	reporter.Register("mesos", New)
}
