package dbmanager

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"hysql/internal/executil"
)

// MinimumJavaMajor is the oldest runtime supported by the embedded bridge.
const MinimumJavaMajor = 11

// JavaInfo describes the detected host JDK/JRE.
type JavaInfo struct {
	Available bool   `json:"available"`
	Supported bool   `json:"supported"`
	Path      string `json:"path"`
	Version   string `json:"version"`
	Major     int    `json:"major"`
	Minimum   int    `json:"minimum"`
	Error     string `json:"error"`
	CheckedAt string `json:"checkedAt"`
}

var javaVersionPattern = regexp.MustCompile(`version\s+"([^"]+)"`)

// DetectJava locates a Java runtime and parses `java -version` output.
func DetectJava(override string) JavaInfo {
	info := JavaInfo{Minimum: MinimumJavaMajor, CheckedAt: time.Now().UTC().Format(time.RFC3339)}

	path, resolveErr := resolveJavaBinary(override)
	if resolveErr != nil {
		info.Error = resolveErr.Error()
		return info
	}
	info.Path = path

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := executil.CommandContext(ctx, path, "-version")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	cmd.Stdout = &stderr
	runErr := cmd.Run()
	output := stderr.String()

	match := javaVersionPattern.FindStringSubmatch(output)
	if runErr != nil && match == nil {
		info.Error = fmt.Sprintf("`java -version` failed: %v", runErr)
		return info
	}
	if match == nil {
		info.Error = "could not parse the Java version output; install a standard JDK/JRE 11+"
		return info
	}

	info.Available = true
	info.Version = match[1]
	info.Major = parseJavaMajor(match[1])
	if info.Major < MinimumJavaMajor {
		info.Error = fmt.Sprintf("Java %s detected, but HySQL requires Java %d or newer", info.Version, MinimumJavaMajor)
		return info
	}
	info.Supported = true
	return info
}

func resolveJavaBinary(override string) (string, error) {
	if trimmed := strings.TrimSpace(override); trimmed != "" {
		if fileExists(trimmed) {
			return trimmed, nil
		}
		return "", fmt.Errorf("configured Java path does not exist: %s", trimmed)
	}
	// Prefer JAVA_HOME over PATH: Windows installs a javapath launcher shim on
	// PATH that spawns a second JVM process, which complicates lifecycle
	// tracking and orphan cleanup.
	if javaHome := strings.TrimSpace(os.Getenv("JAVA_HOME")); javaHome != "" {
		candidate := filepath.Join(javaHome, "bin", "java")
		if isWindows() {
			candidate += ".exe"
		}
		if fileExists(candidate) {
			return candidate, nil
		}
	}
	if found, err := exec.LookPath("java"); err == nil {
		return found, nil
	}
	return "", errors.New("Java runtime not found. Install Java 11+ and ensure `java` is on PATH, or set settings.javaPath in ~/.hysql/instances.json")
}

func parseJavaMajor(version string) int {
	parts := strings.Split(version, ".")
	if len(parts) == 0 {
		return 0
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0
	}
	if major == 1 && len(parts) > 1 {
		if legacy, err := strconv.Atoi(parts[1]); err == nil {
			return legacy
		}
	}
	return major
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func isWindows() bool {
	return filepath.Separator == '\\'
}
