package engine

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"

	"github.com/dropbox/changes-client/client"
	"github.com/dropbox/changes-client/client/adapter"
	"github.com/dropbox/changes-client/client/reporter"
	"github.com/dropbox/changes-client/common/version"

	_ "github.com/dropbox/changes-client/adapter/basic"
	_ "github.com/dropbox/changes-client/adapter/lxc"
	_ "github.com/dropbox/changes-client/reporter/artifactstore"
	_ "github.com/dropbox/changes-client/reporter/jenkins"
	_ "github.com/dropbox/changes-client/reporter/mesos"
	_ "github.com/dropbox/changes-client/reporter/multireporter"
)

const (
	STATUS_QUEUED      = "queued"
	STATUS_IN_PROGRESS = "in_progress"
	STATUS_FINISHED    = "finished"

	RESULT_PASSED  Result = "passed"
	RESULT_FAILED  Result = "failed"
	RESULT_ABORTED Result = "aborted"
	// Test results unreliable or unavailable due to infrastructure
	// issues.
	RESULT_INFRA_FAILED Result = "infra_failed"

	SNAPSHOT_ACTIVE = "active"
	SNAPSHOT_FAILED = "failed"
)

type Result string

func (r Result) String() string {
	return string(r)
}

// Convenience method to check for all types of failure.
func (r Result) IsFailure() bool {
	switch r {
	case RESULT_FAILED, RESULT_INFRA_FAILED:
		return true
	}
	return false
}

var (
	selectedAdapter  string
	selectedReporter string
	outputSnapshot   string
)

type Engine struct {
	config    *client.Config
	clientLog *client.Log
	adapter   adapter.Adapter
	reporter  reporter.Reporter
}

func RunBuildPlan(config *client.Config) (Result, error) {
	var err error

	currentAdapter, err := adapter.Create(selectedAdapter)
	if err != nil {
		return RESULT_INFRA_FAILED, err
	}

	currentReporter, err := reporter.Create(selectedReporter)
	if err != nil {
		log.Printf("[engine] failed to initialize reporter: %s", selectedReporter)
		return RESULT_INFRA_FAILED, err
	}
	log.Printf("[engine] started with reporter %s, adapter %s", selectedReporter, selectedAdapter)

	engine := &Engine{
		config:    config,
		clientLog: client.NewLog(),
		adapter:   currentAdapter,
		reporter:  currentReporter,
	}

	return engine.Run()
}

func (e *Engine) Run() (Result, error) {
	e.reporter.Init(e.config)
	defer e.reporter.Shutdown()

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		reportLogChunks("console", e.clientLog, e.reporter)
		wg.Done()
	}()

	e.clientLog.Writeln("changes-client version: " + version.GetVersion())
	e.clientLog.Printf("Running jobstep %s for %s (%s)", e.config.JobstepID, e.config.Project.Name, e.config.Project.Slug)

	e.reporter.PushJobstepStatus(STATUS_IN_PROGRESS, "")

	result, err := e.runBuildPlan()

	e.clientLog.Printf("==> Build finished! Recorded result as %s", result)

	e.reporter.PushJobstepStatus(STATUS_FINISHED, result.String())

	e.clientLog.Close()

	wg.Wait()

	return result, err
}

func (e *Engine) executeCommands() (Result, error) {
	wg := sync.WaitGroup{}
	defer wg.Wait()

	for _, cmdConfig := range e.config.Cmds {
		e.clientLog.Printf("==> Running command %s", cmdConfig.ID)
		e.clientLog.Printf("==>     with script %s", cmdConfig.Script)
		cmd, err := client.NewCommand(cmdConfig.ID, cmdConfig.Script)
		if err != nil {
			e.reporter.PushCommandStatus(cmd.ID, STATUS_FINISHED, 255)
			e.clientLog.Printf("==> Error: %s", err)
			return RESULT_INFRA_FAILED, err
		}
		e.reporter.PushCommandStatus(cmd.ID, STATUS_IN_PROGRESS, -1)

		cmd.CaptureOutput = cmdConfig.CaptureOutput

		env := os.Environ()
		for k, v := range cmdConfig.Env {
			env = append(env, k+"="+v)
		}
		cmd.Env = env

		if len(cmdConfig.Cwd) > 0 {
			cmd.Cwd = cmdConfig.Cwd
		}

		cmdResult, err := e.adapter.Run(cmd, e.clientLog)

		if err != nil {
			e.reporter.PushCommandStatus(cmd.ID, STATUS_FINISHED, 255)
			e.clientLog.Printf("==> Error: %s", err)
			return RESULT_INFRA_FAILED, err
		}
		result := RESULT_FAILED
		if cmdResult.Success {
			result = RESULT_PASSED
			if cmd.CaptureOutput {
				e.reporter.PushCommandOutput(cmd.ID, STATUS_FINISHED, 0, cmdResult.Output)
			} else {
				e.reporter.PushCommandStatus(cmd.ID, STATUS_FINISHED, 0)
			}
		} else {
			e.reporter.PushCommandStatus(cmd.ID, STATUS_FINISHED, 1)
		}

		wg.Add(1)
		go func(cfgcmd client.ConfigCmd) {
			// publishArtifacts is a synchronous operation and doesnt follow the normal queue flow of
			// other operations
			e.reporter.PublishArtifacts(cfgcmd, e.adapter, e.clientLog)
			wg.Done()
		}(cmdConfig)

		if result.IsFailure() {
			return result, nil
		}
	}

	// Made it through all commands without failure. Success.
	return RESULT_PASSED, nil
}

func (e *Engine) captureSnapshot() error {
	log.Printf("[adapter] Capturing snapshot %s", outputSnapshot)
	err := e.adapter.CaptureSnapshot(outputSnapshot, e.clientLog)
	if err != nil {
		log.Printf("[adapter] Failed to capture snapshot: %s", err.Error())
		return err
	}
	return nil
}

func (e *Engine) runBuildPlan() (Result, error) {
	// cancellation signal
	cancel := make(chan struct{})

	// capture ctrl+c and enforce a clean shutdown
	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, os.Interrupt)
	go func() {
		shuttingDown := false
		for _ = range sigchan {
			if shuttingDown {
				log.Printf("Second interrupt received. Terminating!")
				os.Exit(1)
			}

			shuttingDown = true

			log.Printf("Interrupted! Cancelling execution and cleaning up..")
			cancel <- struct{}{}
		}
	}()

	// We need to ensure that we're able to abort the build if upstream suggests
	// that it's been cancelled.
	if !e.config.Debug {
		go func() {
			um := &UpstreamMonitor{
				Config: e.config,
			}
			um.WaitUntilAbort()
			cancel <- struct{}{}
		}()
	}

	err := e.adapter.Init(e.config)
	if err != nil {
		log.Print(fmt.Sprintf("[adapter] %s", err))
		e.clientLog.Printf("==> ERROR: Failed to initialize %s adapter", selectedAdapter)
		return RESULT_INFRA_FAILED, err
	}

	err = e.adapter.Prepare(e.clientLog)
	if err != nil {
		log.Print(fmt.Sprintf("[adapter] %s", err))
		e.clientLog.Printf("==> ERROR: %s adapter failed to prepare: %s", selectedAdapter, err)
		return RESULT_INFRA_FAILED, err
	}
	defer e.adapter.Shutdown(e.clientLog)

	type cmdResult struct {
		result Result
		err    error
	}
	// actually begin executing the build plan
	finished := make(chan cmdResult)
	go func() {
		r, cmderr := e.executeCommands()
		finished <- cmdResult{r, cmderr}
	}()

	var result Result
	select {
	case cmdresult := <-finished:
		if cmdresult.err != nil {
			return cmdresult.result, cmdresult.err
		}
		result = cmdresult.result
	case <-cancel:
		e.clientLog.Writeln("==> ERROR: Build was aborted by upstream")
		return RESULT_ABORTED, nil
	}

	if result == RESULT_PASSED && outputSnapshot != "" {
		var snapshotStatus string
		sserr := e.captureSnapshot()
		if sserr != nil {
			snapshotStatus = SNAPSHOT_FAILED
		} else {
			snapshotStatus = SNAPSHOT_ACTIVE
		}
		e.reporter.PushSnapshotImageStatus(outputSnapshot, snapshotStatus)
		if sserr != nil {
			return RESULT_INFRA_FAILED, sserr
		}
	}
	return result, nil
}

func reportLogChunks(name string, clientLog *client.Log, r reporter.Reporter) {
	for ch, ok := clientLog.GetChunk(); ok; ch, ok = clientLog.GetChunk() {
		r.PushLogChunk(name, ch)
	}
}

func init() {
	flag.StringVar(&selectedAdapter, "adapter", "basic", "Adapter to run build against")
	flag.StringVar(&selectedReporter, "reporter", "multireporter", "Reporter to send results to")
	flag.StringVar(&outputSnapshot, "save-snapshot", "", "Save the resulting container snapshot")
}
