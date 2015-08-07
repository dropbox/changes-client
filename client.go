package main

import (
	"errors"
	"flag"
	"fmt"
	"log"

	"github.com/dropbox/changes-client/client"
	"github.com/dropbox/changes-client/common/sentry"
	"github.com/dropbox/changes-client/common/version"
	"github.com/dropbox/changes-client/engine"
	"github.com/getsentry/raven-go"
)

func main() {
	showVersion := flag.Bool("version", false, "Prints changes-client version")
	exitResult := flag.Bool("exit-result", false, "Determine exit code from result")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.GetVersion())
		return
	}

	success := run()
	if !success && *exitResult {
		log.Fatal("[client] exit: 1")
	}
	log.Println("[client] exit: 0")
}

// Returns whether run was successful.
func run() bool {
	if sentryClient := sentry.GetClient(); sentryClient != nil {
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
				<-ch
				panic(p)
			}
		}()
	}

	// Error handling in place; now we begin.

	config, err := client.GetConfig()
	if err != nil {
		panic(err)
	}

	result, err := engine.RunBuildPlan(config)
	log.Printf("[client] Finished: %s", result)
	if err != nil {
		log.Printf("[client] error: %s", err)
	}
	return err == nil && result == engine.RESULT_PASSED
}
