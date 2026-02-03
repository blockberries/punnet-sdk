package types

import (
	"bytes"
	"encoding/json"
	"log"
	"strings"
	"testing"
	"time"
)

// mockMessage is a test message that does NOT implement SignDocSerializable
type mockMessage struct {
	sender    AccountName
	recipient AccountName
	amount    uint64
}

func (m *mockMessage) Type() string {
	return "/test.mock.v1.MsgMock"
}

func (m *mockMessage) ValidateBasic() error {
	return nil
}

func (m *mockMessage) GetSigners() []AccountName {
	return []AccountName{m.sender}
}

// mockSerializableMessage implements SignDocSerializable
type mockSerializableMessage struct {
	sender    AccountName
	recipient AccountName
	amount    uint64
}

func (m *mockSerializableMessage) Type() string {
	return "/test.mock.v1.MsgSerializable"
}

func (m *mockSerializableMessage) ValidateBasic() error {
	return nil
}

func (m *mockSerializableMessage) GetSigners() []AccountName {
	return []AccountName{m.sender}
}

func (m *mockSerializableMessage) SignDocData() (json.RawMessage, error) {
	return json.Marshal(map[string]interface{}{
		"sender":    m.sender,
		"recipient": m.recipient,
		"amount":    m.amount,
	})
}

func TestSignersOnlyFallbackDeprecation_LogsWarning(t *testing.T) {
	// Capture log output
	var buf bytes.Buffer
	SetDeprecationLogger(log.New(&buf, "", 0))
	SetDeprecationWarningInterval(0) // Disable rate limiting for test
	defer resetDeprecationLogger()

	msg := &mockMessage{
		sender:    "alice",
		recipient: "bob",
		amount:    100,
	}

	SignersOnlyFallbackDeprecation(msg)

	output := buf.String()
	if !strings.Contains(output, "DEPRECATION WARNING") {
		t.Errorf("expected deprecation warning in output, got: %s", output)
	}
	if !strings.Contains(output, "/test.mock.v1.MsgMock") {
		t.Errorf("expected message type in output, got: %s", output)
	}
	if !strings.Contains(output, "SignDocSerializable") {
		t.Errorf("expected SignDocSerializable mention in output, got: %s", output)
	}
	if !strings.Contains(output, "signatures do not bind to full message content") {
		t.Errorf("expected security note in output, got: %s", output)
	}
}

func TestSignersOnlyFallbackDeprecation_RateLimiting(t *testing.T) {
	var buf bytes.Buffer
	SetDeprecationLogger(log.New(&buf, "", 0))
	SetDeprecationWarningInterval(100 * time.Millisecond)
	defer resetDeprecationLogger()

	msg := &mockMessage{
		sender:    "alice",
		recipient: "bob",
		amount:    100,
	}

	// First call should log
	SignersOnlyFallbackDeprecation(msg)
	firstOutput := buf.String()
	if !strings.Contains(firstOutput, "DEPRECATION WARNING") {
		t.Errorf("first call should log warning")
	}

	// Second call within rate limit should not log
	buf.Reset()
	SignersOnlyFallbackDeprecation(msg)
	secondOutput := buf.String()
	if secondOutput != "" {
		t.Errorf("second call within rate limit should not log, got: %s", secondOutput)
	}

	// Wait for rate limit to expire
	time.Sleep(150 * time.Millisecond)

	// Third call after rate limit should log
	buf.Reset()
	SignersOnlyFallbackDeprecation(msg)
	thirdOutput := buf.String()
	if !strings.Contains(thirdOutput, "DEPRECATION WARNING") {
		t.Errorf("call after rate limit expiry should log warning")
	}
}

func TestSignersOnlyFallbackDeprecation_DifferentMessageTypesNotRateLimited(t *testing.T) {
	var buf bytes.Buffer
	SetDeprecationLogger(log.New(&buf, "", 0))
	SetDeprecationWarningInterval(time.Hour) // Long rate limit
	defer resetDeprecationLogger()

	msg1 := &mockMessage{sender: "alice"}
	msg2 := &mockMessageTypeB{sender: "bob"}

	// First message type
	SignersOnlyFallbackDeprecation(msg1)
	if !strings.Contains(buf.String(), "/test.mock.v1.MsgMock") {
		t.Errorf("first message type should log")
	}

	// Second message type (different) should also log
	buf.Reset()
	SignersOnlyFallbackDeprecation(msg2)
	if !strings.Contains(buf.String(), "/test.mock.v1.MsgTypeB") {
		t.Errorf("different message type should log independently")
	}
}

// mockMessageTypeB is another test message type
type mockMessageTypeB struct {
	sender AccountName
}

func (m *mockMessageTypeB) Type() string {
	return "/test.mock.v1.MsgTypeB"
}

func (m *mockMessageTypeB) ValidateBasic() error {
	return nil
}

func (m *mockMessageTypeB) GetSigners() []AccountName {
	return []AccountName{m.sender}
}

func TestSignersOnlyFallbackDeprecation_DisableLogging(t *testing.T) {
	var buf bytes.Buffer
	SetDeprecationLogger(log.New(&buf, "", 0))
	SetDeprecationWarningInterval(0)
	SetDeprecationLoggingEnabled(false)
	defer resetDeprecationLogger()

	msg := &mockMessage{sender: "alice"}
	SignersOnlyFallbackDeprecation(msg)

	if buf.String() != "" {
		t.Errorf("disabled logging should produce no output, got: %s", buf.String())
	}
}

func TestConvertMessages_LogsDeprecationForNonSerializable(t *testing.T) {
	var buf bytes.Buffer
	SetDeprecationLogger(log.New(&buf, "", 0))
	SetDeprecationWarningInterval(0)
	defer resetDeprecationLogger()

	// Non-serializable message should trigger deprecation warning
	msgs := []Message{
		&mockMessage{sender: "alice", recipient: "bob", amount: 100},
	}

	_, err := convertMessages(msgs)
	if err != nil {
		t.Fatalf("convertMessages failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "DEPRECATION WARNING") {
		t.Errorf("convertMessages should log deprecation for non-SignDocSerializable message")
	}
}

func TestConvertMessages_NoDeprecationForSerializable(t *testing.T) {
	var buf bytes.Buffer
	SetDeprecationLogger(log.New(&buf, "", 0))
	SetDeprecationWarningInterval(0)
	defer resetDeprecationLogger()

	// Serializable message should NOT trigger deprecation warning
	msgs := []Message{
		&mockSerializableMessage{sender: "alice", recipient: "bob", amount: 100},
	}

	_, err := convertMessages(msgs)
	if err != nil {
		t.Fatalf("convertMessages failed: %v", err)
	}

	output := buf.String()
	if strings.Contains(output, "DEPRECATION WARNING") {
		t.Errorf("convertMessages should not log deprecation for SignDocSerializable message, got: %s", output)
	}
}
