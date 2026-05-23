package blacklist

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultRules(t *testing.T) {
	rules := DefaultRules()
	if len(rules) == 0 {
		t.Fatal("默认规则不应为空")
	}

	hasRoot := false
	hasWildcard := false
	for _, r := range rules {
		if r.User == "root" {
			hasRoot = true
		}
		if r.User == "*" {
			hasWildcard = true
		}
	}
	if !hasRoot {
		t.Error("默认规则应包含 root 用户规则")
	}
	if !hasWildcard {
		t.Error("默认规则应包含 * 全局规则")
	}
}

func TestLoadConfig_FileNotExist(t *testing.T) {
	cfg, err := LoadConfig()
	if err != nil {
		t.Logf("加载配置返回错误(预期内): %v", err)
	}
	if cfg == nil {
		t.Fatal("即使文件不存在也应返回配置")
	}
	if len(cfg.Rules) == 0 {
		t.Fatal("配置规则不应为空")
	}
}

func TestLoadConfig_FileExists(t *testing.T) {
	tmpDir := t.TempDir()
	owlDir := filepath.Join(tmpDir, ".owl")
	os.MkdirAll(owlDir, 0755)

	configContent := `
rules:
  - user: root
    patterns:
      - "rm "
      - "reboot"
  - user: "*"
    patterns:
      - "rm -rf /"
`
	configFile := filepath.Join(owlDir, "blacklist.yaml")
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if len(cfg.Rules) != 2 {
		t.Fatalf("期望 2 条规则，实际 %d 条", len(cfg.Rules))
	}
	if cfg.Rules[0].User != "root" {
		t.Errorf("第一条规则用户期望 root，实际 %s", cfg.Rules[0].User)
	}
	if len(cfg.Rules[0].Patterns) != 2 {
		t.Errorf("root 规则期望 2 个模式，实际 %d 个", len(cfg.Rules[0].Patterns))
	}
}

func TestSaveConfig(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	cfg := &Config{
		Rules: []Rule{
			{User: "root", Patterns: []string{"rm ", "reboot"}},
		},
	}

	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("保存配置失败: %v", err)
	}

	expectedPath := filepath.Join(tmpDir, ".owl", "blacklist.yaml")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Fatal("配置文件未创建")
	}
}

func TestCheck_RootHitsRm(t *testing.T) {
	checker := NewDefaultChecker()
	result := checker.Check("root", "rm -rf /var/log/test.log")
	if !result.Blocked {
		t.Fatal("root 用户执行 rm 应该命中黑名单")
	}
	if len(result.Matches) == 0 {
		t.Fatal("应该有匹配项")
	}
}

func TestCheck_NormalUserNoHit(t *testing.T) {
	checker := NewDefaultChecker()
	result := checker.Check("webuser", "rm ./test.log")
	if result.Blocked {
		t.Fatal("普通用户执行 rm 不应命中 root 专属规则")
	}
}

func TestCheck_WildcardHitsAll(t *testing.T) {
	checker := NewDefaultChecker()
	result := checker.Check("webuser", "rm -rf /")
	if !result.Blocked {
		t.Fatal("任意用户执行 rm -rf / 应该命中全局黑名单")
	}
	if len(result.Matches) == 0 {
		t.Fatal("应该有匹配项")
	}
}

func TestCheck_SafeCommand(t *testing.T) {
	checker := NewDefaultChecker()
	result := checker.Check("root", "ls -la /var/log")
	if result.Blocked {
		t.Fatal("安全命令不应命中黑名单")
	}
}

func TestCheck_EmptyCommand(t *testing.T) {
	checker := NewDefaultChecker()
	result := checker.Check("root", "")
	if result.Blocked {
		t.Fatal("空命令不应命中黑名单")
	}
}

func TestCheck_MultipleMatches(t *testing.T) {
	checker := NewDefaultChecker()
	result := checker.Check("root", "rm -rf /tmp && shutdown -h now")
	if !result.Blocked {
		t.Fatal("root 用户执行 rm + shutdown 应该命中多条")
	}
	if len(result.Matches) < 2 {
		t.Fatalf("期望至少 2 条匹配，实际 %d 条", len(result.Matches))
	}
}

func TestCheck_MultipleLines(t *testing.T) {
	checker := NewDefaultChecker()
	result := checker.Check("root", "ls -la\nrm -rf /tmp\nuptime")
	if !result.Blocked {
		t.Fatal("多行命令中包含 rm 应命中")
	}
}

func TestCheck_SemicolonSplit(t *testing.T) {
	checker := NewDefaultChecker()
	result := checker.Check("root", "ls -la; rm -rf /tmp")
	if !result.Blocked {
		t.Fatal("分号分隔的命令应正确分割并检测 rm")
	}
}

func TestCheck_PipeCommand(t *testing.T) {
	checker := NewDefaultChecker()
	result := checker.Check("root", "echo hello | rm -rf /tmp")
	if !result.Blocked {
		t.Fatal("管道后的 rm 应被检测")
	}
}

func TestCheck_QuotedCommand(t *testing.T) {
	checker := NewDefaultChecker()
	result := checker.Check("root", "echo 'rm -rf /'")
	if result.Blocked {
		t.Fatal("引号内的 rm 不应被视为危险命令行")
	}
}

func TestCheck_DoubleQuotedCommand(t *testing.T) {
	checker := NewDefaultChecker()
	result := checker.Check("root", `echo "rm -rf /"`)
	if result.Blocked {
		t.Fatal("双引号内的 rm 不应被视为危险命令行")
	}
}
