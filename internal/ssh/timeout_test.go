package ssh

import (
	"errors"
	"testing"
	"time"
)

func TestDefaultTimeoutConfig(t *testing.T) {
	cfg := DefaultTimeoutConfig()

	if cfg.ConnectTimeout != 10*time.Second {
		t.Errorf("expected ConnectTimeout 10s, got %v", cfg.ConnectTimeout)
	}

	if cfg.CommandTimeout != 30*time.Second {
		t.Errorf("expected CommandTimeout 30s, got %v", cfg.CommandTimeout)
	}
}

func TestTimeoutError_Error(t *testing.T) {
	causeErr := errors.New("connection refused")
	err := &TimeoutError{
		Type:    TimeoutConnect,
		NodeID:  "test-node",
		Timeout: 5 * time.Second,
		Cause:   causeErr,
	}

	expected := "connect timeout after 5s for node test-node"
	if err.Error() != expected {
		t.Errorf("expected error message '%s', got '%s'", expected, err.Error())
	}
}

func TestTimeoutError_Unwrap(t *testing.T) {
	causeErr := errors.New("connection refused")
	err := &TimeoutError{
		Type:    TimeoutConnect,
		NodeID:  "test-node",
		Timeout: 5 * time.Second,
		Cause:   causeErr,
	}

	unwrapped := errors.Unwrap(err)
	if unwrapped != causeErr {
		t.Errorf("expected unwrapped error to be original cause")
	}
}

func TestIsTimeoutError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "timeout error",
			err:      &TimeoutError{Type: TimeoutConnect, NodeID: "test"},
			expected: true,
		},
		{
			name:     "regular error",
			err:      errors.New("some error"),
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsTimeoutError(tt.err)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestGetTimeoutType(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected TimeoutType
		ok       bool
	}{
		{
			name:     "connect timeout",
			err:      &TimeoutError{Type: TimeoutConnect, NodeID: "test"},
			expected: TimeoutConnect,
			ok:       true,
		},
		{
			name:     "command timeout",
			err:      &TimeoutError{Type: TimeoutCommand, NodeID: "test"},
			expected: TimeoutCommand,
			ok:       true,
		},
		{
			name:     "regular error",
			err:      errors.New("some error"),
			expected: "",
			ok:       false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: "",
			ok:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ok := GetTimeoutType(tt.err)
			if ok != tt.ok {
				t.Errorf("expected ok %v, got %v", tt.ok, ok)
			}
			if result != tt.expected {
				t.Errorf("expected type %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestTimeoutType_Constants(t *testing.T) {
	if TimeoutConnect != "connect" {
		t.Errorf("expected TimeoutConnect 'connect', got '%s'", TimeoutConnect)
	}
	if TimeoutCommand != "command" {
		t.Errorf("expected TimeoutCommand 'command', got '%s'", TimeoutCommand)
	}
}

func TestTimeoutError_NoCause(t *testing.T) {
	err := &TimeoutError{
		Type:    TimeoutCommand,
		NodeID:  "node-1",
		Timeout: 30 * time.Second,
	}

	expectedContains := []string{"command", "timeout", "30s", "node-1"}
	msg := err.Error()
	for _, s := range expectedContains {
		if !containsString(msg, s) {
			t.Errorf("expected error message to contain '%s', got: %s", s, msg)
		}
	}
}

func TestTimeoutError_UnwrapNilCause(t *testing.T) {
	err := &TimeoutError{
		Type:    TimeoutConnect,
		NodeID:  "node-1",
		Timeout: 5 * time.Second,
	}

	unwrapped := errors.Unwrap(err)
	if unwrapped != nil {
		t.Errorf("expected nil unwrapped, got %v", unwrapped)
	}
}

func TestIsTimeoutError_Wrapped(t *testing.T) {
	timeoutErr := &TimeoutError{Type: TimeoutConnect, NodeID: "test"}
	wrapped := errors.New("wrapped: " + timeoutErr.Error())

	if IsTimeoutError(wrapped) {
		t.Error("expected wrapped non-TimeoutError to not be detected as timeout")
	}
}

func containsString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}