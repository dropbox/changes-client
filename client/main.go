package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/dropbox/changes-client"
)

func reportChunks(r *runner.Reporter, cId string, c chan runner.LogChunk) {
	for l := range c {
		fmt.Printf("Got another chunk from %s (%d-%d)\n", l.Source, l.Offset, l.Length)
		fmt.Printf("%s", l.Payload)
		r.PushLogChunk(cId, l)
	}
}

func publishArtifacts(reporter *runner.Reporter, cId string, artifacts []string) {
	if len(artifacts) == 0 {
		return
	}
	var matches []string
	for _, pattern := range artifacts {
		m, err := filepath.Glob(pattern)
		if err != nil {
			panic("Invalid artifact pattern" + err.Error())
		}
		matches = append(matches, m...)
	}
	reporter.PushArtifacts(cId, matches)
}

func runCmds(reporter *runner.Reporter, config *runner.Config) {
	wg := sync.WaitGroup{}
	for _, cmd := range config.Cmds {
		fmt.Println("Running", cmd.Id)
		reporter.PushStatus(cmd.Id, "STARTED")
		r := runner.NewRunner(cmd.Id, cmd.Script)

		// Set job parameters
		var env []string = os.Environ()
		for k, v := range cmd.Env {
			env = append(env, k+"="+v)
		}

		r.Cmd.Env = env
		r.Cmd.Dir = cmd.Cwd

		wg.Add(1)
		go func() {
			reportChunks(reporter, cmd.Id, r.ChunkChan)
			wg.Done()
		}()

		pState, err := r.Run()
		if err != nil {
			fmt.Println(err)
			reporter.PushStatus(cmd.Id, "FAILED")
			break
		} else {
			reporter.PushStatus(cmd.Id, pState.String())
		}
		publishArtifacts(reporter, cmd.Id, cmd.Artifacts)
	}

	wg.Wait()
}

func main() {
	flag.Parse()

	config, err := runner.GetConfig()
	if err != nil {
		panic(err)
	}

	// Make a reporter and use it
	reporter := runner.NewReporter(config.ApiUri)
	runCmds(reporter, config)
	reporter.Shutdown()
}
