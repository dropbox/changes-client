package engine

import (
	"flag"
	"fmt"
	"github.com/dropbox/changes-client/shared/adapter"
	"github.com/dropbox/changes-client/shared/reporter"
	"github.com/dropbox/changes-client/shared/runner"
	"log"
	"os"
	"os/signal"
	"sync"

	_ "github.com/dropbox/changes-client/adapters/basic"
	_ "github.com/dropbox/changes-client/adapters/lxc"
)

const (
	STATUS_QUEUED      = "queued"
	STATUS_IN_PROGRESS = "in_progress"
	STATUS_FINISHED    = "finished"

	RESULT_PASSED  = "passed"
	RESULT_FAILED  = "failed"
	RESULT_ABORTED = "aborted"

	SNAPSHOT_ACTIVE = "active"
	SNAPSHOT_FAILED = "failed"
)

var (
	selectedAdapter string
	outputSnapshot  string
)

type Engine struct {
	config    *runner.Config
	clientLog *runner.Log
	adapter   adapter.Adapter
}

func NewEngine(config *runner.Config) (*Engine, error) {
	currentAdapter, err := adapter.Get(selectedAdapter)
	if err != nil {
		return nil, err
	}

	engine := &Engine{
		config:    config,
		clientLog: runner.NewLog(),
		adapter:   currentAdapter,
	}

	return engine, nil
}

func (e *Engine) Run(r reporter.Reporter, outputSnapshot string) error {
	var err error

	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		reportLogChunks("console", e.clientLog, r)
		wg.Done()
	}()

	r.PushBuildStatus(STATUS_IN_PROGRESS, "")

	result := e.runBuildPlan(r, outputSnapshot)

	e.clientLog.Writeln(fmt.Sprintf("==> Build finished! Recorded result as %s", result))

	r.PushBuildStatus(STATUS_FINISHED, result)

	e.clientLog.Close()

	wg.Wait()

	return err
}

func (e *Engine) executeCommands(r reporter.Reporter) string {
	var result string = RESULT_PASSED

	wg := sync.WaitGroup{}

	for _, cmdConfig := range e.config.Cmds {
		cmd, err := runner.NewCommand(cmdConfig.ID, cmdConfig.Script)
		if err != nil {
			r.PushCommandStatus(cmdConfig.ID, STATUS_FINISHED, 255)
			result = RESULT_FAILED
			break
		}
		r.PushCommandStatus(cmdConfig.ID, STATUS_IN_PROGRESS, -1)

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
			r.PushCommandStatus(cmdConfig.ID, STATUS_FINISHED, 255)
			result = RESULT_FAILED
		} else {
			if cmdResult.Success {
				if cmdConfig.CaptureOutput {
					r.PushCommandOutput(cmdConfig.ID, STATUS_FINISHED, 0, cmdResult.Output)
				} else {
					r.PushCommandStatus(cmdConfig.ID, STATUS_FINISHED, 0)
				}
			} else {
				r.PushCommandStatus(cmdConfig.ID, STATUS_FINISHED, 1)
				result = RESULT_FAILED
			}
		}

		wg.Add(1)
		go func(artifacts []string) {
			// publishArtifacts is a synchronous operation and doesnt follow the normal queue flow of
			// other operations
			e.publishArtifacts(r, artifacts)
			wg.Done()
		}(cmdConfig.Artifacts)

		if result == RESULT_FAILED {
			break
		}
	}

	wg.Wait()

	return result
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

func (e *Engine) runBuildPlan(r reporter.Reporter, outputSnapshot string) string {
	var (
		result string
		err    error
	)

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

	err = e.adapter.Init(e.config)
	if err != nil {
		log.Print(fmt.Sprintf("[adapter] %s", err.Error()))
		e.clientLog.Writeln(fmt.Sprintf("==> ERROR: Failed to initialize %s adapter", selectedAdapter))
		return RESULT_FAILED
	}

	err = e.adapter.Prepare(e.clientLog)
	defer e.adapter.Shutdown(e.clientLog)
	if err != nil {
		log.Print(fmt.Sprintf("[adapter] %s", err.Error()))
		e.clientLog.Writeln(fmt.Sprintf("==> ERROR: %s adapter failed to prepare: %s", selectedAdapter, err))
		return RESULT_FAILED
	}

	// actually begin executing the build plan
	finished := make(chan struct{})
	go func() {
		result = e.executeCommands(r)
		finished <- struct{}{}
	}()

	select {
	case <-finished:
	case <-cancel:
		e.clientLog.Writeln("==> ERROR: Build was aborted by upstream")
		result = RESULT_ABORTED
	}

	if result == RESULT_PASSED && outputSnapshot != "" {
		err = e.captureSnapshot()
		if err != nil {
			result = RESULT_FAILED
		}
	}

	return result

}

func (e *Engine) publishArtifacts(r reporter.Reporter, artifacts []string) {
	if len(artifacts) == 0 {
		e.clientLog.Writeln("==> Skipping artifact collection")
		return
	}

	e.clientLog.Writeln(fmt.Sprintf("==> Collecting artifacts matching %s", artifacts))

	matches, err := e.adapter.CollectArtifacts(artifacts, e.clientLog)
	if err != nil {
		e.clientLog.Writeln(fmt.Sprintf("==> ERROR: " + err.Error()))
		return
	}

	for _, artifact := range matches {
		e.clientLog.Writeln(fmt.Sprintf("==> Found: %s", artifact))
	}

	r.PushArtifacts(matches)
}

func reportLogChunks(name string, clientLog *runner.Log, r reporter.Reporter) {
	for chunk := range clientLog.Chan {
		r.PushLogChunk(name, chunk)
	}
}

func init() {
	flag.StringVar(&selectedAdapter, "adapter", "basic", "Adapter to run build against")
}
