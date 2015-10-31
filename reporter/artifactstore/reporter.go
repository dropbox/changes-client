package artifactstorereporter

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	artifacts "github.com/dropbox/changes-artifacts/client"
	"github.com/dropbox/changes-client/client"
	"github.com/dropbox/changes-client/client/adapter"
	"github.com/dropbox/changes-client/client/reporter"
	"github.com/dropbox/changes-client/common/atomicflag"
	"github.com/dropbox/changes-client/common/sentry"
)

var (
	// Artifact server endpoint, like http://localhost:8001/
	artifactServer string

	// Bucket in the artifact server where content is being stored.
	// Defaults to jobstepID if blank string is given.
	artifactBucketId string

	// Keep errors from escaping the Reporter.
	discardErrors bool
)

const DefaultDeadline time.Duration = 30 * time.Second

// Reporter instance to interact with artifact store API to post console logs and artifact files to
// the artifact store. It uses the artifact store client to perform most operations.
type Reporter struct {
	client           *artifacts.ArtifactStoreClient
	bucket           *artifacts.Bucket
	chunkedArtifacts map[string]*artifacts.ChunkedArtifact
	disabled         atomicflag.AtomicFlag
	deadline         time.Duration
}

func (r *Reporter) markDeadlineExceeded() {
	r.disabled.Set(true)
}

func (r *Reporter) isDisabled() bool {
	return r.disabled.Get()
}

func (r *Reporter) Init(c *client.Config) {
	r.runWithDeadline(r.deadline, func() {
		if artifactServer == "" {
			log.Printf("[artifactstorereporter] No artifact server url provided. Disabling reporter.")
			return
		}

		log.Printf("[artifactstorereporter] Setting up artifact store client: %s\n", artifactServer)
		r.client = artifacts.NewArtifactStoreClient(artifactServer)

		if len(artifactBucketId) == 0 {
			artifactBucketId = c.JobstepID
		}

		// TODO(anupc): At some point in the future, creating a new bucket should be driven by Changes
		// server, rather than being done by the test itself. It makes the process of integrating with
		// Changes common across both Mesos and Jenkins builds.
		//
		// TODO retry
		if bucket, err := r.client.NewBucket(artifactBucketId, "changes", 60); err != nil {
			sentry.Error(err, map[string]string{})
			log.Printf("Error creating new bucket '%s' on artifact server: %s\n", artifactBucketId, err)
			return
		} else {
			log.Printf("Created new bucket %s\n", artifactBucketId)
			r.bucket = bucket
		}
	})
}

func (r *Reporter) PushJobstepStatus(status string, result string) {
	// IGNORED - Not relevant
}

func (r *Reporter) PushCommandStatus(cID string, status string, retCode int) {
	// IGNORED - Not relevant
}

func (r *Reporter) PushSnapshotImageStatus(iID string, status string) {
	// IGNORED - Not relevant
}

func (r *Reporter) ReportMetrics(metrics client.Metrics) {
	// IGNORED - Not relevant
}

// source: Name of the log stream. Usually, differentiates between stdout and stderr streams.
// payload: Stream of bytes to append to this stream.
func (r *Reporter) PushLogChunk(source string, payload []byte) bool {
	retch := make(chan bool, 1)
	r.runWithDeadline(r.deadline, func() {
		if r.bucket == nil {
			retch <- false
			return
		}

		if _, ok := r.chunkedArtifacts[source]; !ok {
			if artifact, err := r.bucket.NewChunkedArtifact(source); err != nil {
				sentry.Error(err, map[string]string{})
				log.Printf("Error creating log artifact for %s: %s", source, err)
				retch <- false
				return
			} else {
				log.Printf("Created new artifact with name %s", source)
				r.chunkedArtifacts[source] = artifact
			}
		}

		logstream := r.chunkedArtifacts[source]
		logstream.AppendLog(string(payload[:]))
		retch <- true
	})
	select {
	case ret := <-retch:
		return ret
	default:
		return false
	}
}

func (r *Reporter) PushCommandOutput(cID string, status string, retCode int, output []byte) {
	// IGNORED - We don't support command level outputs yet.
	// TODO: At some point in the future, we can add a per-command artifact to track output of each different command.
}

// Transforms a function returning an error into a function with no return value,
// with the error instead sent to a channel if not nil.
// If the channel send can't immediately succeed, the error will be discarded.
// The intended use is to allow multiple operations to run (potentially concurrently) and
// to capture only the first non-nil error.
func captureError(errch chan error, fn func() error) func() {
	return func() {
		if err := fn(); err != nil {
			select {
			case errch <- err:
			default:
			}
		}
	}
}

func constructArtifactRelativePath(absoluteArtifactPath string, artifactSourceDir string) string {
	if relativeName, err := filepath.Rel(artifactSourceDir, absoluteArtifactPath); err == nil {
		// If relative path was calculated without issues, use it to construct artifact name.
		return relativeName
	}

	// This will only happen if the artifact path was not absolute (we expect result from
	// CollectArtifacts to only have absolute file paths).
	log.Printf("[artifactstore] Could not determine relative file path. Using base file name as artifact name.")
	return absoluteArtifactPath
}

func (r *Reporter) PublishArtifacts(cmdCnf client.ConfigCmd, a adapter.Adapter, clientLog *client.Log) error {
	// first non-nil error
	firstError := make(chan error, 1)
	r.runWithDeadline(r.deadline, captureError(firstError, func() error {
		if r.bucket == nil {
			return nil
		}

		if len(cmdCnf.Artifacts) == 0 {
			return nil
		}

		matches, err := a.CollectArtifacts(cmdCnf.Artifacts, clientLog)
		if err != nil {
			clientLog.Printf("[artifactstore] ERROR filtering artifacts: %s", err)
			return err
		}

		var wg sync.WaitGroup
		for _, artifact := range matches {
			wg.Add(1)
			go func(artifact string) {
				defer wg.Done()
				artifactName := filepath.Base(artifact)

				log.Printf("[artifactstore] Uploading %s (from %s)", artifactName, artifact)
				send := func() error {
					if f, err := os.Open(artifact); err != nil {
						clientLog.Printf("[artifactstore] Error opening file for streaming %s: %s", artifact, err)
						return err
					} else if stat, err := f.Stat(); err != nil {
						clientLog.Printf("[artifactstore] Error stat'ing file for streaming %s: %s", artifact, err)
						return err
					} else if stat.Size() == 0 {
						clientLog.Printf("[artifactstore] Ignoring zero-length artifact %s", artifact)
						return nil
					} else if sAfct, err := r.bucket.NewStreamedArtifact(constructArtifactRelativePath(artifact, a.GetArtifactRoot()), stat.Size()); err != nil {
						clientLog.Printf("[artifactstore] Error creating streaming artifact for %s: %s", artifact, err)
						return err
					} else {
						// TODO: If possible, avoid reading entire contents of the file into memory, and pass the
						// file io.Reader directly to http.Post.
						//
						// The reason it is done this way is because, using bytes.NewReader() ensures that
						// Content-Length header is set to a correct value. If not, it is left blank. Alternately,
						// we could remove this requirement from the server where Content-Length is verified before
						// starting upload to S3.
						if contents, err := ioutil.ReadAll(f); err != nil {
							clientLog.Printf("[artifactstore] Error reading file for streaming %s: %s", artifact, err)
							return err
						} else if err := sAfct.UploadArtifact(bytes.NewReader(contents)); err != nil {
							// TODO retry if not a terminal error
							clientLog.Printf("[artifactstore] Error uploading contents of %s: %s", artifact, err)
							return err
						} else {
							clientLog.Printf("[artifactstore] Successfully uploaded artifact %s to %s", artifact, sAfct.GetContentURL())
							return nil
						}
					}
				}
				captureError(firstError, send)()
			}(artifact)
		}

		wg.Wait()
		// Any async errors will be sent to the channel
		return nil
	}))
	select {
	case err := <-firstError:
		if !discardErrors {
			return err
		}
	default:
	}
	return nil
}

func (r *Reporter) Shutdown() {
	r.runWithDeadline(r.deadline, r.shutdown)
}

func (r *Reporter) shutdown() {
	if r.bucket == nil {
		return
	}

	// Wait for queued uploads to complete.
	log.Printf("[artifactstore] Waiting for artifacts to upload...")
	for _, cArt := range r.chunkedArtifacts {
		if err := cArt.Flush(); err != nil {
			sentry.Error(err, map[string]string{})
		}
	}
	log.Printf("[artifactstore] Artifacts finished uploading.")

	// Close the bucket. This implicitly closes all artifacts in the bucket.
	// TODO retry
	err := r.bucket.Close()
	if err != nil {
		sentry.Error(err, map[string]string{})
	}
}

func (r *Reporter) runWithDeadline(t time.Duration, f func()) {
	if r.isDisabled() {
		log.Println("Reporter is disabled. Not calling method")
		return
	}

	done := make(chan bool, 1)
	go func() {
		f()
		done <- true
	}()

	select {
	case <-time.After(t):
		sentry.Error(fmt.Errorf("Timed out after %s\n", t), map[string]string{})
		r.markDeadlineExceeded()
		return
	case <-done:
		return
	}
}

func New() reporter.Reporter {
	return &Reporter{chunkedArtifacts: make(map[string]*artifacts.ChunkedArtifact), deadline: DefaultDeadline}
}

func init() {
	reporter.Register("artifactstore", New)
	flag.StringVar(&artifactServer, "artifacts-server", "", "Artifacts server URL. If blank, this reporter is disabled.")
	flag.StringVar(&artifactBucketId, "artifacts-bucket-id", "", "Artifacts Bucket ID (inside the main bucket; not a real s3 bucket; must not exist)")
	flag.BoolVar(&discardErrors, "discard-artifactstore-errors", true, "Whether to keep ArtifactStore reporter errors from escaping and potentially triggering a failure")
}
