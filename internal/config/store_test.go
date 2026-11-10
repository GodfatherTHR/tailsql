package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func testInstance(name string) Instance {
	now := time.Now().UTC()
	return Instance{
		ID:             name + "-id",
		Name:           name,
		Username:       "sa",
		Password:       "test-password-1",
		ServerPassword: "server-secret",
		Port:           9092,
		Status:         StatusStopped,
		DatabaseFile:   name + ".mv.db",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

func TestStorePersistsAcrossOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "instances.json")
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	instance := testInstance("inventory_db")
	if err := store.Create(instance); err != nil {
		t.Fatalf("Create: %v", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	loaded, ok := reopened.Get(instance.ID)
	if !ok {
		t.Fatal("instance missing after reopen")
	}
	if loaded.Name != instance.Name || loaded.Password != instance.Password {
		t.Fatalf("unexpected loaded instance: %+v", loaded)
	}
	if loaded.ServerPassword != instance.ServerPassword {
		t.Fatal("server password was not persisted")
	}
}

func TestStoreRejectsDuplicateNamesCaseInsensitive(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "instances.json"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := store.Create(testInstance("inventory_db")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	duplicate := testInstance("Inventory_DB")
	duplicate.ID = "another-id"
	if err := store.Create(duplicate); err == nil {
		t.Fatal("expected duplicate name error")
	}
}

func TestStoreUpdateAndDelete(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "instances.json"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	instance := testInstance("orders")
	if err := store.Create(instance); err != nil {
		t.Fatalf("Create: %v", err)
	}
	instance.Status = StatusRunning
	instance.PID = 42
	if err := store.Update(instance); err != nil {
		t.Fatalf("Update: %v", err)
	}
	loaded, _ := store.Get(instance.ID)
	if loaded.Status != StatusRunning || loaded.PID != 42 {
		t.Fatalf("update not persisted: %+v", loaded)
	}
	if err := store.Delete(instance.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := store.Get(instance.ID); ok {
		t.Fatal("instance still present after delete")
	}
}

func TestStoreRecoversFromCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "instances.json")
	if err := os.WriteFile(path, []byte("{not-json"), 0o600); err != nil {
		t.Fatalf("seed corrupt file: %v", err)
	}
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open corrupt: %v", err)
	}
	if len(store.Warnings()) == 0 {
		t.Fatal("expected corruption warning")
	}
	if len(store.List()) != 0 {
		t.Fatal("expected empty store after corruption recovery")
	}
	matches, err := filepath.Glob(path + ".corrupt-*")
	if err != nil || len(matches) != 1 {
		t.Fatalf("expected one backup file, got %v (err=%v)", matches, err)
	}
}

func TestStoreConcurrentAccess(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "instances.json"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	instance := testInstance("concurrent")
	if err := store.Create(instance); err != nil {
		t.Fatalf("Create: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				instance.PID = n*100 + j
				_ = store.Update(instance)
				_ = store.List()
				_, _ = store.Get(instance.ID)
			}
		}(i)
	}
	wg.Wait()
	if _, ok := store.Get(instance.ID); !ok {
		t.Fatal("instance disappeared under concurrent access")
	}
}
