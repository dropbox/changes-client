package jenkinsreporter

import (
	"encoding/json"
	"io/ioutil"
	"flag"
	"github.com/dropbox/changes-client/client"
	"github.com/dropbox/changes-client/client/adapter"
	"github.com/dropbox/changes-client/client/reporter"
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

type SnapshotResponse struct {
	Image	string `json:"image"`
	Status	string `json:"status"`
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
	flag.StringVar(&artifactDestination, "artifact-destination", "/dev/null", "Jenkins artifact destination")

	reporter.Register("jenkins", &Reporter{})
}
