package runner

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
)

func TestProgressChunks(t *testing.T) {
	flag.Set("log_chunk_size", "3")

	in := []byte("aaa\naaa\naaa\n")
	ch := make(chan LogChunk)

	go func() {
		processChunks(ch, bytes.NewReader(in), "test")
		close(ch)
	}()

	cnt := 0
	for _ = range ch {
		cnt++
	}

	if cnt != 3 {
		t.Fail()
	}
}

type FormData struct {
	params map[string]string
	files  map[string]string
    path   string
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

	// Current running program is definitely an artifact which will be present in the pogram
	required_artifact := os.Args[0]

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
		]
	}
	`

	config := &Config{}
    config.Server = ts.URL
    config.JobID = "job_1"
	if json.Unmarshal([]byte(fmt.Sprintf(template, required_artifact)), config) != nil {
		t.Errorf("Failed to parse build config")
	}

	reporter := NewReporter(config.Server)
	RunCmds(reporter, config)
	reporter.Shutdown()

	if err != nil {
		t.Errorf(err.Error())
	}

	expectedFileContents, _ := ioutil.ReadFile(os.Args[0])
	expected := []FormData{
		FormData{
            path: "/jobsteps/job_1/",
			params: map[string]string{
				"status": STATUS_IN_PROGRESS,
			},
		},
		FormData{
            path: "/commands/cmd_1/",
			params: map[string]string{
				"status": STATUS_IN_PROGRESS,
			},
		},
		FormData{
            path: "/jobsteps/job_1/logappend/",
			params: map[string]string{
				"text":   "hello world",
				"source": "stdout",
				"offset": "0",
			},
		},
		FormData{
            path: "/commands/cmd_1/",
			params: map[string]string{
				"status": STATUS_FINISHED,
                "return_code": "0",
			},
		},
		FormData{
            path: "/jobsteps/job_1/artifacts/",
            params: map[string]string{
                "name": os.Args[0],
            },
			files: map[string]string{
                "file": string(expectedFileContents),
			},
		},
		FormData{
            path: "/commands/cmd_2/",
			params: map[string]string{
				"status": STATUS_IN_PROGRESS,
			},
		},
		FormData{
            path: "/jobsteps/job_1/logappend/",
			params: map[string]string{
				"text":   "test\n",
				"source": "stdout",
				"offset": "11",
			},
		},
		FormData{
            path: "/commands/cmd_2/",
			params: map[string]string{
				"status": STATUS_FINISHED,
                "return_code": "0",
			},
		},
		FormData{
            path: "/jobsteps/job_1/",
			params: map[string]string{
				"status": STATUS_FINISHED,
                "result": "passed",
			},
		},
	}

    for i, v := range formData {
        if !reflect.DeepEqual(v, expected[i]) {
            fmt.Println("A", i, v.params, expected[i].params)
            t.Fail()
        }
    }

	if !reflect.DeepEqual(formData, expected) {
		t.Errorf("Form data does not match")
	}
}
