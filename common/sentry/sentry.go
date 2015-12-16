package sentry

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"strings"

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

// Warningf takes a format string and arguments with the same meaning as with fmt.Printf and
// sends the message as a warning to Sentry if Sentry is configured.
// It makes a best-effort attempt to process the arguments to ensure that Sentry buckets the
// warning by the format string rather than by the specific values of the arguments.
func Warningf(msgfmt string, args ...interface{}) {
	if sentryClient := GetClient(); sentryClient != nil {
		msg := fmt.Sprintf(msgfmt, args...)
		packet := makePacket(raven.WARNING, msg, fmtSanitize(msgfmt, args))
		log.Printf("[Sentry Warning] %s", msg)
		sentryClient.Capture(packet, map[string]string{})
	} else {
		log.Printf("[Sentry Warning Unsent] %s", fmt.Sprintf(msgfmt, args...))
	}
}

func makePacket(severity raven.Severity, message string, ravenMsg *raven.Message) *raven.Packet {
	var ifaces []raven.Interface
	if ravenMsg != nil {
		ifaces = append(ifaces, ravenMsg)
	}
	p := raven.NewPacket(message, ifaces...)
	p.Level = severity
	return p
}

// The Sentry server doesn't speak Go fmt strings, so this tries to translate,
// or if it can't, return nil so we can just fall back to the locally fmt'd version.
func fmtSanitize(msg string, args []interface{}) *raven.Message {
	var newmsg bytes.Buffer
	newargs := append([]interface{}(nil), args...)
	argidx := 0
	const supportedVerbs = "qvds"
	lastpct := false
	for _, m := range msg {
		if lastpct {
			if strings.ContainsRune(supportedVerbs, m) {
				if argidx >= len(args) {
					return nil
				}
				newargs[argidx] = fmt.Sprintf("%"+string(m), args[argidx])
				newmsg.WriteRune('s')
				argidx++
			} else if m == '%' {
				newmsg.WriteRune('%')
			} else {
				return nil
			}
			lastpct = false
		} else {
			newmsg.WriteRune(m)
			lastpct = m == '%'
		}
	}
	if lastpct || argidx < len(args) {
		return nil
	}
	return &raven.Message{Message: newmsg.String(), Params: newargs}
}

func init() {
	flag.StringVar(&sentryDsn, "sentry-dsn", "", "Sentry DSN for reporting errors")
}
