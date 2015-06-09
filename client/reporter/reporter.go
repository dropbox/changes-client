package reporter

import (
	"github.com/dropbox/changes-client/client"
	"github.com/dropbox/changes-client/client/adapter"
)

type Reporter interface {
	Init(config *client.Config)
	Shutdown()

	PublishArtifacts(cmd client.ConfigCmd, adapter adapter.Adapter, clientLog *client.Log)

	// These are optional, implement empty functions to just not provide
	// this functionality as a reporter (ie, Jenkins)
	PushCommandStatus(cID string, status string, retCode int)
	PushCommandOutput(cID string, status string, retCode int, output []byte)
	PushJobstepStatus(status string, result string)
	PushLogChunk(source string, payload []byte)
	PushSnapshotImageStatus(iID string, status string)
}
