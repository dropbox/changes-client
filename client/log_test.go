package client

import (
	"bytes"
	"flag"
	"testing"
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
	for _ = range log.Chan {
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
	for c := range log.Chan {
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
	for c := range l.Chan {
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
