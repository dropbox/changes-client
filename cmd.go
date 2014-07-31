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
	"strings"
	"sync"
)

var (
	chunkSize = 4096
)

type WrappedCommand struct {
	Name      string
	Cmd       *exec.Cmd
	LogSource *LogSource
	ChunkChan chan LogChunk
}

// A wrapped command will ensure that all stdin/out/err gets piped
// into a buffer that can then be reported upstream to the Changes
// master server
func NewWrappedCommand(cmd *exec.Cmd) (*WrappedCommand, error) {
	return &WrappedCommand{
		Cmd:       cmd,
		ChunkChan: make(chan LogChunk),
	}, nil
}

// Build a new WrappedCommand out of an arbitrary script
// The script is written to disk and then executed ensuring that it can
// be fairly arbitrary and provide its own shebang
func NewWrappedScriptCommand(script string, name string) (*WrappedCommand, error) {
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

	wc, err := NewWrappedCommand(exec.Command(f.Name()))
	wc.Name = name
	return wc, err
}

func (c *WrappedCommand) Run() (*os.ProcessState, error) {
	defer close(c.ChunkChan)

	stdin, err := c.Cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	stdout, err := c.Cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	stderr, err := c.Cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	var cmdname string
	if c.Name != "" {
		cmdname = c.Name
	} else {
		cmdname = strings.Join(c.Cmd.Args, " ")
	}

	log.Printf("[cmd] Executing %s", cmdname)
	processMessage(c.ChunkChan, fmt.Sprintf(">> %s", cmdname))

	err = c.Cmd.Start()
	if err != nil {
		log.Printf("[cmd] Start failed %s %s", c.Cmd.Args, err.Error())
		processMessage(c.ChunkChan, err.Error())
		return nil, err
	}

	// Start chunking from stdin and stdout and close stdin
	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		processChunks(c.ChunkChan, stdout)
		log.Printf("[cmd] Stdout processed %s", c.Cmd.Args)
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		processChunks(c.ChunkChan, stderr)
		log.Printf("[cmd] Stderr processed %s", c.Cmd.Args)
		wg.Done()
	}()
	stdin.Close()

	wg.Wait()
	err = c.Cmd.Wait()
	if err != nil {
		return nil, err
	}

	return c.Cmd.ProcessState, nil
}

func processMessage(out chan LogChunk, payload string) {
	out <- LogChunk{
		Length:  len(payload),
		Payload: []byte(fmt.Sprintf("%s\n", payload)),
	}
}

func processChunks(out chan LogChunk, pipe io.Reader) {
	r := bufio.NewReader(pipe)

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
				line = []byte(fmt.Sprintf("%s", err))
				payload = append(payload, line...)
				break
			}
		}

		if len(payload) > 0 {
			l := LogChunk{
				Length:  len(payload),
				Payload: payload,
			}

			out <- l
		}
	}
}

func init() {
	flag.IntVar(&chunkSize, "log_chunk_size", 4096, "Size of log chunks to send to http server")
}
