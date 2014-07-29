package runner

import (
	"os/exec"
)

type GitVcs struct {
	Path string
	URL  string
}

func (v *GitVcs) GetCloneCommand() (*exec.Cmd, error) {
	cmd := exec.Command("git", "clone", v.URL, v.Path)
	return cmd, nil
}

func (v *GitVcs) GetUpdateCommand() (*exec.Cmd, error) {
	cmd := exec.Command("git", "fetch", "--all")
	cmd.Dir = v.Path
	return cmd, nil
}

func (v *GitVcs) GetCheckoutRevisionCommand(sha string) (*exec.Cmd, error) {
	cmd := exec.Command("git", "reset", "--hard", sha)
	cmd.Dir = v.Path
	return cmd, nil
}

func (v *GitVcs) GetApplyPatchCommand(path string) (*exec.Cmd, error) {
	cmd := exec.Command("git", "apply", path)
	cmd.Dir = v.Path
	return cmd, nil
}

func (v *GitVcs) GetPath() string {
	return v.Path
}
