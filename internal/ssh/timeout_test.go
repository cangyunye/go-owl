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