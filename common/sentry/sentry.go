package sentry

import (
	"flag"
	"log"

	"github.com/dropbox/changes-client/common/version"
	"github.com/getsentry/raven-go"
	"sync"
)

var (
	sentryDsn       = ""
	sentryClient    *raven.Client
	sentryClientMux sync.Mutex
)

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

	sentryClient, err := raven.NewClient(sentryDsn, map[string]string{
		"version": version.Version,
	})
	if err != nil {
		// TODO: Try to avoid potentially dying fatally in a getter;
		// we may want to log an error and move on, we might want defers
		// to fire, etc. This will probably mean not creating the client
		// lazily.
		log.Fatal(err)
	}

	return sentryClient
}

func Error(err error, tags map[string]string) {
	if sentryClient := GetClient(); sentryClient != nil {
		sentryClient.CaptureError(err, tags)
	} else {
		log.Printf("[Sentry Error] %s", err.Error())
	}
}

func Message(str string, tags map[string]string) {
	if sentryClient := GetClient(); sentryClient != nil {
		sentryClient.CaptureMessage(str, tags)
	} else {
		log.Printf("[Sentry Message] %s")
	}
}

func init() {
	flag.StringVar(&sentryDsn, "sentry-dsn", "", "Sentry DSN for reporting errors")
}
