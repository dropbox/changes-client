package client

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"time"
)

// After this many bytes are buffered, the buffered log data will be flushed.
var byteFlushThreshold = flag.Int("log_chunk_size", 10240, "Size of log chunks to send to http server")

// After this much time has elapsed, buffered log data will be flushed.
const timeFlushThreshold = 4 * time.Second

type Log struct {
	chunkchan chan []byte
	closed    chan struct{}
}

type LogLine struct {
	line []byte
	err  error
}

func NewLog() *Log {
	return &Log{
		chunkchan: make(chan []byte),
		closed:    make(chan struct{}),
	}
}

func (l *Log) Close() {
	close(l.closed)
}

// Sends the payload to the log, blocking until it is handled, and
// returning an error only if it can't be (such as after
// the log is closed).
func (l *Log) Write(payload []byte) error {
	select {
	case <-l.closed:
		// TODO: Too noisy?
		log.Printf("WRITE AFTER CLOSE: %s", payload)
		return errors.New("Write after close")
	case l.chunkchan <- payload:
		return nil
	}
}

// Writes the payload (with a newline appended) to the console, and
// uses Write to send it to the log.
func (l *Log) Writeln(payload string) error {
	e := l.Write([]byte(payload + "\n"))
	log.Println(payload)
	return e
}

// Repeatedly calls GetChunk() until Close is called.
// Mostly useful for tests.
func (l *Log) Drain() {
	for _, ok := l.GetChunk(); ok; _, ok = l.GetChunk() {
	}
}

// Returns the next log chunk, or a nil slice and false if
// Close was called.
func (l *Log) GetChunk() ([]byte, bool) {
	select {
	case ch := <-l.chunkchan:
		return ch, true
	case <-l.closed:
		return nil, false
	}
}

// Printf calls l.Writeln to print to the log. Arguments are handled in
// the manner of fmt.Printf.
// The output is guaranteed to be newline-terminated.
func (l *Log) Printf(format string, v ...interface{}) error {
	return l.Writeln(fmt.Sprintf(format, v...))
}

func (l *Log) WriteStream(pipe io.Reader) {
	lines := newLogLineReader(pipe)

	finished := false
	for !finished {
		var payload []byte
		timeLimit := time.After(timeFlushThreshold)

		for len(payload) < *byteFlushThreshold {
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
			l.Write(payload)
			log.Println(string(payload))
		}
	}
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
