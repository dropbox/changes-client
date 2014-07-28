package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const (
	STATUS_QUEUED      = "queued"
	STATUS_IN_PROGRESS = "in_progress"
	STATUS_FINISHED    = "finished"
)

type OffsetMap struct {
	mu            sync.Mutex
	sourceOffsets map[string]int
}

func (m *OffsetMap) get(source string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sourceOffsets[source]
}

func (m *OffsetMap) set(source string, offset int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sourceOffsets[source] = offset
}

func reportChunks(r *Reporter, cID string, c chan LogChunk, offsetMap *OffsetMap) {
	for l := range c {
		// Override offset until we figure how to show multi streams in changes UI
		sourceOffset := offsetMap.get(l.Source)
		l.Offset = sourceOffset

		sourceOffset += l.Length
		offsetMap.set(l.Source, sourceOffset)

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

func RunCmds(reporter *Reporter, config *Config) {
	result := "passed"
	defer reporter.PushJobStatus(config.JobstepID, STATUS_FINISHED, result)

	wg := sync.WaitGroup{}
	reporter.PushJobStatus(config.JobstepID, STATUS_IN_PROGRESS, "")

	offsetMap := OffsetMap{sourceOffsets: make(map[string]int)}

	for _, cmd := range config.Cmds {
		reporter.PushStatus(cmd.Id, STATUS_IN_PROGRESS, -1)
		r, err := NewRunner(cmd.Id, cmd.Script)
		if err != nil {
			reporter.PushStatus(cmd.Id, STATUS_FINISHED, 255)
			result = "failed"
			break
		}

		env := os.Environ()
		for k, v := range cmd.Env {
			env = append(env, k+"="+v)
		}
		r.Cmd.Env = env

		if len(cmd.Cwd) > 0 {
			r.Cmd.Dir = cmd.Cwd
		}

		wg.Add(1)
		go func() {
			reportChunks(reporter, config.JobstepID, r.ChunkChan, &offsetMap)
			wg.Done()
		}()

		pState, err := r.Run()
		if err != nil {
			reporter.PushStatus(cmd.Id, STATUS_FINISHED, 255)
			result = "failed"
			break
		} else {
			if pState.Success() {
				reporter.PushStatus(cmd.Id, STATUS_FINISHED, 0)
			} else {
				reporter.PushStatus(cmd.Id, STATUS_FINISHED, 1)
				result = "failed"
				break
			}
		}

		publishArtifacts(reporter, config.JobstepID, cmd.Artifacts)
	}

	wg.Wait()
}
