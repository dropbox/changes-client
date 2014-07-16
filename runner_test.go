package runner

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	var formData []FormData
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

	}))
	defer ts.Close()

	template := `
	{
		"api-uri": "%s/",
		"cmds": [
			{
				"id": "cmd_1",
				"script": "echo $VAR",
				"Env": {"VAR": "hello world"},
				"Cwd": "/tmp"
			}
		]
	}
	`

	config := &Config{}
	err := json.Unmarshal([]byte(fmt.Sprintf(template, ts.URL)), config)
	if err != nil {
		t.Errorf("Failed to parse build config")
	}

	reporter := NewReporter(config.ApiUri)
	runCmds(reporter, config)
	reporter.Shutdown()
}
