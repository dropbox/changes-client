package scopedlogger

import "log"

// ScopedLogger prefixes any prints with [Scope]
type ScopedLogger struct {
	Scope string
}

// Printf is a normal printf but with a scoped prefix
func (sl ScopedLogger) Printf(format string, v ...interface{}) {
	log.Printf("["+sl.Scope+"] "+format, v...)
}

// Println is a normal println but with a scoped prefix
func (sl ScopedLogger) Println(v ...interface{}) {
	log.Println(append([]interface{}{"[" + sl.Scope + "] "}, v...)...)
}

// Sub returns a ScopedLogger that is sub-scoped: it will prefix any prints with [Scope:name]
func (sl ScopedLogger) Sub(name string) ScopedLogger {
	return ScopedLogger{sl.Scope + ":" + name}
}
