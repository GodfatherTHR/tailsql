package snapshots

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSnapshotLifecycle(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	source := filepath.Join(t.TempDir(), "demo.mv.db")
	if err := os.WriteFile(source, []byte("database-bytes"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	entry, err := store.SaveDatabaseFile("demo", source, "Manual checkpoint")
	if err != nil {
		t.Fatalf("save snapshot: %v", err)
	}
	if !strings.HasSuffix(entry.ID, ".mv.db") || entry.SizeBytes != int64(len("database-bytes")) {
		t.Fatalf("unexpected snapshot entry: %+v", entry)
	}
	list, err := store.List("demo")
	if err != nil {
		t.Fatalf("list snapshots: %v", err)
	}
	if len(list) != 1 || list[0].ID != entry.ID {
		t.Fatalf("unexpected snapshot list: %+v", list)
	}

	target := filepath.Join(t.TempDir(), "demo.mv.db")
	if err := os.WriteFile(target, []byte("old"), 0o600); err != nil {
		t.Fatalf("write old target: %v", err)
	}
	if err := store.Restore("demo", entry.ID, target); err != nil {
		t.Fatalf("restore snapshot: %v", err)
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read restored target: %v", err)
	}
	if string(content) != "database-bytes" {
		t.Fatalf("unexpected restored content: %q", content)
	}
	if err := store.Delete("demo", entry.ID); err != nil {
		t.Fatalf("delete snapshot: %v", err)
	}
	if list, err := store.List("demo"); err != nil || len(list) != 0 {
		t.Fatalf("snapshot was not deleted: %v %+v", err, list)
	}
}

func TestSaveBackupExtractsDatabaseMember(t *testing.T) {
	zipPath := filepath.Join(t.TempDir(), "backup.zip")
	file, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	writer := zip.NewWriter(file)
	member, err := writer.Create("demo.mv.db")
	if err != nil {
		t.Fatalf("create zip member: %v", err)
	}
	if _, err := member.Write([]byte("online-backup")); err != nil {
		t.Fatalf("write zip member: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close zip file: %v", err)
	}

	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	entry, err := store.SaveBackup("demo", zipPath, "Pre-Agent DDL")
	if err != nil {
		t.Fatalf("save backup: %v", err)
	}
	path, err := store.RestorePath("demo", entry.ID)
	if err != nil {
		t.Fatalf("restore path: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read extracted backup: %v", err)
	}
	if string(content) != "online-backup" {
		t.Fatalf("unexpected extracted content: %q", content)
	}
}

func TestSnapshotPruneKeepsNewest(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	source := filepath.Join(t.TempDir(), "demo.mv.db")
	if err := os.WriteFile(source, []byte("x"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	for index := 0; index < 4; index++ {
		if _, err := store.SaveDatabaseFile("demo", source, "checkpoint"); err != nil {
			t.Fatalf("save snapshot %d: %v", index, err)
		}
	}
	if err := store.Prune("demo", 2); err != nil {
		t.Fatalf("prune: %v", err)
	}
	list, err := store.List("demo")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 snapshots after prune, got %d", len(list))
	}
}
