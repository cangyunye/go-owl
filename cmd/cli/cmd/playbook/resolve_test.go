package playbook

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultPlaybookDir(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T) string
		wantMatch func(t *testing.T, got string)
	}{
		{
			name: "home dir fails returns ./playbooks",
			setup: func(t *testing.T) string {
				t.Setenv("HOME", "")
				return ""
			},
			wantMatch: func(t *testing.T, got string) {
				if got != "./playbooks" {
					home, err := os.UserHomeDir()
					if err == nil {
						t.Skipf("os.UserHomeDir() did not fail on this platform (returned %q), skipping", home)
					}
					t.Errorf("expected './playbooks', got %q", got)
				}
			},
		},
		{
			name: "~/.owl/playbooks exists",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				t.Setenv("HOME", tmpDir)
				p := filepath.Join(tmpDir, ".owl", "playbooks")
				if err := os.MkdirAll(p, 0755); err != nil {
					t.Fatalf("failed to create dir: %v", err)
				}
				return p
			},
			wantMatch: func(t *testing.T, got string) {
				tmpDir := os.Getenv("HOME")
				want := filepath.Join(tmpDir, ".owl", "playbooks")
				if got != want {
					t.Errorf("expected %q, got %q", want, got)
				}
			},
		},
		{
			name: "./playbooks exists, ~/.owl/playbooks does not",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				t.Setenv("HOME", tmpDir)
				cwd := t.TempDir()
				if err := os.MkdirAll(filepath.Join(cwd, "playbooks"), 0755); err != nil {
					t.Fatalf("failed to create dir: %v", err)
				}
				if err := os.Chdir(cwd); err != nil {
					t.Fatalf("failed to chdir: %v", err)
				}
				return ""
			},
			wantMatch: func(t *testing.T, got string) {
				want, _ := filepath.Abs("./playbooks")
				if got != want {
					t.Errorf("expected %q, got %q", want, got)
				}
			},
		},
		{
			name: "neither exists returns ~/.owl/playbooks",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				t.Setenv("HOME", tmpDir)
				cwd := t.TempDir()
				if err := os.Chdir(cwd); err != nil {
					t.Fatalf("failed to chdir: %v", err)
				}
				return ""
			},
			wantMatch: func(t *testing.T, got string) {
				tmpDir := os.Getenv("HOME")
				want := filepath.Join(tmpDir, ".owl", "playbooks")
				if got != want {
					t.Errorf("expected %q, got %q", want, got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup(t)
			got := defaultPlaybookDir()
			tt.wantMatch(t, got)
		})
	}
}
