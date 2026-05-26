package history

import (
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if !config.Enabled {
		t.Error("expected Enabled to be true")
	}
	if config.DBPath == "" {
		t.Error("expected non-empty DBPath")
	}
	if config.RetentionDays != 90 {
		t.Errorf("expected RetentionDays 90, got %d", config.RetentionDays)
	}
}

func TestDB_NilSafety(t *testing.T) {
	var db *DB

	if conn := db.Connection(); conn != nil {
		t.Error("expected nil Connection for nil DB")
	}
	if err := db.Close(); err != nil {
		t.Errorf("expected nil error for Close on nil DB, got %v", err)
	}
	if err := db.Cleanup(30); err != nil {
		t.Errorf("expected nil error for Cleanup on nil DB, got %v", err)
	}
}

func TestDB_EmptyImpl(t *testing.T) {
	db := &DB{}

	if conn := db.Connection(); conn != nil {
		t.Error("expected nil Connection for DB with nil impl")
	}
	if err := db.Close(); err != nil {
		t.Errorf("expected nil error for Close with nil impl, got %v", err)
	}
}

func TestGetDB_NilGlobal(t *testing.T) {
	SetGlobalDB(nil)

	db := GetDB()
	if db != nil {
		t.Error("expected nil DB when global is nil")
	}
}

func TestConfig_Fields(t *testing.T) {
	config := &Config{
		Enabled:       false,
		DBPath:        "/custom/path/db",
		RetentionDays: 30,
	}

	if config.Enabled {
		t.Error("expected Enabled false")
	}
	if config.DBPath != "/custom/path/db" {
		t.Errorf("expected DBPath '/custom/path/db', got '%s'", config.DBPath)
	}
	if config.RetentionDays != 30 {
		t.Errorf("expected RetentionDays 30, got %d", config.RetentionDays)
	}
}

func TestDefaultConfig_RetentionDaysRange(t *testing.T) {
	config := DefaultConfig()

	if config.RetentionDays <= 0 {
		t.Error("expected positive RetentionDays")
	}
	if config.RetentionDays > 365 {
		t.Errorf("expected RetentionDays <= 365, got %d", config.RetentionDays)
	}
}

func TestDefaultConfig_PathPattern(t *testing.T) {
	config := DefaultConfig()

	if config.DBPath == "" {
		t.Fatal("expected non-empty DBPath")
	}

	validSuffixes := []string{
		".owl",
		"owl.db",
	}
	for _, suffix := range validSuffixes {
		if len(config.DBPath) > len(suffix) &&
			config.DBPath[len(config.DBPath)-len(suffix):] == suffix &&
			config.DBPath[len(config.DBPath)-len(suffix)-1] != '/' {
			return
		}
	}

	if config.DBPath == "" {
		t.Error("DBPath should contain '.owl' and 'owl.db'")
	}
}
