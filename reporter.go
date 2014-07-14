package runner

import (
    "os"
    "fmt"
    "net/http"
    "net/url"
)

type JobStatus int

const (
    RUNNING JobStatus     = 0
    SUCCEEDED             = 1
    FAILED                = 2
)

func (j JobStatus) String() string {
    switch j {
        case RUNNING: return "RUNNING"
        case SUCCEEDED: return "SUCCEEDED"
        case FAILED: return "FAILED"
    }
    return "UNKNOWN"
}

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

func NewReporter(publishUri string) (*Reporter, error) {
    r := new (Reporter)
    r.publishUri = publishUri
    r.publishChannel = make(chan ReportPayload)
    r.shutdownChannel = make(chan bool)
    go transportSend(r)
    return r, nil
}

func (r *Reporter) PushJobStatus(s JobStatus) {
    data := make(url.Values)
    data.Add("job_status", string(s))
    r.publishChannel <- ReportPayload {"/status", data}
}

func (r *Reporter) PushCmdStatus(cId string, s *os.ProcessState) {
    form := make(url.Values)
    form.Add("cmd_id", cId)
    form.Add("cmd_status", s.String())
    r.publishChannel <- ReportPayload {"/status/" + cId, form}
}

func (r *Reporter) PushCmdOutStream(cId string, source string, offset int, length int, data []byte) {
    form := make(url.Values)
    form.Add("cmd_id", cId)
    form.Add("cmd_stream_" + source, string(data))
    form.Add("cmd_stream_offset", string(offset))
    r.publishChannel <- ReportPayload {"/status/" + cId, form}
}

func (r *Reporter) shutdown() {
    close(r.publishChannel)
    <-r.shutdownChannel
    close(r.shutdownChannel)
}
