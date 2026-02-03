package types

import (
	"sync"
)

// DeprecationLogger is a minimal logger interface for deprecation warnings.
// It is intentionally simpler than cosmossdk.io/log.Logger to avoid coupling
// to the full SDK logger interface, which doesn't have a Warn method.
//
// RATIONALE: Applications can implement this interface to receive deprecation
// warnings, or use DeprecationLoggerFromSDK to adapt an SDK logger.
type DeprecationLogger interface {
	// Warn logs a warning message with key-value pairs.
	Warn(msg string, keyvals ...interface{})
}

// nopDeprecationLogger is a no-op implementation of DeprecationLogger.
type nopDeprecationLogger struct{}

func (nopDeprecationLogger) Warn(msg string, keyvals ...interface{}) {}

// DeprecationLoggerProvider provides rate-limited deprecation warnings for the types package.
//
// RATIONALE: Deprecation warnings help teams identify code that needs migration
// without flooding logs in high-throughput scenarios. Each unique message type
// is logged only once per process lifetime.
//
// USAGE:
//
//	// Configure logger at application startup
//	types.SetDeprecationLogger(myLogger)
//
// INVARIANT: Logging is rate-limited per message type (at most once per type).
// INVARIANT: Default logger is no-op to avoid unexpected output in libraries.
var deprecationLogger = &rateLimitedLogger{
	logger:   nopDeprecationLogger{},
	loggedMu: sync.RWMutex{},
	logged:   make(map[string]struct{}),
}

// rateLimitedLogger wraps a logger to emit warnings at most once per unique key.
type rateLimitedLogger struct {
	logger   DeprecationLogger
	loggedMu sync.RWMutex
	logged   map[string]struct{}
}

// WarnOnce logs a warning at most once for the given key.
//
// INVARIANT: For any given key, the warning is emitted at most once.
// THREAD-SAFE: Safe for concurrent calls from multiple goroutines.
func (r *rateLimitedLogger) WarnOnce(key string, msg string, keyvals ...interface{}) {
	// Fast path: check if already logged (read lock)
	r.loggedMu.RLock()
	_, alreadyLogged := r.logged[key]
	r.loggedMu.RUnlock()

	if alreadyLogged {
		return
	}

	// Slow path: acquire write lock and double-check
	r.loggedMu.Lock()
	defer r.loggedMu.Unlock()

	// Double-check after acquiring write lock (another goroutine may have logged)
	if _, alreadyLogged := r.logged[key]; alreadyLogged {
		return
	}

	// Mark as logged before emitting (prevent re-entry)
	r.logged[key] = struct{}{}

	// Emit the warning
	r.logger.Warn(msg, keyvals...)
}

// SetDeprecationLogger configures the logger used for deprecation warnings.
//
// PRECONDITION: logger is not nil (no-op logger is used if nil is passed).
// POSTCONDITION: Subsequent deprecation warnings use the provided logger.
//
// NOTE: Previously logged warnings are NOT re-emitted. Rate-limiting state
// is preserved across SetDeprecationLogger calls to prevent duplicate logs
// if the logger is reconfigured.
//
// USAGE: Call at application startup before any transactions are processed.
//
//	func main() {
//	    // Option 1: Use a custom DeprecationLogger implementation
//	    types.SetDeprecationLogger(myLogger)
//
//	    // Option 2: Use standard library log package
//	    types.SetDeprecationLogger(&StdLogAdapter{})
//	}
func SetDeprecationLogger(logger DeprecationLogger) {
	if logger == nil {
		logger = nopDeprecationLogger{}
	}

	deprecationLogger.loggedMu.Lock()
	defer deprecationLogger.loggedMu.Unlock()

	deprecationLogger.logger = logger
}

// ResetDeprecationLogger resets the deprecation logger to its default state.
// This clears the rate-limiting state and resets the logger to no-op.
//
// USAGE: Primarily for testing to ensure clean state between tests.
func ResetDeprecationLogger() {
	deprecationLogger.loggedMu.Lock()
	defer deprecationLogger.loggedMu.Unlock()

	deprecationLogger.logger = nopDeprecationLogger{}
	deprecationLogger.logged = make(map[string]struct{})
}

// warnSignersOnlyFallback logs a deprecation warning for a message type that
// does not implement SignDocSerializable.
//
// SECURITY NOTE: This warning indicates that signatures for this message type
// do not bind to the full message content, only to the signers. This is a
// security weakness that should be addressed by implementing SignDocSerializable.
//
// INVARIANT: Warning is emitted at most once per message type.
func warnSignersOnlyFallback(msgType string) {
	deprecationLogger.WarnOnce(
		"signers-only-fallback:"+msgType,
		"DEPRECATION: message does not implement SignDocSerializable, using signers-only fallback",
		"msg_type", msgType,
		"security_note", "signatures do not bind to full message content",
		"action", "implement SignDocSerializable interface for this message type",
	)
}
