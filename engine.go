package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

func reportChunks(r *Reporter, cID string, c chan LogChunk) {
	for l := range c {
		fmt.Printf("Got another chunk from %s (%d-%d)\n", l.Source, l.Offset, l.Length)
		fmt.Printf("%s", l.Payload)
		r.PushLogChunk(cID, l)
	}
}

func publishArtifacts(reporter *Reporter, cID string, artifacts []string) {
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

	reporter.PushArtifacts(cID, matches)
}

func runCmds(reporter *Reporter, config *Config) {
	wg := sync.WaitGroup{}
	for _, cmd := range config.Cmds {
		fmt.Println("Running", cmd.Id)
		reporter.PushStatus(cmd.Id, "STARTED")
		r := NewRunner(cmd.Id, cmd.Script)

		// Set job parameters
		env := os.Environ()
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
