package client

import (
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

const jobStepResponse = `
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
	},
	"debugConfig": {
		"some_env": {"Name": "wat", "Val": 4}
	}
}
`

func TestGetConfig(t *testing.T) {
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
		t.Fatal(err)
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

	if config.Cmds[0].ID != "cmd_1" {
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

	{
		var envthing struct {
			Name string
			Val  int
		}
		dok, derr := config.GetDebugConfig("some_env", &envthing)
		if !dok {
			t.Fail()
		}
		if derr != nil {
			t.Fail()
		}
		if envthing.Name != "wat" || envthing.Val != 4 {
			t.Errorf(`Expected ("wat", 4), got %#v`, envthing)
		}
	}
}

func TestDebugConfig(t *testing.T) {
	cases := []struct {
		json, key string
		dest      interface{}
		Ok        bool
		Error     bool
	}{
		// missing, no error, no ok
		{json: "{}",
			key: "absent", dest: new(string),
			Ok: false, Error: false},
		// type mismatch, ok, but error
		{json: `{"debugConfig": {"foo": 44}}`,
			key: "foo", dest: new(string),
			Ok: true, Error: true},
		// same as above, but proper type.
		{json: `{"debugConfig": {"foo": 44}}`,
			key: "foo", dest: new(int),
			Ok: true, Error: false},
	}

	for _, c := range cases {
		cfg, e := LoadConfig([]byte(c.json))
		if e != nil {
			panic(e)
		}
		ok, err := cfg.GetDebugConfig(c.key, c.dest)
		if ok != c.Ok {
			t.Errorf("For %q, extracting %q to %T, expected ok=%v, but was %v",
				c.json, c.key, c.dest, c.Ok, ok)
		}
		if (err != nil) != c.Error {
			msg := "expected"
			if !c.Error {
				msg = "didn't expect"
			}
			t.Errorf("For %q, extracting %q to %T, %s error, but got %#v",
				c.json, c.key, c.dest, msg, err)
		}
	}
}
