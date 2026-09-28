package blacklist

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// Web 设置页编辑 ~/.owl/blacklist.yaml 后需热重载：进程内所有持有
// 共享检查器的 handler 必须立即看到新规则，无需重启。

func writeHomeConfig(t *testing.T, content string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".owl"), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".owl", "blacklist.yaml")
	if content != "" {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestConfigPath_UnderHomeOwl(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if got, want := ConfigPath(), filepath.Join(home, ".owl", "blacklist.yaml"); got != want {
		t.Fatalf("ConfigPath = %q, want %q", got, want)
	}
}

func TestShared_ReloadSwapsRulesInPlace(t *testing.T) {
	writeHomeConfig(t, `
rules:
  - user: "*"
    patterns:
      - "forbidden-e2e-cmd "
`)
	if _, err := ReloadShared(); err != nil {
		t.Fatalf("ReloadShared: %v", err)
	}

	c := Shared()
	if !c.Check("anyone", "forbidden-e2e-cmd --now").Blocked {
		t.Fatal("重载后的共享检查器应命中新规则")
	}
	if c.Check("anyone", "uptime").Blocked {
		t.Fatal("安全命令不应命中")
	}

	// 再次重载：规则消失后不再命中（原地换规则而非换实例）
	writeHomeConfig(t, `
rules:
  - user: "*"
    patterns:
      - "other-cmd "
`)
	if _, err := ReloadShared(); err != nil {
		t.Fatalf("ReloadShared: %v", err)
	}
	if c2 := Shared(); c2 != c {
		t.Fatal("Shared 必须返回同一实例（原地 Reload）")
	}
	if c.Check("anyone", "forbidden-e2e-cmd --now").Blocked {
		t.Fatal("规则移除后不应再命中")
	}
}

func TestShared_MissingFileUsesDefaults(t *testing.T) {
	writeHomeConfig(t, "")
	if _, err := ReloadShared(); err != nil {
		t.Fatalf("ReloadShared: %v", err)
	}
	c := Shared()
	if !c.Check("root", "rm -rf /tmp/x").Blocked {
		t.Fatal("无配置文件时应回退默认规则")
	}
}

func TestChecker_ReloadConcurrentWithCheck(t *testing.T) {
	// Reload 与 Check 并发不得数据竞争（Web 执行路径热重载时正在跑的 run）
	c := NewChecker(&Config{Rules: DefaultRules()})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			c.Reload(&Config{Rules: DefaultRules()})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			_ = c.Check("root", "rm -rf /tmp").Blocked
		}
	}()
	wg.Wait()
}
