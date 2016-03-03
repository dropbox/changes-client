package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/dropbox/changes-client/client"
	"github.com/dropbox/changes-client/client/adapter"
	"github.com/dropbox/changes-client/client/filelog"
	"github.com/dropbox/changes-client/client/reporter"
	"github.com/dropbox/changes-client/common/sentry"
	"github.com/dropbox/changes-client/common/version"
	"github.com/dropbox/changes-client/engine"
	"github.com/getsentry/raven-go"
)

func main() {
	log.SetFlags(log.Lmicroseconds | log.Ldate)
	var (
		showVersion = flag.Bool("version", false, "Prints changes-client version")
		exitResult  = flag.Bool("exit-result", false, "Determine exit code from result--exit 1 on any execution failure or 99 on any infrastructure failure")
		showInfo    = flag.Bool("showinfo", false, "Prints basic information about this binary in a stable json format and exits.")
		jobstepID   = flag.String("jobstep_id", "", "Jobstep ID whose commands are to be executed")
	)
	flag.Parse()

	if *showVersion {
		fmt.Println(version.GetVersion())
		return
	}
	if *showInfo {
		// This is intended to be a reliable way to externally verify
		// the available functionality in this binary; change the format
		// only with great care.
		if d, e := json.MarshalIndent(map[string]interface{}{
			"adapters":  adapter.Names(),
			"reporters": reporter.Names(),
			"version":   version.GetVersion(),
		}, "", "   "); e != nil {
			panic(e)
		} else {
			fmt.Println(string(d))
			return
		}
	}

	result := run(*jobstepID)
	exitCode := 0
	if *exitResult {
		switch result {
		case engine.RESULT_PASSED:
			exitCode = 0
		case engine.RESULT_INFRA_FAILED:
			// We use exit code 99 to signal to the generic-build script that
			// there was an infrastructure failure. Eventually, changes-client
			// will probably report infra failures to Changes directly.
			exitCode = 99
		default:
			exitCode = 1
		}
	}
	log.Println("[client] exit:", exitCode)
	os.Exit(exitCode)
}

// Returns whether run was successful.
func run(jobstepID string) (result engine.Result) {
	infraLog, err := filelog.New(jobstepID, "infralog")
	if err != nil {
		log.Printf("[client] error creating infralog: %s", err)
		sentry.Error(err, map[string]string{})
	} else {
		log.SetOutput(io.MultiWriter(os.Stderr, infraLog))
	}

	var sentryClient *raven.Client
	if sentryClient = sentry.GetClient(); sentryClient != nil {
		log.Printf("Using Sentry; ProjectID=%s, URL=%s", sentryClient.ProjectID(), sentryClient.URL())
		// Don't return until we're finished sending to Sentry.
		defer sentryClient.Wait()
		// Ensure main thread panics are caught and reported.
		defer func() {
			if p := recover(); p != nil {
				var err error
				switch rval := p.(type) {
				case error:
					err = rval
				default:
					err = errors.New(fmt.Sprint(rval))
				}
				packet := raven.NewPacket(err.Error(), raven.NewException(err, raven.NewStacktrace(2, 3, nil)))
				log.Printf("[client] Sending panic to Sentry")
				_, ch := sentryClient.Capture(packet, map[string]string{})
				if serr := <-ch; serr != nil {
					log.Printf("SENTRY ERROR: %s", serr)
				}
				// We consider panics an infra failure
				result = engine.RESULT_INFRA_FAILED
			}
		}()
	} else {
		log.Println("Sentry NOT ENABLED.")
	}

	// Error handling in place; now we begin.

	config, err := client.GetConfig(jobstepID)
	if err != nil {
		log.Printf("[client] error getting config: %s", err)
		sentry.Error(err, map[string]string{})
		return engine.RESULT_INFRA_FAILED
	}
	if sentryClient != nil {
		sentryClient.SetTagsContext(map[string]string{
			"projectslug": config.Project.Slug,
			"jobstep_id":  config.JobstepID,
		})
	}

	result, err = engine.RunBuildPlan(config, infraLog)
	if err != nil {
		sentry.Error(err, map[string]string{})
		result = engine.RESULT_INFRA_FAILED
	}
	return result
}
