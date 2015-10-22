package reporter

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/dropbox/changes-client/client"
	"github.com/dropbox/changes-client/common/sentry"
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

// With each reporter there is a goroutine associated with it that
// listens to PublishChannel and shutdownChannel, publishing all
// data from PublishChannel to the publishUri for the jobstepID
// associated with the current build. Sending any information
// to shutdownChannel causes the goroutine to stop.
//
// Notably this means that all of the methods in this module
// are asynchronous and as a result there is a delay between
// them successfully finishing and Changes actually acknowledging
// them at the endpoint. More importantly, however, because the
// requests are sent in a separate goroutine, the methods here may
// succeed even when the endpoing requests fail.
type DefaultReporter struct {
	// Note that this is not safe to send to after Shutdown() is called.
	PublishChannel  chan ReportPayload
	jobstepID       string
	publishUri      string
	shutdownChannel chan struct{}
}

// All data that goes to the server is encompassed in a payload.
type ReportPayload struct {
	Path string
	// A map of fields to their values. Note that the date field
	// will be automatically set when the data is sent.
	Data     map[string]string
	Filename string
}

// Utility function for posting a request to a uri. The parameters
// here, which more or less correspond to ReportPayload.Data,
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
func (r *DefaultReporter) SendPayload(rp ReportPayload) error {
	var (
		resp   *http.Response
		err    error
		status string
	)

	path := r.publishUri + rp.Path
	if rp.Data == nil {
		rp.Data = make(map[string]string)
	}

	rp.Data["date"] = time.Now().UTC().Format("2006-01-02T15:04:05.0Z")
	for tryCnt := 1; tryCnt <= numPublishRetries; tryCnt++ {
		log.Printf("[reporter] POST %s try: %d", path, tryCnt)
		resp, err = httpPost(path, rp.Data, rp.Filename)

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
			return fmt.Errorf("reporter couldn't connect to publish endpoint %s; %s", path, errmsg)
		}
		log.Printf("[reporter] Sleep for %d ms", backoffTimeMs)
		time.Sleep(time.Duration(backoffTimeMs) * time.Millisecond)
	}
	return nil
}

// Continually listens to the publish channel and sends the payloads
func transportSend(r *DefaultReporter) {
	for rp := range r.PublishChannel {
		r.SendPayload(rp)
	}
	r.shutdownChannel <- struct{}{}
}

func (r *DefaultReporter) JobstepAPIPath() string {
	return "/jobsteps/" + r.jobstepID + "/"
}

func (r *DefaultReporter) Init(c *client.Config) {
	log.Printf("[reporter] Construct reporter with publish uri: %s", c.Server)
	r.publishUri = c.Server
	r.shutdownChannel = make(chan struct{})
	r.jobstepID = c.JobstepID
	r.PublishChannel = make(chan ReportPayload, maxPendingReports)
	// Initialize the goroutine that actually sends the requests.
	go transportSend(r)
}

// Close the publish and shutdown channels, which causes the inner goroutines to
// terminate, thus cleaning up what is created by Init.
func (r *DefaultReporter) Shutdown() {
	close(r.PublishChannel)
	<-r.shutdownChannel
	close(r.shutdownChannel)
	log.Print("[reporter] Shutdown complete")
}

func (r *DefaultReporter) PushSnapshotImageStatus(iID string, status string) {
	form := make(map[string]string)
	form["status"] = status
	r.PublishChannel <- ReportPayload{"/snapshotimages/" + iID + "/", form, ""}
}

func (r *DefaultReporter) ReportMetrics(metrics client.Metrics) {
	if metrics.Empty() {
		return
	}
	data, err := json.Marshal(metrics)
	if err != nil {
		log.Printf("[reporter] Error encoding metrics: %s", err)
		sentry.Error(err, map[string]string{})
		return
	}
	form := map[string]string{"metrics": string(data)}
	r.PublishChannel <- ReportPayload{r.JobstepAPIPath(), form, ""}
}

func init() {
	flag.IntVar(&maxPendingReports, "max_pending_reports", 64, "Backlog size")
	flag.IntVar(&numPublishRetries, "num_publish_retries", 8,
		"Number of times to retry")
	flag.IntVar(&backoffTimeMs, "backoff_time_ms", 1000,
		"Time to wait between two consecutive retries")
}
