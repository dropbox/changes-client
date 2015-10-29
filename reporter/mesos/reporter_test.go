package mesosreporter

import (
	"fmt"
	"github.com/dropbox/changes-client/client"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestPushArtifacts(t *testing.T) {
	jobstepID := "test_jobstep"
	testDirName, _ := ioutil.TempDir("/tmp", "pushArtifactsTest")
	testFile, _ := ioutil.TempFile(testDirName, "test")
	defer os.RemoveAll(testDirName)

	expectedName := filepath.Join(filepath.Base(testDirName), filepath.Base(testFile.Name()))
	expectedReqPath := "/jobsteps/" + jobstepID + "/artifacts/"

	err := fmt.Errorf("No request made")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != expectedReqPath {
			err = fmt.Errorf("Incorrect path, expected %q got %q", expectedReqPath, req.URL.Path)
		} else if req.FormValue("name") != expectedName {
			err = fmt.Errorf("Incorrect expectedName, expected %q got %q", expectedName, req.FormValue("name"))
		} else {
			err = nil
		}
	}))
	defer server.Close()

	r := Reporter{}
	r.Init(&client.Config{Server: server.URL, JobstepID: jobstepID})
	artifacts := []string{testFile.Name()}
	r.pushArtifacts(artifacts, "/tmp")

	if err != nil {
		t.Fatal(err)
	}
}
