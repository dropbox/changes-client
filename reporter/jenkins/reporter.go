package jenkinsreporter

import (
	"encoding/json"
	"flag"
	"github.com/dropbox/changes-client/client"
	"github.com/dropbox/changes-client/client/adapter"
	"github.com/dropbox/changes-client/client/reporter"
	"io/ioutil"
	"log"
	"os/exec"
	"path"
)

var (
	artifactDestination string
)

type Reporter struct {
	artifactDestination string
	debug               bool
}

// This structure is used purely for json marshalling. The fields have to
// be public otherwise the json marshaller will ignore them, but this
// results in their automatically generated fields using capital letters
// so we set them back to lowercase manually.
type SnapshotResponse struct {
	Image  string `json:"image"`
	Status string `json:"status"`
}

func (r *Reporter) Init(c *client.Config) {
	log.Printf("[reporter] Construct reporter with artifact destination: %s", artifactDestination)
	r.artifactDestination = artifactDestination
	r.debug = c.Debug
}

func (r *Reporter) PushJobstepStatus(status string, result string) {
}

func (r *Reporter) PushCommandStatus(cID string, status string, retCode int) {
}

// In order to actually update the snapshot image status, we simply produce
// a json file and rely on Changes to recognize this as a "critical artifact"
// and change the status for us. Note that this artifact is produced directly
// in the artifact destination and as such is not affected by publish artifacts.
func (r *Reporter) PushSnapshotImageStatus(imageID string, status string) {
	log.Printf("[reporter] image-id = %s, status = %s", imageID, status)
	response, err := json.Marshal(SnapshotResponse{Image: imageID, Status: status})
	if err != nil {
		/* if this happens its an error in changes-client and not use-case
		 * so in theory we don't have to worry about err ever being non-nil,
		 * but report it anyway in case theres a bug
		 */
		log.Printf("[reporter] Failed to encode snapshot reponse.")
		return
	}
	err = ioutil.WriteFile(path.Join(r.artifactDestination, "snapshot_status.json"), response, 0644)
	if err != nil {
		log.Printf("[reporter] Failed to write snapshot_status.json")
		log.Printf("    was writing to directory %s", r.artifactDestination)
		log.Printf("    error: %s", err.Error())
	}
}

func (r *Reporter) PushLogChunk(source string, payload []byte) {
}

func (r *Reporter) PushCommandOutput(cID string, status string, retCode int, output []byte) {
}

// If we were running in an lxc container, the artifacts are already grouped
// but they need to be removed from the container and placed in the actual
// artifact destination. Because we pass through the Jenkins environment
// variables to the commands inside of the container, we expect that they
// be in the same location as we expect them to be, except nested within
// the mounted filesystem.
func (r *Reporter) PublishArtifacts(cmdCnf client.ConfigCmd, a adapter.Adapter, clientLog *client.Log) {
	if a.GetRootFs() == "/" {
		log.Printf("[reporter] RootFs is /, no need to move artifacts")
		return
	}

	artifactSource := path.Join(a.GetRootFs(), r.artifactDestination)
	log.Printf("[reporter] Moving artifacts from %s to: %s\n", artifactSource, r.artifactDestination)
	cmd := exec.Command("mkdir", "-p", artifactDestination)
	err := cmd.Run()
	if err != nil {
		log.Printf("[reporter] Failed to create artifact destination")
	}
	cmd = exec.Command("cp", "-f", "-r", path.Join(artifactSource, "."), r.artifactDestination)
	err = cmd.Run()
	if err != nil {
		log.Printf("[reporter] Failed to push artifacts; possibly the source artifact folder did not exist")
	}
}

func (r *Reporter) Shutdown() {
	log.Print("[reporter] Shutdown complete [no-op]")
}

func init() {
	// XXX figure out a reasonable default for this value or default to ""
	// and sanity-check the reporter during Init. If this value is invalid
	// we should trigger an infastracture failure.
	flag.StringVar(&artifactDestination, "artifact-destination", "/dev/null", "Jenkins artifact destination")

	reporter.Register("jenkins", &Reporter{})
}
