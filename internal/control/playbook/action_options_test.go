package playbook

import (
	"testing"
	"time"

	"github.com/cangyunye/go-owl/internal/ssh"
)

func TestDefaultPlaybookDefaults(t *testing.T) {
	defaults := DefaultPlaybookDefaults()

	if defaults.TimeoutConfig.ConnectTimeout != 10*time.Second {
		t.Errorf("expected ConnectTimeout 10s, got %v", defaults.TimeoutConfig.ConnectTimeout)
	}

	if defaults.TimeoutConfig.CommandTimeout != 5*time.Minute {
		t.Errorf("expected CommandTimeout 5m, got %v", defaults.TimeoutConfig.CommandTimeout)
	}
}

func TestNewPlaybookDefaults(t *testing.T) {
	defaults := NewPlaybookDefaults(
		5*time.Second,
		10*time.Minute,
		3,
		0,
		2*time.Second,
		60*time.Second,
	)

	if defaults.TimeoutConfig.ConnectTimeout != 5*time.Second {
		t.Errorf("expected ConnectTimeout 5s, got %v", defaults.TimeoutConfig.ConnectTimeout)
	}

	if defaults.TimeoutConfig.CommandTimeout != 10*time.Minute {
		t.Errorf("expected CommandTimeout 10m, got %v", defaults.TimeoutConfig.CommandTimeout)
	}
}

func TestActionOptions_GetTimeout(t *testing.T) {
	opts := &ActionOptions{
		Timeout: &TimeoutOption{
			Command: 30 * time.Second,
		},
	}

	if opts.GetTimeout() != 30*time.Second {
		t.Errorf("expected 30s, got %v", opts.GetTimeout())
	}

	nilOpts := (*ActionOptions)(nil)
	if nilOpts.GetTimeout() != 5*time.Minute {
		t.Errorf("expected default 5m, got %v", nilOpts.GetTimeout())
	}
}

func TestActionOptions_GetConnectTimeout(t *testing.T) {
	opts := &ActionOptions{
		Timeout: &TimeoutOption{
			Connect: 5 * time.Second,
		},
	}

	if opts.GetConnectTimeout() != 5*time.Second {
		t.Errorf("expected 5s, got %v", opts.GetConnectTimeout())
	}
}

func TestActionOptions_ShouldRetry(t *testing.T) {
	opts := &ActionOptions{
		Retry: &RetryOption{
			Max: 3,
		},
	}

	if !opts.ShouldRetry() {
		t.Error("expected ShouldRetry to return true")
	}

	opts.Retry.Max = 0
	if opts.ShouldRetry() {
		t.Error("expected ShouldRetry to return false when Max=0")
	}

	if (*ActionOptions)(nil).ShouldRetry() {
		t.Error("expected ShouldRetry to return false for nil")
	}
}

func TestActionOptions_GetRetryConfig(t *testing.T) {
	opts := &ActionOptions{
		Retry: &RetryOption{
			Max:         3,
			Interval:    2 * time.Second,
			MaxInterval: 30 * time.Second,
		},
	}

	cfg := opts.GetRetryConfig()
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}

	if cfg.MaxRetries != 3 {
		t.Errorf("expected MaxRetries 3, got %d", cfg.MaxRetries)
	}
}

func TestActionOptions_GetTimeoutConfig(t *testing.T) {
	opts := &ActionOptions{
		Timeout: &TimeoutOption{
			Connect: 5 * time.Second,
			Command: 30 * time.Second,
		},
	}

	cfg := opts.GetTimeoutConfig()
	if cfg.ConnectTimeout != 5*time.Second {
		t.Errorf("expected ConnectTimeout 5s, got %v", cfg.ConnectTimeout)
	}

	if cfg.CommandTimeout != 30*time.Second {
		t.Errorf("expected CommandTimeout 30s, got %v", cfg.CommandTimeout)
	}
}

func TestMergeActionOptions(t *testing.T) {
	globalOpts := &PlaybookDefaults{
		TimeoutConfig: &ssh.TimeoutConfig{
			ConnectTimeout: 10 * time.Second,
			CommandTimeout: 5 * time.Minute,
		},
	}

	t.Run("task overrides global", func(t *testing.T) {
		taskOpts := &ActionOptions{
			Timeout: &TimeoutOption{
				Connect: 5 * time.Second,
				Command: 10 * time.Minute,
			},
		}

		merged := MergeActionOptions(taskOpts, globalOpts)

		if merged.Timeout.Connect != 5*time.Second {
			t.Errorf("expected Connect 5s, got %v", merged.Timeout.Connect)
		}
		if merged.Timeout.Command != 10*time.Minute {
			t.Errorf("expected Command 10m, got %v", merged.Timeout.Command)
		}
	})

	t.Run("use global when task has no timeout", func(t *testing.T) {
		taskOpts := &ActionOptions{}

		merged := MergeActionOptions(taskOpts, globalOpts)

		if merged.Timeout.Connect != 10*time.Second {
			t.Errorf("expected Connect 10s, got %v", merged.Timeout.Connect)
		}
		if merged.Timeout.Command != 5*time.Minute {
			t.Errorf("expected Command 5m, got %v", merged.Timeout.Command)
		}
	})

	t.Run("use defaults when no global", func(t *testing.T) {
		taskOpts := &ActionOptions{}

		merged := MergeActionOptions(taskOpts, nil)

		if merged.Timeout.Connect != 10*time.Second {
			t.Errorf("expected Connect 10s, got %v", merged.Timeout.Connect)
		}
		if merged.Timeout.Command != 5*time.Minute {
			t.Errorf("expected Command 5m, got %v", merged.Timeout.Command)
		}
	})
}

func TestTimeoutOption_ToSSHConfig(t *testing.T) {
	opts := &TimeoutOption{
		Connect: 5 * time.Second,
		Command: 30 * time.Second,
	}

	cfg := opts.ToSSHConfig()

	if cfg.ConnectTimeout != 5*time.Second {
		t.Errorf("expected ConnectTimeout 5s, got %v", cfg.ConnectTimeout)
	}

	if cfg.CommandTimeout != 30*time.Second {
		t.Errorf("expected CommandTimeout 30s, got %v", cfg.CommandTimeout)
	}
}

func TestRetryOption_ToRetryConfig(t *testing.T) {
	opts := &RetryOption{
		Max:         5,
		Interval:   2 * time.Second,
		MaxInterval: 60 * time.Second,
	}

	cfg := opts.ToRetryConfig()

	if cfg.MaxRetries != 5 {
		t.Errorf("expected MaxRetries 5, got %d", cfg.MaxRetries)
	}

	if cfg.InitialInterval != 2*time.Second {
		t.Errorf("expected InitialInterval 2s, got %v", cfg.InitialInterval)
	}
}
