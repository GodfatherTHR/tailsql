package main

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"tailsql/internal/config"
)

const (
	realtimePollInterval = time.Second
	// Writes made through TailSQL update the H2 file too. A short echo window
	// keeps the file watcher from emitting a duplicate "external" event.
	realtimeEchoWindow = 2 * time.Second
)

// RealtimeChangeEvent is pushed to the frontend whenever a database changes,
// either through TailSQL or through another JDBC client.
type RealtimeChangeEvent struct {
	InstanceID  string `json:"instanceId"`
	Schema      string `json:"schema,omitempty"`
	Table       string `json:"table,omitempty"`
	Operation   string `json:"operation"`
	Source      string `json:"source"`
	UpdateCount int64  `json:"updateCount,omitempty"`
	At          string `json:"at"`
}

type databaseStamp struct {
	modified time.Time
	size     int64
}

var (
	insertTargetPattern = regexp.MustCompile(`(?is)^\s*INSERT\s+INTO\s+([^\s(;]+)`)
	mergeTargetPattern  = regexp.MustCompile(`(?is)^\s*MERGE\s+INTO\s+([^\s(;]+)`)
	updateTargetPattern = regexp.MustCompile(`(?is)^\s*UPDATE\s+([^\s(;]+)`)
	deleteTargetPattern = regexp.MustCompile(`(?is)^\s*DELETE\s+FROM\s+([^\s(;]+)`)
	ddlTargetPattern    = regexp.MustCompile(`(?is)^\s*(CREATE|ALTER|DROP|TRUNCATE|RENAME)\s+(TABLE|VIEW|SCHEMA|INDEX|SEQUENCE|TRIGGER|ALIAS)\s+(?:IF\s+(?:NOT\s+)?EXISTS\s+)?([^\s(;]+)`)
)

// startRealtimeWatcher observes the H2 database files of running instances.
// H2 appends committed transactions to the .mv.db file, so a file change is a
// reliable signal that data changed even when another client made the write.
func (a *App) startRealtimeWatcher() {
	ctx, cancel := context.WithCancel(context.Background())
	a.mu.Lock()
	a.realtimeCancel = cancel
	if a.realtimeStamps == nil {
		a.realtimeStamps = map[string]databaseStamp{}
	}
	if a.realtimeEcho == nil {
		a.realtimeEcho = map[string]time.Time{}
	}
	a.mu.Unlock()
	go a.watchRealtime(ctx)
}

func (a *App) stopRealtimeWatcher() {
	a.mu.Lock()
	cancel := a.realtimeCancel
	a.realtimeCancel = nil
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (a *App) watchRealtime(ctx context.Context) {
	ticker := time.NewTicker(realtimePollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.scanRealtimeChanges()
		}
	}
}

func (a *App) scanRealtimeChanges() {
	manager, _, _ := a.components()
	if manager == nil {
		return
	}
	dbDir := filepath.Join(manager.DataDir(), "dbs")
	next := make(map[string]databaseStamp)
	now := time.Now()

	for _, instance := range manager.List() {
		if instance.Status != config.StatusRunning {
			continue
		}
		fileName := instance.DatabaseFile
		if strings.TrimSpace(fileName) == "" {
			fileName = instance.Name + ".mv.db"
		}
		info, err := os.Stat(filepath.Join(dbDir, fileName))
		if err != nil {
			continue
		}
		stamp := databaseStamp{modified: info.ModTime(), size: info.Size()}
		next[instance.ID] = stamp

		a.mu.Lock()
		previous, known := a.realtimeStamps[instance.ID]
		lastWrite := a.realtimeEcho[instance.ID]
		a.mu.Unlock()
		if !known || stamp == previous {
			continue
		}
		if !lastWrite.IsZero() && now.Sub(lastWrite) < realtimeEchoWindow {
			continue
		}
		a.emit("realtime:change", RealtimeChangeEvent{
			InstanceID: instance.ID,
			Operation:  "external",
			Source:     "h2",
			At:         now.UTC().Format(time.RFC3339Nano),
		})
	}

	a.mu.Lock()
	a.realtimeStamps = next
	a.mu.Unlock()
}

// emitRealtimeChange records app-originated writes so the file watcher can
// ignore their echo, then pushes the change to the frontend.
func (a *App) emitRealtimeChange(event RealtimeChangeEvent) {
	if event.At == "" {
		event.At = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if event.Source == "" {
		event.Source = "tailsql"
	}
	if event.Operation != "external" {
		a.markDatabaseActivity(event.InstanceID)
	}
	a.emit("realtime:change", event)
}

// markDatabaseActivity records that TailSQL itself just used the database. H2
// rewrites the .mv.db file when a database is opened and closed, even for a
// plain SELECT, so without this a table read would look like an external
// change and immediately trigger another read.
func (a *App) markDatabaseActivity(instanceID string) {
	if instanceID == "" {
		return
	}
	a.mu.Lock()
	if a.realtimeEcho == nil {
		a.realtimeEcho = map[string]time.Time{}
	}
	a.realtimeEcho[instanceID] = time.Now()
	a.mu.Unlock()
}

// mutationTarget extracts the operation and target of a mutating SQL statement.
// It intentionally returns no target for queries and unknown statements.
func mutationTarget(sql string) (operation, schema, table string) {
	statement := trimSQLPrefix(sql)
	if statement == "" {
		return "", "", ""
	}
	for _, candidate := range []struct {
		pattern   *regexp.Regexp
		operation string
	}{
		{insertTargetPattern, "insert"},
		{mergeTargetPattern, "merge"},
		{updateTargetPattern, "update"},
		{deleteTargetPattern, "delete"},
	} {
		if match := candidate.pattern.FindStringSubmatch(statement); match != nil {
			schema, table = splitQualifiedName(match[1])
			return candidate.operation, schema, table
		}
	}
	if match := ddlTargetPattern.FindStringSubmatch(statement); match != nil {
		objectType := strings.ToUpper(strings.TrimSpace(match[2]))
		target := match[3]
		if objectType == "SCHEMA" {
			return "ddl", unquoteIdentifier(target), ""
		}
		schema, table = splitQualifiedName(target)
		return "ddl", schema, table
	}
	return "", "", ""
}

func trimSQLPrefix(sql string) string {
	value := strings.TrimSpace(sql)
	for value != "" {
		switch {
		case strings.HasPrefix(value, "--"):
			if index := strings.IndexByte(value, '\n'); index >= 0 {
				value = strings.TrimSpace(value[index+1:])
				continue
			}
			return ""
		case strings.HasPrefix(value, "/*"):
			if index := strings.Index(value, "*/"); index >= 0 {
				value = strings.TrimSpace(value[index+2:])
				continue
			}
			return ""
		default:
			return value
		}
	}
	return ""
}

func splitQualifiedName(raw string) (schema, table string) {
	parts := splitIdentifierParts(strings.TrimSpace(raw))
	switch len(parts) {
	case 0:
		return "", ""
	case 1:
		return "", unquoteIdentifier(parts[0])
	default:
		return unquoteIdentifier(parts[len(parts)-2]), unquoteIdentifier(parts[len(parts)-1])
	}
}

func splitIdentifierParts(raw string) []string {
	var parts []string
	var current strings.Builder
	quoted := false
	for i := 0; i < len(raw); i++ {
		switch char := raw[i]; {
		case char == '"':
			if quoted && i+1 < len(raw) && raw[i+1] == '"' {
				current.WriteByte('"')
				i++
				continue
			}
			quoted = !quoted
			current.WriteByte(char)
		case char == '.' && !quoted:
			parts = append(parts, current.String())
			current.Reset()
		default:
			current.WriteByte(char)
		}
	}
	parts = append(parts, current.String())
	return parts
}

func unquoteIdentifier(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		return strings.ReplaceAll(value[1:len(value)-1], `""`, `"`)
	}
	return value
}
