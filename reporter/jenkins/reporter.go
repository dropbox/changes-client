package jenkinsreporter

import (
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

func (r *Reporter) Init(c *client.Config) {
	log.Printf("[reporter] Construct reporter with artifact destination: %s", artifactDestination)
	r.artifactDestination = artifactDestination
	r.debug = c.Debug
}

func (r *Reporter) PushJobstepStatus(status string, result string) {
}

func (r *Reporter) PushCommandStatus(cID string, status string, retCode int) {
}

func (r *Reporter) PushSnapshotImageStatus(iID string, status string) {
	/* TODO this should essentially be shared with what mesos does */
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
