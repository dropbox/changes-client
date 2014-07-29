package runner

import (
	"reflect"
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
				"artifacts": ["junit.xml"]
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

	if config.Source.Patch.ID != "patch_1" {
		t.Fail()
	}

	if config.Source.Revision.Sha != "aaaaaa" {
		t.Fail()
	}

	if len(config.Cmds) != 2 {
		t.Fail()
	}

	if config.Cmds[0].Id != "cmd_1" {
		t.Fail()
	}

	if config.Cmds[0].Script != "#!/bin/bash\necho -n $VAR" {
		t.Fail()
	}

	if config.Cmds[0].Cwd != "/tmp" {
		t.Fail()
	}

	if len(config.Cmds[0].Artifacts) != 1 {
		t.Fail()
	}

	if config.Cmds[0].Artifacts[0] != "junit.xml" {
		t.Fail()
	}

	expected := map[string]string{"VAR": "hello world"}
	if !reflect.DeepEqual(config.Cmds[0].Env, expected) {
		t.Fail()
	}
}
