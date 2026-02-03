package types

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// DEPRECATION LOGGING TESTS
// =============================================================================

// captureLogger captures log output for testing.
// Implements DeprecationLogger interface.
type captureLogger struct {
	mu       sync.Mutex
	warnings []string
}

func newCaptureLogger() *captureLogger {
	return &captureLogger{
		warnings: make([]string, 0),
	}
}

func (c *captureLogger) Warn(msg string, keyvals ...interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var b bytes.Buffer
	b.WriteString(msg)
	for i := 0; i < len(keyvals); i += 2 {
		if i+1 < len(keyvals) {
			b.WriteString(" ")
			b.WriteString(keyvals[i].(string))
			b.WriteString("=")
			switch v := keyvals[i+1].(type) {
			case string:
				b.WriteString(v)
			default:
				b.WriteString("(value)")
			}
		}
	}
	c.warnings = append(c.warnings, b.String())
}

func (c *captureLogger) getWarnings() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]string, len(c.warnings))
	copy(result, c.warnings)
	return result
}

// =============================================================================
// TESTS
// =============================================================================

func TestDeprecationLogger_RateLimiting(t *testing.T) {
	// INVARIANT: Each unique message type is logged at most once
	defer ResetDeprecationLogger()

	logger := newCaptureLogger()
	SetDeprecationLogger(logger)

	// Log same type multiple times
	warnSignersOnlyFallback("/punnet.bank.v1.MsgSend")
	warnSignersOnlyFallback("/punnet.bank.v1.MsgSend")
	warnSignersOnlyFallback("/punnet.bank.v1.MsgSend")

	warnings := logger.getWarnings()
	assert.Len(t, warnings, 1, "same message type should only be logged once")
	assert.Contains(t, warnings[0], "DEPRECATION")
	assert.Contains(t, warnings[0], "msg_type=/punnet.bank.v1.MsgSend")
}

func TestDeprecationLogger_DifferentMessageTypes(t *testing.T) {
	// INVARIANT: Different message types each get their own warning
	defer ResetDeprecationLogger()

	logger := newCaptureLogger()
	SetDeprecationLogger(logger)

	warnSignersOnlyFallback("/punnet.bank.v1.MsgSend")
	warnSignersOnlyFallback("/punnet.staking.v1.MsgDelegate")
	warnSignersOnlyFallback("/punnet.gov.v1.MsgVote")

	warnings := logger.getWarnings()
	assert.Len(t, warnings, 3, "different message types should each be logged")

	// Verify each type is present
	combined := strings.Join(warnings, "\n")
	assert.Contains(t, combined, "MsgSend")
	assert.Contains(t, combined, "MsgDelegate")
	assert.Contains(t, combined, "MsgVote")
}

func TestDeprecationLogger_WarningContent(t *testing.T) {
	// INVARIANT: Warning contains required information
	defer ResetDeprecationLogger()

	logger := newCaptureLogger()
	SetDeprecationLogger(logger)

	warnSignersOnlyFallback("/punnet.test.v1.TestMsg")

	warnings := logger.getWarnings()
	require.Len(t, warnings, 1)

	warning := warnings[0]
	assert.Contains(t, warning, "DEPRECATION")
	assert.Contains(t, warning, "SignDocSerializable")
	assert.Contains(t, warning, "signers-only")
	assert.Contains(t, warning, "msg_type=/punnet.test.v1.TestMsg")
	assert.Contains(t, warning, "security_note=")
}

func TestDeprecationLogger_DefaultIsNop(t *testing.T) {
	// INVARIANT: Default logger is no-op (does not panic or produce output)
	defer ResetDeprecationLogger()

	// Reset to default state
	ResetDeprecationLogger()

	// This should not panic and should not produce any output
	warnSignersOnlyFallback("/punnet.test.v1.TestMsg")
}

func TestDeprecationLogger_NilLoggerUseNop(t *testing.T) {
	// INVARIANT: Setting nil logger uses no-op logger (no panic)
	defer ResetDeprecationLogger()

	SetDeprecationLogger(nil)

	// This should not panic
	warnSignersOnlyFallback("/punnet.test.v1.TestMsg")
}

func TestDeprecationLogger_ConcurrentAccess(t *testing.T) {
	// INVARIANT: Concurrent calls are safe and each type is logged at most once
	defer ResetDeprecationLogger()

	logger := newCaptureLogger()
	SetDeprecationLogger(logger)

	var wg sync.WaitGroup
	msgTypes := []string{
		"/punnet.bank.v1.MsgSend",
		"/punnet.staking.v1.MsgDelegate",
		"/punnet.gov.v1.MsgVote",
	}

	// Launch multiple goroutines that each call the warning multiple times
	for i := 0; i < 10; i++ {
		for _, msgType := range msgTypes {
			wg.Add(1)
			go func(mt string) {
				defer wg.Done()
				warnSignersOnlyFallback(mt)
			}(msgType)
		}
	}

	wg.Wait()

	warnings := logger.getWarnings()
	// Each message type should be logged exactly once despite concurrent calls
	assert.Len(t, warnings, 3, "each message type should be logged exactly once")
}

func TestConvertMessages_TriggersDeprecationWarning(t *testing.T) {
	// INVARIANT: Using a message without SignDocSerializable triggers warning
	defer ResetDeprecationLogger()

	logger := newCaptureLogger()
	SetDeprecationLogger(logger)

	// testMessage does not implement SignDocSerializable
	msg := &testMessage{
		MsgType: "/punnet.test.v1.DeprecatedMsg",
		Signers: []AccountName{"alice"},
	}

	_, err := convertMessages([]Message{msg})
	require.NoError(t, err)

	warnings := logger.getWarnings()
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "/punnet.test.v1.DeprecatedMsg")
}

func TestConvertMessages_NoWarningForSignDocSerializable(t *testing.T) {
	// INVARIANT: Messages implementing SignDocSerializable do not trigger warning
	defer ResetDeprecationLogger()

	logger := newCaptureLogger()
	SetDeprecationLogger(logger)

	msg := &serializableMessage{
		MsgType: "/punnet.bank.v1.MsgSend",
		Signers: []AccountName{"alice"},
		From:    "alice",
		To:      "bob",
		Amount:  1000,
		Denom:   "uatom",
	}

	_, err := convertMessages([]Message{msg})
	require.NoError(t, err)

	warnings := logger.getWarnings()
	assert.Len(t, warnings, 0, "SignDocSerializable messages should not trigger warning")
}

func TestResetDeprecationLogger(t *testing.T) {
	// INVARIANT: Reset clears rate-limiting state
	defer ResetDeprecationLogger()

	logger := newCaptureLogger()
	SetDeprecationLogger(logger)

	warnSignersOnlyFallback("/punnet.test.v1.TestMsg")
	assert.Len(t, logger.getWarnings(), 1)

	// Reset and set new logger
	ResetDeprecationLogger()

	logger2 := newCaptureLogger()
	SetDeprecationLogger(logger2)

	// Same message type should be logged again after reset
	warnSignersOnlyFallback("/punnet.test.v1.TestMsg")
	assert.Len(t, logger2.getWarnings(), 1, "reset should clear rate-limiting state")
}
