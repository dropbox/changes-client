package runner

import (
    "os"
    "io/ioutil"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
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

		if !strings.HasPrefix(r.URL.Path, "/cmd_1/") {
			err = fmt.Errorf("Command ID missing from path: %s", r.URL.Path)
			return
		}

		if r.URL.Path == "/cmd_1/status" || r.URL.Path == "/cmd_1/logappend" {
			r.ParseMultipartForm(1024)
			f := FormData{params: make(map[string]string)}

			for k, v := range r.MultipartForm.Value {
				if len(v) != 1 {
					err = fmt.Errorf("Multiple values for form field: %s", k)
					return
				}

				fmt.Println(r.URL.Path, k, v)
				f.params[k] = v[0]
			}

			formData = append(formData, f)
			return
        }

        if r.URL.Path == "/cmd_1/artifact" {
            r.ParseMultipartForm(1 << 20)
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
                f := FormData{files: make(map[string]string)}
                f.files[filename] = string(fileContents)
                formData = append(formData, f)
            }
            return
        }

		err = fmt.Errorf("Unexpected path: %s", r.URL.Path)
	}))
	defer ts.Close()
    // Current running program is definitely an artifact which will be present in the pogram
    required_artifact := os.Args[0]

	template := `
	{
		"api-uri": "%s/",
		"cmds": [
			{
				"id": "cmd_1",
				"script": "#!/bin/bash\necho -n $VAR",
				"env": {"VAR": "hello world"},
				"cwd": "/tmp",
                "artifacts": ["%s"]
			}
		]
	}
	`

	config := &Config{}
	if json.Unmarshal([]byte(fmt.Sprintf(template, ts.URL, required_artifact)), config) != nil {
		t.Errorf("Failed to parse build config")
	}

	reporter := NewReporter(config.ApiUri)
	runCmds(reporter, config)
	reporter.Shutdown()

	if err != nil {
		t.Errorf(err.Error())
	}

    expectedFileContents, _ := ioutil.ReadFile(os.Args[0])
	expected := []FormData{
		FormData{
			params: map[string]string{
				"status": "STARTED",
			},
		},
		FormData{
			params: map[string]string{
				"text":   "hello world",
				"source": "stdout",
				"offset": "0",
			},
		},
		FormData{
			params: map[string]string{
				"status": "exit status 0",
			},
		},
        FormData {
            files: map[string]string {
                os.Args[0]: string(expectedFileContents),
            },
        },
	}

	if !reflect.DeepEqual(formData, expected) {
		t.Errorf("Form data does not match")
	}
}
