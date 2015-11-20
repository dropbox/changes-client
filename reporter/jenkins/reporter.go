package jenkinsreporter

import (
	"flag"
	"log"
	"os/exec"
	"path"

	"github.com/dropbox/changes-client/client"
	"github.com/dropbox/changes-client/client/adapter"
	"github.com/dropbox/changes-client/client/reporter"
)

var (
	artifactDestination string
)

type Reporter struct {
	reporter.DefaultReporter
	artifactDestination string
}

func (r *Reporter) Init(c *client.Config) {
	log.Printf("[reporter] Construct reporter with artifact destination: %s", artifactDestination)
	r.artifactDestination = artifactDestination
	r.DefaultReporter.Init(c)
}

func (r *Reporter) PushJobstepStatus(status string, result string) {
}

func (r *Reporter) PushCommandStatus(cID string, status string, retCode int) {
}

func (r *Reporter) PushLogChunk(source string, payload []byte) bool {
	return true
}

func (r *Reporter) PushCommandOutput(cID string, status string, retCode int, output []byte) {
}

// If we were running in an lxc container, the artifacts are already grouped
// but they need to be copied from the container to the actual artifact
// destination. Because we pass through the Jenkins environment variables
// to the commands inside of the container, we expect that they be in the
// same location as we expect them to be, except nested within the mounted filesystem.
func (r *Reporter) PublishArtifacts(cmdCnf client.ConfigCmd, a adapter.Adapter, clientLog *client.Log) error {
	if a.GetRootFs() == "/" {
		log.Printf("[reporter] RootFs is /, no need to move artifacts")
		return nil
	}

	// TODO: Create and use a.GetWorkspace() as artifactSource instead of double using
	// artifactDestination.
	artifactSource := path.Join(a.GetRootFs(), r.artifactDestination)
	log.Printf("[reporter] Copying artifacts from %s to: %s\n", artifactSource, r.artifactDestination)
	mkdircmd := exec.Command("mkdir", "-p", artifactDestination)
	if output, err := mkdircmd.CombinedOutput(); err != nil {
		log.Printf("[reporter] Failed to create artifact destination: %s", output)
		return err
	}
	cpcmd := exec.Command("cp", "-f", "-r", path.Join(artifactSource, "."), r.artifactDestination)
	if output, err := cpcmd.CombinedOutput(); err != nil {
		log.Printf("[reporter] Failed to push artifacts; possibly the source artifact folder did not exist: %s", output)
		return err
	}
	return nil
}

func New() reporter.Reporter {
	return &Reporter{}
}

func init() {
	// XXX figure out a reasonable default for this value or default to ""
	// and sanity-check the reporter during Init. If this value is invalid
	// we should trigger an infastracture failure.
	flag.StringVar(&artifactDestination, "artifact-destination", "/dev/null", "Jenkins artifact destination")

	reporter.Register("jenkins", New)
}
