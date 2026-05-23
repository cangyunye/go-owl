package command

import (
	"errors"
	"testing"
	"time"
)

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()

	if cfg.MaxRetries != 3 {
		t.Errorf("expected MaxRetries 3, got %d", cfg.MaxRetries)
	}

	if cfg.InitialInterval != 1*time.Second {
		t.Errorf("expected InitialInterval 1s, got %v", cfg.InitialInterval)
	}

	if cfg.MaxInterval != 30*time.Second {
		t.Errorf("expected MaxInterval 30s, got %v", cfg.MaxInterval)
	}

	if cfg.BackoffMultiplier != 2.0 {
		t.Errorf("expected BackoffMultiplier 2.0, got %v", cfg.BackoffMultiplier)
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		config   *RetryConfig
		expected bool
	}{
		{
			name:     "connection refused",
			err:      errors.New("connection refused"),
			config:   nil,
			expected: true,
		},
		{
			name:     "timeout",
			err:      errors.New("command timeout"),
			config:   nil,
			expected: true,
		},
		{
			name:     "network unreachable",
			err:      errors.New("network is unreachable"),
			config:   nil,
			expected: true,
		},
		{
			name:     "custom retryable error",
			err:      errors.New("my custom error"),
			config:   &RetryConfig{RetryableErrors: []string{"custom error"}},
			expected: true,
		},
		{
			name:     "non-retryable error",
			err:      errors.New("permission denied"),
			config:   nil,
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			config:   nil,
			expected: false,
		},
		{
			name:     "i/o timeout",
			err:      errors.New("dial tcp 192.168.1.1:22: i/o timeout"),
			config:   nil,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRetryable(tt.err, tt.config)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestCalculateInterval(t *testing.T) {
	config := &RetryConfig{
		InitialInterval:         1 * time.Second,
		MaxInterval:             30 * time.Second,
		BackoffMultiplier:       2.0,
		EnableExponentialBackoff: true,
	}

	// 测试指数退避
	interval0 := calculateInterval(0, config)
	if interval0 < 1*time.Second || interval0 > 1100*time.Millisecond {
		t.Errorf("expected interval ~1s, got %v", interval0)
	}

	interval1 := calculateInterval(1, config)
	if interval1 < 2*time.Second || interval1 > 2200*time.Millisecond {
		t.Errorf("expected interval ~2s, got %v", interval1)
	}

	interval2 := calculateInterval(2, config)
	if interval2 < 4*time.Second || interval2 > 4400*time.Millisecond {
		t.Errorf("expected interval ~4s, got %v", interval2)
	}
}

func TestCalculateInterval_Linear(t *testing.T) {
	config := &RetryConfig{
		InitialInterval:         1 * time.Second,
		MaxInterval:             30 * time.Second,
		EnableExponentialBackoff: false,
	}

	interval0 := calculateInterval(0, config)
	if interval0 < 1*time.Second || interval0 > 1100*time.Millisecond {
		t.Errorf("expected interval ~1s, got %v", interval0)
	}

	interval1 := calculateInterval(1, config)
	if interval1 < 2*time.Second || interval1 > 2200*time.Millisecond {
		t.Errorf("expected interval ~2s, got %v", interval1)
	}
}

func TestRetryableError_Error(t *testing.T) {
	causeErr := errors.New("connection refused")
	err := &RetryableError{
		OriginalError: causeErr,
		RetryCount:    3,
	}

	expected := "retryable error after 3 attempts: connection refused"
	if err.Error() != expected {
		t.Errorf("expected error message '%s', got '%s'", expected, err.Error())
	}
}

func TestRetryResult_Success(t *testing.T) {
	result := RetryResult{
		CommandResult: CommandResult{Success: true},
		TotalAttempts: 2,
		Retried:       true,
	}

	if !result.Success {
		t.Error("expected success")
	}
	if result.TotalAttempts != 2 {
		t.Errorf("expected 2 attempts, got %d", result.TotalAttempts)
	}
	if !result.Retried {
		t.Error("expected retried to be true")
	}
}

func TestRetryResult_Failure(t *testing.T) {
	result := RetryResult{
		CommandResult: CommandResult{Success: false, Error: errors.New("timeout")},
		TotalAttempts: 3,
		FinalError:    errors.New("max retries exceeded"),
		Retried:       true,
	}

	if result.Success {
		t.Error("expected failure")
	}
	if result.FinalError == nil {
		t.Error("expected final error")
	}
}

