package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateLegacyDataDir(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, ".tailsql")
	dataDir := filepath.Join(root, ".hysql")
	if err := os.MkdirAll(filepath.Join(legacy, "dbs"), 0o700); err != nil {
		t.Fatalf("create legacy data dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "instances.json"), []byte("{}"), 0o600); err != nil {
		t.Fatalf("write legacy state: %v", err)
	}

	if err := migrateLegacyDataDir(legacy, dataDir); err != nil {
		t.Fatalf("migrate legacy data dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "instances.json")); err != nil {
		t.Fatalf("migrated state is missing: %v", err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy directory still exists: %v", err)
	}
}

func TestMigrateLegacyDataDirKeepsExistingTarget(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, ".tailsql")
	dataDir := filepath.Join(root, ".hysql")
	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatalf("create legacy data dir: %v", err)
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("create data dir: %v", err)
	}
	marker := filepath.Join(dataDir, "instances.json")
	if err := os.WriteFile(marker, []byte(`{"instances":[]}`), 0o600); err != nil {
		t.Fatalf("write current state: %v", err)
	}

	if err := migrateLegacyDataDir(legacy, dataDir); err != nil {
		t.Fatalf("migrate legacy data dir: %v", err)
	}
	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read current state: %v", err)
	}
	if string(data) != `{"instances":[]}` {
		t.Fatalf("current state was overwritten: %s", data)
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Fatalf("legacy directory should be left in place: %v", err)
	}
}

func TestMigrateLegacyDataDirWithoutLegacy(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, ".tailsql")
	dataDir := filepath.Join(root, ".hysql")

	if err := migrateLegacyDataDir(legacy, dataDir); err != nil {
		t.Fatalf("migrate missing legacy dir: %v", err)
	}
	if _, err := os.Stat(dataDir); !os.IsNotExist(err) {
		t.Fatalf("data dir should not be created: %v", err)
	}
}
