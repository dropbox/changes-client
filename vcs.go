package runner

import (
	"fmt"
	"log"
	"os"
	"os/exec"
)

type Vcs interface {
	GetCloneCommand() (*exec.Cmd, error)
	GetUpdateCommand() (*exec.Cmd, error)
	GetCheckoutRevisionCommand(string) (*exec.Cmd, error)
	GetApplyPatchCommand(string) (*exec.Cmd, error)
	GetPath() string
}

func runCmd(cmd *exec.Cmd) error {
	log.Printf("[vcs] Executing command %s from %s", cmd.Args, cmd.Dir)
	err := cmd.Start()
	if err != nil {
		return err
	}

	err = cmd.Wait()
	if err != nil {
		return err
	}

	if !cmd.ProcessState.Success() {
		err = fmt.Errorf("[vcs] Command failed: %s", cmd.Path)
		return err
	}

	return nil
}

func CloneOrUpdate(v Vcs) error {
	// if the workspace already exists, update
	// otherwise create a new checkout
	if _, err := os.Stat(v.GetPath()); os.IsNotExist(err) {
		cmd, err := v.GetCloneCommand()
		if err != nil {
			return err
		}
		err = runCmd(cmd)
		if err != nil {
			return err
		}
	} else {
		cmd, err := v.GetUpdateCommand()
		if err != nil {
			return err
		}
		err = runCmd(cmd)
		if err != nil {
			return err
		}
	}

	return nil
}

func CheckoutRevision(v Vcs, sha string) error {
	// if the workspace already exists, update
	// otherwise create a new checkout
	cmd, err := v.GetCheckoutRevisionCommand(sha)
	if err != nil {
		return err
	}
	err = runCmd(cmd)
	if err != nil {
		return err
	}

	return nil
}

func ApplyPatch(v Vcs, path string) error {
	cmd, err := v.GetApplyPatchCommand(path)
	if err != nil {
		return err
	}
	err = runCmd(cmd)
	if err != nil {
		return err
	}
	return nil
}
