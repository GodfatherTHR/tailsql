// Package snapshots stores and restores consistent .mv.db checkpoints.
package snapshots

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	defaultLabel = "checkpoint"
	timeLayout   = "20060102T150405.000000000Z"
	maxLabelLen  = 48
)

// Snapshot is one checkpoint file on disk.
type Snapshot struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Path      string `json:"-"`
	SizeBytes int64  `json:"sizeBytes"`
	CreatedAt string `json:"createdAt"`
}

// Store owns the ~/.hysql/snapshots tree.
type Store struct {
	root string
	now  func() time.Time
}

// New creates the snapshot root when needed.
func New(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("snapshot root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve snapshot root: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("create snapshot root: %w", err)
	}
	return &Store{root: absolute, now: time.Now}, nil
}

// Root returns the snapshot root directory.
func (s *Store) Root() string { return s.root }

// Dir returns the per-database snapshot directory.
func (s *Store) Dir(dbName string) string {
	return filepath.Join(s.root, safeSegment(dbName))
}

// List returns snapshots for one database, newest first.
func (s *Store) List(dbName string) ([]Snapshot, error) {
	entries, err := os.ReadDir(s.Dir(dbName))
	if errors.Is(err, os.ErrNotExist) {
		return []Snapshot{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read snapshot directory: %w", err)
	}
	out := make([]Snapshot, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".mv.db") {
			continue
		}
		info, statErr := entry.Info()
		if statErr != nil {
			continue
		}
		created, label := parseName(entry.Name())
		out = append(out, Snapshot{
			ID:        entry.Name(),
			Label:     label,
			Path:      filepath.Join(s.Dir(dbName), entry.Name()),
			SizeBytes: info.Size(),
			CreatedAt: created.UTC().Format(time.RFC3339),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt > out[j].CreatedAt
	})
	return out, nil
}

// SaveDatabaseFile copies a stopped database file into the snapshot store.
func (s *Store) SaveDatabaseFile(dbName, databasePath, label string) (Snapshot, error) {
	source, err := os.Open(databasePath)
	if err != nil {
		return Snapshot{}, fmt.Errorf("open database file: %w", err)
	}
	defer source.Close()
	return s.writeSnapshot(dbName, label, source)
}

// SaveBackup extracts the .mv.db member from an H2 BACKUP zip archive.
func (s *Store) SaveBackup(dbName, zipPath, label string) (Snapshot, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return Snapshot{}, fmt.Errorf("open H2 backup archive: %w", err)
	}
	defer reader.Close()
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		lower := strings.ToLower(file.Name)
		if !strings.HasSuffix(lower, ".mv.db") {
			continue
		}
		member, openErr := file.Open()
		if openErr != nil {
			return Snapshot{}, fmt.Errorf("open backup member: %w", openErr)
		}
		defer member.Close()
		return s.writeSnapshot(dbName, label, member)
	}
	return Snapshot{}, errors.New("the H2 backup archive did not contain a .mv.db file")
}

// Delete removes one snapshot.
func (s *Store) Delete(dbName, id string) error {
	path, err := s.path(dbName, id)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("checkpoint %q was not found", id)
		}
		return fmt.Errorf("delete checkpoint: %w", err)
	}
	return nil
}

// RestorePath validates a snapshot and returns its absolute path.
func (s *Store) RestorePath(dbName, id string) (string, error) {
	return s.path(dbName, id)
}

// Restore copies a snapshot over the target database file atomically. The
// caller must stop the H2 server first.
func (s *Store) Restore(dbName, id, targetPath string) error {
	sourcePath, err := s.path(dbName, id)
	if err != nil {
		return err
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open checkpoint: %w", err)
	}
	defer source.Close()

	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create database directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".restore-*.mv.db")
	if err != nil {
		return fmt.Errorf("create restore temporary file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	if _, err := io.Copy(tmp, source); err != nil {
		cleanup()
		return fmt.Errorf("copy checkpoint: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("flush restored database: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close restored database: %w", err)
	}

	for _, suffix := range []string{"", ".trace.db", ".lock.db", ".temp.db"} {
		if removeErr := os.Remove(targetPath + suffix); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			_ = os.Remove(tmpName)
			return fmt.Errorf("remove current database file: %w", removeErr)
		}
	}
	if err := os.Rename(tmpName, targetPath); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("install restored database: %w", err)
	}
	return nil
}

// Prune keeps the newest keep snapshots and removes older ones.
func (s *Store) Prune(dbName string, keep int) error {
	if keep <= 0 {
		return nil
	}
	list, err := s.List(dbName)
	if err != nil {
		return err
	}
	for index := keep; index < len(list); index++ {
		_ = os.Remove(list[index].Path)
	}
	return nil
}

func (s *Store) writeSnapshot(dbName, label string, source io.Reader) (Snapshot, error) {
	dir := s.Dir(dbName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Snapshot{}, fmt.Errorf("create snapshot directory: %w", err)
	}
	created := s.now().UTC()
	base := created.Format(timeLayout) + "_" + sanitizeLabel(label)
	name := base + ".mv.db"
	for attempt := 2; ; attempt++ {
		if _, err := os.Stat(filepath.Join(dir, name)); errors.Is(err, os.ErrNotExist) {
			break
		}
		name = base + "-" + strconv.Itoa(attempt) + ".mv.db"
	}
	finalPath := filepath.Join(dir, name)

	tmp, err := os.CreateTemp(dir, ".snapshot-*.tmp")
	if err != nil {
		return Snapshot{}, fmt.Errorf("create snapshot temporary file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	written, err := io.Copy(tmp, source)
	if err != nil {
		cleanup()
		return Snapshot{}, fmt.Errorf("write snapshot: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return Snapshot{}, fmt.Errorf("flush snapshot: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return Snapshot{}, fmt.Errorf("close snapshot: %w", err)
	}
	if written == 0 {
		_ = os.Remove(tmpName)
		return Snapshot{}, errors.New("database backup was empty")
	}
	if err := os.Rename(tmpName, finalPath); err != nil {
		_ = os.Remove(tmpName)
		return Snapshot{}, fmt.Errorf("install snapshot: %w", err)
	}
	return Snapshot{
		ID:        name,
		Label:     sanitizeLabel(label),
		Path:      finalPath,
		SizeBytes: written,
		CreatedAt: created.Format(time.RFC3339),
	}, nil
}

func (s *Store) path(dbName, id string) (string, error) {
	if strings.TrimSpace(id) == "" || filepath.Base(id) != id {
		return "", fmt.Errorf("invalid checkpoint id %q", id)
	}
	if !strings.HasSuffix(strings.ToLower(id), ".mv.db") {
		return "", fmt.Errorf("invalid checkpoint id %q", id)
	}
	full := filepath.Join(s.Dir(dbName), id)
	if _, err := os.Stat(full); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("checkpoint %q was not found", id)
		}
		return "", fmt.Errorf("read checkpoint: %w", err)
	}
	return full, nil
}

func parseName(name string) (time.Time, string) {
	trimmed := strings.TrimSuffix(name, filepath.Ext(name))
	trimmed = strings.TrimSuffix(trimmed, ".mv")
	parts := strings.SplitN(trimmed, "_", 2)
	created, err := time.Parse(timeLayout, parts[0])
	if err != nil {
		created = time.Time{}
	}
	label := defaultLabel
	if len(parts) > 1 {
		label = strings.ReplaceAll(parts[1], "-", " ")
	}
	return created, label
}

func sanitizeLabel(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return defaultLabel
	}
	var builder strings.Builder
	for _, char := range strings.ToLower(label) {
		switch {
		case unicode.IsLetter(char) || unicode.IsDigit(char):
			builder.WriteRune(char)
		case char == '-' || char == '_' || char == ' ':
			builder.WriteByte('-')
		}
		if builder.Len() >= maxLabelLen {
			break
		}
	}
	out := strings.Trim(builder.String(), "-")
	if out == "" {
		return defaultLabel
	}
	return out
}

func safeSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "database"
	}
	var builder strings.Builder
	for _, char := range value {
		if unicode.IsLetter(char) || unicode.IsDigit(char) || char == '_' || char == '-' || char == '.' {
			builder.WriteRune(char)
		} else {
			builder.WriteByte('-')
		}
	}
	return builder.String()
}
