package main

import (
    "encoding/json"
    "flag"
    "fmt"
    "io/ioutil"
    "sync"

    "github.com/dropbox/changes-client"
)

type Config struct {
    ApiUri string       `json:"api-uri"`
    Cmds []struct {
        Name string         `json:"name"`
        Bin  string         `json:"bin"`
        Args []string       `json:"args"`
    }                       `json:"cmds"`
}

func parseConfig(filename string) (*Config, error) {
    conf, err := ioutil.ReadFile(filename)
    if err != nil {
        return nil, err
    }

    r := &Config{}
    err = json.Unmarshal(conf, r)
    if err != nil {
        return nil, err
    }

    return r, nil
}

func main() {
    var filename string
    flag.StringVar(&filename, "conf", "", "Config file containing cmds to execute")
    flag.Int("log_chunk_size", 4096, "Size of log chunks to send to http server")
    flag.Int("max_pending_reports", 64, "Maximum number of pending reports which are not (yet) published by the client")
    flag.Parse()

    config, err := parseConfig(filename)
    if err != nil {
        panic(err)
    }

    // Make a reporter and use it
    reporter := runner.NewReporter(config.ApiUri)

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
    reporter.Shutdown()
}

func reportChunks(r *runner.Reporter, cId string, c chan runner.LogChunk) {
    for l := range c {
        fmt.Printf("Got another chunk from %s (offset: %d, length %d)\n", l.Source, l.Offset, l.Length)
        fmt.Printf("%s", l.Payload)
        r.PushLogChunk(cId, l)
    }
}
