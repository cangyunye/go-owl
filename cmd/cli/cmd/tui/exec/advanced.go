package exec

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"

	"github.com/cangyunye/go-owl/internal/control/command"
	"github.com/cangyunye/go-owl/internal/ssh"
)

type FieldKind int

const (
	KindText FieldKind = iota
	KindBool
)

type AdvancedField struct {
	key     string
	label   string
	kind    FieldKind
	input   textinput.Model
	checked bool
}

type AdvancedForm struct {
	fields []*AdvancedField
	cursor int
	error  string
}

func newAdvancedForm() *AdvancedForm {
	f := &AdvancedForm{}
	specs := []struct {
		key, label, def string
		kind            FieldKind
		checked         bool
	}{
		{"timeout", "timeout", "60s", KindText, false},
		{"connect-timeout", "connect-timeout", "10s", KindText, false},
		{"command-timeout", "command-timeout", "30s", KindText, false},
		{"parallel", "parallel", "", KindBool, true},
		{"serial", "serial", "", KindBool, false},
		{"retry", "retry", "3", KindText, false},
		{"retry-interval", "retry-interval", "1s", KindText, false},
		{"retry-max-interval", "retry-max-interval", "30s", KindText, false},
		{"no-retry", "no-retry", "", KindBool, false},
		{"async", "async", "", KindBool, false},
		{"async-timeout", "async-timeout", "1h", KindText, false},
		{"async-poll-interval", "async-poll-interval", "10s", KindText, false},
		{"async-max-poll-count", "async-max-poll-count", "3600", KindText, false},
		{"async-remote-dir", "async-remote-dir", "/tmp/owl", KindText, false},
		{"status", "status", "", KindText, false},
		{"no-color", "no-color", "", KindBool, false},
		{"debug", "debug", "", KindBool, false},
		{"force", "force", "", KindBool, false},
		{"sync-nodes", "sync-nodes", "", KindBool, false},
		{"silent", "silent", "", KindBool, false},
	}
	for _, s := range specs {
		ti := textinput.New()
		ti.SetValue(s.def)
		ti.Width = 20
		ti.CharLimit = 64
		ti.Blur()
		f.fields = append(f.fields, &AdvancedField{key: s.key, label: s.label, kind: s.kind, input: ti, checked: s.checked})
	}
	return f
}

func (f *AdvancedForm) value(key string) string {
	for _, fd := range f.fields {
		if fd.key == key {
			return strings.TrimSpace(fd.input.Value())
		}
	}
	return ""
}

func (f *AdvancedForm) isOn(key string) bool {
	for _, fd := range f.fields {
		if fd.key == key {
			return fd.checked
		}
	}
	return false
}

func (f *AdvancedForm) move(d int) {
	f.cursor = (f.cursor + d + len(f.fields)) % len(f.fields)
}

func (f *AdvancedForm) toggle(i int) {
	f.fields[i].checked = !f.fields[i].checked
}

func (f *AdvancedForm) buildOpts() (*command.ExecuteOptions, error) {
	for _, key := range []string{"timeout", "connect-timeout", "command-timeout", "retry-interval", "retry-max-interval"} {
		if v := f.value(key); v != "" {
			if _, err := time.ParseDuration(v); err != nil {
				return nil, fmt.Errorf("%s 无效: %s", key, v)
			}
		}
	}
	opts := &command.ExecuteOptions{Parallel: f.isOn("parallel") && !f.isOn("serial")}
	if v := f.value("timeout"); v != "" {
		d, _ := time.ParseDuration(v)
		opts.Timeout = d
	}
	opts.TimeoutConfig = &ssh.TimeoutConfig{
		ConnectTimeout: mustDuration(f.value("connect-timeout")),
		CommandTimeout: mustDuration(f.value("command-timeout")),
	}
	if retry := f.value("retry"); retry != "" && !f.isOn("no-retry") {
		n, err := strconv.Atoi(retry)
		if err != nil {
			return nil, fmt.Errorf("retry 必须是整数: %s", retry)
		}
		opts.RetryConfig = &command.RetryConfig{
			MaxRetries:      n,
			InitialInterval: mustDuration(f.value("retry-interval")),
			MaxInterval:     mustDuration(f.value("retry-max-interval")),
		}
	}
	return opts, nil
}

func mustDuration(s string) time.Duration {
	d, _ := time.ParseDuration(s)
	return d
}

func advancedSummary(f *AdvancedForm) string {
	parts := []string{}
	if v := f.value("timeout"); v != "" {
		parts = append(parts, "timeout="+v)
	}
	if f.isOn("serial") {
		parts = append(parts, "串行")
	} else {
		parts = append(parts, "并行")
	}
	if v := f.value("retry"); v != "" && !f.isOn("no-retry") {
		parts = append(parts, "retry="+v)
	}
	if f.isOn("async") {
		parts = append(parts, "async")
	}
	if f.isOn("force") {
		parts = append(parts, "force")
	}
	if f.isOn("debug") {
		parts = append(parts, "debug")
	}
	if f.isOn("silent") {
		parts = append(parts, "silent")
	}
	return strings.Join(parts, " ")
}
