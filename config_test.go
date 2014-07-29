package runner

import (
	"testing"
)

func TestLoadConfig(t *testing.T) {
	template := `
	{
		"commands": [
			{
				"id": "cmd_1",
				"script": "#!/bin/bash\necho -n $VAR",
				"env": {"VAR": "hello world"},
				"cwd": "/tmp",
                "artifacts": ["%s"]
			},
			{
				"id": "cmd_2",
				"script": "#!/bin/bash\necho test",
				"cwd": "/tmp"
			}
		],
		"repository": {
			"url": "git@github.com:dropbox/changes.git",
			"backend": {
				"id": "git"
			}
		},
		"source": {
			"patch": {
				"id": "patch_1"
			},
			"revision": {
				"sha": "aaaaaa"
			}
		}
	}
	`

	config, err := LoadConfig([]byte(template))
	if err != nil {
		t.Errorf(err.Error())
	}

	if config.Repository.Backend.ID != "git" {
		t.Fail()
	}

	if config.Repository.URL != "git@github.com:dropbox/changes.git" {
		t.Fail()
	}

	if len(config.Cmds) != 2 {
		t.Fail()
	}

}
