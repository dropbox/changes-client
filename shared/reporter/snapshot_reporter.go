package reporter

import (
	"log"
)

type SnapshotReporter struct {
	snapshotID string
	httpStream *HttpStream
	debug      bool
}

func NewSnapshotReporter(publishUri string, snapshotID string, debug bool) *SnapshotReporter {
	log.Printf("[reporter] Constructing snapshot reporter with publish uri: %s", publishUri)
	r := &SnapshotReporter{
		snapshotID: snapshotID,
		httpStream: NewHttpStream(publishUri, debug),
		debug:      debug,
	}

	return r
}

func (r *SnapshotReporter) PushBuildStatus(status string, result string) {
	form := make(map[string]string)
	if status != "" {
		form["status"] = status
	}
	if result != "" {
		form["result"] = result
	}
	r.httpStream.Push(HttpPayload{"/snapshotimages/" + r.snapshotID + "/", form, ""})
}

func (r *SnapshotReporter) PushCommandStatus(cID string, status string, retCode int) {
	// NOOP
}

func (r *SnapshotReporter) PushLogChunk(source string, payload []byte) {
	// NOOP
}

func (r *SnapshotReporter) PushCommandOutput(cID string, status string, retCode int, output []byte) {
	// NOOP
}

func (r *SnapshotReporter) PushArtifacts(artifacts []string) {
	// NOOP
}

func (r *SnapshotReporter) Shutdown() {
	r.httpStream.Shutdown()
}
