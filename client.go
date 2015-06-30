package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/dropbox/changes-client/client"
	"github.com/dropbox/changes-client/common/sentry"
	"github.com/dropbox/changes-client/common/version"
	"github.com/dropbox/changes-client/engine"
	"github.com/getsentry/raven-go"
)

var (
	exitResult = false
)

func main() {
	showVersion := flag.Bool("version", false, "Prints changes-client version")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.Version)
		return
	}

	if sentryClient := sentry.GetClient(); sentryClient != nil {
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

		run()
	} else {
		run()
	}
}

func run() {
	config, err := client.GetConfig()
	if err != nil {
		panic(err)
	}

	result, err := engine.RunBuildPlan(config)
	log.Printf("[client] Finished: %s", result)
	if err != nil {
		log.Printf("[client] error: %s", err.Error())
	}
	if (err != nil || result != engine.RESULT_PASSED) && exitResult {
		log.Printf("[client] exit: 1")
		os.Exit(1)
	}
	log.Printf("[client] exit: 0")
}

func init() {
	flag.BoolVar(&exitResult, "exit-result", false, "Determine exit code from result")
}
