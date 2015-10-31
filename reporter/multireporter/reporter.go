package multireporter

import (
	"flag"
	"log"
	"strings"

	"github.com/dropbox/changes-client/client"
	"github.com/dropbox/changes-client/client/adapter"
	"github.com/dropbox/changes-client/client/reporter"
	"github.com/dropbox/changes-client/common/sentry"
)

// Colon-separated list of downstream reporters to multiplex all Reporter operations to
var reporterDestinations string

// Sends out notifications to multiple reporters.
// Currently used to dual-write logs and artifacts to both Changes DB and Artifact Store, while the
// store is being evaluated for stability and performance.
type Reporter struct {
	reporterDestinations []reporter.Reporter
}

func (r *Reporter) Init(c *client.Config) {
	reporters := strings.Split(reporterDestinations, ":")
	for _, rep := range reporters {
		if newRep, err := reporter.Create(rep); err != nil {
			if sentryClient := sentry.GetClient(); sentryClient != nil {
				sentryClient.CaptureError(err, map[string]string{})
			}

			// Allow other reporters to proceed
			continue
		} else {
			log.Printf("[multireporter] Initialization successful: %s", rep)
			r.reporterDestinations = append(r.reporterDestinations, newRep)
		}
	}

	log.Printf("[multireporter] Setting up multiple client reporters: %s\n", reporters)

	for _, rep := range r.reporterDestinations {
		rep.Init(c)
	}
}

func (r *Reporter) PushJobstepStatus(status string, result string) {
	for _, r := range r.reporterDestinations {
		r.PushJobstepStatus(status, result)
	}
}

func (r *Reporter) PushCommandStatus(cID string, status string, retCode int) {
	for _, r := range r.reporterDestinations {
		r.PushCommandStatus(cID, status, retCode)
	}
}

func (r *Reporter) PushSnapshotImageStatus(iID string, status string) {
	for _, r := range r.reporterDestinations {
		r.PushSnapshotImageStatus(iID, status)
	}
}

func (r *Reporter) PushLogChunk(source string, payload []byte) bool {
	success := true
	for _, r := range r.reporterDestinations {
		success = success && r.PushLogChunk(source, payload)
	}
	return success
}

func (r *Reporter) PushCommandOutput(cID string, status string, retCode int, output []byte) {
	for _, r := range r.reporterDestinations {
		r.PushCommandOutput(cID, status, retCode, output)
	}
}

func (r *Reporter) PublishArtifacts(cmdCnf client.ConfigCmd, a adapter.Adapter, clientLog *client.Log) error {
	var firstError error
	for _, r := range r.reporterDestinations {
		if e := r.PublishArtifacts(cmdCnf, a, clientLog); e != nil && firstError == nil {
			firstError = e
		}
	}
	return firstError
}

func (r *Reporter) ReportMetrics(metrics client.Metrics) {
	for _, r := range r.reporterDestinations {
		r.ReportMetrics(metrics)
	}
}

func (r *Reporter) Shutdown() {
	for _, r := range r.reporterDestinations {
		r.Shutdown()
	}
}

func New() reporter.Reporter {
	return &Reporter{}
}

func init() {
	flag.StringVar(&reporterDestinations, "reporter-destinations", "mesos:artifactstore", "Colon-separated list of reporter destinations")

	reporter.Register("multireporter", New)
}
