package runner

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

var (
	chunkSize = 4096
)

type WrappedCommand struct {
	Name      string
	Cmd       *exec.Cmd
	LogSource *LogSource
	ChunkChan chan LogChunk
	Output    []byte // buffered output if requested
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

func (wc *WrappedCommand) CombinedOutputPipe() (io.ReadCloser, io.WriteCloser) {
	c := wc.Cmd

	pr, pw := io.Pipe()

	c.Stdout = pw
	c.Stderr = pw

	return pr, pw
}

func (wc *WrappedCommand) GetLabel() string {
	if wc.Name != "" {
		return wc.Name
	} else {
		return strings.Join(wc.Cmd.Args, " ")
	}
}

func (wc *WrappedCommand) Run(bufferOutput bool) (*os.ProcessState, error) {
	defer close(wc.ChunkChan)

	cmdreader, cmdwriter := wc.CombinedOutputPipe()

	cmdname := wc.GetLabel()
	log.Printf("[cmd] Executing %s", cmdname)
	processMessage(wc.ChunkChan, fmt.Sprintf(">> %s", cmdname))

	var buffer *bytes.Buffer
	var reader io.Reader = cmdreader

	// If user has requested to buffer command output, tee output to in memory buffer.
	if bufferOutput {
		buffer = &bytes.Buffer{}
		reader = io.TeeReader(cmdreader, buffer)
	}

	err := wc.Cmd.Start()

	if err != nil {
		log.Printf("[cmd] Start failed %s %s", wc.Cmd.Args, err.Error())
		processMessage(wc.ChunkChan, err.Error())
		return nil, err
	}

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		processChunks(wc.ChunkChan, reader)
		log.Printf("[cmd] Stdout processed %s", wc.Cmd.Args)
		wg.Done()
	}()

	err = wc.Cmd.Wait()
	cmdwriter.Close()

	wg.Wait()

	if err != nil {
		return nil, err
	}

	if bufferOutput {
		wc.Output = buffer.Bytes()
	}

	return wc.Cmd.ProcessState, nil
}

func processMessage(out chan LogChunk, payload string) {
	out <- LogChunk{
		Length:  len(payload),
		Payload: []byte(fmt.Sprintf("%s\n", payload)),
	}
}

type LogLine struct {
	line []byte
	err  error
}

func newLogLineReader(pipe io.Reader) <-chan *LogLine {
	r := bufio.NewReader(pipe)
	ch := make(chan *LogLine)

	go func() {
		for {
			line, err := r.ReadBytes('\n')
			l := &LogLine{line: line, err: err}
			ch <- l

			if err != nil {
				return
			}
		}
	}()

	return ch
}

func processChunks(out chan LogChunk, pipe io.Reader) {
	lines := newLogLineReader(pipe)

	finished := false
	for !finished {
		var payload []byte
		timeLimit := time.After(2 * time.Second)

		for len(payload) < chunkSize {
			var logLine *LogLine
			timeLimitExceeded := false

			select {
			case logLine = <-lines:
			case <-timeLimit:
				timeLimitExceeded = true
			}

			if timeLimitExceeded {
				break
			}

			payload = append(payload, logLine.line...)
			if logLine.err == io.EOF {
				finished = true
				break
			}

			if logLine.err != nil {
				finished = true
				line := []byte(fmt.Sprintf("%s", logLine.err))
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
