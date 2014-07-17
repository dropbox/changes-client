package main

import (
	"flag"

	"github.com/dropbox/changes-client"
)

func main() {
	flag.Parse()

	config, err := runner.GetConfig()
	if err != nil {
		panic(err)
	}

	reporter := runner.NewReporter(config.Server)
	runner.RunCmds(reporter, config)
	reporter.Shutdown()
}
