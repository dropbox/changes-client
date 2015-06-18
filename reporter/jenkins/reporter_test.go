package jenkinsreporter

import (
	"io/ioutil"
	"encoding/json"
	"testing"
	"os"
)

func TestSnapshotImageArtifact(t *testing.T) {
	os.MkdirAll("./jenkins-reporter-tmp/artifacts", os.ModeDir)
	defer os.RemoveAll("./jenkins-reporter-tmp")
	r := Reporter{}
	r.artifactDestination = "./jenkins-reporter-tmp/artifacts"
	r.PushSnapshotImageStatus("testimage", "active")

	file, err := ioutil.ReadFile("./jenkins-reporter-tmp/artifacts/snapshot_status.json")
	if err != nil {
		t.Fatal("snapshot_status.json not created at artifact path")
	}

	var j interface{}
	err = json.Unmarshal(file, &j)
	if err != nil || j == nil {
		t.Fatal("could not parse snapshot_status.json")
	}
	m := j.(map[string]interface{})

	if (m["image"] != "testimage" || m["status"] != "active") {
		t.Fatal("incorrect json created")
	}
}
