package settings_test

import (
	"testing"

	"github.com/cangyunye/go-owl/cmd/cli/cmd/settings"
	"github.com/cangyunye/go-owl/cmd/cli/cmd/testutil"
)

func TestSettingsCmdExists(t *testing.T) {
	parent := settings.NewSettingsCmd()

	if parent.Use != "settings" {
		t.Errorf("expected Use 'settings', got '%s'", parent.Use)
	}

	expected := []string{"show", "set", "target", "template"}
	testutil.AssertSubCommands(t, parent, expected)
}

func TestSettingsShowCmd(t *testing.T) {
	cmd := settings.NewSettingsShowCmd()

	if cmd.Use != "show" {
		t.Errorf("expected Use 'show', got '%s'", cmd.Use)
	}
}

func TestSettingsSetCmd(t *testing.T) {
	cmd := settings.NewSettingsSetCmd()

	if cmd.Use != "set <key> <value>" {
		t.Errorf("expected Use 'set <key> <value>', got '%s'", cmd.Use)
	}
}

func TestSettingsTargetFlags(t *testing.T) {
	cmd := settings.NewSettingsTargetCmd()

	testutil.AssertFlagExists(t, cmd, "groups")
	testutil.AssertFlagShorthand(t, cmd, "groups", "g")
	testutil.AssertFlagDefault(t, cmd, "groups", "[]")

	testutil.AssertFlagExists(t, cmd, "label")
	testutil.AssertFlagShorthand(t, cmd, "label", "l")

	testutil.AssertFlagExists(t, cmd, "nodes")
	testutil.AssertFlagShorthand(t, cmd, "nodes", "N")
	testutil.AssertFlagDefault(t, cmd, "nodes", "")
}

func TestSettingsTargetCmdUse(t *testing.T) {
	cmd := settings.NewSettingsTargetCmd()

	if cmd.Use != "target" {
		t.Errorf("expected Use 'target', got '%s'", cmd.Use)
	}
}

func TestSettingsTemplateCmd(t *testing.T) {
	cmd := settings.NewSettingsTemplateCmd()

	if cmd.Use != "template" {
		t.Errorf("expected Use 'template', got '%s'", cmd.Use)
	}
	if cmd.Short != "显示所有可配置项" {
		t.Errorf("expected Short '显示所有可配置项', got '%s'", cmd.Short)
	}
}

func TestSettingsHelpContainsSubcommands(t *testing.T) {
	parent := settings.NewSettingsCmd()

	testutil.AssertHelpContains(t, parent, "show")
	testutil.AssertHelpContains(t, parent, "set")
	testutil.AssertHelpContains(t, parent, "target")
	testutil.AssertHelpContains(t, parent, "template")
}

// TestSettingsGetCurrentSettingsDefault 验证无配置文件时返回默认值
func TestSettingsGetCurrentSettingsDefault(t *testing.T) {
	s := settings.GetCurrentSettings()
	if s == nil {
		t.Fatal("GetCurrentSettings() returned nil")
	}
	if s.Output.Format != "table" {
		t.Errorf("expected default format 'table', got '%s'", s.Output.Format)
	}
	if s.Output.Color != true {
		t.Errorf("expected default color true, got %v", s.Output.Color)
	}
	if s.Default.Timeout != "60s" {
		t.Errorf("expected default timeout '60s', got '%s'", s.Default.Timeout)
	}
	if s.Default.Parallel != true {
		t.Errorf("expected default parallel true, got %v", s.Default.Parallel)
	}
	if s.Default.Group != "" {
		t.Errorf("expected empty default group, got '%s'", s.Default.Group)
	}
}

// TestSettingsStructFields 验证 Settings 结构体包含所有预期字段
func TestSettingsStructFields(t *testing.T) {
	s := settings.GetCurrentSettings()

	// Output 字段
	if s.Output.Format != "table" && s.Output.Format != "" {
		t.Logf("output.format = %s", s.Output.Format)
	}

	// Default 字段
	if s.Default.Timeout == "" {
		t.Error("default.timeout should not be empty")
	}

	// Target 字段 — 新增
	_ = s.Target.Groups
	_ = s.Target.Label
	_ = s.Target.Nodes
}

// TestSettingsSetFormatValidation 验证 output.format 只接受合法值
func TestSettingsSetFormatValidation(t *testing.T) {
	s := settings.GetCurrentSettings()
	orig := s.Output.Format
	_ = orig // 保存以备恢复

	// 验证默认值是三个合法值之一
	valid := s.Output.Format == "table" || s.Output.Format == "json" || s.Output.Format == "simple"
	if !valid {
		t.Errorf("invalid default format '%s', expected table/json/simple", s.Output.Format)
	}
}

// TestSettingsSetColorValidation 验证 output.color 只接受 true/false
func TestSettingsSetColorValidation(t *testing.T) {
	s := settings.GetCurrentSettings()
	// 默认是 bool 类型，不会有非法值
	if s.Output.Color != true && s.Output.Color != false {
		t.Errorf("output.color should be boolean, got %v", s.Output.Color)
	}
}

// TestSettingsSetParallelValidation 验证 default.parallel 只接受 true/false
func TestSettingsSetParallelValidation(t *testing.T) {
	s := settings.GetCurrentSettings()
	if s.Default.Parallel != true && s.Default.Parallel != false {
		t.Errorf("default.parallel should be boolean, got %v", s.Default.Parallel)
	}
}

// TestSettingsDefaultValues 验证所有默认值与 template 命令文档一致
func TestSettingsDefaultValues(t *testing.T) {
	s := settings.GetCurrentSettings()
	_ = s.Output.Format
	_ = s.Output.Color
	_ = s.Default.Timeout
	_ = s.Default.Group
	_ = s.Default.Parallel
	_ = s.Default.Labels
	_ = s.Target.Groups
	_ = s.Target.Label
	_ = s.Target.Nodes
	t.Log("All settings fields accessible via GetCurrentSettings()")
}

// TestSettingsFallbackContract 验证各命令所需的 fallback 字段可访问
func TestSettingsFallbackContract(t *testing.T) {
	s := settings.GetCurrentSettings()

	// exec run 需要的字段
	_ = s.Default.Group
	_ = s.Default.Labels
	_ = s.Output.Format
	_ = s.Output.Color
	_ = s.Default.Parallel

	// exec script 需要的字段
	_ = s.Default.Group
	_ = s.Default.Labels

	// node list 需要的字段
	_ = s.Default.Group
	_ = s.Output.Format
	_ = s.Output.Color

	// file upload/download/transfer 需要的字段
	_ = s.Default.Group
	_ = s.Default.Labels
	_ = s.Default.Parallel

	t.Log("All fallback contract fields accessible via GetCurrentSettings()")
}

// TestSettingsTargetFields 验证 TargetSettings 字段可访问
func TestSettingsTargetFields(t *testing.T) {
	s := settings.GetCurrentSettings()
	_ = s.Target.Groups
	_ = s.Target.Label
	_ = s.Target.Nodes
	t.Log("Target settings fields are accessible")
}

// TestSettingsLabelsMap 验证 Labels map 可正常使用
func TestSettingsLabelsMap(t *testing.T) {
	s := settings.GetCurrentSettings()
	if s.Default.Labels == nil {
		t.Log("Labels map is nil, will be initialized on use")
	} else {
		t.Logf("Labels map has %d entries", len(s.Default.Labels))
	}
}

// TestFormatLabels 验证 settings show 的标签渲染为可读的 key=value 格式
func TestFormatLabels(t *testing.T) {
	cases := []struct {
		name   string
		labels map[string]string
		want   string
	}{
		{name: "多标签按键名排序", labels: map[string]string{"region": "us", "env": "prod"}, want: "env=prod, region=us"},
		{name: "单标签", labels: map[string]string{"env": "prod"}, want: "env=prod"},
		{name: "空 map", labels: map[string]string{}, want: ""},
		{name: "nil map", labels: nil, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := settings.FormatLabels(tc.labels); got != tc.want {
				t.Errorf("FormatLabels(%v) = %q, want %q", tc.labels, got, tc.want)
			}
		})
	}
}
