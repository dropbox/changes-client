package reporter

import (
	"github.com/dropbox/changes-client/client"
	"github.com/dropbox/changes-client/client/adapter"
)

// An abstract way of communicating things to Changes.
type Reporter interface {
	Init(config *client.Config)
	Shutdown()

	// This function is not required to be synchronous, but it must do
	// something that will cause the artifacts to be published in the future.
	// In the case of Jenkins reporter builds, it moves the artifacts to a
	// location known by Jenkins, and considers these artifacts to be reported
	// as it relies on Jenkins to later pull those artifacts and send them to
	// Changes. Mesos sends the artifacts in a separate goroutine, so neither
	// reporter immediately publishes the artifacts.
	//
	// Jenkins and Mesos also take different approaches to detecting artifacts,
	// so this function is responsible for this as well. For Mesos builds, each
	// command lists the artifacts it is expected to return, but Jenkins builds
	// are expected to return any artifact within a folder. Since the detection
	// is different for each reporter and each detection relies on the adapter
	// to figure out where to actually look for files, a reference to the adapter
	// is required here.
	PublishArtifacts(cmd client.ConfigCmd, adapter adapter.Adapter, clientLog *client.Log) error

	// Like above, this is responsible for doing something that will at
	// some point in the future update the status of a snapshot image
	// but it is not required to happen immediately (which allows Jenkins
	// to transfer this information through the artifact pipeline rather
	// than through http).
	PushSnapshotImageStatus(iID string, status string)

	// These are optional, implement empty functions to just not provide
	// this functionality as a reporter (ie, Jenkins). However it should
	// be noted that if no other machinery provides this functionality
	// (as is the case for Mesos builds) then these are absolutely required
	// as without them Changes will never receive updates.
	PushCommandStatus(cID string, status string, retCode int)
	PushCommandOutput(cID string, status string, retCode int, output []byte)
	PushJobstepStatus(status string, result string)
	// returns false if pushing the log chunk failed
	PushLogChunk(source string, payload []byte) bool

	// Report any collected metrics. This is optional, but can be used to e.g.
	// send metrics to Changes.
	ReportMetrics(metrics client.Metrics)
}
