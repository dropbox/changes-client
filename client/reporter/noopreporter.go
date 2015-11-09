package reporter

import (
	"github.com/dropbox/changes-client/client"
	"github.com/dropbox/changes-client/client/adapter"
)

type NoopReporter struct{}

func (noop *NoopReporter) Init(_ *client.Config) {}
func (noop *NoopReporter) PublishArtifacts(_ client.ConfigCmd, _ adapter.Adapter, _ *client.Log) error {
	return nil
}
func (noop *NoopReporter) PushCommandOutput(_, _ string, _ int, _ []byte) {}
func (noop *NoopReporter) PushCommandStatus(_, _ string, _ int)           {}
func (noop *NoopReporter) PushJobstepStatus(_, _ string)                  {}
func (noop *NoopReporter) PushLogChunk(_ string, _ []byte) bool           { return true }
func (noop *NoopReporter) PushSnapshotImageStatus(_, _ string) error      { return nil }
func (noop *NoopReporter) ReportMetrics(_ client.Metrics)                 {}
func (noop *NoopReporter) Shutdown()                                      {}

var _ Reporter = (*NoopReporter)(nil)
