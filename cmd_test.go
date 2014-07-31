package runner

import (
	"bytes"
	"flag"
	"testing"
)

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
