package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveDBPath_HonorsOWLDBPath(t *testing.T) {
	orig := os.Getenv("OWL_DB_PATH")
	defer os.Setenv("OWL_DB_PATH", orig)

	if err := os.Setenv("OWL_DB_PATH", "/custom/path/owl.db"); err != nil {
		t.Fatalf("setenv: %v", err)
	}
	if got := resolveDBPath(); got != "/custom/path/owl.db" {
		t.Errorf("resolveDBPath() = %q, want /custom/path/owl.db", got)
	}
}

func TestResolveDBPath_DefaultHome(t *testing.T) {
	orig := os.Getenv("OWL_DB_PATH")
	defer os.Setenv("OWL_DB_PATH", orig)
	os.Unsetenv("OWL_DB_PATH")

	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".owl", "owl.db")
	if got := resolveDBPath(); got != want {
		t.Errorf("resolveDBPath() = %q, want %q", got, want)
	}
}
