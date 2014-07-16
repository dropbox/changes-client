package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/dropbox/changes-client"
)

func main() {
	flag.Parse()

	config, err := runner.GetConfig()
	if err != nil {
		panic(err)
	}

	reporter := runner.NewReporter(config.ApiUri)
	runner.runCmds(reporter, config)

	reporter.Shutdown()
}
