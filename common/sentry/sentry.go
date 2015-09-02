package sentry

import (
	"flag"
	"log"

	"sync"

	"github.com/dropbox/changes-client/common/taggederr"
	"github.com/dropbox/changes-client/common/version"
	"github.com/getsentry/raven-go"
)

var (
	sentryDsn       = ""
	sentryClient    *raven.Client
	sentryClientMux sync.Mutex
)

type NoisyTransport struct {
	inner raven.Transport
}

// By default Sentry quietly drops errors and it is awkward to retrieve them at reporting sites,
// so this wraps the Transport to log any errors encountered.
func (nt *NoisyTransport) Send(url, authHeader string, packet *raven.Packet) error {
	err := nt.inner.Send(url, authHeader, packet)
	if err != nil {
		log.Printf("Error reporting to Sentry: %s (%s)", err, url)
	}
	return err
}

// Return our global Sentry client, or nil if none was configured.
func GetClient() *raven.Client {
	sentryClientMux.Lock()
	defer sentryClientMux.Unlock()
	if sentryClient != nil {
		return sentryClient
	}

	if sentryDsn == "" {
		return nil
	}

	if client, err := raven.NewClient(sentryDsn, map[string]string{
		"version": version.GetVersion(),
	}); err != nil {
		// TODO: Try to avoid potentially dying fatally in a getter;
		// we may want to log an error and move on, we might want defers
		// to fire, etc. This will probably mean not creating the client
		// lazily.
		log.Fatal(err)
	} else {
		sentryClient = client
	}
	sentryClient.Transport = &NoisyTransport{sentryClient.Transport}

	return sentryClient
}

func extractFromTagged(terr taggederr.TaggedErr, tags map[string]string) (error, map[string]string) {
	result := terr.GetTags()
	for k, v := range tags {
		result[k] = v
	}
	return terr.GetInner(), result
}

func Error(err error, tags map[string]string) {
	if sentryClient := GetClient(); sentryClient != nil {
		log.Printf("[Sentry Error] %s", err)
		if terr, ok := err.(taggederr.TaggedErr); ok {
			err, tags = extractFromTagged(terr, tags)
		}
		sentryClient.CaptureError(err, tags)
	} else {
		log.Printf("[Sentry Error Unsent] %s", err)
	}
}

func Message(str string, tags map[string]string) {
	if sentryClient := GetClient(); sentryClient != nil {
		log.Printf("[Sentry Message] %s", str)
		sentryClient.CaptureMessage(str, tags)
	} else {
		log.Printf("[Sentry Message Unsent] %s", str)
	}
}

func init() {
	flag.StringVar(&sentryDsn, "sentry-dsn", "", "Sentry DSN for reporting errors")
}
