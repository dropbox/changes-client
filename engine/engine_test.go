package engine

import (
	"errors"
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

	"github.com/dropbox/changes-client/client"
	"github.com/dropbox/changes-client/client/adapter"
	"github.com/dropbox/changes-client/client/reporter"

	"gopkg.in/check.v1"
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

func testHttpCall(c *check.C, allData []FormData, lookIdx int, expectedData FormData) {
	if len(allData) < lookIdx+1 {
		c.Errorf("Expected data for call #%d, found none", lookIdx)
		c.Fail()
	} else if !reflect.DeepEqual(expectedData, allData[lookIdx]) {
		c.Error("A", lookIdx, allData[lookIdx].params, expectedData.params)
		c.Fail()
	}
}

func TestEngine(t *testing.T) { check.TestingT(t) }

type EngineSuite struct{}

var _ = check.Suite(&EngineSuite{})

type noartReporter struct{}

func (nar *noartReporter) Init(_ *client.Config) {}
func (nar *noartReporter) PublishArtifacts(_ client.ConfigCmd, _ adapter.Adapter, _ *client.Log) error {
	return errors.New("Couldn't publish artifacts somehow")
}

func (nar *noartReporter) PushCommandOutput(_, _ string, _ int, _ []byte) {}
func (nar *noartReporter) PushCommandStatus(_, _ string, _ int)           {}
func (nar *noartReporter) PushJobstepStatus(_, _ string)                  {}
func (nar *noartReporter) PushLogChunk(_ string, _ []byte)                {}
func (nar *noartReporter) PushSnapshotImageStatus(_, _ string)            {}
func (nar *noartReporter) Shutdown()                                      {}

var _ reporter.Reporter = &noartReporter{}

type noopAdapter struct{}

func (_ *noopAdapter) Init(*client.Config) error { return nil }
func (_ *noopAdapter) Prepare(*client.Log) error { return nil }
func (_ *noopAdapter) Run(*client.Command, *client.Log) (*client.CommandResult, error) {
	return &client.CommandResult{
		Success: true,
	}, nil
}
func (_ *noopAdapter) Shutdown(*client.Log) error                { return nil }
func (_ *noopAdapter) CaptureSnapshot(string, *client.Log) error { return nil }
func (_ *noopAdapter) GetRootFs() string {
	return "/"
}
func (_ *noopAdapter) CollectArtifacts([]string, *client.Log) ([]string, error) {
	return nil, nil
}

func (s *EngineSuite) TestFailedArtifactInfraFails(c *check.C) {
	nar := new(noartReporter)
	log := client.NewLog()
	defer log.Close()
	go log.Drain()
	eng := Engine{reporter: nar,
		clientLog: log,
		adapter:   &noopAdapter{},
		config: &client.Config{Cmds: []client.ConfigCmd{
			{Artifacts: []string{"result.xml"}},
		}}}
	r, e := eng.executeCommands()
	c.Assert(r, check.Equals, RESULT_INFRA_FAILED)
	c.Assert(e, check.NotNil)
}

func (s *EngineSuite) ensureContainerRemoved(c *check.C) {
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
	}))
	defer ts.Close()

	host, _ := os.Hostname()

	artifactPath := os.Args[0]
	args := strings.Split(artifactPath, "/")
	workspaceRoot := strings.Join(args[0:len(args)-2], "/")
	artifactName := args[len(args)-1]

	config := &client.Config{}
	config.Server = ts.URL
	config.JobstepID = "job_1"
	config.ArtifactSearchPath = workspaceRoot
	config.Repository.Backend.ID = "git"
	config.Repository.URL = "https://github.com/dropbox/changes.git"
	config.Source.Revision.Sha = "master"
	config.Cmds = append(config.Cmds, client.ConfigCmd{
		ID:     "cmd_1",
		Script: "#!/bin/bash\necho -n $VAR",
		Env: map[string]string{
			"VAR": "hello world",
		},
		Cwd:       "/tmp",
		Artifacts: []string{artifactName},
	}, client.ConfigCmd{
		ID:     "cmd_2",
		Script: "#!/bin/bash\nexit 1",
		Cwd:    "/tmp",
	}, client.ConfigCmd{
		ID:     "cmd_3",
		Script: "#!/bin/bash\necho test",
		Cwd:    "/tmp",
	})

	RunBuildPlan(config)

	c.Assert(err, check.IsNil)

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

	c.Assert(len(formData), check.Equals, 15)
}
