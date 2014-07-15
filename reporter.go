package runner

import (
    "os"
    "fmt"
    "net/http"
    "net/url"
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
    // FIXME -- This should not block the caller
    for req := range r.publishChannel {
        fmt.Println(req.path)
        _, err := http.PostForm(r.publishUri + req.path, req.data)
        // FIXME retry on error
        if err != nil {
            fmt.Println(err)
        }
    }
    r.shutdownChannel <- true
}

func NewReporter(publishUri string) *Reporter {
    r := &Reporter{}
    r.publishUri = publishUri
    r.publishChannel = make(chan ReportPayload)
    r.shutdownChannel = make(chan bool)

    go transportSend(r)
    return r
}

func (r *Reporter) PushStatus(cId string, s *os.ProcessState) {
    form := make(url.Values)
    form.Add("status", s.String())
    r.publishChannel <- ReportPayload {cId + "/status", form}
}

func (r *Reporter) PushLogChunk(cId string, l LogChunk) {
    form := make(url.Values)
    form.Add("source", l.Source)
    form.Add("offset", string(l.Offset))
    form.Add("text", string(l.Payload))
    r.publishChannel <- ReportPayload {cId + "/logappend", form}
}

func (r *Reporter) shutdown() {
    close(r.publishChannel)
    <-r.shutdownChannel
    close(r.shutdownChannel)
}
