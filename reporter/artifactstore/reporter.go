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

	artifacts "github.com/dropbox/changes-artifacts/client"
	"github.com/dropbox/changes-client/client"
	"github.com/dropbox/changes-client/client/adapter"
	"github.com/dropbox/changes-client/client/reporter"
	"github.com/dropbox/changes-client/common/sentry"
)

var (
	// Artifact server endpoint, like http://localhost:8001/
	artifactServer string

	// Bucket in the artifact server where content is being stored.
	// Defaults to jobstepID if blank string is given.
	artifactBucketId string
)

// Reporter instance to interact with artifact store API to post console logs and artifact files to
// the artifact store. It uses the artifact store client to perform most operations.
type Reporter struct {
	publishUri       string
	bucketID         string
	client           *artifacts.ArtifactStoreClient
	bucket           *artifacts.Bucket
	chunkedArtifacts map[string]*artifacts.ChunkedArtifact

	// We use nil value of bucket to indicate that the reporter is disabled. Every method (except
	// init) should test this value before proceeding.
}

func (r *Reporter) Init(c *client.Config) {
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

// source: Name of the log stream. Usually, differentiates between stdout and stderr streams.
// payload: Stream of bytes to append to this stream.
func (r *Reporter) PushLogChunk(source string, payload []byte) {
	if r.bucket == nil {
		return
	}

	if _, ok := r.chunkedArtifacts[source]; !ok {
		if artifact, err := r.bucket.NewChunkedArtifact(source); err != nil {
			sentry.Error(err, map[string]string{})

			log.Printf("Error creating console log artifact: %s\n", err)
			return
		} else {
			log.Printf("Created new artifact with name %s\n", source)
			r.chunkedArtifacts[source] = artifact
		}
	}

	logstream := r.chunkedArtifacts[source]
	logstream.AppendLog(string(payload[:]))
}

func (r *Reporter) PushCommandOutput(cID string, status string, retCode int, output []byte) {
	// IGNORED - We don't support command level outputs yet.
	// TODO: At some point in the future, we can add a per-command artifact to track output of each different command.
}

func (r *Reporter) PublishArtifacts(cmdCnf client.ConfigCmd, a adapter.Adapter, clientLog *client.Log) {
	if r.bucket == nil {
		return
	}

	if len(cmdCnf.Artifacts) == 0 {
		return
	}

	matches, err := a.CollectArtifacts(cmdCnf.Artifacts, clientLog)
	if err != nil {
		clientLog.Writeln(fmt.Sprintf("[artifactstore] ERROR filtering artifacts: " + err.Error()))
		return
	}

	var wg sync.WaitGroup
	for _, artifact := range matches {
		wg.Add(1)
		go func(artifact string) {
			defer wg.Done()

			clientLog.Writeln(fmt.Sprintf("[artifactstore] Uploading: %s", artifact))
			fileBaseName := filepath.Base(artifact)

			if f, err := os.Open(artifact); err != nil {
				clientLog.Writeln(fmt.Sprintf("[artifactstore] Error opening file for streaming %s: %s", artifact, err))
				return
			} else if stat, err := f.Stat(); err != nil {
				clientLog.Writeln(fmt.Sprintf("[artifactstore] Error stat'ing file for streaming %s: %s", artifact, err))
				return
			} else if sAfct, err := r.bucket.NewStreamedArtifact(fileBaseName, stat.Size()); err != nil {
				clientLog.Writeln(fmt.Sprintf("[artifactstore] Error creating streaming artifact for %s: %s", artifact, err))
				return
			} else {
				// TODO: If possible, avoid reading entire contents of the file into memory, and pass the
				// file io.Reader directly to http.Post.
				//
				// The reason it is done this way is because, using bytes.NewReader() ensures that
				// Content-Length header is set to a correct value. If not, it is left blank. Alternately,
				// we could remove this requirement from the server where Content-Length is verified before
				// starting upload to S3.
				if contents, err := ioutil.ReadAll(f); err != nil {
					clientLog.Writeln(fmt.Sprintf("[artifactstore] Error reading file for streaming %s: %s", artifact, err))
					return
				} else if err := sAfct.UploadArtifact(bytes.NewReader(contents)); err != nil {
					// TODO retry if not a terminal error
					clientLog.Writeln(fmt.Sprintf("[artifactstore] Error uploading contents of %s", artifact, err))
					return
				}
			}
			clientLog.Writeln(fmt.Sprintf("[artifactstore] Successfully uploaded artifact %s", artifact))
		}(artifact)
	}

	wg.Wait()
}

func (r *Reporter) Shutdown() {
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

func init() {
	reporter.Register("artifactstore", &Reporter{chunkedArtifacts: make(map[string]*artifacts.ChunkedArtifact)})
	// TODO(anupc): We're currently hard-coding the address of a deployed artifacts server. Change
	// this to use a well known DNS address, or have Changes send this value in.
	flag.StringVar(&artifactServer, "artifactServer", "https://artifacts.build.itc.dropbox.com", "Artifacts server URL. If blank, this reporter is disabled.")
	flag.StringVar(&artifactBucketId, "artifactBucketId", "", "Artifacts Bucket ID")
}
