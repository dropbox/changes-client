package runner

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"sync"
)

var (
	chunkSize = 4096
)

type Runner struct {
	Id        string
	Cmd       *exec.Cmd
	ChunkChan chan LogChunk
}

func NewRunner(id string, script string) (*Runner, error) {
	f, err := ioutil.TempFile("", "script-")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	_, err = f.WriteString(script)
	if err != nil {
		return nil, err
	}

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}

	err = f.Chmod((info.Mode() & os.ModePerm) | 0111)
	if err != nil {
		return nil, err
	}

	return &Runner{
		Id:        id,
		Cmd:       exec.Command(f.Name()),
		ChunkChan: make(chan LogChunk),
	}, nil
}

func (r *Runner) Run() (*os.ProcessState, error) {
	defer close(r.ChunkChan)

	stdin, err := r.Cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	stdout, err := r.Cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	stderr, err := r.Cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	log.Printf("[runner] Execute command %s", r.Id)
	err = r.Cmd.Start()
	if err != nil {
		log.Printf("[runner] Command start failed %s %s", r.Id, err.Error())
		return nil, err
	}

	// Start chunking from stdin and stdout and close stdin
	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		processChunks(r.ChunkChan, stdout, "console")
		log.Printf("[runner] Command stdout processed %s", r.Id)
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		processChunks(r.ChunkChan, stderr, "console")
		log.Printf("[runner] Command stderr processed %s", r.Id)
		wg.Done()
	}()
	stdin.Close()

	wg.Wait()
	err = r.Cmd.Wait()
	if err != nil {
		return nil, err
	}

	return r.Cmd.ProcessState, nil
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
				Source:  source,
				Offset:  offset,
				Length:  len(payload),
				Payload: payload,
			}

			out <- l
			offset += len(payload)
		}
	}
}

func init() {
	flag.IntVar(&chunkSize, "log_chunk_size", 4096, "Size of log chunks to send to http server")
}
