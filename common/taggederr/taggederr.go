package taggederr

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
)

// Tags are stored internally as an immutable linked list
// with a shared tail rather than a map or slice to make adding
// tags cheap and thread-safe. This could behave badly if we had
// many duplicate tags, but that isn't expected.
type tag struct {
	k, v string
	next *tag
}

type TaggedErr struct {
	inner error
	tags  *tag
}

// New creates a new TaggedErr from a string describing the issue.
// This is a convenience wrapper around errors.New.
func New(msg string) TaggedErr {
	return Wrap(errors.New(msg))
}

// Newf creates a new TaggedErr from a format string and arguments
// in the manner of fmt.Printf.
// Currently, there is no benefit over fmt.Errorf aside from convenience,
// but in the future the format string and arguments may be retained to
// assist in error grouping in backends such as Sentry.
func Newf(fmtstr string, args ...interface{}) TaggedErr {
	// TODO: When possible, retain the format string and args
	// so they can be supplied independent to backends such as Sentry
	// for improved grouping.
	return Wrap(fmt.Errorf(fmtstr, args...))
}

// GetTags returns a new map with tags, preferring the value most recently
// applied when a tag has been set more than once.
func (t TaggedErr) GetTags() map[string]string {
	m := make(map[string]string)
	for tt := t.tags; tt != nil; tt = tt.next {
		if _, ok := m[tt.k]; !ok {
			m[tt.k] = tt.v
		}
	}
	return m
}

// GetInner returns the wrapped error value, which will never be nil.
func (t TaggedErr) GetInner() error {
	return t.inner
}

func (t TaggedErr) Error() string {
	if t.tags != nil {
		var keys []string
		m := make(map[string]string)
		// We ignore all but the first occurance of a tag;
		// they are ordered from most to least recently tagged, and
		// newer tags override older.
		for tt := t.tags; tt != nil; tt = tt.next {
			if _, ok := m[tt.k]; !ok {
				m[tt.k] = tt.v
				keys = append(keys, tt.k)
			}
		}
		sort.Strings(keys)
		var buf bytes.Buffer
		buf.WriteByte('[')
		for i, k := range keys {
			if i != 0 {
				buf.WriteByte(',')
			}
			buf.WriteString(k)
			buf.WriteByte('=')
			buf.WriteString(m[k])
		}
		buf.WriteString("]: ")
		buf.WriteString(t.inner.Error())
		return buf.String()
	}
	return t.inner.Error()
}

// AddTag creates a new TaggedErr value with the specified tag from the
// existing TaggedErr, keeping the wrapped value and all previous tags.
// If a tag already exists with the given key, it will be ignored in the new
// value.
// No operations on the new TaggedErr will impact the parent value.
func (t TaggedErr) AddTag(k, v string) TaggedErr {
	return TaggedErr{t.inner, &tag{k, v, t.tags}}
}

// Wrap wraps an error value to make it a TaggedErr, or exposes the
// TaggedErr if the parameter is already a TaggedErr.
// Passing nil will result in a panic.
func Wrap(e error) TaggedErr {
	if e == nil {
		panic("taggederr.Wrap called with nil error")
	}
	if te, ok := e.(TaggedErr); ok {
		return te
	}
	return TaggedErr{inner: e, tags: nil}
}
