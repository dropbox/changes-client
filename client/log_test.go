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
