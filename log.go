package runner

import (
	"fmt"
	"sync"
)

type LogSource struct {
	Name      string
	Offset    int
	JobstepID string
	Reporter  *Reporter
	mu        sync.Mutex
}

type LogChunk struct {
	Length  int
	Payload []byte
}

func (logsource *LogSource) reportChunks(chunks chan LogChunk) {
	for chunk := range chunks {
		logsource.reportBytes(chunk.Payload)
	}
}

func (logsource *LogSource) reportBytes(bytes []byte) {
	length := len(bytes)

	logsource.mu.Lock()
	offset := logsource.Offset
	logsource.Offset += length
	logsource.mu.Unlock()

	fmt.Printf("Got another chunk from %s (%d-%d)\n", logsource.Name, offset, length)
	fmt.Printf("%s", bytes)
	logsource.Reporter.PushLogChunk(logsource.JobstepID, logsource.Name, offset, bytes)
}
