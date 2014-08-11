package runner

import (
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

var jobStepResponse = `
{
	"id": "549db9a70d4d4d258e0a6d475ccd8a15",
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

func TestGetConfig(t *testing.T) {
	var err error

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// XXX(dcramer): the input URL is the API base so these paths wouldn't include it
		if r.Method == "GET" {
			switch r.URL.Path {
			case "/jobsteps/549db9a70d4d4d258e0a6d475ccd8a15/":
				io.WriteString(w, jobStepResponse)
			}
		}
	}))
	defer ts.Close()

	server = ts.URL
	jobstepID = "549db9a70d4d4d258e0a6d475ccd8a15"

	config, err := GetConfig()
	if err != nil {
		t.Errorf(err.Error())
	}

	if config.Server != strings.TrimRight(ts.URL, "/") {
		t.Fail()
	}

	if config.JobstepID != jobstepID {
		t.Fail()
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
