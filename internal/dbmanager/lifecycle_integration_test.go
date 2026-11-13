package dbmanager

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"hysql/internal/config"
	"hysql/internal/executil"
)

func integrationPrerequisites(t *testing.T) (JavaInfo, H2Info) {
	t.Helper()
	if os.Getenv("HYSQL_INTEGRATION") != "1" {
		t.Skip("set HYSQL_INTEGRATION=1 to run H2 lifecycle integration tests")
	}
	java := DetectJava("")
	if !java.Supported {
		t.Skipf("Java 11+ not available: %s", java.Error)
	}
	assets, err := filepath.Abs(filepath.Join("..", "..", "assets"))
	if err != nil {
		t.Fatalf("resolve assets: %v", err)
	}
	h2 := ResolveH2Jar("", []string{filepath.Join(assets, H2FileName)})
	if !h2.Available {
		t.Skipf("H2 jar not available: %s", h2.Error)
	}
	return java, h2
}

func TestLifecycleIntegration(t *testing.T) {
	java, h2 := integrationPrerequisites(t)
	_ = java
	_ = h2

	dataDir := t.TempDir()
	store, err := config.Open(filepath.Join(dataDir, "instances.json"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	manager, err := New(Config{
		Store:     store,
		DataDir:   dataDir,
		AssetsDir: filepath.Join("..", "..", "assets"),
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if !manager.Java().Supported {
		t.Fatalf("Java lost between prerequisite check and manager creation: %s", manager.Java().Error)
	}

	created, err := manager.Create(CreateInput{
		Name:     "integration_db",
		Username: "sa",
		Password: "Integration99",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Port < 9092 {
		t.Fatalf("expected auto-assigned port, got %d", created.Port)
	}
	if created.Password != "Integration99" {
		t.Fatal("password was not preserved")
	}

	started, err := manager.Start(created.ID)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if started.Status != config.StatusRunning || started.PID <= 0 {
		t.Fatalf("unexpected started state: %+v", started)
	}
	t.Cleanup(func() {
		_, _ = manager.Stop(created.ID)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	bridge := manager.Bridge()
	url := LocalJDBCURL(created.Name, created.Port)

	durability, err := bridge.Execute(ctx, url, created.Username, created.Password,
		"SELECT SETTING_VALUE FROM INFORMATION_SCHEMA.SETTINGS WHERE SETTING_NAME='WRITE_DELAY'")
	if err != nil {
		t.Fatalf("read durability setting: %v", err)
	}
	if durability.RowCount == 0 {
		t.Fatal("WRITE_DELAY was not reported for a fresh connection")
	}
	for _, row := range durability.Rows {
		if len(row) == 0 || row[0] == nil || *row[0] != "0" {
			t.Fatalf("WRITE_DELAY was not 0 on every reported setting: %+v", durability.Rows)
		}
	}
	// Execute always launches a new JVM and opens a new connection. The live
	// MVStore value must be 0 there too, not just in the first session.
	fresh, err := manager.Bridge().Execute(ctx, url, created.Username, created.Password,
		"SELECT SETTING_VALUE FROM INFORMATION_SCHEMA.SETTINGS WHERE SETTING_NAME='WRITE_DELAY'")
	if err != nil {
		t.Fatalf("read durability setting from a fresh connection: %v", err)
	}
	for _, row := range fresh.Rows {
		if len(row) == 0 || row[0] == nil || *row[0] != "0" {
			t.Fatalf("WRITE_DELAY was not durable on a new connection: %+v", fresh.Rows)
		}
	}

	if _, err := bridge.Execute(ctx, url, created.Username, created.Password,
		"CREATE TABLE items(id INT PRIMARY KEY, name VARCHAR(40))"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	result, err := bridge.Execute(ctx, url, created.Username, created.Password,
		"INSERT INTO items VALUES (1, 'integration'), (2, 'bridge')")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if result.HasResultSet || result.UpdateCount != 2 {
		t.Fatalf("unexpected insert result: %+v", result)
	}

	result, err = bridge.Execute(ctx, url, created.Username, created.Password,
		"SELECT id, name FROM items ORDER BY id")
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if !result.HasResultSet || result.RowCount != 2 || len(result.Columns) != 2 {
		t.Fatalf("unexpected select result: %+v", result)
	}
	if result.Rows[0][1] == nil || *result.Rows[0][1] != "integration" {
		t.Fatalf("unexpected row data: %+v", result.Rows)
	}

	schema, err := bridge.Tables(ctx, url, created.Username, created.Password)
	if err != nil {
		t.Fatalf("tables: %v", err)
	}
	foundTable := false
	for _, table := range schema.Tables {
		if strings.EqualFold(table.Name, "ITEMS") {
			foundTable = true
			if len(table.Columns) != 2 {
				t.Fatalf("unexpected columns: %+v", table.Columns)
			}
		}
	}
	if !foundTable {
		t.Fatalf("ITEMS table missing from schema: %+v", schema.Tables)
	}

	if _, err := bridge.Execute(ctx, url, created.Username, "wrong-password", "SELECT 1"); err == nil {
		t.Fatal("expected authentication failure with wrong password")
	}

	stopped, err := manager.Stop(created.ID)
	if err != nil {
		t.Fatalf("stop: %v", err)
	}
	if stopped.Status != config.StatusStopped || stopped.PID != 0 {
		t.Fatalf("unexpected stopped state: %+v", stopped)
	}
	if tcpReady("127.0.0.1", created.Port) {
		t.Fatal("H2 port still accepting connections after stop")
	}
	fileInfo, err := os.Stat(filepath.Join(dataDir, "dbs", created.DatabaseFile))
	if err != nil {
		t.Fatalf("database file missing after stop: %v", err)
	}
	if fileInfo.Size() == 0 {
		t.Fatal("database file is empty")
	}

	reopened, err := config.Open(filepath.Join(dataDir, "instances.json"))
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	persisted, ok := reopened.Get(created.ID)
	if !ok || persisted.Status != config.StatusStopped {
		t.Fatalf("unexpected persisted state: %+v (ok=%v)", persisted, ok)
	}

	if err := manager.Delete(created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "dbs", created.DatabaseFile)); err != nil {
		t.Fatalf("delete should retain database files: %v", err)
	}
}

func TestReconcileAdoptsOrphan(t *testing.T) {
	java, h2 := integrationPrerequisites(t)
	dataDir := t.TempDir()
	dbDir := filepath.Join(dataDir, "dbs")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatalf("create db dir: %v", err)
	}

	port, err := FindAvailablePort("127.0.0.1", 20000, 100)
	if err != nil {
		t.Fatalf("allocate port: %v", err)
	}
	instance := config.Instance{
		ID:             "orphan-id",
		Name:           "orphan_db",
		Username:       "sa",
		Password:       "OrphanPass99",
		ServerPassword: "OrphanServer99",
		Port:           port,
		Status:         config.StatusRunning,
		TailscalePort:  29092,
		DatabaseFile:   "orphan_db.mv.db",
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	store, err := config.Open(filepath.Join(dataDir, "instances.json"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := store.Create(instance); err != nil {
		t.Fatalf("seed store: %v", err)
	}

	logFile, err := os.Create(filepath.Join(dataDir, "orphan.log"))
	if err != nil {
		t.Fatalf("create log: %v", err)
	}
	args := []string{
		"-Dh2.bindAddress=127.0.0.1",
		"-cp", h2.Path,
		"org.h2.tools.Server",
		"-tcp",
		"-tcpPort", strconv.Itoa(port),
		"-tcpPassword", instance.ServerPassword,
		"-baseDir", dbDir,
		"-ifNotExists",
	}
	cmd := executil.CommandContext(context.Background(), java.Path, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		t.Fatalf("start orphan H2: %v", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = logFile.Close()
	})
	if err := waitForPort("127.0.0.1", port, 20*time.Second, make(chan struct{})); err != nil {
		t.Fatalf("orphan H2 did not start: %v", err)
	}
	instance.PID = cmd.Process.Pid
	if err := store.Update(instance); err != nil {
		t.Fatalf("persist orphan pid: %v", err)
	}

	manager, err := New(Config{
		Store:     store,
		DataDir:   dataDir,
		AssetsDir: filepath.Join("..", "..", "assets"),
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	adoptedBridge := manager.Bridge()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := adoptedBridge.Ping(ctx, LocalJDBCURL(instance.Name, instance.Port), instance.Username, instance.Password); err != nil {
		t.Fatalf("initialize orphan database: %v", err)
	}

	manager.Reconcile(ctx)
	reconciled, ok := manager.Get(instance.ID)
	if !ok {
		t.Fatal("instance disappeared during reconcile")
	}
	if reconciled.Status != config.StatusRunning || reconciled.PID != cmd.Process.Pid {
		t.Fatalf("orphan was not adopted: %+v", reconciled)
	}
	if reconciled.TailscaleEnabled {
		t.Fatal("Tailscale hosting must not be auto-restored after restart")
	}

	stopped, err := manager.Stop(instance.ID)
	if err != nil {
		t.Fatalf("stop adopted instance: %v", err)
	}
	if stopped.Status != config.StatusStopped {
		t.Fatalf("unexpected adopted stop state: %+v", stopped)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !inspectProcess(cmd.Process.Pid).Alive {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("adopted H2 process is still alive after stop")
}

func startUntrackedH2(t *testing.T, java JavaInfo, h2 H2Info, instance config.Instance, dataDir, dbDir string) *exec.Cmd {
	t.Helper()
	logFile, err := os.Create(filepath.Join(dataDir, "orphan-"+instance.ID+".log"))
	if err != nil {
		t.Fatalf("create orphan log: %v", err)
	}
	args := []string{
		"-Dh2.bindAddress=127.0.0.1",
		"-cp", h2.Path,
		"org.h2.tools.Server",
		"-tcp",
		"-tcpPort", strconv.Itoa(instance.Port),
		"-tcpPassword", instance.ServerPassword,
		"-baseDir", dbDir,
		"-ifNotExists",
	}
	cmd := executil.CommandContext(context.Background(), java.Path, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		t.Fatalf("start untracked H2: %v", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = logFile.Close()
	})
	if err := waitForPort("127.0.0.1", instance.Port, 20*time.Second, make(chan struct{})); err != nil {
		t.Fatalf("untracked H2 did not start: %v", err)
	}
	return cmd
}

func newOrphanTestStore(t *testing.T, dataDir string, instance config.Instance) *config.Store {
	t.Helper()
	store, err := config.Open(filepath.Join(dataDir, "instances.json"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := store.Create(instance); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	return store
}

func TestReconcileAdoptsUntrackedOrphan(t *testing.T) {
	java, h2 := integrationPrerequisites(t)
	dataDir := t.TempDir()
	dbDir := filepath.Join(dataDir, "dbs")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatalf("create db dir: %v", err)
	}
	port, err := FindAvailablePort("127.0.0.1", 20000, 100)
	if err != nil {
		t.Fatalf("allocate port: %v", err)
	}
	instance := config.Instance{
		ID:             "untracked-reconcile",
		Name:           "untracked_db",
		Username:       "sa",
		Password:       "OrphanPass99",
		ServerPassword: "OrphanServer99",
		Port:           port,
		Status:         config.StatusError,
		TailscalePort:  29093,
		DatabaseFile:   "untracked_db.mv.db",
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
		LastError:      "previous handshake failed",
	}
	store := newOrphanTestStore(t, dataDir, instance)
	startUntrackedH2(t, java, h2, instance, dataDir, dbDir)

	manager, err := New(Config{
		Store:     store,
		DataDir:   dataDir,
		AssetsDir: filepath.Join("..", "..", "assets"),
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	manager.Reconcile(ctx)

	reconciled, ok := manager.Get(instance.ID)
	if !ok {
		t.Fatal("instance disappeared during reconcile")
	}
	if reconciled.Status != config.StatusRunning {
		t.Fatalf("untracked orphan was not adopted: %+v", reconciled)
	}
	if !containsPID(manager.findInstanceH2Processes(instance), reconciled.PID) {
		t.Fatalf("adopted PID %d is not a matching H2 process", reconciled.PID)
	}
	if reconciled.LastError != "" {
		t.Fatalf("stale error was not cleared: %s", reconciled.LastError)
	}
	if _, err := manager.Stop(instance.ID); err != nil {
		t.Fatalf("stop adopted untracked instance: %v", err)
	}
}

func TestStartAdoptsUntrackedOrphan(t *testing.T) {
	java, h2 := integrationPrerequisites(t)
	dataDir := t.TempDir()
	dbDir := filepath.Join(dataDir, "dbs")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatalf("create db dir: %v", err)
	}
	port, err := FindAvailablePort("127.0.0.1", 20100, 100)
	if err != nil {
		t.Fatalf("allocate port: %v", err)
	}
	instance := config.Instance{
		ID:             "untracked-start",
		Name:           "untracked_start_db",
		Username:       "sa",
		Password:       "OrphanPass99",
		ServerPassword: "OrphanServer98",
		Port:           port,
		Status:         config.StatusStopped,
		TailscalePort:  29094,
		DatabaseFile:   "untracked_start_db.mv.db",
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	store := newOrphanTestStore(t, dataDir, instance)
	startUntrackedH2(t, java, h2, instance, dataDir, dbDir)

	manager, err := New(Config{
		Store:     store,
		DataDir:   dataDir,
		AssetsDir: filepath.Join("..", "..", "assets"),
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	before := manager.findInstanceH2Processes(instance)
	started, err := manager.Start(instance.ID)
	if err != nil {
		t.Fatalf("start with untracked orphan: %v", err)
	}
	if started.Status != config.StatusRunning {
		t.Fatalf("start did not adopt the untracked server: %+v", started)
	}
	after := manager.findInstanceH2Processes(instance)
	if !containsPID(after, started.PID) {
		t.Fatalf("adopted PID %d is not a matching H2 process", started.PID)
	}
	if len(after) > len(before) {
		t.Fatalf("start launched an extra H2 server: before=%v after=%v", before, after)
	}
	if _, err := manager.Stop(instance.ID); err != nil {
		t.Fatalf("stop adopted untracked instance: %v", err)
	}
}
