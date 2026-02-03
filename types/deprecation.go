package types

import (
	"log"
	"reflect"
	"sync"
	"time"
)

// DeprecationLogger provides rate-limited deprecation warnings.
//
// RATIONALE: In high-throughput scenarios, logging every deprecation warning
// would cause log spam and performance degradation. Rate limiting ensures
// visibility of deprecated usage patterns without overwhelming logs.
//
// THREAD-SAFETY: All methods are safe for concurrent use.
type DeprecationLogger struct {
	mu sync.Mutex

	// lastWarningTime tracks when we last logged a warning for each message type
	lastWarningTime map[string]time.Time

	// warningInterval is the minimum time between warnings for the same message type
	warningInterval time.Duration

	// enabled controls whether warnings are logged
	enabled bool

	// logger is the underlying logger (nil uses default log package)
	logger *log.Logger
}

// defaultDeprecationLogger is the package-level deprecation logger instance.
// It is initialized with sensible defaults for production use.
var defaultDeprecationLogger = &DeprecationLogger{
	lastWarningTime: make(map[string]time.Time),
	warningInterval: 60 * time.Second, // Log at most once per minute per message type
	enabled:         true,
	logger:          nil, // Use default log package
}

// SignersOnlyFallbackDeprecation logs a deprecation warning when a message
// does not implement SignDocSerializable and falls back to signers-only mode.
//
// DEPRECATION TIMELINE:
// - v0.x: Warning logged (current behavior)
// - v1.0: Consider making SignDocSerializable required for all messages
// - Future: Remove signers-only fallback entirely
//
// SECURITY NOTE: The signers-only fallback is a security weakness because
// signatures do not bind to the full message content. This means two different
// messages with the same signers could potentially share signatures.
//
// This function is rate-limited to prevent log spam in high-throughput scenarios.
// At most one warning per message type is logged per warningInterval (default: 60s).
func SignersOnlyFallbackDeprecation(msg Message) {
	defaultDeprecationLogger.warnSignersOnlyFallback(msg)
}

// warnSignersOnlyFallback logs a rate-limited deprecation warning.
func (dl *DeprecationLogger) warnSignersOnlyFallback(msg Message) {
	if !dl.enabled {
		return
	}

	msgType := getMsgTypeName(msg)

	dl.mu.Lock()
	defer dl.mu.Unlock()

	now := time.Now()
	if lastTime, exists := dl.lastWarningTime[msgType]; exists {
		if now.Sub(lastTime) < dl.warningInterval {
			// Rate limited - skip this warning
			return
		}
	}

	dl.lastWarningTime[msgType] = now

	// Log the warning
	warning := "DEPRECATION WARNING: message does not implement SignDocSerializable, " +
		"using signers-only fallback. " +
		"msg_type=" + msgType + " " +
		"security_note=\"signatures do not bind to full message content\""

	if dl.logger != nil {
		dl.logger.Println(warning)
	} else {
		log.Println(warning)
	}
}

// getMsgTypeName returns a human-readable type name for the message.
func getMsgTypeName(msg Message) string {
	// First try the Type() method which gives the canonical message type
	if msgType := msg.Type(); msgType != "" {
		return msgType
	}
	// Fallback to reflection for the Go type name
	return reflect.TypeOf(msg).String()
}

// SetDeprecationLoggingEnabled enables or disables deprecation warnings.
// This is useful for testing or for environments where warnings are not desired.
func SetDeprecationLoggingEnabled(enabled bool) {
	defaultDeprecationLogger.mu.Lock()
	defer defaultDeprecationLogger.mu.Unlock()
	defaultDeprecationLogger.enabled = enabled
}

// SetDeprecationWarningInterval sets the minimum time between warnings
// for the same message type. Use 0 to disable rate limiting (log every warning).
func SetDeprecationWarningInterval(interval time.Duration) {
	defaultDeprecationLogger.mu.Lock()
	defer defaultDeprecationLogger.mu.Unlock()
	defaultDeprecationLogger.warningInterval = interval
}

// SetDeprecationLogger sets a custom logger for deprecation warnings.
// Pass nil to use the default log package.
func SetDeprecationLogger(logger *log.Logger) {
	defaultDeprecationLogger.mu.Lock()
	defer defaultDeprecationLogger.mu.Unlock()
	defaultDeprecationLogger.logger = logger
}

// resetDeprecationLogger resets the deprecation logger state.
// This is primarily for testing purposes.
func resetDeprecationLogger() {
	defaultDeprecationLogger.mu.Lock()
	defer defaultDeprecationLogger.mu.Unlock()
	defaultDeprecationLogger.lastWarningTime = make(map[string]time.Time)
	defaultDeprecationLogger.warningInterval = 60 * time.Second
	defaultDeprecationLogger.enabled = true
	defaultDeprecationLogger.logger = nil
}
