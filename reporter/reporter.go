package reporter

import (
	"bytes"
	"flag"
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
	shutdownChannel chan bool
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

func transportSend(r *Reporter) {
	var resp *http.Response
	var err error
	var status string

	for req := range r.publishChannel {
		// dont send reports when running in debug mode
		if r.debug == true {
			continue
		}

		path := r.publishUri + req.path
		if req.data == nil {
			req.data = make(map[string]string)
		}

		req.data["date"] = time.Now().UTC().Format("2006-01-02T15:04:05.0Z")
		for tryCnt := 1; tryCnt <= numPublishRetries; tryCnt++ {
			log.Printf("[reporter] POST %s try: %d", path, tryCnt)
			resp, err = httpPost(path, req.data, req.filename)

			if resp != nil {
				status = resp.Status
			} else {
				status = "-1"
			}

			if resp != nil && resp.StatusCode/100 == 2 {
				break
			}

			if resp != nil && resp.StatusCode == http.StatusGone {
				// TODO(dcramer): this shouldn't really be a panic, but
				// we want to exit at this point
				panic("Unknown error occurred with publish endpoint")
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
	r.shutdownChannel <- true
}

func NewReporter(publishUri string, jobstepID string, debug bool) *Reporter {
	log.Printf("[reporter] Construct reporter with publish uri: %s", publishUri)
	r := &Reporter{}
	r.jobstepID = jobstepID
	r.publishUri = publishUri
	r.publishChannel = make(chan ReportPayload, maxPendingReports)
	r.shutdownChannel = make(chan bool)
	r.debug = debug

	go transportSend(r)
	return r
}

func (r *Reporter) PushJobStatus(status string, result string) {
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

func (r *Reporter) PushStatus(cId string, status string, retCode int) {
	form := make(map[string]string)
	form["status"] = status
	if retCode >= 0 {
		form["return_code"] = strconv.Itoa(retCode)
	}
	r.publishChannel <- ReportPayload{"/commands/" + cId + "/", form, ""}
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

func (r *Reporter) PushOutput(cId string, status string, retCode int, output []byte) {
	form := make(map[string]string)
	form["status"] = status
	form["output"] = string(output)
	if retCode >= 0 {
		form["return_code"] = strconv.Itoa(retCode)
	}
	r.publishChannel <- ReportPayload{"/commands/" + cId + "/", form, ""}
}

func (r *Reporter) PushArtifacts(artifacts []string) {
	for _, artifact := range artifacts {
		r.publishChannel <- ReportPayload{"/jobsteps/" + r.jobstepID + "/artifacts/", nil, artifact}
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
}
