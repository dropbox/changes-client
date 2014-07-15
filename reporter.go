package runner

import (
    "net/http"
    "net/url"
    "log"
    "flag"
    "time"
)

var (
    maxPendingReports = 64
    numPublishRetries = 8
    backoffTimeMs     = 1000
)


type ReportPayload struct {
    path string
    data url.Values
}

type Reporter struct {
    publishUri string
    publishChannel chan ReportPayload
    shutdownChannel chan bool
}

func transportSend(r *Reporter) {
    for req := range r.publishChannel {
        path := r.publishUri + req.path
        for tryCnt := 1; tryCnt <= numPublishRetries; tryCnt ++ {
            log.Printf("[reporter] POST %s try: %d", path, tryCnt)
            resp, err := http.PostForm(path, req.data)
            if resp != nil && resp.StatusCode == http.StatusOK {
                break;
            }

            if resp != nil && resp.StatusCode == http.StatusNoContent {
                panic("Unknown error occurred with publish endpoint");
            }

            log.Printf("[reporter] POST %s failed, try: %d, resp: %s, err: %s", path, tryCnt, resp, err)
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
    form := make(url.Values)
    form.Add("status", status)
    r.publishChannel <- ReportPayload {cId + "/status", form}
}

func (r *Reporter) PushLogChunk(cId string, l LogChunk) {
    form := make(url.Values)
    form.Add("source", l.Source)
    form.Add("offset", string(l.Offset))
    form.Add("text", string(l.Payload))
    r.publishChannel <- ReportPayload {cId + "/logappend", form}
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
