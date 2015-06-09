package mesosreporter

import (
	"bytes"
	"flag"
	"fmt"
	"github.com/dropbox/changes-client/client"
	"github.com/dropbox/changes-client/client/adapter"
	"github.com/dropbox/changes-client/client/reporter"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

var (
	maxPendingReports = 64
	numPublishRetries = 8
	backoffTimeMs     = 1000
)

type ReportPayload struct {
	path     string
	data     map[string]string
	filename string
}

type Reporter struct {
	jobstepID       string
	publishUri      string
	publishChannel  chan ReportPayload
	shutdownChannel chan struct{}
	debug           bool
}

func httpPost(uri string, params map[string]string, file string) (resp *http.Response, err error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	for key, val := range params {
		err = writer.WriteField(key, val)
		if err != nil {
			log.Printf("[reporter] Couldn't write field %s", key)
			return nil, err
		}
	}

	if len(file) > 0 {
		handle, err := os.Open(file)
		if err != nil {
			return nil, err
		}

		err = writer.WriteField("name", filepath.Base(file))
		if err != nil {
			return nil, err
		}

		fileField, err := writer.CreateFormFile("file", filepath.Base(file))
		if err != nil {
			return nil, err
		}

		_, err = io.Copy(fileField, handle)
		if err != nil {
			return nil, err
		}
	}

	_ = writer.Close()

	resp, err = http.Post(uri, writer.FormDataContentType(), body)

	if err != nil {
		return nil, err
	}

	// Close the Body channel immediately as we don't use it
	// and this loop can stay open for an extremely long period
	// of time
	resp.Body.Close()

	return resp, nil
}

func sendPayload(r *Reporter, rp ReportPayload) {
	var (
		resp   *http.Response
		err    error
		status string
	)

	path := r.publishUri + rp.path
	if rp.data == nil {
		rp.data = make(map[string]string)
	}

	rp.data["date"] = time.Now().UTC().Format("2006-01-02T15:04:05.0Z")
	for tryCnt := 1; tryCnt <= numPublishRetries; tryCnt++ {
		log.Printf("[reporter] POST %s try: %d", path, tryCnt)
		resp, err = httpPost(path, rp.data, rp.filename)

		if resp != nil {
			status = resp.Status
		} else {
			status = "-1"
		}

		if resp != nil && resp.StatusCode/100 == 2 {
			break
		}

		log.Printf("[reporter] POST %s failed, try: %d, resp: %s, err: %s",
			path, tryCnt, status, err)

		/* We are unable to publish to the endpoint.
		 * Fail fast and let the above layers handle the outage */
		if tryCnt == numPublishRetries {
			panic("Couldn't to connect to publish endpoint")
		}
		log.Printf("[reporter] Sleep for %d ms", backoffTimeMs)
		time.Sleep(time.Duration(backoffTimeMs) * time.Millisecond)
	}
}

func transportSend(r *Reporter) {
	for rp := range r.publishChannel {
		// dont send reports when running in debug mode
		if r.debug == true {
			continue
		}

		sendPayload(r, rp)
	}
	r.shutdownChannel <- struct{}{}
}

func (r *Reporter) Init(c *client.Config) {
	log.Printf("[reporter] Construct reporter with publish uri: %s", c.Server)
	r.jobstepID = c.JobstepID
	r.publishUri = c.Server
	r.publishChannel = make(chan ReportPayload, maxPendingReports)
	r.shutdownChannel = make(chan struct{})
	r.debug = c.Debug

	go transportSend(r)
}

func (r *Reporter) PushJobstepStatus(status string, result string) {
	log.Printf("[reporter] Pushing status %s", status)
	form := make(map[string]string)
	form["status"] = status
	if len(result) > 0 {
		form["result"] = result
	}

	hostname, err := os.Hostname()
	if err == nil {
		form["node"] = hostname
	}
	r.publishChannel <- ReportPayload{"/jobsteps/" + r.jobstepID + "/", form, ""}
}

func (r *Reporter) PushCommandStatus(cID string, status string, retCode int) {
	form := make(map[string]string)
	form["status"] = status
	if retCode >= 0 {
		form["return_code"] = strconv.Itoa(retCode)
	}
	r.publishChannel <- ReportPayload{"/commands/" + cID + "/", form, ""}
}

func (r *Reporter) PushSnapshotImageStatus(iID string, status string) {
	form := make(map[string]string)
	form["status"] = status
	r.publishChannel <- ReportPayload{"/snapshotimages/" + iID + "/", form, ""}
}

func (r *Reporter) PushLogChunk(source string, payload []byte) {
	form := make(map[string]string)
	form["source"] = source
	form["text"] = string(payload)
	if r.debug {
		log.Print(string(payload))
	}
	r.publishChannel <- ReportPayload{"/jobsteps/" + r.jobstepID + "/logappend/", form, ""}
}

func (r *Reporter) PushCommandOutput(cID string, status string, retCode int, output []byte) {
	form := make(map[string]string)
	form["status"] = status
	form["output"] = string(output)
	if retCode >= 0 {
		form["return_code"] = strconv.Itoa(retCode)
	}
	r.publishChannel <- ReportPayload{"/commands/" + cID + "/", form, ""}
}

func (r *Reporter) PublishArtifacts(cmd client.ConfigCmd, a adapter.Adapter, clientLog *client.Log) {
	if len(cmd.Artifacts) == 0 {
		clientLog.Writeln("==> Skipping artifact collection")
		return
	}

	clientLog.Writeln(fmt.Sprintf("==> Collecting artifacts matching %s", cmd.Artifacts))

	matches, err := a.CollectArtifacts(cmd.Artifacts, clientLog)
	if err != nil {
		clientLog.Writeln(fmt.Sprintf("==> ERROR: " + err.Error()))
		return
	}

	for _, artifact := range matches {
		clientLog.Writeln(fmt.Sprintf("==> Found: %s", artifact))
	}

	r.pushArtifacts(matches)
}

func (r *Reporter) pushArtifacts(artifacts []string) {
	// TODO: PushArtifacts is synchronous due to races with Adapter.Shutdown(), but
	// really what we'd want to do is just say "wait until channel empty, ok continue"
	for _, artifact := range artifacts {
		sendPayload(r, ReportPayload{"/jobsteps/" + r.jobstepID + "/artifacts/", nil, artifact})
	}
}

func (r *Reporter) Shutdown() {
	close(r.publishChannel)
	<-r.shutdownChannel
	close(r.shutdownChannel)
	log.Print("[reporter] Shutdown complete")
}

func init() {
	flag.IntVar(&maxPendingReports, "max_pending_reports", 64, "Backlog size")
	flag.IntVar(&numPublishRetries, "num_publish_retries", 8,
		"Number of times to retry")
	flag.IntVar(&maxPendingReports, "backoff_time_ms", 1000,
		"Time to wait between two consecutive retries")

	reporter.Register("mesos", &Reporter{})
}
