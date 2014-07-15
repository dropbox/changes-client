package runner

import (
    "net/http"
    "net/url"
    "log"
    "flag"
    "time"
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
        // TODO: Flag
        for tryCnt := 1; tryCnt <= 8; tryCnt ++ {
            log.Printf("[reporter] POST %s data: %s try: %d", path, req.data, tryCnt)
            _, err := http.PostForm(path, req.data)
            if err != nil {
                log.Printf("[reporter] POST %s failed, try: %d, err: %s", path, err)
                log.Printf("[reporter] Sleep for %d ms", 100)
                // TODO: Flag
                time.Sleep(100 * time.Millisecond)
            } else {
                break
            }

            /* We are unable to publish to the endpoint.
             * Fail fast and let the above layers handle the outage */
             // TODO: Flag
            if tryCnt == 8 {
                panic("Unable to connect to publishUri")
            }
        }
    }
    r.shutdownChannel <- true
}

func NewReporter(publishUri string) *Reporter {
    log.Printf("[reporter] Construct reporter with publish uri: %s", publishUri)
    r := &Reporter{}
    r.publishUri = publishUri
    maxPendingReports := 64
    if f := flag.Lookup("max_pending_reports"); f != nil {
        newVal, ok := f.Value.(flag.Getter)
        if ok {
            maxPendingReports = newVal.Get().(int)
        }
    }
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
