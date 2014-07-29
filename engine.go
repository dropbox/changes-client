package runner

import (
	"os"
	"path/filepath"
	"sync"
)

const (
	STATUS_QUEUED      = "queued"
	STATUS_IN_PROGRESS = "in_progress"
	STATUS_FINISHED    = "finished"
)

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

func RunAllCmds(reporter *Reporter, config *Config, result string, logsource *LogSource) {
	wg := sync.WaitGroup{}

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
			logsource.reportChunks(r.ChunkChan)
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

		wg.Add(1)
		go func(artifacts []string) {
			publishArtifacts(reporter, config.JobstepID, artifacts)
			wg.Done()
		}(cmd.Artifacts)
	}

	wg.Wait()
}

func RunBuildPlan(reporter *Reporter, source *Source, config *Config) {
	result := "passed"

	logsource := &LogSource{
		Name:      "console",
		JobstepID: config.JobstepID,
		Reporter:  reporter,
	}

	reporter.PushJobStatus(config.JobstepID, STATUS_IN_PROGRESS, "")

	// TODO(dcramer): the workspace setup needs to correctly report log chunks
	// back upstream
	err := source.SetupWorkspace(config.Workspace, reporter)
	if err != nil {
		result = "failed"
	} else {
		RunAllCmds(reporter, config, result, logsource)
	}

	reporter.PushJobStatus(config.JobstepID, STATUS_FINISHED, result)
}
