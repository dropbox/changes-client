package main

import (
    "encoding/json"
    "flag"
    "fmt"
    "io/ioutil"

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
    flag.Parse()

    config, err := parseConfig(filename)
    if err != nil {
        panic(err)
    }

    // Make a reporter and use it
    _ = runner.NewReporter(config.ApiUri)

    for _, cmd := range config.Cmds {
        fmt.Println("Running ", cmd.Name)

        r := runner.NewRunner(cmd.Name, cmd.Bin, cmd.Args...)
        go printChunks(r.ChunkChan)
        r.Run()
    }
}

func printChunks(c chan runner.LogChunk) {
    for l := range c {
        fmt.Printf("Got another chunk from %s (offset: %d, length %d)\n", l.Source, l.Offset, l.Length)
        fmt.Printf("%s", l.Payload)
    }
}
