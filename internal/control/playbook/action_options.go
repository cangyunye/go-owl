package playbook

import (
	"time"

	"github.com/cangyunye/go-owl/internal/control/command"
	"github.com/cangyunye/go-owl/internal/ssh"
)

type ActionOptions struct {
	Timeout *TimeoutOption
	Retry   *RetryOption
}

type TimeoutOption struct {
	Connect time.Duration
	Command time.Duration
}

type RetryOption struct {
	Max        int
	Interval   time.Duration
	MaxInterval time.Duration
}

type PlaybookDefaults struct {
	TimeoutConfig *ssh.TimeoutConfig
	RetryConfig   *command.RetryConfig
}

func DefaultPlaybookDefaults() *PlaybookDefaults {
	return &PlaybookDefaults{
		TimeoutConfig: &ssh.TimeoutConfig{
			ConnectTimeout: 10 * time.Second,
			CommandTimeout: 5 * time.Minute,
		},
		RetryConfig: &command.RetryConfig{
			MaxRetries:    0,
			InitialInterval: 1 * time.Second,
			MaxInterval:   30 * time.Second,
		},
	}
}

func NewPlaybookDefaults(connectTimeout, commandTimeout time.Duration, retry, retryMax int, retryInterval, retryMaxInterval time.Duration) *PlaybookDefaults {
	defaults := DefaultPlaybookDefaults()

	if connectTimeout > 0 {
		defaults.TimeoutConfig.ConnectTimeout = connectTimeout
	}
	if commandTimeout > 0 {
		defaults.TimeoutConfig.CommandTimeout = commandTimeout
	}
	if retry > 0 {
		defaults.RetryConfig.MaxRetries = retry
	}
	if retryInterval > 0 {
		defaults.RetryConfig.InitialInterval = retryInterval
	}
	if retryMaxInterval > 0 {
		defaults.RetryConfig.MaxInterval = retryMaxInterval
	}

	return defaults
}

func (opts *ActionOptions) GetTimeout() time.Duration {
	if opts != nil && opts.Timeout != nil && opts.Timeout.Command > 0 {
		return opts.Timeout.Command
	}
	return 5 * time.Minute
}

func (opts *ActionOptions) GetConnectTimeout() time.Duration {
	if opts != nil && opts.Timeout != nil && opts.Timeout.Connect > 0 {
		return opts.Timeout.Connect
	}
	return 10 * time.Second
}

func (opts *ActionOptions) ShouldRetry() bool {
	if opts == nil || opts.Retry == nil {
		return false
	}
	return opts.Retry.Max > 0
}

func (opts *ActionOptions) GetRetryConfig() *command.RetryConfig {
	if opts == nil || opts.Retry == nil {
		return nil
	}
	return &command.RetryConfig{
		MaxRetries:       opts.Retry.Max,
		InitialInterval:  opts.Retry.Interval,
		MaxInterval:      opts.Retry.MaxInterval,
		EnableExponentialBackoff: true,
	}
}

func (opts *ActionOptions) GetTimeoutConfig() *ssh.TimeoutConfig {
	if opts == nil {
		return &ssh.TimeoutConfig{
			ConnectTimeout: 10 * time.Second,
			CommandTimeout: 5 * time.Minute,
		}
	}
	return &ssh.TimeoutConfig{
		ConnectTimeout: opts.GetConnectTimeout(),
		CommandTimeout: opts.GetTimeout(),
	}
}

func MergeActionOptions(taskOpts *ActionOptions, globalOpts *PlaybookDefaults) *ActionOptions {
	result := &ActionOptions{}

	if taskOpts != nil && taskOpts.Timeout != nil {
		result.Timeout = taskOpts.Timeout
	} else if globalOpts != nil && globalOpts.TimeoutConfig != nil {
		result.Timeout = &TimeoutOption{
			Connect: globalOpts.TimeoutConfig.ConnectTimeout,
			Command: globalOpts.TimeoutConfig.CommandTimeout,
		}
	} else {
		result.Timeout = &TimeoutOption{
			Connect: 10 * time.Second,
			Command: 5 * time.Minute,
		}
	}

	if taskOpts != nil && taskOpts.Retry != nil {
		result.Retry = taskOpts.Retry
	} else if globalOpts != nil && globalOpts.RetryConfig != nil && globalOpts.RetryConfig.MaxRetries > 0 {
		result.Retry = &RetryOption{
			Max:        globalOpts.RetryConfig.MaxRetries,
			Interval:   globalOpts.RetryConfig.InitialInterval,
			MaxInterval: globalOpts.RetryConfig.MaxInterval,
		}
	}

	return result
}

func (opts *TimeoutOption) ToSSHConfig() *ssh.TimeoutConfig {
	if opts == nil {
		return &ssh.TimeoutConfig{
			ConnectTimeout: 10 * time.Second,
			CommandTimeout: 5 * time.Minute,
		}
	}
	return &ssh.TimeoutConfig{
		ConnectTimeout: opts.Connect,
		CommandTimeout: opts.Command,
	}
}

func (opts *RetryOption) ToRetryConfig() *command.RetryConfig {
	if opts == nil {
		return nil
	}
	return &command.RetryConfig{
		MaxRetries:      opts.Max,
		InitialInterval: opts.Interval,
		MaxInterval:    opts.MaxInterval,
		EnableExponentialBackoff: true,
	}
}
