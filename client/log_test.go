package client

import (
	"bytes"
	"flag"
	"testing"
	"time"
)

func TestWriteStream(t *testing.T) {
	flag.Set("log_chunk_size", "3")

	in := []byte("aaa\naaa\naaa\n")
	log := NewLog()

	go func() {
		log.WriteStream(bytes.NewReader(in))
		log.Close()
	}()

	cnt := 0
	for _, ok := log.GetChunk(); ok; _, ok = log.GetChunk() {
		cnt++
	}

	if cnt != 3 {
		t.Fail()
	}
}

func TestWriteln(t *testing.T) {
	in := "aaa\naaa\naaa\n"
	log := NewLog()

	go func() {
		log.Writeln(in)
		log.Close()
	}()

	var out [][]byte
	for c, ok := log.GetChunk(); ok; c, ok = log.GetChunk() {
		out = append(out, c)
	}

	if len(out) != 1 {
		t.Fail()
	}

	if bytes.Equal(out[0], []byte(in)) {
		t.Fail()
	}
}

func drain(l *Log) string {
	var out []byte
	for c, ok := l.GetChunk(); ok; c, ok = l.GetChunk() {
		out = append(out, c...)
	}
	return string(out)
}

func TestPrintf(t *testing.T) {
	log := NewLog()

	const expected = "Hello 4 Worlds!\n"
	go func() {
		log.Printf("Hello %d %s!", 4, "Worlds")
		log.Close()
	}()

	result := drain(log)
	if result != expected {
		t.Fatalf("Expected %q, got %q", expected, result)
	}
}

func TestLateSend(t *testing.T) {
	log := NewLog()
	log.Close()
	e := log.Writeln("Hello!")
	if e == nil {
		t.Fatalf("Expected error on Writeln")
	}
}

func TestCloseUnblocks(t *testing.T) {
	log := NewLog()
	rendez := make(chan bool)
	go func() {
		<-rendez
		t.Log(log.Writeln("Late"))
		rendez <- true
	}()
	// Racy validation, but make sure that the writer can run before we close
	rendez <- true
	// Sleep here, because we'd like the Writeln call in the other goroutine to be
	// blocked when we call Close. We can't easily guarantee it, but with rendez and
	// the sleep, it's really likely.
	time.Sleep(20 * time.Millisecond)
	log.Close()
	<-rendez
}
