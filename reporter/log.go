package reporter

type LogSource struct {
	Name      string

	reporter  *Reporter
}

func NewLogSource(name string, reporter *Reporter) *LogSource {
	return &LogSource{
		Name: name,
		reporter: reporter,
	}
}

// TODO(dcramer): these should be part of the reporter
func (ls *LogSource) ReportChunks(chunks chan []byte) {
	for chunk := range chunks {
		ls.ReportBytes(chunk)
	}
}

func (ls *LogSource) ReportBytes(bytes []byte) {
	ls.reporter.PushLogChunk(ls.Name, bytes)
}
