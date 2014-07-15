package runner

import (
    "bytes"
    "flag"
    "testing"
)

func TestProgressChunks(t *testing.T) {
    flag.Set("log_chunk_size", "3")

    in := []byte("aaa\naaa\naaa\n")
    ch := make(chan LogChunk)

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
