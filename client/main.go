package main

import (
    "flag"
    "fmt"
    "sync"

    "github.com/dropbox/changes-client"
)

func reportChunks(r *runner.Reporter, cId string, c chan runner.LogChunk) {
    for l := range c {
        fmt.Printf("Got another chunk from %s (%d-%d)\n", l.Source, l.Offset, l.Length)
        fmt.Printf("%s", l.Payload)
        r.PushLogChunk(cId, l)
    }
}

func runCmds(reporter *runner.Reporter, config *runner.Config) {
    wg := sync.WaitGroup{}
    for _, cmd := range config.Cmds {
        fmt.Println("Running ", cmd.Name)
        reporter.PushStatus(cmd.Name, "STARTED")
        r := runner.NewRunner(cmd.Name, cmd.Bin, cmd.Args...)

        wg.Add(1)
        go func() {
            reportChunks(reporter, cmd.Name, r.ChunkChan)
            wg.Done()
        }()
        pState, err := r.Run()
        if err != nil {
            reporter.PushStatus(cmd.Name, "FAILED")
        } else {
            reporter.PushStatus(cmd.Name, pState.String())
        }
    }

    wg.Wait()
}

func main() {
    flag.Parse()

    config, err := runner.GetConfig()
    if err != nil {
        panic(err)
    }

    // Make a reporter and use it
    reporter := runner.NewReporter(config.ApiUri)
    runCmds(reporter, config)
    reporter.Shutdown()
}
