package engine

import (
	"flag"
	"fmt"
	"github.com/dropbox/changes-client/client"
	"github.com/dropbox/changes-client/client/adapter"
	"github.com/dropbox/changes-client/common"
	"github.com/dropbox/changes-client/reporter"
	"log"
	"os"
	"sync"

	// we import the default adapter here to ensure its always available
	"github.com/dropbox/changes-client/adapter/basic"
)

const (
	STATUS_QUEUED      = "queued"
	STATUS_IN_PROGRESS = "in_progress"
	STATUS_FINISHED    = "finished"

	RESULT_PASSED = "passed"
	RESULT_FAILED = "failed"
)

var (
	selectedAdapter string
)

func RunAllCmds(reporter *reporter.Reporter, config *client.Config, clientLog *client.Log) string {
	var err error

	result := RESULT_PASSED

	currentAdapter, err := adapter.Get(selectedAdapter)
	if err != nil {
		// TODO(dcramer): handle this error. We need to refactor how the log/wg works
		// so that we can report it upstream without giant logic blocks
		log.Print(err.Error())
		return RESULT_FAILED
	}

	err = currentAdapter.Init(config)
	if err != nil {
		log.Print(err.Error())
		// TODO(dcramer): handle this error. We need to refactor how the log/wg works
		// so that we can report it upstream without giant logic blocks
		return RESULT_FAILED
	}

	wg := sync.WaitGroup{}

	err = currentAdapter.Prepare(clientLog)
	if err != nil {
		// TODO(dcramer): we need to ensure that logging gets generated for prepare
		return RESULT_FAILED
	}
	defer currentAdapter.Shutdown(clientLog)

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

		cmdResult, err := currentAdapter.Run(cmd, clientLog)

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
			publishArtifacts(reporter, clientLog, config.Workspace, artifacts)
			wg.Done()
		}(cmdConfig.Artifacts)

		if result == RESULT_FAILED {
			break
		}
	}

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
	clientLog := client.NewLog()

	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		reportLogChunks("console", clientLog, r)
		wg.Done()
	}()

	r.PushJobStatus(STATUS_IN_PROGRESS, "")

	result := RunAllCmds(r, config, clientLog)

	r.PushJobStatus(STATUS_FINISHED, result)

	clientLog.Close()

	wg.Wait()
}

func reportLogChunks(name string, clientLog *client.Log, r *reporter.Reporter) {
	for chunk := range clientLog.Chan {
		r.PushLogChunk(name, chunk)
	}
}

func publishArtifacts(r *reporter.Reporter, clientLog *client.Log, workspace string, artifacts []string) {
	if len(artifacts) == 0 {
		clientLog.Writeln(">> Skipping artifact collection")
		return
	}

	clientLog.Writeln(fmt.Sprintf(">> Collecting artifacts in %s matching %s", workspace, artifacts))

	matches, err := common.GlobTree(workspace, artifacts)
	if err != nil {
		clientLog.Writeln(fmt.Sprintf("Invalid artifact pattern: " + err.Error()))
		return
	}

	clientLog.Writeln(fmt.Sprintf("Found %d matching artifacts", len(matches)))

	r.PushArtifacts(matches)
}

func init() {
	flag.StringVar(&selectedAdapter, "adapter", "basic", "Adapter to run build against")

	// TODO(dcramer): there must be a better way to allow us to "force" the initialization of basic?
	adapter.Register("basic", &basic.Adapter{})
}
