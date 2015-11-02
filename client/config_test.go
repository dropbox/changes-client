package client

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
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
	"expectedSnapshot": {
		"id": "fed13008d3e94f6bb58e53237ad73f1d"
	},
	"resourceLimits": {
		"cpus": 4,
		"memory": 8127
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
	jobstepID := "549db9a70d4d4d258e0a6d475ccd8a15"

	config, err := GetConfig(jobstepID)
	assert.NoError(t, err)

	assert.Equal(t, config.Server, strings.TrimRight(ts.URL, "/"))

	assert.Equal(t, config.JobstepID, jobstepID)

	assert.Equal(t, config.Repository.Backend.ID, "git")

	assert.Equal(t, config.Repository.URL, "git@github.com:dropbox/changes.git")

	assert.Equal(t, config.Source.Patch.ID, "patch_1")

	assert.Equal(t, config.Source.Revision.Sha, "aaaaaa")

	assert.Equal(t, config.ExpectedSnapshot.ID, "fed13008d3e94f6bb58e53237ad73f1d")

	assert.Equal(t, len(config.Cmds), 2)

	assert.Equal(t, config.Cmds[0], ConfigCmd{
		ID:        "cmd_1",
		Script:    "#!/bin/bash\necho -n $VAR",
		Cwd:       "/tmp",
		Artifacts: []string{"junit.xml"},
		Env: map[string]string{
			"VAR": "hello world",
		},
	})

	assert.Equal(t, *config.ResourceLimits.Cpus, 4)
	assert.Equal(t, *config.ResourceLimits.Memory, 8127)

	type Pair struct {
		Name string
		Val  int
	}
	var envthing Pair
	dok, derr := config.GetDebugConfig("some_env", &envthing)
	assert.True(t, dok)
	assert.NoError(t, derr)
	assert.Equal(t, envthing, Pair{"wat", 4})
}

func TestParseResourceLimits(t *testing.T) {
	ptrto := func(p int) *int {
		return &p
	}
	cases := []struct {
		json     string
		expected ResourceLimits
	}{
		{`{"resourceLimits": {"cpus": 8}}`, ResourceLimits{Cpus: ptrto(8)}},
		{`{"resourceLimits": {"memory": 8000}}`, ResourceLimits{Memory: ptrto(8000)}},
		{`{"resourceLimits": {"cpus": 9, "memory": 8008}}`,
			ResourceLimits{Cpus: ptrto(9), Memory: ptrto(8008)}},
		{`{"resourceLimits": {}}`, ResourceLimits{}},
		{`{}`, ResourceLimits{}},
	}
	for i, c := range cases {
		cfg, e := LoadConfig([]byte(c.json))
		if e != nil {
			panic(e)
		}
		assert.Equal(t, c.expected, cfg.ResourceLimits, "case %v: %v", i, c.json)
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
