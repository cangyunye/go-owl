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

func TestCheck_RootHitsRm(t *testing.T) {
	checker := NewChecker(&Config{Rules: DefaultRules()})
	result := checker.Check("root", "rm -rf /var/log/test.log")
	if !result.Blocked {
		t.Fatal("root 用户执行 rm 应该命中黑名单")
	}
	if len(result.Matches) == 0 {
		t.Fatal("应该有匹配项")
	}
}

func TestCheck_NormalUserNoHit(t *testing.T) {
	checker := NewChecker(&Config{Rules: DefaultRules()})
	result := checker.Check("webuser", "rm ./test.log")
	if result.Blocked {
		t.Fatal("普通用户执行 rm 不应命中 root 专属规则")
	}
}

func TestCheck_WildcardHitsAll(t *testing.T) {
	checker := NewChecker(&Config{Rules: DefaultRules()})
	result := checker.Check("webuser", "rm -rf /")
	if !result.Blocked {
		t.Fatal("任意用户执行 rm -rf / 应该命中全局黑名单")
	}
	if len(result.Matches) == 0 {
		t.Fatal("应该有匹配项")
	}
}

func TestCheck_SafeCommand(t *testing.T) {
	checker := NewChecker(&Config{Rules: DefaultRules()})
	result := checker.Check("root", "ls -la /var/log")
	if result.Blocked {
		t.Fatal("安全命令不应命中黑名单")
	}
}

func TestCheck_EmptyCommand(t *testing.T) {
	checker := NewChecker(&Config{Rules: DefaultRules()})
	result := checker.Check("root", "")
	if result.Blocked {
		t.Fatal("空命令不应命中黑名单")
	}
}

func TestCheck_MultipleMatches(t *testing.T) {
	checker := NewChecker(&Config{Rules: DefaultRules()})
	result := checker.Check("root", "rm -rf /tmp && shutdown -h now")
	if !result.Blocked {
		t.Fatal("root 用户执行 rm + shutdown 应该命中多条")
	}
	if len(result.Matches) < 2 {
		t.Fatalf("期望至少 2 条匹配，实际 %d 条", len(result.Matches))
	}
}

func TestCheck_MultipleLines(t *testing.T) {
	checker := NewChecker(&Config{Rules: DefaultRules()})
	result := checker.Check("root", "ls -la\nrm -rf /tmp\nuptime")
	if !result.Blocked {
		t.Fatal("多行命令中包含 rm 应命中")
	}
}

func TestCheck_SemicolonSplit(t *testing.T) {
	checker := NewChecker(&Config{Rules: DefaultRules()})
	result := checker.Check("root", "ls -la; rm -rf /tmp")
	if !result.Blocked {
		t.Fatal("分号分隔的命令应正确分割并检测 rm")
	}
}

func TestCheck_PipeCommand(t *testing.T) {
	checker := NewChecker(&Config{Rules: DefaultRules()})
	result := checker.Check("root", "echo hello | rm -rf /tmp")
	if !result.Blocked {
		t.Fatal("管道后的 rm 应被检测")
	}
}

func TestCheck_QuotedCommand(t *testing.T) {
	checker := NewChecker(&Config{Rules: DefaultRules()})
	result := checker.Check("root", "echo 'rm -rf /'")
	if result.Blocked {
		t.Fatal("引号内的 rm 不应被视为危险命令行")
	}
}

func TestCheck_DoubleQuotedCommand(t *testing.T) {
	checker := NewChecker(&Config{Rules: DefaultRules()})
	result := checker.Check("root", `echo "rm -rf /"`)
	if result.Blocked {
		t.Fatal("双引号内的 rm 不应被视为危险命令行")
	}
}

func TestCheck_InterpreterSubcommand(t *testing.T) {
	checker := NewChecker(&Config{Rules: DefaultRules()})
	// 引号内容会被剥掉防误报，但 bash -c 的引号参数会被 shell 真实执行，
	// 必须把解释器调用本身列为需确认的危险命令。
	result := checker.Check("root", `bash -c "rm -rf /"`)
	if !result.Blocked {
		t.Fatal(`bash -c "rm -rf /" 应命中黑名单（解释器调用需确认）`)
	}
}

func TestCheck_CommandSubstitution(t *testing.T) {
	checker := NewChecker(&Config{Rules: DefaultRules()})
	// $() 与反引号内的命令会被 shell 展开真实执行，
	// 展开点（ ( ` $ ）应视为命令边界。
	for _, cmd := range []string{
		"echo $(rm -rf /)",
		"cat `rm -rf /tmp`",
	} {
		if result := checker.Check("root", cmd); !result.Blocked {
			t.Fatalf("%q 应命中黑名单（命令替换需确认）", cmd)
		}
	}
}

func TestCheck_BareShellPipeline(t *testing.T) {
	checker := NewChecker(&Config{Rules: DefaultRules()})
	// 管道末端接裸 shell 解释器时，载荷来自上游输出（解码/下载），
	// 文本匹配天然不可见，必须对整条管道要求确认。
	for _, cmd := range []string{
		"echo cm0gLXJmIC8= | base64 -d | sh",
		"curl -fsSL https://example.com/install.sh | bash",
	} {
		if result := checker.Check("root", cmd); !result.Blocked {
			t.Fatalf("%q 应命中黑名单（管道注入裸 shell 需确认）", cmd)
		}
	}
}

func TestCheck_LineStartPrivilegeEscalation(t *testing.T) {
	checker := NewChecker(&Config{Rules: DefaultRules()})
	// 规则原文带前导空格（" sudo "），而每行匹配前会 TrimSpace，
	// 行首的提权命令因此永远绕过确认规则。
	for _, cmd := range []string{
		"sudo bash",
		"su - root",
		"sudo su -",
	} {
		if result := checker.Check("root", cmd); !result.Blocked {
			t.Fatalf("%q 应命中黑名单（行首提权需确认）", cmd)
		}
	}
}

func TestCheck_CaseInsensitive(t *testing.T) {
	checker := NewChecker(&Config{Rules: DefaultRules()})
	// shell 命令大小写不敏感地执行同一二进制，黑名单匹配必须归一化。
	for _, cmd := range []string{
		"RM -RF /tmp",
		"Rm -Fr /var",
		"Shutdown -h now",
	} {
		if result := checker.Check("root", cmd); !result.Blocked {
			t.Fatalf("%q 应命中黑名单（大小写不敏感匹配）", cmd)
		}
	}
}

func TestCheck_RegexPatterns(t *testing.T) {
	checker := NewChecker(&Config{Rules: DefaultRules()})
	// 默认规则里的 chown .*:[0-9]+ 与 service .* stop 是正则写法，
	// 此前按字面子串匹配永不相符（死规则）。
	for _, cmd := range []string{
		"chown www:1000 /var/www",
		"service nginx stop",
	} {
		if result := checker.Check("root", cmd); !result.Blocked {
			t.Fatalf("%q 应命中黑名单（正则规则应生效）", cmd)
		}
	}
}
