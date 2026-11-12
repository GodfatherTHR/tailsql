package dbmanager

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
	"time"
)

func TestValidateName(t *testing.T) {
	valid := []string{"inventory_db", "db1", "my_database_123"}
	for _, name := range valid {
		if err := ValidateName(name); err != nil {
			t.Errorf("ValidateName(%q) rejected: %v", name, err)
		}
	}
	invalid := []string{"", "A", "1db", "db-name", "../db", "db name", "db!", "a"}
	for _, name := range invalid {
		if err := ValidateName(name); err == nil {
			t.Errorf("ValidateName(%q) should fail", name)
		}
	}
}

func TestNormalizeName(t *testing.T) {
	if got := NormalizeName("  Inventory_DB  "); got != "inventory_db" {
		t.Fatalf("NormalizeName = %q", got)
	}
}

func TestGeneratePassword(t *testing.T) {
	alphanumeric := regexp.MustCompile(`^[A-Za-z0-9]{16}$`)
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		password, err := GeneratePassword()
		if err != nil {
			t.Fatalf("GeneratePassword: %v", err)
		}
		if !alphanumeric.MatchString(password) {
			t.Fatalf("password is not 16-char alphanumeric: %q", password)
		}
		if seen[password] {
			t.Fatalf("duplicate password generated: %q", password)
		}
		seen[password] = true
	}
	serverPassword, err := GenerateServerPassword()
	if err != nil || len(serverPassword) != 24 {
		t.Fatalf("unexpected server password: %q (err=%v)", serverPassword, err)
	}
}

func TestValidatePassword(t *testing.T) {
	if err := ValidatePassword("short"); err == nil {
		t.Fatal("short password should fail")
	}
	if err := ValidatePassword("long-enough-password"); err != nil {
		t.Fatalf("valid password rejected: %v", err)
	}
	if err := ValidatePassword("has\nnewline"); err == nil {
		t.Fatal("control characters should fail")
	}
}

func TestFindAvailablePortSkipsOccupied(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	occupied := listener.Addr().(*net.TCPAddr).Port

	port, err := FindAvailablePort("127.0.0.1", occupied, 20)
	if err != nil {
		t.Fatalf("FindAvailablePort: %v", err)
	}
	if port == occupied {
		t.Fatal("allocated an occupied port")
	}
}

func TestLocalJDBCURL(t *testing.T) {
	got := LocalJDBCURL("inventory_db", 9092)
	want := "jdbc:h2:tcp://127.0.0.1:9092/./inventory_db;WRITE_DELAY=0"
	if got != want {
		t.Fatalf("LocalJDBCURL = %q, want %q", got, want)
	}
}

func TestParseJavaMajor(t *testing.T) {
	cases := map[string]int{
		"24.0.1":    24,
		"17":        17,
		"1.8.0_392": 8,
		"11.0.2":    11,
	}
	for version, want := range cases {
		if got := parseJavaMajor(version); got != want {
			t.Errorf("parseJavaMajor(%q) = %d, want %d", version, got, want)
		}
	}
}

func TestValidateH2Jar(t *testing.T) {
	jar := filepath.Join("..", "..", "assets", H2FileName)
	if !fileExists(jar) {
		t.Skip("assets/h2.jar not present")
	}
	if err := ValidateH2Jar(jar); err != nil {
		t.Fatalf("project H2 jar failed validation: %v", err)
	}

	fake := filepath.Join(t.TempDir(), "fake.jar")
	if err := os.WriteFile(fake, []byte("not a zip"), 0o600); err != nil {
		t.Fatalf("write fake jar: %v", err)
	}
	if err := ValidateH2Jar(fake); err == nil {
		t.Fatal("invalid jar should fail validation")
	}
}

func TestResolveH2JarFindsProjectAsset(t *testing.T) {
	assets := filepath.Join("..", "..", "assets")
	if !fileExists(filepath.Join(assets, H2FileName)) {
		t.Skip("assets/h2.jar not present")
	}
	info := ResolveH2Jar("", []string{filepath.Join(assets, H2FileName)})
	if !info.Available {
		t.Fatalf("expected available H2: %s", info.Error)
	}
	if info.Version != H2Version {
		t.Fatalf("unexpected version: %s", info.Version)
	}
	if info.Path == "" {
		t.Fatal("expected resolved path")
	}
}

func TestResolveH2JarMissing(t *testing.T) {
	info := ResolveH2Jar("", []string{filepath.Join(t.TempDir(), "missing.jar")})
	if info.Available || info.Error == "" {
		t.Fatalf("expected unavailable H2 info, got %+v", info)
	}
}

func TestValidatePort(t *testing.T) {
	if err := ValidatePort(80); err == nil {
		t.Fatal("privileged port should be rejected")
	}
	port, err := strconv.Atoi("9092")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidatePort(port); err != nil {
		t.Fatalf("valid port rejected: %v", err)
	}
}

func TestDownloadH2Integration(t *testing.T) {
	if os.Getenv("TAILSQL_DOWNLOAD_TEST") != "1" {
		t.Skip("set TAILSQL_DOWNLOAD_TEST=1 to exercise the H2 downloader")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	dir := t.TempDir()
	path, err := DownloadH2(ctx, dir)
	if err != nil {
		t.Fatalf("DownloadH2: %v", err)
	}
	if path != filepath.Join(dir, H2FileName) {
		t.Fatalf("unexpected download path: %s", path)
	}
	if err := ValidateH2Jar(path); err != nil {
		t.Fatalf("downloaded jar failed validation: %v", err)
	}
	if _, err := os.Stat(path + ".sha256"); err != nil {
		t.Fatalf("checksum sidecar missing: %v", err)
	}
	info := ResolveH2Jar("", []string{path})
	if !info.Available {
		t.Fatalf("downloaded jar was not resolved: %s", info.Error)
	}
}
