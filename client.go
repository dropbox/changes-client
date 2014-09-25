package main

import (
	"flag"
	"fmt"

	"github.com/dropbox/changes-client/client"
	"github.com/dropbox/changes-client/engine"
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

	config, err := client.GetConfig()
	if err != nil {
		panic(err)
	}

	reporter := client.NewReporter(config.Server, config.Debug)
	engine.RunBuildPlan(reporter, config)
	reporter.Shutdown()
}
