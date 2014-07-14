package runner

import (
    "bytes"
    "flag"
    "testing"
)

func TestProgressChunks(t *testing.T) {
    flag.Int("log_chunk_size", 3, "Chunk size")

    in := []byte("aaa\naaa\naaa\n")
    ch := make(chan LogChunk, 100)

    go func () {
        processChunks(ch, bytes.NewReader(in), "test")
        close(ch)
    }()

    cnt := 0
    for _ = range ch {
        cnt += 1
    }

    if cnt != 3 {
        t.Fail()
    }
}
