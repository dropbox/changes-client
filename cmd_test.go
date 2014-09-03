package runner

import (
	"bytes"
	"flag"
	"testing"
)

func TestRun(t *testing.T) {
	wc, err := NewWrappedScriptCommand("#!/bin/bash\necho -n 1", "foo")
	if err != nil {
		t.Fail()
	}

	cnt := 0
	sem := make(chan bool)
	go func() {
		for _ = range wc.ChunkChan {
			cnt++
		}
		sem <- true
	}()

	_, err = wc.Run(true)
	if err != nil {
		t.Fail()
	}
	<-sem

	if !bytes.Equal(wc.Output, []byte("1")) {
		t.Error("Did not buffer output")
	}
}

// if stdin is allowed this test will hang
func TestRunIgnoresStdin(t *testing.T) {
	wc, err := NewWrappedScriptCommand("#!/bin/bash\nread foo", "foo")
	if err != nil {
		t.Fail()
	}

	sem := make(chan bool)
	go func() {
		for _ = range wc.ChunkChan {
		}
		sem <- true
	}()

	_, err = wc.Run(true)
	if err == nil {
		t.Error("Expected command to fail")
	}
	<-sem
}

func TestRunFailToStart(t *testing.T) {
	wc, err := NewWrappedScriptCommand("echo 1", "foo")
	if err != nil {
		t.Fail()
	}

	cnt := 0
	sem := make(chan bool)
	go func() {
		for _ = range wc.ChunkChan {
			cnt++
		}
		sem <- true
	}()

	_, err = wc.Run(false)
	if err == nil {
		t.Fail()
	}
	<-sem
}

func TestProcessChunks(t *testing.T) {
	flag.Set("log_chunk_size", "3")

	in := []byte("aaa\naaa\naaa\n")
	ch := make(chan LogChunk)

	go func() {
		processChunks(ch, bytes.NewReader(in))
		close(ch)
	}()

	cnt := 0
	for _ = range ch {
		cnt++
	}

	if cnt != 3 {
		t.Fail()
	}
}

func TestProcessMessage(t *testing.T) {
	in := "aaa\naaa\naaa\n"
	ch := make(chan LogChunk)

	go func() {
		processMessage(ch, in)
		close(ch)
	}()

	var out []LogChunk
	for c := range ch {
		out = append(out, c)
	}

	if len(out) != 1 {
		t.Fail()
	}

	if bytes.Equal(out[0].Payload, []byte(in)) {
		t.Fail()
	}
}
