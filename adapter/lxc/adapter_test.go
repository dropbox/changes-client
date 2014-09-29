package lxcadapter

import (
	"github.com/dropbox/changes-client/client"
	"github.com/dropbox/changes-client/client/adapter"
	"log"
	"sync"
	"testing"
)

func reportLogChunks(clientLog *client.Log) {
	for chunk := range clientLog.Chan {
		log.Print(string(chunk))
	}
}

// TODO(dcramer): this needs to guarantee that the container isnt running already
func TestCompleteFlow(t *testing.T) {
	clientLog := client.NewLog()
	adapter, err := adapter.Get("lxc")
	if err != nil {
		t.Fatal(err.Error())
	}

	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		reportLogChunks(clientLog)
		wg.Done()
	}()

	config := &client.Config{}
	config.JobstepID = "job_1"

	err = adapter.Init(config)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = adapter.Prepare(clientLog)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer adapter.Shutdown(clientLog)

	wg.Wait()
}
