package player

import "sync/atomic"

var traceLogEnabled atomic.Bool

// SetTraceLoggingEnabled enables or disables trace logging functionality globally based on the provided boolean flag.
func SetTraceLoggingEnabled(enabled bool) {
	traceLogEnabled.Store(enabled)
}

// isTraceLoggingEnabled checks if trace-level logging is currently enabled and returns the corresponding boolean value.
func isTraceLoggingEnabled() bool {
	return traceLogEnabled.Load()
}
