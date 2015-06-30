package sentry

import (
	"flag"
	"log"

	"github.com/dropbox/changes-client/common/version"
	"github.com/getsentry/raven-go"
)

var (
	sentryDsn    = ""
	sentryClient *raven.Client
)

func GetClient() *raven.Client {
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
