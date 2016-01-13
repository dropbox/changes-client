package filelog

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dropbox/changes-client/client/reporter"
)

type logChunkReporter struct {
	logged chan string
	reporter.NoopReporter
}

func (lcr *logChunkReporter) PushLogChunk(name string, data []byte) bool {
	lcr.logged <- string(data)
	return true
}

type failReporter struct {
	reporter.NoopReporter
}

func (fr *failReporter) PushLogChunk(name string, data []byte) bool {
	return false
}

func newTempDir(t *testing.T) string {
	tempdir, err := ioutil.TempDir("", "filelog_test")
	require.NoError(t, err)
	return tempdir
}

func newLog(t *testing.T, flushDelay time.Duration, testDir string) *FileLog {
	log, err := NewWithOptions("1", "infralog", flushDelay, testDir)
	require.NoError(t, err)
	return log
}

func TestCreatesTempDir(t *testing.T) {
	tempdir := newTempDir(t)
	defer os.RemoveAll(tempdir)
	// will fail if this directory isn't created
	log := newLog(t, 0, filepath.Join(tempdir, "doesnt_exist"))
	log.Shutdown()
}

func TestWriteAfterReporter(t *testing.T) {
	tempdir := newTempDir(t)
	defer os.RemoveAll(tempdir)
	log := newLog(t, 0, tempdir)

	msg := "foo"
	reporter := &logChunkReporter{logged: make(chan string)}
	log.StartReporting(reporter)
	log.Write([]byte(msg))
	require.Equal(t, msg, <-reporter.logged)
	log.Shutdown()
}

func TestWriteBeforeReporter(t *testing.T) {
	tempdir := newTempDir(t)
	defer os.RemoveAll(tempdir)
	log := newLog(t, 0, tempdir)

	bufferMsg1 := "foo"
	bufferMsg2 := "bar"
	noBufferMsg := "baz"
	reporter := &logChunkReporter{logged: make(chan string)}
	log.Write([]byte(bufferMsg1))
	log.Write([]byte(bufferMsg2))

	log.StartReporting(reporter)
	require.Equal(t, bufferMsg1+bufferMsg2, <-reporter.logged)

	log.Write([]byte(noBufferMsg))
	require.Equal(t, noBufferMsg, <-reporter.logged)
	log.Shutdown()
}

func TestDelay(t *testing.T) {
	const delay = 20 * time.Millisecond
	tempdir := newTempDir(t)
	defer os.RemoveAll(tempdir)
	log := newLog(t, delay, tempdir)

	msg := "foo"
	reporter := &logChunkReporter{logged: make(chan string, 1)}
	log.StartReporting(reporter)
	log.Write([]byte(msg))
	select {
	case <-reporter.logged:
		t.Fatalf("Should be waiting %s to log", delay)
	case <-time.After(delay / 2):
	}
	require.Equal(t, msg, <-reporter.logged)
	log.Shutdown()
}

func TestShutdownFlushes(t *testing.T) {
	tempdir := newTempDir(t)
	defer os.RemoveAll(tempdir)
	log := newLog(t, 1*time.Hour, tempdir)

	msg := "foo"
	reporter := &logChunkReporter{logged: make(chan string, 1)}
	log.StartReporting(reporter)
	log.Write([]byte(msg))
	log.Shutdown()
	require.Equal(t, msg, <-reporter.logged)
}

func TestLateWrite(t *testing.T) {
	tempdir := newTempDir(t)
	defer os.RemoveAll(tempdir)
	log := newLog(t, 0, tempdir)

	log.Shutdown()
	_, err := log.Write([]byte("foo"))
	require.NoError(t, err)
}

func TestShutdownWaitsForSend(t *testing.T) {
	tempdir := newTempDir(t)
	defer os.RemoveAll(tempdir)
	log := newLog(t, 0, tempdir)
	rendez := make(chan bool)
	reporter := &logChunkReporter{logged: make(chan string)}
	log.StartReporting(reporter)

	didWait := false
	go func() {
		<-rendez
		_, err := log.Write([]byte("foo"))
		require.NoError(t, err)
		time.Sleep(30 * time.Millisecond)
		// if log.Shutdown() isn't waiting for the reporter to finish (and send to reporter.logged)
		// we should get a race detection and/or didWait will be false
		didWait = true
		<-reporter.logged
	}()
	rendez <- true
	time.Sleep(20 * time.Millisecond)
	log.Shutdown()
	require.True(t, didWait)
}

func TestStartReportingShutdownRace(t *testing.T) {
	tempdir := newTempDir(t)
	defer os.RemoveAll(tempdir)
	log := newLog(t, 0, tempdir)
	reporter := &logChunkReporter{logged: make(chan string, 1)}

	go log.StartReporting(reporter)
	log.Shutdown()
}

func TestStartReportingAfterShutdown(t *testing.T) {
	tempdir := newTempDir(t)
	defer os.RemoveAll(tempdir)
	log := newLog(t, 0, tempdir)
	reporter := &logChunkReporter{logged: make(chan string, 1)}

	log.Shutdown()
	log.Shutdown()
	log.StartReporting(reporter)
	require.Nil(t, log.reporter)
}

func TestShutdownRace(t *testing.T) {
	tempdir := newTempDir(t)
	defer os.RemoveAll(tempdir)
	log := newLog(t, 0, tempdir)
	reporter := &logChunkReporter{logged: make(chan string, 1)}
	log.StartReporting(reporter)

	go log.Shutdown()
	log.Shutdown()
}

func TestFailedReporter(t *testing.T) {
	tempdir := newTempDir(t)
	defer os.RemoveAll(tempdir)
	log := newLog(t, 0, tempdir)
	log.Write([]byte("foo"))
	reporter := &failReporter{}
	log.StartReporting(reporter)
	log.Shutdown()
}

func TestNoConflict(t *testing.T) {
	tempdir := newTempDir(t)
	defer os.RemoveAll(tempdir)
	log1, err1 := NewWithOptions("1", "infralog", 0, tempdir)
	require.NoError(t, err1)
	log1.Shutdown()
	log2, err2 := NewWithOptions("1", "infralog", 0, tempdir)
	require.NoError(t, err2)
	log2.Shutdown()
}
