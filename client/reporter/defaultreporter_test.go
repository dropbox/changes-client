package reporter

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dropbox/changes-client/client"
)

func TestSnapshotImageArtifact(t *testing.T) {
	const (
		imageID = "testimage"
		status  = "Active"
	)
	err := fmt.Errorf("No request made")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expectedPath := "/snapshotimages/" + imageID + "/"
		if r.URL.Path != expectedPath {
			err = fmt.Errorf("Incorrect path, expected %q got %q", expectedPath, r.URL.Path)
		} else if r.FormValue("status") != status {
			err = fmt.Errorf("Incorrect status, expected %q got %q", status, r.FormValue("status"))
		} else {
			err = nil
		}
	}))
	defer server.Close()

	r := DefaultReporter{}
	r.Init(&client.Config{Server: server.URL})
	r.PushSnapshotImageStatus(imageID, status)

	// this won't return until the snapshot image status has been pushed
	// (or we've given up from too many retries)
	r.Shutdown()

	if err != nil {
		t.Fatal(err)
	}
}
