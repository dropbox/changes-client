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
	Source  string
	Offset  int
	Length  int
	Payload []byte
}

func (logsource *LogSource) reportChunks(chunks chan LogChunk) {
	for chunk := range chunks {
		logsource.mu.Lock()
		offset := logsource.Offset
		logsource.Offset += chunk.Length
		logsource.mu.Unlock()

		fmt.Printf("Got another chunk from %s (%d-%d)\n", logsource.Name, offset, chunk.Length)
		fmt.Printf("%s", chunk.Payload)
		logsource.Reporter.PushLogChunk(logsource.JobstepID, logsource.Name, offset, chunk.Payload)
	}
}
