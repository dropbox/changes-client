package main

import (
	"flag"

	"github.com/dropbox/changes-client/shared/engine"
	"github.com/dropbox/changes-client/shared/reporter"
	"github.com/dropbox/changes-client/shared/runner"
)

var (
	snapshotID = ""
)

func main() {
	var (
		snapshotID string = ""
	)

	flag.Parse()

	run(snapshotID)
}

func run(snapshotID string) {
	var err error

	conf, err := runner.GetSnapshotConfig(snapshotID)
	if err != nil {
		panic(err)
	}

	r := reporter.NewSnapshotReporter(conf.Server, snapshotID, conf.Debug)
	defer r.Shutdown()

	engine, err := engine.NewEngine(conf)
	if err != nil {
		panic(err)
	}
	engine.Run(r, snapshotID)
}

func init() {
	flag.StringVar(&snapshotID, "snapshot-id", "", "Snapshot ID whose commands are to be executed")
}
