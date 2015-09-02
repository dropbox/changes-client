package artifactstorereporter

import (
	"testing"
	"time"

	"github.com/dropbox/changes-artifacts/client/testserver"
	"github.com/dropbox/changes-client/client"
	"github.com/dropbox/changes-client/client/adapter"
)

func TestRunWithDeadline(t *testing.T) {
	r := &Reporter{}
	r.runWithDeadline(20*time.Millisecond, func() {
		time.Sleep(5 * time.Second)
	})
	if !r.isDisabled() {
		t.Error("runWithDeadline did not intercept long running method")
	}
}

func TestInitTimeout(t *testing.T) {
	ts := testserver.NewTestServer(t)
	defer ts.CloseAndAssertExpectations()

	r := &Reporter{deadline: 100 * time.Millisecond}
	artifactServer = ts.URL
	ts.ExpectAndHang("POST", "/buckets/")

	r.Init(&client.Config{JobstepID: "jobstep"})

	if !r.isDisabled() {
		t.Error("Init did not fail with deadline exceeded")
	}
}

type mockAdapter struct {
	adapter.Adapter
}

func (m *mockAdapter) CollectArtifacts([]string, *client.Log) ([]string, error) {
	return []string{"/etc/hosts"}, nil
}

func drainLog(l *client.Log) {
	go func() {
		for _ = range l.Chan {
		}
	}()
}

func TestPublishArtifactsTimeout(t *testing.T) {
	ts := testserver.NewTestServer(t)
	defer ts.CloseAndAssertExpectations()

	r := &Reporter{deadline: 100 * time.Millisecond}
	artifactServer = ts.URL
	ts.ExpectAndRespond("POST", "/buckets/", 200, `{"Id": "jobstep"}`)

	r.Init(&client.Config{JobstepID: "jobstep"})
	if r.isDisabled() {
		t.Error("Init should not fail with deadline exceeded")
	}

	ma := &mockAdapter{}
	ts.ExpectAndHang("POST", "/buckets/jobstep/artifacts")
	l := client.NewLog()
	drainLog(l)
	defer l.Close()
	r.PublishArtifacts(client.ConfigCmd{Artifacts: []string{"*hosts*"}}, ma, l)

	if !r.isDisabled() {
		t.Error("PublishArtifacts did not fail with deadline exceeded")
	}
}

func TestShutdownTimeout(t *testing.T) {
	ts := testserver.NewTestServer(t)
	defer ts.CloseAndAssertExpectations()

	r := &Reporter{deadline: 100 * time.Millisecond}
	artifactServer = ts.URL
	ts.ExpectAndRespond("POST", "/buckets/", 200, `{"Id": "jobstep"}`)

	r.Init(&client.Config{JobstepID: "jobstep"})
	if r.isDisabled() {
		t.Error("Init should not fail with deadline exceeded")
	}

	ts.ExpectAndHang("POST", "/buckets/jobstep/close")
	r.Shutdown()
	if !r.isDisabled() {
		t.Error("Shutdown did not fail with deadline exceeded")
	}
}

func TestPushLogChunkTimeout(t *testing.T) {
	ts := testserver.NewTestServer(t)
	defer ts.CloseAndAssertExpectations()

	r := &Reporter{deadline: 150 * time.Millisecond}
	artifactServer = ts.URL
	ts.ExpectAndRespond("POST", "/buckets/", 200, `{"Id": "jobstep"}`)

	r.Init(&client.Config{JobstepID: "jobstep"})
	if r.isDisabled() {
		t.Error("Init should not fail with deadline exceeded")
	}

	ts.ExpectAndHang("POST", "/buckets/jobstep/artifacts")
	r.PushLogChunk("console", []byte("console contents"))
	if !r.isDisabled() {
		t.Error("PushLogChunk did not fail with deadline exceeded")
	}

	// This call should not even create a new request. If it does, testserver will throw an error
	// about an unexpected request.
	r.PushLogChunk("console", []byte("console contents"))
}
