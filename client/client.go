package main

import (
	"errors"
	"flag"
	"fmt"
	"log"

	"github.com/dropbox/changes-client/shared/engine"
	"github.com/dropbox/changes-client/shared/reporter"
	"github.com/dropbox/changes-client/shared/runner"
	"github.com/getsentry/raven-go"
)

const (
	Version = "0.0.8"
)

var (
	jobstepID = ""
	sentryDsn = ""
)

func runWithErrorHandler(f func()) {
	if sentryDsn != "" {
		sentryClient, err := raven.NewClient(sentryDsn, map[string]string{
			"version": Version,
		})
		if err != nil {
			log.Fatal(err)
		}

		defer func() {
			var packet *raven.Packet
			p := recover()
			switch rval := p.(type) {
			case nil:
				return
			case error:
				packet = raven.NewPacket(rval.Error(), raven.NewException(rval, raven.NewStacktrace(2, 3, nil)))
			default:
				rvalStr := fmt.Sprint(rval)
				packet = raven.NewPacket(rvalStr, raven.NewException(errors.New(rvalStr), raven.NewStacktrace(2, 3, nil)))
			}

			log.Printf("[client] Sending panic to Sentry")
			_, ch := sentryClient.Capture(packet, map[string]string{})
			<-ch
			panic(p)
		}()
	}
	f()
}

func main() {
	showVersion := flag.Bool("version", false, "Prints changes-client version")

	flag.Parse()

	if jobstepID == "" {
		panic(fmt.Errorf("Missing required configuration: jobstep_id"))
	}

	if *showVersion {
		fmt.Println(Version)
		return
	}
	runWithErrorHandler(func() { run(jobstepID) })
}

func run(jobstepID string) {
	var err error

	config, err := runner.GetJobStepConfig(jobstepID)
	if err != nil {
		panic(err)
	}

	r := reporter.NewJobStepReporter(config.Server, jobstepID, config.Debug)
	defer r.Shutdown()

	engine, err := engine.NewEngine(config)
	if err != nil {
		panic(err)
	}
	engine.Run(r, "")
}

func init() {
	flag.StringVar(&jobstepID, "jobstep_id", "", "Job ID whose commands are to be executed")
	flag.StringVar(&sentryDsn, "sentry-dsn", "", "Sentry DSN for reporting errors")
}
