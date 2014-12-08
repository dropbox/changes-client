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
	"time"
)

var (
	maxPendingReports = 64
	numPublishRetries = 8
	backoffTimeMs     = 1000
)

type Reporter interface {
	PushArtifacts([]string)
	PushBuildStatus(string, string)
	PushCommandOutput(string, string, int, []byte)
	PushCommandStatus(string, string, int)
	PushLogChunk(string, []byte)
	Shutdown()
}

type HttpPayload struct {
	path     string
	data     map[string]string
	filename string
}

type HttpStream struct {
	publishUri      string
	publishChannel  chan HttpPayload
	shutdownChannel chan struct{}
	debug           bool
}

func NewHttpStream(publishUri string, debug bool) *HttpStream {
	hs := &HttpStream{
		publishUri:      publishUri,
		publishChannel:  make(chan HttpPayload, maxPendingReports),
		shutdownChannel: make(chan struct{}),
		debug:           debug,
	}

	go transportSend(hs)

	return hs
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

func (hs *HttpStream) sendPayload(hp HttpPayload) {
	var (
		resp   *http.Response
		err    error
		status string
	)

	path := hs.publishUri + hp.path
	if hp.data == nil {
		hp.data = make(map[string]string)
	}

	hp.data["date"] = time.Now().UTC().Format("2006-01-02T15:04:05.0Z")
	for tryCnt := 1; tryCnt <= numPublishRetries; tryCnt++ {
		log.Printf("[reporter] POST %s try: %d", path, tryCnt)
		resp, err = httpPost(path, hp.data, hp.filename)

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

func transportSend(hs *HttpStream) {
	for hp := range hs.publishChannel {
		// dont send reports when running in debug mode
		if hs.debug == true {
			continue
		}

		hs.sendPayload(hp)
	}
	hs.shutdownChannel <- struct{}{}
}

func (hs *HttpStream) Shutdown() {
	close(hs.publishChannel)
	<-hs.shutdownChannel
	close(hs.shutdownChannel)
}

func (hs *HttpStream) Push(hp HttpPayload) {
	hs.publishChannel <- hp
}

func (hs *HttpStream) PushSync(hp HttpPayload) {
	hs.sendPayload(hp)
}

func init() {
	flag.IntVar(&maxPendingReports, "max_pending_reports", 64, "Backlog size")
	flag.IntVar(&numPublishRetries, "num_publish_retries", 8,
		"Number of times to retry")
	flag.IntVar(&maxPendingReports, "backoff_time_ms", 1000,
		"Time to wait between two consecutive retries")
}
