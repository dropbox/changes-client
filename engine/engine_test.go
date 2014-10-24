package engine

import (
	"fmt"
	"github.com/dropbox/changes-client/client"
	"github.com/dropbox/changes-client/reporter"
	"github.com/jarcoal/httpmock"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func formDataFromRequest(req *http.Request) (FormData, error) {
	var err error

	f := FormData{
		params: make(map[string]string),
		path: req.URL.Path,
	}

	req.ParseMultipartForm(1 << 20)
	for k, v := range req.MultipartForm.Value {
		if k == "date" {
			continue
		}
		if len(v) != 1 {
			err = fmt.Errorf("Multiple values for form field: %s", k)
			return f, err
		}

		f.params[k] = v[0]
	}

	if len(req.MultipartForm.File) > 0 {
		f.files = make(map[string]string)

		files := req.MultipartForm.File
		if len(files) != 1 {
			err = fmt.Errorf("Invalid number of artifacts found")
			return f, err
		}

		for filename, fileHeaders := range files {
			if len(fileHeaders) != 1 {
				err = fmt.Errorf("Multiple file headers found")
				return f, err
			}
			file, err := fileHeaders[0].Open()
			if err != nil {
				return f, err
			}

			fileContents, err := ioutil.ReadAll(file)
			if err != nil {
				return f, err
			}

			f.files[filename] = string(fileContents)
		}
	}
	return f, nil
}

func testHttpCall(c *C, allData []FormData, lookIdx int, expectedData FormData) {
	if len(allData) < lookIdx+1 {
		c.Errorf("Expected data for call #%d, found none", lookIdx)
		c.Fail()
	} else {
		c.Assert(allData[lookIdx].params, DeepEquals, expectedData.params)
	}
}

func TestEngine(t *testing.T) { TestingT(t) }

type EngineSuite struct{}

var _ = Suite(&EngineSuite{})

func (s *EngineSuite) TearDownSuite(c *C) {
	httpmock.Reset()
}

func (s *EngineSuite) TestCompleteFlow(c *C) {
	var err error
	var formData []FormData
	captureFormDataResponder := func(successResponse *http.Response) httpmock.Responder {
		return func(req *http.Request) (*http.Response, error) {
			f, err := formDataFromRequest(req)
			if err != nil {
				return httpmock.NewStringResponse(500, ""), err
			}

			formData = append(formData, f)

			return successResponse, nil
		}
	}

	httpmock.RegisterResponder("GET", "https://changes.example.com/api/0/jobsteps/job_1/",
		captureFormDataResponder(httpmock.NewStringResponse(200, jobStepResponse)))

	httpmock.RegisterResponder("POST", "https://changes.example.com/api/0/jobsteps/job_1/",
		captureFormDataResponder(httpmock.NewStringResponse(200, jobStepResponse)))

	httpmock.RegisterResponder("POST", "https://changes.example.com/api/0/jobsteps/job_1/logappend/",
		captureFormDataResponder(httpmock.NewStringResponse(200, "")))

	httpmock.RegisterResponder("POST", "https://changes.example.com/api/0/jobsteps/job_1/artifacts/",
		captureFormDataResponder(httpmock.NewStringResponse(200, "")))

	httpmock.RegisterResponder("POST", "https://changes.example.com/api/0/commands/cmd_1/",
		captureFormDataResponder(httpmock.NewStringResponse(200, "")))

	httpmock.RegisterResponder("POST", "https://changes.example.com/api/0/commands/cmd_2/",
		captureFormDataResponder(httpmock.NewStringResponse(200, "")))

	artifactPath := os.Args[0]
	args := strings.Split(artifactPath, "/")
	workspaceRoot := strings.Join(args[0:len(args)-2], "/")
	artifactName := args[len(args)-1]
	host, _ := os.Hostname()

	config := &client.Config{}
	config.Server = "https://changes.example.com/api/0"
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

	r := reporter.NewReporter(httpmock.DefaultTransport, config.Server, config.JobstepID, false)
	err = RunBuildPlan(r, config)
	r.Shutdown()

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
	//      path: "/jobsteps/job_1/logappend/",
	//      params: map[string]string{
	//              "text":   ">> cmd_1\n",
	//              "source": "console",
	//      },
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
	// call #8 is the "running command" log
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

	c.Assert(len(formData), Equals, 14)
}
