package client

import (
	"fmt"
)

type LogSource struct {
	Name      string
	Offset    int
	JobstepID string
	Reporter  *Reporter
}

type LogChunk struct {
	Length  int
	Payload []byte
}

func (logsource *LogSource) ReportChunks(chunks chan LogChunk) {
	for chunk := range chunks {
		logsource.ReportBytes(chunk.Payload)
	}
}

func (logsource *LogSource) ReportBytes(bytes []byte) {
	length := len(bytes)

	offset := logsource.Offset
	logsource.Offset += length

	fmt.Printf("Got another chunk from %s (%d-%d)\n", logsource.Name, offset, length)
	fmt.Printf("%s", bytes)
	logsource.Reporter.PushLogChunk(logsource.JobstepID, logsource.Name, offset, bytes)
}
