package client

import (
	"log"
	"time"
)

// type for recording metrics we might want to report (e.g. to Changes)
type Metrics map[string]float64

// convenience object for timing functions/sections.
// See StartTimer() and Record() for usage.
type Timer struct {
	startTime time.Time
	m         Metrics
}

// set the duration for the given key
func (m Metrics) SetDuration(key string, value time.Duration) {
	log.Printf("==> %q = %s", key, value)
	m[key] = value.Seconds()
}

// calculate duration since `start` and use that as duration for given key
func (m Metrics) SetDurationSince(key string, start time.Time) time.Duration {
	dur := time.Since(start)
	m.SetDuration(key, dur)
	return dur
}

// return true if there are no actual metrics recorded
func (m Metrics) Empty() bool {
	return len(m) == 0
}

// create and return a timer starting now
func (m Metrics) StartTimer() Timer {
	return Timer{time.Now(), m}
}

// record a metric with the given key of the duration since this timer was created
func (t Timer) Record(key string) time.Duration {
	return t.m.SetDurationSince(key, t.startTime)
}
