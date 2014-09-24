package main

import (
	"flag"
	"fmt"

	"github.com/dropbox/changes-client"
)

const (
	Version = "0.0.8"
)

func main() {
	showVersion := flag.Bool("version", false, "Prints changes-client version")
	flag.Parse()

	if *showVersion {
		fmt.Println(Version)
		return
	}

	config, err := runner.GetConfig()
	if err != nil {
		panic(err)
	}

	reporter := runner.NewReporter(config.Server, config.Debug)
	runner.RunBuildPlan(reporter, config)
	reporter.Shutdown()
}
