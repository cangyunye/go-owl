package logger

import (
	"os"
	"testing"
)

func clearLogEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"OWL_LOG_MAX_SIZE_MB", "OWL_LOG_MAX_BACKUPS"} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
}

// 默认轮转策略：10MB × 10 份，保留 30 天并压缩 —— 家目录磁盘占用
// 上限约 100MB（压缩后更低），足够定位问题又不至于吃满数据盘。
func TestDefaultConfig_Rotation10MBx10(t *testing.T) {
	clearLogEnv(t)
	cfg := DefaultConfig()
	if cfg.MaxSize != 10 {
		t.Fatalf("MaxSize = %d, want 10", cfg.MaxSize)
	}
	if cfg.MaxBackups != 10 {
		t.Fatalf("MaxBackups = %d, want 10", cfg.MaxBackups)
	}
	if cfg.MaxAge != 30 {
		t.Fatalf("MaxAge = %d, want 30", cfg.MaxAge)
	}
	if !cfg.Compress {
		t.Fatal("Compress = false, want true")
	}
}

// 环境变量覆盖默认值：部署时无需改代码即可调整轮转策略。
func TestDefaultConfig_EnvOverrides(t *testing.T) {
	t.Run("size", func(t *testing.T) {
		clearLogEnv(t)
		t.Setenv("OWL_LOG_MAX_SIZE_MB", "50")
		if got := DefaultConfig().MaxSize; got != 50 {
			t.Fatalf("MaxSize = %d, want 50", got)
		}
	})
	t.Run("backups", func(t *testing.T) {
		clearLogEnv(t)
		t.Setenv("OWL_LOG_MAX_BACKUPS", "3")
		if got := DefaultConfig().MaxBackups; got != 3 {
			t.Fatalf("MaxBackups = %d, want 3", got)
		}
	})
	t.Run("invalid falls back to default", func(t *testing.T) {
		clearLogEnv(t)
		t.Setenv("OWL_LOG_MAX_SIZE_MB", "abc")
		t.Setenv("OWL_LOG_MAX_BACKUPS", "-2")
		cfg := DefaultConfig()
		if cfg.MaxSize != 10 || cfg.MaxBackups != 10 {
			t.Fatalf("MaxSize = %d, MaxBackups = %d, want defaults 10/10", cfg.MaxSize, cfg.MaxBackups)
		}
	})
}
