package filelog

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/dropbox/changes-client/client/reporter"
	"github.com/dropbox/changes-client/common/sentry"
)

const chunkSize = 4096

// FileLog is an io.Writer that appends to a /tmp file and then periodically
// tails these contents to a given reporter
type FileLog struct {
	name             string
	flushDelay       time.Duration
	reporter         reporter.Reporter
	reporterLock     *sync.Mutex
	readFile         *os.File
	writeFile        *os.File
	shutdown         chan struct{}
	shutdownComplete chan struct{}
}

// Create a new FileLog. Must use this rather than creating struct directly.
func New(jobstepID, name string) (*FileLog, error) {
	return NewWithOptions(jobstepID, name, 4*time.Second, "")
}

// Same as New() but allows specifying how long to wait between flushing to the
// reporter, and the root directory for the log file
// (which has a sensible default if empty)
func NewWithOptions(jobstepID, name string, flushDelay time.Duration, rootDir string) (*FileLog, error) {
	if rootDir == "" {
		rootDir = filepath.Join(os.TempDir(), "changes-client")
	}
	directory := filepath.Join(rootDir, jobstepID)
	filename := filepath.Join(directory, fmt.Sprintf("%s.log", name))
	f := &FileLog{name: name, flushDelay: flushDelay, reporterLock: &sync.Mutex{},
		shutdown: make(chan struct{}), shutdownComplete: make(chan struct{})}
	if err := os.MkdirAll(directory, 0755); err != nil {
		return nil, err
	}
	var err error
	f.writeFile, err = os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0666)
	if err != nil {
		return nil, err
	}
	f.readFile, err = os.Open(filename)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// Writes payload to temp file, which will eventually be sent to a reporter
func (f *FileLog) Write(p []byte) (int, error) {
	return f.writeFile.Write(p)
}

// Begins reporting the contents of the log (as it is appended to) to the
// given reporter. Before this, Write() calls only go into the temp file.
// Should only be called once.
func (f *FileLog) StartReporting(reporter reporter.Reporter) {
	f.reporterLock.Lock()
	defer f.reporterLock.Unlock()
	if f.reporter == nil {
		select {
		case <-f.shutdownComplete:
			// called after shutdown, just exit
			return
		default:
		}
		f.reporter = reporter
		go f.readFromFile()
	} else {
		panic("StartReporting called more than once--panicking")
	}
}

// Shutdown the log, blocking until any remaining contents are sent to the
// reporter. Write() still goes to the temp file after this is called.
func (f *FileLog) Shutdown() {
	f.reporterLock.Lock()
	defer f.reporterLock.Unlock()
	if f.reporter != nil {
		select {
		case f.shutdown <- struct{}{}:
			// block until readFromFile() finishes sending any remaining data
			<-f.shutdownComplete
		// in case shutdown has already completed
		case <-f.shutdownComplete:
		}
	} else {
		// normally readFromFile() closes f.shutdownComplete but if there's
		// no reporter/goroutine spawned, we close f.shutdownComplete
		// ourselves (if it's not closed already) to mark this filelog as
		// closed (e.g. to StartReporting()).
		select {
		case <-f.shutdownComplete: // already closed
		default:
			close(f.shutdownComplete)
		}
	}
	f.readFile.Close()
	// we don't close writeFile so that any remaining logs still at least go in
	// the temp file
}

// goroutine which periodically reads from the temp file and tails it to the reporter
func (f *FileLog) readFromFile() {
	defer close(f.shutdownComplete)
	running := true
	for running {
		select {
		case <-time.After(f.flushDelay):
		case <-f.shutdown:
			running = false
		}
		var err error
		var send []byte
		b := make([]byte, chunkSize)
		for err == nil {
			var n int
			n, err = f.readFile.Read(b)
			send = append(send, b[:n]...)
			if len(send) >= chunkSize {
				if !f.reporter.PushLogChunk(f.name, send) {
					// push failed, wait for shutdown
					if running {
						<-f.shutdown
					}
					return
				}
				send = nil
			}
		}
		if len(send) > 0 {
			if !f.reporter.PushLogChunk(f.name, send) {
				// push failed, wait for shutdown
				if running {
					<-f.shutdown
				}
				return
			}
		}
		if err != io.EOF {
			log.Printf("Encountered error reading from %s log file: %s", f.name, err)
			sentry.Error(err, map[string]string{})
		}
	}
}
