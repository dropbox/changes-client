package runner

import (
    "bufio"
    "flag"
    "fmt"
    "io"
    "os"
    "os/exec"
    "sync"
)

var (
    chunkSize = 4096
)

type Runner struct {
    Name string
    Bin  string
    Args []string

    ChunkChan chan LogChunk
}

type LogChunk struct {
    Source  string
    Offset  int
    Length  int
    Payload []byte
}

func NewRunner(name string, bin string, args ...string) *Runner {
    return &Runner{
        Name: name,
        Bin: bin,
        Args: args,
        ChunkChan: make(chan LogChunk),
    }
}

func (r *Runner) Run() (*os.ProcessState, error) {
    cmd := exec.Command(r.Bin, r.Args...)

    stdin, err := cmd.StdinPipe()
    if err != nil {
        return nil, err
    }

    stdout, err := cmd.StdoutPipe()
    if err != nil {
        return nil, err
    }

    stderr, err := cmd.StderrPipe()
    if err != nil {
        return nil, err
    }

    err = cmd.Start()
    if err != nil {
        return nil, err
    }

    // Start chunking from stdin and stdout and close stdin
    wg := sync.WaitGroup{}

    wg.Add(1)
    go func () {
        processChunks(r.ChunkChan, stdout, "stdout")
        wg.Done()
    }()

    wg.Add(1)
    go func () {
        processChunks(r.ChunkChan, stderr, "stderr")
        wg.Done()
    }()
    stdin.Close()

    wg.Wait()
    err = cmd.Wait()
    if err != nil {
        return nil, err
    }

    close(r.ChunkChan)
    return cmd.ProcessState, nil
}

func processChunks(out chan LogChunk, pipe io.Reader, source string) {
    r := bufio.NewReader(pipe)

    offset := 0
    finished := false
    for !finished {
        var payload []byte
        for len(payload) < chunkSize {
            line, err := r.ReadBytes('\n')
            payload = append(payload, line...)

            if err == nil {
                continue
            } else if err == io.EOF {
                finished = true
                break
            } else {
                finished = true
                line = []byte(fmt.Sprintf("%s: %s", source, err))
                payload = append(payload, line...)
                break
            }
        }

        if len(payload) > 0 {
            l := LogChunk{
                Source: source,
                Offset: offset,
                Length: len(payload),
                Payload: payload,
            }

            out <-l
            offset += len(payload)
        }
    }
}

func init() {
    flag.IntVar(&chunkSize, "log_chunk_size", 4096, "Size of log chunks to send to http server")
}
