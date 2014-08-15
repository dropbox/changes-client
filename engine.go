package runner

import (
	"log"
	"os"
	"sync"
)

const (
	STATUS_QUEUED      = "queued"
	STATUS_IN_PROGRESS = "in_progress"
	STATUS_FINISHED    = "finished"
)

func publishArtifacts(reporter *Reporter, cID string, workspace string, artifacts []string) {
	if len(artifacts) == 0 {
		log.Printf("[engine] Skipping artifact collection")
		return
	}

	log.Printf("[engine] Collecting artifacts in %s matching %s", workspace, artifacts)

	matches, err := GlobTree(workspace, artifacts)
	if err != nil {
		panic("Invalid artifact pattern" + err.Error())
	}

	log.Printf("[engine] Found %d matching artifacts", len(matches))

	reporter.PushArtifacts(cID, matches)
}

func RunAllCmds(reporter *Reporter, config *Config, logsource *LogSource) string {
	result := "passed"

	wg := sync.WaitGroup{}

	for _, cmd := range config.Cmds {
		reporter.PushStatus(cmd.Id, STATUS_IN_PROGRESS, -1)
		wc, err := NewWrappedScriptCommand(cmd.Script, cmd.Id)
		if err != nil {
			reporter.PushStatus(cmd.Id, STATUS_FINISHED, 255)
			result = "failed"
			break
		}

		env := os.Environ()
		for k, v := range cmd.Env {
			env = append(env, k+"="+v)
		}
		wc.Cmd.Env = env

		if len(cmd.Cwd) > 0 {
			wc.Cmd.Dir = cmd.Cwd
		}

		bufferOutput := false
		if cmd.Type == "collect_jobs" || cmd.Type == "collect_tests" {
			bufferOutput = true
		}

		// Aritifacts can go out-of-band but we want to send logs synchronously.
		sem := make(chan bool)
		go func() {
			logsource.reportChunks(wc.ChunkChan)
			sem <- true
		}()

		pState, err := wc.Run(bufferOutput)

		// Wait for all the logs to be sent to reporter before sending command status.
		<-sem

		if bufferOutput {
			reporter.PushOutput(cmd.Id, cmd.Type, wc.Output)
		}

		if err != nil {
			reporter.PushStatus(cmd.Id, STATUS_FINISHED, 255)
			result = "failed"
		} else {
			if pState.Success() {
				reporter.PushStatus(cmd.Id, STATUS_FINISHED, 0)
			} else {
				reporter.PushStatus(cmd.Id, STATUS_FINISHED, 1)
				result = "failed"
			}
		}

		wg.Add(1)
		go func(artifacts []string) {
			publishArtifacts(reporter, config.JobstepID, config.Workspace, artifacts)
			wg.Done()
		}(cmd.Artifacts)

		if result == "failed" {
			break
		}
	}

	wg.Wait()
	return result
}

func RunBuildPlan(reporter *Reporter, config *Config) {
	logsource := &LogSource{
		Name:      "console",
		JobstepID: config.JobstepID,
		Reporter:  reporter,
	}

	reporter.PushJobStatus(config.JobstepID, STATUS_IN_PROGRESS, "")

	result := RunAllCmds(reporter, config, logsource)

	reporter.PushJobStatus(config.JobstepID, STATUS_FINISHED, result)
}
