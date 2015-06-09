package client

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"time"
)

var (
	chunkSize = 4096
)

type Log struct {
	Chan chan []byte
}

type LogLine struct {
	line []byte
	err  error
}

func NewLog() *Log {
	return &Log{
		Chan: make(chan []byte),
	}
}

func (l *Log) Close() {
	close(l.Chan)
}

func (l *Log) Write(payload []byte) error {
	l.Chan <- payload

	return nil
}

func (l *Log) Writeln(payload string) error {
	l.Chan <- []byte(fmt.Sprintf("%s\n", payload))
	log.Println(payload)

	return nil
}

func (l *Log) WriteStream(pipe io.Reader) {
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
			l.Chan <- payload
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

func init() {
	flag.IntVar(&chunkSize, "log_chunk_size", 4096, "Size of log chunks to send to http server")
}
