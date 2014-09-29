package engine

import (
	"fmt"
	"github.com/dropbox/changes-client/adapter/basic"
	"github.com/dropbox/changes-client/client"
	"github.com/dropbox/changes-client/common"
	"github.com/dropbox/changes-client/reporter"
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

func RunAllCmds(reporter *reporter.Reporter, config *client.Config, log *client.Log) string {
	var err error

	result := RESULT_PASSED

	adapter, err := basic.NewAdapter(config)
	if err != nil {
		// TODO(dcramer): handle this error. We need to refactor how the log/wg works
		// so that we can report it upstream without giant logic blocks
		return RESULT_FAILED
	}

	wg := sync.WaitGroup{}

	err = adapter.Prepare(log)
	if err != nil {
		// TODO(dcramer): we need to ensure that logging gets generated for prepare
		return RESULT_FAILED
	}

	for _, cmdConfig := range config.Cmds {
		cmd, err := client.NewCommand(cmdConfig.ID, cmdConfig.Script)
		if err != nil {
			reporter.PushStatus(cmd.ID, STATUS_FINISHED, 255)
			result = RESULT_FAILED
			break
		}
		reporter.PushStatus(cmd.ID, STATUS_IN_PROGRESS, -1)

		cmd.CaptureOutput = cmdConfig.CaptureOutput

		env := os.Environ()
		for k, v := range cmdConfig.Env {
			env = append(env, k+"="+v)
		}
		cmd.Env = env

		if len(cmdConfig.Cwd) > 0 {
			cmd.Cwd = cmdConfig.Cwd
		}

		cmdResult, err := adapter.Run(cmd, log)

		if err != nil {
			reporter.PushStatus(cmd.ID, STATUS_FINISHED, 255)
			result = RESULT_FAILED
		} else {
			if cmdResult.Success() {
				if cmd.CaptureOutput {
					reporter.PushOutput(cmd.ID, STATUS_FINISHED, 0, cmdResult.Output)
				} else {
					reporter.PushStatus(cmd.ID, STATUS_FINISHED, 0)
				}
			} else {
				reporter.PushStatus(cmd.ID, STATUS_FINISHED, 1)
				result = RESULT_FAILED
			}
		}

		wg.Add(1)
		go func(artifacts []string) {
			publishArtifacts(reporter, log, config.Workspace, artifacts)
			wg.Done()
		}(cmdConfig.Artifacts)

		if result == RESULT_FAILED {
			break
		}
	}

	err = adapter.Shutdown(log)

	wg.Wait()

	if err != nil {
		// TODO(dcramer): we need to ensure that logging gets generated for prepare
		// XXX(dcramer): we probably don't need to fail here as a shutdown operation
		// should be recoverable
		return RESULT_FAILED
	}

	return result
}

func RunBuildPlan(r *reporter.Reporter, config *client.Config) {
	log := client.NewLog()

	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		reportLogChunks("console", log, r)
		wg.Done()
	}()

	r.PushJobStatus(STATUS_IN_PROGRESS, "")

	result := RunAllCmds(r, config, log)

	r.PushJobStatus(STATUS_FINISHED, result)

	log.Close()

	wg.Wait()
}

func reportLogChunks(name string, l *client.Log, r *reporter.Reporter) {
	for chunk := range l.Chan {
		r.PushLogChunk(name, chunk)
	}
}

func publishArtifacts(r *reporter.Reporter, log *client.Log, workspace string, artifacts []string) {
	if len(artifacts) == 0 {
		log.Writeln(">> Skipping artifact collection")
		return
	}

	log.Writeln(fmt.Sprintf(">> Collecting artifacts in %s matching %s", workspace, artifacts))

	matches, err := common.GlobTree(workspace, artifacts)
	if err != nil {
		log.Writeln(fmt.Sprintf("Invalid artifact pattern: " + err.Error()))
		return
	}

	log.Writeln(fmt.Sprintf("Found %d matching artifacts", len(matches)))

	r.PushArtifacts(matches)
}
