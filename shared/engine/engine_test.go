package engine

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dropbox/changes-client/shared/reporter"
	"github.com/dropbox/changes-client/shared/runner"

	. "gopkg.in/check.v1"
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
	"result": {
		"id": "unknown"
	},
	"status": {
		"id": "unknown"
	},
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

type FormData struct {
	params map[string]string
	files  map[string]string
	path   string
}

func testHttpCall(c *C, allData []FormData, lookIdx int, expectedData FormData) {
	if len(allData) < lookIdx+1 {
		c.Errorf("Expected data for call #%d, found none", lookIdx)
		c.Fail()
	} else if !reflect.DeepEqual(expectedData, allData[lookIdx]) {
		c.Errorf("A", lookIdx, allData[lookIdx].params, expectedData.params)
		c.Fail()
	}
}

func TestEngine(t *testing.T) { TestingT(t) }

type EngineSuite struct{}

var _ = Suite(&EngineSuite{})

func (s *EngineSuite) ensureContainerRemoved(c *C) {
	var err error
	var formData []FormData

	c.ExpectFailure("This test is brittle and needs rewritten")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			err = fmt.Errorf("Unexpected %s request received: %s", r.Method, r.URL.Path)
			return
		}

		if r.URL.Path == "/jobsteps/job_1/heartbeat/" {
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, jobStepResponse)
			return
		}

		w.Write([]byte("OK"))

		r.ParseMultipartForm(1 << 20)
		f := FormData{params: make(map[string]string), path: r.URL.Path}

		if r.MultipartForm != nil {
			for k, v := range r.MultipartForm.Value {
				if k == "date" {
					continue
				}
				if len(v) != 1 {
					err = fmt.Errorf("Multiple values for form field: %s", k)
					return
				}

				f.params[k] = v[0]
			}

			if len(r.MultipartForm.File) > 0 {
				f.files = make(map[string]string)

				files := r.MultipartForm.File
				if len(files) != 1 {
					err = fmt.Errorf("Invalid number of artifacts found")
					return
				}

				for filename, fileHeaders := range files {
					if len(fileHeaders) != 1 {
						err = fmt.Errorf("Multiple file headers found")
						return
					}

					file, err := fileHeaders[0].Open()
					if err != nil {
						return
					}
					fileContents, err := ioutil.ReadAll(file)
					if err != nil {
						return
					}

					f.files[filename] = string(fileContents)
				}
			}
		}

		formData = append(formData, f)
		return

		err = fmt.Errorf("Unexpected path: %s", r.URL.Path)
	}))
	defer ts.Close()

	host, _ := os.Hostname()

	artifactPath := os.Args[0]
	args := strings.Split(artifactPath, "/")
	workspaceRoot := strings.Join(args[0:len(args)-2], "/")
	artifactName := args[len(args)-1]

	config := &runner.Config{}
	config.Server = ts.URL
	config.Workspace = workspaceRoot
	config.Repository.Backend.ID = "git"
	config.Repository.URL = "https://github.com/dropbox/changes.git"
	config.Source.Revision.Sha = "master"
	config.Cmds = append(config.Cmds, runner.ConfigCmd{
		ID:     "cmd_1",
		Script: "#!/bin/bash\necho -n $VAR",
		Env: map[string]string{
			"VAR": "hello world",
		},
		Cwd:       "/tmp",
		Artifacts: []string{artifactName},
	}, runner.ConfigCmd{
		ID:     "cmd_2",
		Script: "#!/bin/bash\nexit 1",
		Cwd:    "/tmp",
	}, runner.ConfigCmd{
		ID:     "cmd_3",
		Script: "#!/bin/bash\necho test",
		Cwd:    "/tmp",
	})

	r := reporter.NewJobStepReporter(config.Server, "job_1", config.Debug)
	defer r.Shutdown()

	engine, err := NewEngine(config)

	c.Assert(err, IsNil)

	engine.Run(r, "")

	c.Assert(err, IsNil)

	expectedFileContents, _ := ioutil.ReadFile(os.Args[0])

	testHttpCall(c, formData, 0, FormData{
		path: "/jobsteps/job_1/",
		params: map[string]string{
			"status": STATUS_IN_PROGRESS,
			"node":   host,
		},
	})

	testHttpCall(c, formData, 1, FormData{
		path: "/commands/cmd_1/",
		params: map[string]string{
			"status": STATUS_IN_PROGRESS,
		},
	})

	// testHttpCall(c, formData, 2, FormData{
	// 	path: "/jobsteps/job_1/logappend/",
	// 	params: map[string]string{
	// 		"text":   ">> cmd_1\n",
	// 		"source": "console",
	// 	},
	// })

	testHttpCall(c, formData, 3, FormData{
		path: "/jobsteps/job_1/logappend/",
		params: map[string]string{
			"text":   "hello world",
			"source": "console",
		},
	})

	testHttpCall(c, formData, 4, FormData{
		path: "/commands/cmd_1/",
		params: map[string]string{
			"status":      STATUS_FINISHED,
			"return_code": "0",
		},
	})

	testHttpCall(c, formData, 5, FormData{
		path: "/jobsteps/job_1/artifacts/",
		params: map[string]string{
			"name": filepath.Base(artifactPath),
		},
		files: map[string]string{
			"file": string(expectedFileContents),
		},
	})

	testHttpCall(c, formData, 6, FormData{
		path: "/commands/cmd_2/",
		params: map[string]string{
			"status": STATUS_IN_PROGRESS,
		},
	})

	// call #7 is the "running command" log
	// call #8 is the "collecting artifacts" log
	// call #9 is the "found N artifacts" log

	testHttpCall(c, formData, 10, FormData{
		path: "/commands/cmd_2/",
		params: map[string]string{
			"status":      STATUS_FINISHED,
			"return_code": "255",
		},
	})

	testHttpCall(c, formData, 11, FormData{
		path: "/jobsteps/job_1/logappend/",
		params: map[string]string{
			"text":   "exit status 1\n",
			"source": "console",
		},
	})

	// call #12 is the "skipping artifact collection" log

	testHttpCall(c, formData, 13, FormData{
		path: "/jobsteps/job_1/",
		params: map[string]string{
			"status": STATUS_FINISHED,
			"result": "failed",
			"node":   host,
		},
	})

	c.Assert(len(formData), Equals, 15)
}
