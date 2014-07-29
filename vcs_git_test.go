package runner

import (
	"os"
	"os/exec"
	"testing"
)

func TestCloneBehavior(t *testing.T) {
	// Awful test which just makes sure things run, more or less
	// TODO(dcramer): actually make valid assertions
	cmd := exec.Command("rm", "-rf", "/tmp/changes-client-go-test")
	err := cmd.Start()
	if err != nil {
		t.Errorf(err.Error())
	}
	err = cmd.Wait()
	if err != nil {
		t.Errorf(err.Error())
	}

	vcs := &GitVcs{
		Path: "/tmp/changes-client-go-test",
		URL: "file://.",
	}

	err = CloneOrUpdate(vcs)
	if err != nil {
		t.Errorf(err.Error())
	}

	if _, err = os.Stat(vcs.GetPath()); os.IsNotExist(err) {
		t.Errorf(err.Error())
	}

	// Ensure things dont error when we run it again
	err = CloneOrUpdate(vcs)
	if err != nil {
		t.Errorf(err.Error())
	}

	// hardcoded first commit
	err = CheckoutRevision(vcs, "a7538a8c8c5e64ae115b3cbad91d7a6ac91303c2")
	if err != nil {
		t.Errorf(err.Error())
	}

	err = CheckoutRevision(vcs, "f5b3ef7ff8a146aa255d812a9ce68cae968a65ac")
	if err != nil {
		t.Errorf(err.Error())
	}
}
