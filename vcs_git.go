package runner

import (
	"os/exec"
)

type GitVcs struct {
	Path string
	URL  string
}

func (v *GitVcs) GetCloneCommand() (*exec.Cmd, error) {
	cmd := exec.Command("git", "clone", "--mirror", v.URL, v.Path)
	return cmd, nil
}

func (v *GitVcs) GetUpdateCommand() (*exec.Cmd, error) {
	cmd := exec.Command("git", "fetch", "--all")
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
