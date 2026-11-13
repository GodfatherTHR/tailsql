// Package sqlrunner exposes the application-level SQL operations on top of the
// JDBC bridge.
package sqlrunner

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"hysql/internal/jdbc"
)

// MaxQueryLength protects the IPC boundary from oversized payloads.
const MaxQueryLength = 200_000

// ConnectionProvider resolves the loopback JDBC details for a running instance.
type ConnectionProvider interface {
	ConnectionInfo(id string) (url, username, password string, err error)
}

// Runner executes SQL and metadata reads for one request.
type Runner struct {
	provider ConnectionProvider
	bridge   *jdbc.Bridge
}

// New builds a runner. The bridge is stateless and may be created per request
// so it always reflects the latest Java/H2 discovery.
func New(provider ConnectionProvider, bridge *jdbc.Bridge) *Runner {
	return &Runner{provider: provider, bridge: bridge}
}

// Execute runs exactly one SQL statement.
func (r *Runner) Execute(ctx context.Context, instanceID, sql string) (jdbc.ExecResult, error) {
	query := strings.TrimSpace(sql)
	if query == "" {
		return jdbc.ExecResult{}, errors.New("enter a SQL statement to run")
	}
	if len(query) > MaxQueryLength {
		return jdbc.ExecResult{}, fmt.Errorf("SQL statement is too large (%d characters; limit %d)", len(query), MaxQueryLength)
	}
	url, username, password, err := r.provider.ConnectionInfo(instanceID)
	if err != nil {
		return jdbc.ExecResult{}, err
	}
	return r.bridge.Execute(ctx, url, username, password, query)
}

// Tables reads the current user schema.
func (r *Runner) Tables(ctx context.Context, instanceID string) (jdbc.SchemaSnapshot, error) {
	url, username, password, err := r.provider.ConnectionInfo(instanceID)
	if err != nil {
		return jdbc.SchemaSnapshot{}, err
	}
	return r.bridge.Tables(ctx, url, username, password)
}
