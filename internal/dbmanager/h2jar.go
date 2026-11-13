package dbmanager

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Pinned H2 engine distributed with HySQL.
const (
	H2Version     = "2.3.232"
	H2FileName    = "h2.jar"
	H2DownloadURL = "https://repo1.maven.org/maven2/com/h2database/h2/2.3.232/h2-2.3.232.jar"
)

// H2Info describes the discovered H2 engine jar.
type H2Info struct {
	Available       bool   `json:"available"`
	Path            string `json:"path"`
	Version         string `json:"version"`
	Source          string `json:"source"`
	ExpectedVersion string `json:"expectedVersion"`
	Error           string `json:"error"`
}

var requiredH2Classes = []string{
	"org/h2/Driver.class",
	"org/h2/tools/Server.class",
	"org/h2/tools/RunScript.class",
}

// ResolveH2Jar searches the configured path and candidate locations, validating
// each file before trusting it.
func ResolveH2Jar(configuredPath string, candidates []string) H2Info {
	info := H2Info{ExpectedVersion: H2Version}
	var failures []string
	checked := map[string]bool{}

	try := func(path, source string) bool {
		if strings.TrimSpace(path) == "" {
			return false
		}
		absolute, err := filepath.Abs(path)
		if err == nil {
			path = absolute
		}
		if checked[path] {
			return false
		}
		checked[path] = true
		if !fileExists(path) {
			return false
		}
		if err := ValidateH2Jar(path); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", path, err))
			return false
		}
		info.Available = true
		info.Path = path
		info.Version = H2Version
		info.Source = source
		return true
	}

	if strings.TrimSpace(configuredPath) != "" {
		if try(configuredPath, "settings") {
			return info
		}
		if !fileExists(configuredPath) {
			failures = append(failures, fmt.Sprintf("configured h2JarPath does not exist: %s", configuredPath))
		}
	}
	for _, candidate := range candidates {
		if try(candidate, describeH2Source(candidate)) {
			return info
		}
	}

	if len(failures) > 0 {
		info.Error = "H2 engine not available: " + strings.Join(failures, "; ")
	} else {
		info.Error = fmt.Sprintf("H2 engine (%s) was not found. Use InstallH2Engine or run scripts/fetch-h2.ps1.", H2Version)
	}
	return info
}

// ValidateH2Jar verifies the archive is a real H2 distribution containing the
// classes HySQL invokes.
func ValidateH2Jar(path string) error {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("not a valid jar archive: %w", err)
	}
	defer reader.Close()

	found := make(map[string]bool, len(requiredH2Classes))
	for _, file := range reader.File {
		found[file.Name] = true
	}
	for _, required := range requiredH2Classes {
		if !found[required] {
			return fmt.Errorf("jar is missing %s", required)
		}
	}
	return nil
}

// DownloadH2 fetches the pinned H2 jar into destDir, validates it, and writes a
// SHA-256 sidecar for future integrity checks.
func DownloadH2(ctx context.Context, destDir string) (string, error) {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("create H2 asset directory: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, H2DownloadURL, nil)
	if err != nil {
		return "", fmt.Errorf("build H2 download request: %w", err)
	}
	request.Header.Set("User-Agent", "HySQL/0.1 (+local desktop)")

	client := &http.Client{Timeout: 3 * time.Minute}
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("download H2 engine: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download H2 engine: unexpected HTTP status %s", response.Status)
	}

	tmp, err := os.CreateTemp(destDir, ".h2-*.jar.tmp")
	if err != nil {
		return "", fmt.Errorf("create temporary H2 file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}

	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(tmp, hash), response.Body)
	if err != nil {
		cleanup()
		return "", fmt.Errorf("save H2 engine: %w", err)
	}
	if written < 1_000_000 {
		cleanup()
		return "", fmt.Errorf("downloaded H2 engine is unexpectedly small (%d bytes)", written)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return "", fmt.Errorf("flush H2 engine: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("close H2 engine file: %w", err)
	}
	if err := ValidateH2Jar(tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("downloaded file is not a usable H2 engine: %w", err)
	}

	destination := filepath.Join(destDir, H2FileName)
	if err := os.Rename(tmpPath, destination); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("install H2 engine: %w", err)
	}
	checksum := hex.EncodeToString(hash.Sum(nil))
	_ = os.WriteFile(destination+".sha256", []byte(checksum+"  h2.jar\n"), 0o644)
	return destination, nil
}

func describeH2Source(path string) string {
	lower := strings.ToLower(filepath.ToSlash(path))
	switch {
	case strings.Contains(lower, "/assets/"):
		return "application assets"
	case strings.Contains(lower, ".hysql/"):
		return "application data"
	default:
		return "local file"
	}
}
