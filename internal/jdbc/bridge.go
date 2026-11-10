// Package jdbc executes SQL through a small Java bridge against the bundled H2
// engine. Go has no H2 driver, so the bridge is the narrow, auditable boundary
// between the Go host process and JDBC.
package jdbc

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"tailsql/internal/executil"
)

//go:embed JdbcBridge*.class
var embeddedClasses embed.FS

// Column is one result-set column.
type Column struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
}

// ExecResult is the structured outcome of one SQL statement.
type ExecResult struct {
	Columns      []Column    `json:"columns"`
	Rows         [][]*string `json:"rows"`
	RowCount     int         `json:"rowCount"`
	UpdateCount  int64       `json:"updateCount"`
	HasResultSet bool        `json:"hasResultSet"`
	Truncated    bool        `json:"truncated"`
	DurationMS   int64       `json:"durationMs"`
}

// SchemaSnapshot contains user schemas/tables and their columns.
type SchemaSnapshot struct {
	Tables []TableInfo `json:"tables"`
}

// TableInfo describes one table or view.
type TableInfo struct {
	Schema     string       `json:"schema"`
	Name       string       `json:"name"`
	Type       string       `json:"type"`
	PrimaryKey []string     `json:"primaryKey"`
	Columns    []ColumnInfo `json:"columns"`
}

// ColumnInfo describes one table column.
type ColumnInfo struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	Size          int    `json:"size"`
	Nullable      bool   `json:"nullable"`
	Ordinal       int    `json:"ordinal"`
	Default       string `json:"default"`
	AutoIncrement bool   `json:"autoIncrement"`
	Generated     bool   `json:"generated"`
}

// BridgeError represents a SQL or connection failure reported by the Java side.
type BridgeError struct {
	Message   string
	SQLState  string
	ErrorCode int
	ExitCode  int
}

func (e *BridgeError) Error() string {
	if e.SQLState != "" {
		return fmt.Sprintf("%s [SQLState %s]", e.Message, e.SQLState)
	}
	return e.Message
}

// Bridge invokes the embedded JdbcBridge class with the pinned H2 jar.
type Bridge struct {
	JavaPath string
	JarPath  string
	ClassDir string
}

// EnsureClass materializes the embedded bridge classes under dir.
func EnsureClass(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create bridge directory: %w", err)
	}
	entries, err := embeddedClasses.ReadDir(".")
	if err != nil {
		return "", fmt.Errorf("read embedded bridge classes: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".class") {
			continue
		}
		content, err := embeddedClasses.ReadFile(entry.Name())
		if err != nil {
			return "", fmt.Errorf("read embedded class %s: %w", entry.Name(), err)
		}
		destination := filepath.Join(dir, entry.Name())
		if existing, readErr := os.ReadFile(destination); readErr == nil && bytes.Equal(existing, content) {
			continue
		}
		if err := writeAtomic(destination, content); err != nil {
			return "", err
		}
	}
	return dir, nil
}

func writeAtomic(path string, content []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".bridge-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary bridge file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("write bridge file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close bridge file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("install bridge file: %w", err)
	}
	return nil
}

// Ping opens the database (creating it on first use) and verifies credentials.
func (b *Bridge) Ping(ctx context.Context, url, username, password string) (string, error) {
	response, err := b.run(ctx, url, username, password, "ping", "")
	if err != nil {
		return "", err
	}
	return response.ServerVersion, nil
}

// RunScript executes a multi-statement SQL script.
func (b *Bridge) RunScript(ctx context.Context, url, username, password, script string) error {
	_, err := b.run(ctx, url, username, password, "script", script)
	return err
}

// Execute runs exactly one SQL statement.
func (b *Bridge) Execute(ctx context.Context, url, username, password, sql string) (ExecResult, error) {
	response, err := b.run(ctx, url, username, password, "exec", sql)
	if err != nil {
		return ExecResult{}, err
	}
	result := ExecResult{
		Columns:      response.Columns,
		Rows:         response.Rows,
		RowCount:     response.RowCount,
		UpdateCount:  response.UpdateCount,
		HasResultSet: response.HasResultSet,
		Truncated:    response.Truncated,
		DurationMS:   response.DurationMS,
	}
	if result.Columns == nil {
		result.Columns = []Column{}
	}
	if result.Rows == nil {
		result.Rows = [][]*string{}
	}
	return result, nil
}

// Tables reads user table and column metadata.
func (b *Bridge) Tables(ctx context.Context, url, username, password string) (SchemaSnapshot, error) {
	response, err := b.run(ctx, url, username, password, "tables", "")
	if err != nil {
		return SchemaSnapshot{}, err
	}
	snapshot := SchemaSnapshot{Tables: response.Tables}
	if snapshot.Tables == nil {
		snapshot.Tables = []TableInfo{}
	}
	for i := range snapshot.Tables {
		if snapshot.Tables[i].PrimaryKey == nil {
			snapshot.Tables[i].PrimaryKey = []string{}
		}
		if snapshot.Tables[i].Columns == nil {
			snapshot.Tables[i].Columns = []ColumnInfo{}
		}
	}
	return snapshot, nil
}

type bridgeResponse struct {
	OK            bool        `json:"ok"`
	Error         string      `json:"error"`
	SQLState      string      `json:"sqlState"`
	ErrorCode     int         `json:"errorCode"`
	DurationMS    int64       `json:"durationMs"`
	ServerVersion string      `json:"serverVersion"`
	HasResultSet  bool        `json:"hasResultSet"`
	UpdateCount   int64       `json:"updateCount"`
	RowCount      int         `json:"rowCount"`
	Truncated     bool        `json:"truncated"`
	Columns       []Column    `json:"columns"`
	Rows          [][]*string `json:"rows"`
	Tables        []TableInfo `json:"tables"`
}

func (b *Bridge) run(ctx context.Context, url, username, password, mode, input string) (bridgeResponse, error) {
	if strings.TrimSpace(b.JavaPath) == "" {
		return bridgeResponse{}, errors.New("Java runtime is not available")
	}
	if strings.TrimSpace(b.JarPath) == "" {
		return bridgeResponse{}, errors.New("H2 engine jar is not available")
	}
	if strings.TrimSpace(b.ClassDir) == "" {
		return bridgeResponse{}, errors.New("JDBC bridge class directory is not available")
	}

	classpath := strings.Join([]string{b.JarPath, b.ClassDir}, string(os.PathListSeparator))
	cmd := executil.CommandContext(ctx, b.JavaPath,
		"-Dfile.encoding=UTF-8",
		"-Dstdout.encoding=UTF-8",
		"-Dstderr.encoding=UTF-8",
		"-cp", classpath,
		"JdbcBridge", mode,
	)
	cmd.Env = envWith(os.Environ(), map[string]string{
		"TAILSQL_JDBC_URL":      url,
		"TAILSQL_JDBC_USER":     username,
		"TAILSQL_JDBC_PASSWORD": password,
	})
	cmd.Stdin = strings.NewReader(input)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return bridgeResponse{}, fmt.Errorf("JDBC operation timed out: %w", ctxErr)
	}

	var response bridgeResponse
	decodeErr := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &response)

	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			return bridgeResponse{}, fmt.Errorf("launch JDBC bridge: %w", runErr)
		}
	}

	if decodeErr != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		if detail == "" {
			detail = runErr.Error()
		}
		return bridgeResponse{}, fmt.Errorf("JDBC bridge returned an unreadable response: %s", detail)
	}
	if !response.OK {
		message := response.Error
		if message == "" {
			message = "JDBC operation failed"
		}
		return bridgeResponse{}, &BridgeError{
			Message:   message,
			SQLState:  response.SQLState,
			ErrorCode: response.ErrorCode,
			ExitCode:  exitCode,
		}
	}
	return response, nil
}

func envWith(base []string, values map[string]string) []string {
	filtered := make([]string, 0, len(base)+len(values))
	for _, entry := range base {
		name := entry
		if index := strings.IndexByte(entry, '='); index >= 0 {
			name = entry[:index]
		}
		if _, replace := values[name]; replace {
			continue
		}
		filtered = append(filtered, entry)
	}
	for name, value := range values {
		filtered = append(filtered, name+"="+value)
	}
	return filtered
}
