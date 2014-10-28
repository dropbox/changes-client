package engine

import (
	"flag"
	"fmt"
	"github.com/dropbox/changes-client/client"
	"github.com/dropbox/changes-client/client/adapter"
	"github.com/dropbox/changes-client/reporter"
	"log"
	"os"
	"os/signal"
	"sync"

	_ "github.com/dropbox/changes-client/adapter/basic"
	_ "github.com/dropbox/changes-client/adapter/lxc"
)

const (
	STATUS_QUEUED      = "queued"
	STATUS_IN_PROGRESS = "in_progress"
	STATUS_FINISHED    = "finished"

	RESULT_PASSED  = "passed"
	RESULT_FAILED  = "failed"
	RESULT_ABORTED = "aborted"

	SNAPSHOT_ACTIVE = "active"
	SNAPSHOT_FAILED = "failed'"
)

var (
	selectedAdapter string
	outputSnapshot  string
)

type Engine struct {
	config    *client.Config
	clientLog *client.Log
	adapter   adapter.Adapter
}

func RunBuildPlan(config *client.Config) error {
	var err error

	currentAdapter, err := adapter.Get(selectedAdapter)
	if err != nil {
		// TODO(dcramer): handle this error
		return err
	}

	engine := &Engine{
		config:    config,
		clientLog: client.NewLog(),
		adapter:   currentAdapter,
	}

	return engine.Run()
}

func (e *Engine) Run() error {
	var err error

	r := reporter.NewReporter(e.config.Server, e.config.JobstepID, e.config.Debug)
	defer r.Shutdown()

	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		reportLogChunks("console", e.clientLog, r)
		wg.Done()
	}()

	r.PushJobstepStatus(STATUS_IN_PROGRESS, "")

	err = e.adapter.Init(e.config)
	if err != nil {
		log.Print(fmt.Sprintf("[adapter] %s", err.Error()))
		r.PushJobstepStatus(STATUS_FINISHED, RESULT_FAILED)
		e.clientLog.Close()
		return err
	}

	err = e.adapter.Prepare(e.clientLog)
	// its important that shutdown happens in the same context as reportLogChunks
	// that is, we need all data from reporter to be sent before calling Shutdown
	defer e.adapter.Shutdown(e.clientLog)
	if err != nil {
		log.Print(fmt.Sprintf("[adapter] %s", err.Error()))
		r.PushJobstepStatus(STATUS_FINISHED, RESULT_FAILED)
		e.clientLog.Close()
		return err
	}

	result := e.runBuildPlan(r)

	if result == RESULT_PASSED && outputSnapshot != "" {
		err = e.captureSnapshot()
		if err != nil {
			r.PushSnapshotImageStatus(outputSnapshot, SNAPSHOT_FAILED)
		} else {
			r.PushSnapshotImageStatus(outputSnapshot, SNAPSHOT_ACTIVE)
		}
	}

	r.PushJobstepStatus(STATUS_FINISHED, result)

	e.clientLog.Close()

	wg.Wait()

	return err
}

func (e *Engine) executeCommands(r *reporter.Reporter) string {
	var result string = RESULT_PASSED

	wg := sync.WaitGroup{}

	for _, cmdConfig := range e.config.Cmds {
		cmd, err := client.NewCommand(cmdConfig.ID, cmdConfig.Script)
		if err != nil {
			r.PushCommandStatus(cmd.ID, STATUS_FINISHED, 255)
			result = RESULT_FAILED
			break
		}
		r.PushCommandStatus(cmd.ID, STATUS_IN_PROGRESS, -1)

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
			r.PushCommandStatus(cmd.ID, STATUS_FINISHED, 255)
			result = RESULT_FAILED
		} else {
			if cmdResult.Success {
				if cmd.CaptureOutput {
					r.PushCommandOutput(cmd.ID, STATUS_FINISHED, 0, cmdResult.Output)
				} else {
					r.PushCommandStatus(cmd.ID, STATUS_FINISHED, 0)
				}
			} else {
				r.PushCommandStatus(cmd.ID, STATUS_FINISHED, 1)
				result = RESULT_FAILED
			}
		}

		wg.Add(1)
		go func(artifacts []string) {
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

func (e *Engine) runBuildPlan(r *reporter.Reporter) string {
	var result string

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

			go func() {
				log.Printf("Interrupted! Cancelling execution and cleaning up..")
				cancel <- struct{}{}
			}()
		}
	}()

	// We need to ensure that we're able to abort the build if upstream suggests
	// that it's been cancelled.
	if !e.config.Debug {
		go func() {
			um := &UpstreamMonitor{
				Config: e.config,
			}
			err := um.WaitUntilAbort()
			if err != nil {
				cancel <- struct{}{}
			}
		}()
	}

	// actually begin executing our the build plan
	rc := make(chan string)
	go func() {
		rc <- e.executeCommands(r)
	}()

	select {
	case result = <-rc:
	case <-cancel:
		e.clientLog.Writeln("==> ERROR: Build was aborted by upstream")
		result = RESULT_ABORTED
	}

	return result

}

func (e *Engine) publishArtifacts(r *reporter.Reporter, artifacts []string) {
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

	e.clientLog.Writeln(fmt.Sprintf("==> Found %d matching artifacts", len(matches)))

	r.PushArtifacts(matches)
}

func reportLogChunks(name string, clientLog *client.Log, r *reporter.Reporter) {
	for chunk := range clientLog.Chan {
		r.PushLogChunk(name, chunk)
	}
}

func init() {
	flag.StringVar(&selectedAdapter, "adapter", "basic", "Adapter to run build against")
	flag.StringVar(&outputSnapshot, "save-snapshot", "", "Save the resulting container snapshot")
}
