package runner

import (
    "net/http"
    "net/url"
    "log"
    "flag"
)

var (
    maxPendingReports = 64
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
        log.Printf("[reporter] POST %s data: %s", path, req.data)
        _, err := http.PostForm(path, req.data)
        // TODO: Retry on error
        // Add a time based retry login, try @ now + x_ms
        if err != nil {
            log.Printf("[reporter] POST %s failed, err: %s", path, err)
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
    log.Print("[reporter] Shutdown")
    close(r.publishChannel)
    <-r.shutdownChannel
    close(r.shutdownChannel)
}

func init() {
    flag.IntVar(&maxPendingReports, "max_pending_reports", 64, "Backlog size")
}
