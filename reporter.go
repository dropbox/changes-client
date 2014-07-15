package runner

import (
	"bytes"
	"flag"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

var (
	maxPendingReports = 64
	numPublishRetries = 8
	backoffTimeMs     = 1000
)

type ReportPayload struct {
	path  string
	data  map[string]string
	files []string
}

type Reporter struct {
	publishUri      string
	publishChannel  chan ReportPayload
	shutdownChannel chan bool
}

func httpPost(uri string, params map[string]string,
	files []string) (resp *http.Response, err error) {

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	for key, val := range params {
		err = writer.WriteField(key, val)
		if err != nil {
			log.Printf("[reporter] Couldn't write field %s", key)
		}
	}

	for _, file := range files {
		handle, err := os.Open(file)
		if err != nil {
			log.Printf("[reporter] Couldn't open file %s", file)
			continue
		}
		fileField, err := writer.CreateFormFile(file, filepath.Base(file))
		if err == nil {
			log.Printf("[reporter] Couldn't write file field %s", file)
			continue
		}
		_, err = io.Copy(fileField, handle)
		if err != nil {
			log.Printf("[reporter] Couldn't copy file %s", file)
		}
	}
	_ = writer.Close()
	return http.Post(uri, writer.FormDataContentType(), body)
}

func transportSend(r *Reporter) {
	for req := range r.publishChannel {
		path := r.publishUri + req.path
		for tryCnt := 1; tryCnt <= numPublishRetries; tryCnt++ {
			log.Printf("[reporter] POST %s try: %d", path, tryCnt)
			resp, err := httpPost(path, req.data, req.files)
			if resp != nil && resp.StatusCode == http.StatusOK {
				break
			}

			if resp != nil && resp.StatusCode == http.StatusNoContent {
				panic("Unknown error occurred with publish endpoint")
			}

			log.Printf("[reporter] POST %s failed, try: %d, resp: %s, err: %s",
				path, tryCnt, resp, err)
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

func NewReporter(publishUri string) *Reporter {
	log.Printf("[reporter] Construct reporter with publish uri: %s", publishUri)
	r := &Reporter{}
	r.publishUri = publishUri
	r.publishChannel = make(chan ReportPayload, maxPendingReports)
	r.shutdownChannel = make(chan bool)

	go transportSend(r)
	return r
}

func (r *Reporter) PushStatus(cId string, status string) {
	form := make(map[string]string)
	form["status"] = status
	r.publishChannel <- ReportPayload{cId + "/status", form, nil}
}

func (r *Reporter) PushLogChunk(cId string, l LogChunk) {
	form := make(map[string]string)
	form["source"] = l.Source
	form["offset"] = string(l.Offset)
	form["text"] = string(l.Payload)
	r.publishChannel <- ReportPayload{cId + "/logappend", form, nil}
}

func (r *Reporter) PushArtifacts(cId string, artifacts []string) {
	form := make(map[string]string)
	r.publishChannel <- ReportPayload{cId + "/artifacts", form, artifacts}
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
