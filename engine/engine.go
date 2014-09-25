package engine

import (
	"github.com/dropbox/changes-client/client"
	"github.com/dropbox/changes-client/common"
	"github.com/dropbox/changes-client/adapter/basic"
	"log"
	"os"
	"sync"
)

const (
	STATUS_QUEUED      = "queued"
	STATUS_IN_PROGRESS = "in_progress"
	STATUS_FINISHED    = "finished"

	RESULT_PASSED = "passed"
	RESULT_FAILED = "failed"
)

func RunAllCmds(reporter *client.Reporter, config *client.Config, logsource *client.LogSource) string {
	var err error

	result := RESULT_PASSED

	adapter := basic.NewAdapter(config)

	err = adapter.Prepare()
	if err != nil {
		// TODO(dcramer): we need to ensure that logging gets generated for prepare
		return RESULT_FAILED
	}

	wg := sync.WaitGroup{}

	for _, cmd := range config.Cmds {
		reporter.PushStatus(cmd.Id, STATUS_IN_PROGRESS, -1)
		wc, err := client.NewScriptCommand(cmd.Script, cmd.Id)
		if err != nil {
			reporter.PushStatus(cmd.Id, STATUS_FINISHED, 255)
			result = RESULT_FAILED
			break
		}

		wc.BufferOutput = cmd.CaptureOutput

		env := os.Environ()
		for k, v := range cmd.Env {
			env = append(env, k+"="+v)
		}
		wc.Cmd.Env = env

		if len(cmd.Cwd) > 0 {
			wc.Cmd.Dir = cmd.Cwd
		}

		// Aritifacts can go out-of-band but we want to send logs synchronously.
		sem := make(chan bool)
		go func() {
			logsource.ReportChunks(wc.ChunkChan)
			sem <- true
		}()

		pState, err := adapter.Run(wc)

		// Wait for all the logs to be sent to reporter before sending command status.
		<-sem

		if err != nil {
			reporter.PushStatus(cmd.Id, STATUS_FINISHED, 255)
			result = RESULT_FAILED
		} else {
			if pState.Success() {
				if cmd.CaptureOutput {
					reporter.PushOutput(cmd.Id, STATUS_FINISHED, 0, wc.Output)
				} else {
					reporter.PushStatus(cmd.Id, STATUS_FINISHED, 0)
				}
			} else {
				reporter.PushStatus(cmd.Id, STATUS_FINISHED, 1)
				result = RESULT_FAILED
			}
		}

		wg.Add(1)
		go func(artifacts []string) {
			publishArtifacts(reporter, config.JobstepID, config.Workspace, artifacts)
			wg.Done()
		}(cmd.Artifacts)

		if result == RESULT_FAILED {
			break
		}
	}

	wg.Wait()

	err = adapter.Shutdown()
	if err != nil {
		// TODO(dcramer): we need to ensure that logging gets generated for prepare
		// XXX(dcramer): we probably don't need to fail here as a shutdown operation
		// should be recoverable
		return RESULT_FAILED
	}

	return result
}

func RunBuildPlan(reporter *client.Reporter, config *client.Config) {
	logsource := &client.LogSource{
		Name:      "console",
		JobstepID: config.JobstepID,
		Reporter:  reporter,
	}

	reporter.PushJobStatus(config.JobstepID, STATUS_IN_PROGRESS, "")

	result := RunAllCmds(reporter, config, logsource)

	reporter.PushJobStatus(config.JobstepID, STATUS_FINISHED, result)
}

func publishArtifacts(reporter *client.Reporter, cID string, workspace string, artifacts []string) {
	if len(artifacts) == 0 {
		log.Printf("[engine] Skipping artifact collection")
		return
	}

	log.Printf("[engine] Collecting artifacts in %s matching %s", workspace, artifacts)

	matches, err := common.GlobTree(workspace, artifacts)
	if err != nil {
		panic("Invalid artifact pattern" + err.Error())
	}

	log.Printf("[engine] Found %d matching artifacts", len(matches))

	reporter.PushArtifacts(cID, matches)
}
