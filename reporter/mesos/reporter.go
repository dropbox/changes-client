package mesosreporter

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/dropbox/changes-client/client"
	"github.com/dropbox/changes-client/client/adapter"
	"github.com/dropbox/changes-client/client/reporter"
)

var (
	// The size of the payload queue. Once it reaches this size,
	// all writes (and thus all of the exported functions from this
	// module) will become blocking.
	maxPendingReports = 64

	// Maximum number of times we retry a payload until we give up
	// and panic.
	//
	// XXX the panic occurs during the publish goroutine, which might
	// not be well characterized for properly handling the error.
	numPublishRetries = 8

	// How long we wait before retrying a payload. We always wait
	// this amount of time so the name here is a bit of a misnomer.
	backoffTimeMs = 1000
)

// All data that goes to the server is encompassed in a payload.
type ReportPayload struct {
	path string
	// A map of fields to their values. Note that the date field
	// will be automatically set when the data is sent.
	data     map[string]string
	filename string
}

// A reporter that connects and reports to a specific jobstep id.
// Each jobstep id has a number of endpoints associated with it that
// allows the reporter to update the status of logs, snapshots, etc.
//
// With each reporter there is a goroutine associated with it that
// listens to publishChannel and shutdownChannel, publishing all
// data from publishChannel to the publishUri for the jobstepID
// associated with the current build. Sending any information
// to shutdownChannel causes the goroutine to stop.
//
// Notably this means that all of the methods in this module
// are asynchronous and as a result there is a delay between
// them successfully finishing and Changes actually acknowledging
// them at the endpoint. More importantly, however, because the
// requests are sent in a separate goroutine, the methods here may
// succeed even when the endpoing requests fail.
//
// In debug mode, endpoint requests are still queued in the publish
// channel but never sent by the publishing goroutine, which allows
// the mesos reporter to run without actually connecting
// to the changes server.
type Reporter struct {
	jobstepID       string
	publishUri      string
	publishChannel  chan ReportPayload
	shutdownChannel chan struct{}
	debug           bool
}

// Utility function for posting a request to a uri. The parameters
// here, which more or less correspond to ReportPayload.data,
// are serialized to form the request body. The body is encoded
// as a MIME multipart (see RFC 2338).
//
// The file is also added a as field in the request body.
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

// Utility method for sending a payload. This wraps httpPost in a framework
// nicer for the Reporter itself, as it turns the ReportPayload into its
// associated params (which corresponds to its data). We also attempt
// httpPost multiple times in order to account for flakiness in the
// network connection. This function is synchronous.
func sendPayload(r *Reporter, rp ReportPayload) error {
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

		var errmsg string
		if err != nil {
			errmsg = err.Error()
		} else {
			// If there wasn't an IO error, use the response body as the error message.
			var bodyData bytes.Buffer
			if _, e := bodyData.ReadFrom(resp.Body); e != nil {
				log.Printf("[reporter] Error reading POST %s response body: %s", path, e)
			}
			errmsg = bodyData.String()
			if len(errmsg) > 140 {
				// Keep it a reasonable length.
				errmsg = errmsg[:137] + "..."
			}
		}
		log.Printf("[reporter] POST %s failed, try: %d, resp: %s, err: %s",
			path, tryCnt, status, errmsg)

		/* We are unable to publish to the endpoint.
		 * Fail fast and let the above layers handle the outage */
		if tryCnt == numPublishRetries {
			return fmt.Errorf("mesos reporter couldn't connect to publish endpoint %s; %s", path, errmsg)
		}
		log.Printf("[reporter] Sleep for %d ms", backoffTimeMs)
		time.Sleep(time.Duration(backoffTimeMs) * time.Millisecond)
	}
	return nil
}

// Continually listens to the publish channel and sends the payloads
// if we aren't in debug mode.
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

	// Initialize the goroutine that actually sends the requests. We spawn
	// this even when in debug mode to prevent the payloads from
	// massively queueing. Since we by default use a queue with a maximum
	// limit, once it reaches that limit writes will block causing the
	// main goroutine to halt forever.
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

func (r *Reporter) PublishArtifacts(cmd client.ConfigCmd, a adapter.Adapter, clientLog *client.Log) error {
	if len(cmd.Artifacts) == 0 {
		clientLog.Writeln("==> Skipping artifact collection")
		return nil
	}

	clientLog.Writeln(fmt.Sprintf("==> Collecting artifacts matching %s", cmd.Artifacts))

	matches, err := a.CollectArtifacts(cmd.Artifacts, clientLog)
	if err != nil {
		clientLog.Writeln(fmt.Sprintf("==> ERROR: " + err.Error()))
		return err
	}

	for _, artifact := range matches {
		clientLog.Writeln(fmt.Sprintf("==> Found: %s", artifact))
	}

	return r.pushArtifacts(matches)
}

func (r *Reporter) pushArtifacts(artifacts []string) error {
	// TODO: PushArtifacts is synchronous due to races with Adapter.Shutdown(), but
	// really what we'd want to do is just say "wait until channel empty, ok continue"
	var firstError error
	for _, artifact := range artifacts {
		e := sendPayload(r, ReportPayload{"/jobsteps/" + r.jobstepID + "/artifacts/", nil, artifact})
		if e != nil && firstError == nil {
			firstError = e
		}
	}
	return firstError
}

// Close the publish and shutdown channels, which causes the inner goroutines to
// terminate, thus cleaning up what is created by Init.
func (r *Reporter) Shutdown() {
	close(r.publishChannel)
	<-r.shutdownChannel
	close(r.shutdownChannel)
	log.Print("[reporter] Shutdown complete")
}

func New() reporter.Reporter {
	return &Reporter{}
}

func init() {
	flag.IntVar(&maxPendingReports, "max_pending_reports", 64, "Backlog size")
	flag.IntVar(&numPublishRetries, "num_publish_retries", 8,
		"Number of times to retry")
	flag.IntVar(&maxPendingReports, "backoff_time_ms", 1000,
		"Time to wait between two consecutive retries")

	reporter.Register("mesos", New)
}
