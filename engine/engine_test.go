package engine

import (
	"fmt"
	"github.com/dropbox/changes-client/client"
	"github.com/dropbox/changes-client/reporter"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type FormData struct {
	params map[string]string
	files  map[string]string
	path   string
}

func testHttpCall(t *testing.T, allData []FormData, lookIdx int, expectedData FormData) {
	if len(allData) < lookIdx+1 {
		t.Errorf("Expected data for call #%d, found none", lookIdx)
		t.Fail()
	} else if !reflect.DeepEqual(expectedData, allData[lookIdx]) {
		t.Errorf("A", lookIdx, allData[lookIdx].params, expectedData.params)
		t.Fail()
	}
}

func TestCompleteFlow(t *testing.T) {
	var err error
	var formData []FormData
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))

		if r.Method != "POST" {
			err = fmt.Errorf("Non-POST request received: %s", r.Method)
			return
		}

		r.ParseMultipartForm(1 << 20)
		f := FormData{params: make(map[string]string), path: r.URL.Path}

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

	config := &client.Config{}
	config.Server = ts.URL
	config.JobstepID = "job_1"
	config.Workspace = workspaceRoot
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

	reporter := reporter.NewReporter(config.Server, config.JobstepID, false)
	RunBuildPlan(reporter, config)
	reporter.Shutdown()

	if err != nil {
		t.Errorf(err.Error())
	}

	expectedFileContents, _ := ioutil.ReadFile(os.Args[0])

	testHttpCall(t, formData, 0, FormData{
		path: "/jobsteps/job_1/",
		params: map[string]string{
			"status": STATUS_IN_PROGRESS,
			"node":   host,
		},
	})

	testHttpCall(t, formData, 1, FormData{
		path: "/commands/cmd_1/",
		params: map[string]string{
			"status": STATUS_IN_PROGRESS,
		},
	})

	// testHttpCall(t, formData, 2, FormData{
	// 	path: "/jobsteps/job_1/logappend/",
	// 	params: map[string]string{
	// 		"text":   ">> cmd_1\n",
	// 		"source": "console",
	// 	},
	// })

	testHttpCall(t, formData, 3, FormData{
		path: "/jobsteps/job_1/logappend/",
		params: map[string]string{
			"text":   "hello world",
			"source": "console",
		},
	})

	testHttpCall(t, formData, 4, FormData{
		path: "/commands/cmd_1/",
		params: map[string]string{
			"status":      STATUS_FINISHED,
			"return_code": "0",
		},
	})

	testHttpCall(t, formData, 5, FormData{
		path: "/commands/cmd_2/",
		params: map[string]string{
			"status": STATUS_IN_PROGRESS,
		},
	})

	// call #6 is the "running command" log
	// call #7 is the "collecting artifacts" log

	testHttpCall(t, formData, 8, FormData{
		path: "/jobsteps/job_1/artifacts/",
		params: map[string]string{
			"name": filepath.Base(artifactPath),
		},
		files: map[string]string{
			"file": string(expectedFileContents),
		},
	})

	// call #9 is the "found N artifacts" log

	testHttpCall(t, formData, 10, FormData{
		path: "/commands/cmd_2/",
		params: map[string]string{
			"status":      STATUS_FINISHED,
			"return_code": "255",
		},
	})

	testHttpCall(t, formData, 11, FormData{
		path: "/jobsteps/job_1/logappend/",
		params: map[string]string{
			"text":   "exit status 1\n",
			"source": "console",
		},
	})

	// call #12 is the "skipping artifact collection" log

	testHttpCall(t, formData, 13, FormData{
		path: "/jobsteps/job_1/",
		params: map[string]string{
			"status": STATUS_FINISHED,
			"result": "failed",
			"node":   host,
		},
	})

	if len(formData) != 14 {
		t.Errorf("Expected 14 HTTP calls, found %d", len(formData))
	}
}
