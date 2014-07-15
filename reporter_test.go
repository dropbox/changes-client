package runner

import (
    "testing"
    "net/http"
    "fmt"
)

type Expected struct {
    path string
    data map[string]string
}

func Test_Reporter(t *testing.T) {
    t.Fail()
}

func mkExpectedHandler(exp *Expected) func (w http.ResponseWriter, r *http.Request) {
    return nil
}

func Test_JobStatus(t *testing.T) {
    form := map[string]string {
        "job_status" : "RUNNING",
    }
    expected := Expected {"/status", form}
    expected_handler := mkExpectedHandler(expected)
    http.HandleFunc("/", expected_handler)
    http.ListenAndServe(":8181", nil)
    t.Fail()
}

func Test_CommandStatus(t *testing.T) {
}

func Test_CommmandOut(t *testing.T) {
}

func Test_Shutdown(t *testing.T) {
}
