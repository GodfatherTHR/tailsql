package main

import (
	"tailsql/internal/config"
	"tailsql/internal/dbmanager"
	"tailsql/internal/jdbc"
	"tailsql/internal/tailscale"
)

// SystemHealth is the diagnostics payload rendered by the UI.
type SystemHealth struct {
	AppVersion  string             `json:"appVersion"`
	DataDir     string             `json:"dataDir"`
	Java        dbmanager.JavaInfo `json:"java"`
	H2          dbmanager.H2Info   `json:"h2"`
	Tailscale   tailscale.Status   `json:"tailscale"`
	Warnings    []string           `json:"warnings"`
	Initialized bool               `json:"initialized"`
	Error       string             `json:"error"`
}

// InstanceView is the frontend-safe projection of a database instance. It
// deliberately omits ServerPassword.
type InstanceView struct {
	ID               string        `json:"id"`
	Name             string        `json:"name"`
	Username         string        `json:"username"`
	Password         string        `json:"password"`
	Port             int           `json:"port"`
	Status           config.Status `json:"status"`
	PID              int           `json:"pid"`
	TailscalePort    int           `json:"tailscalePort"`
	TailscaleEnabled bool          `json:"tailscaleEnabled"`
	TailscaleIP      string        `json:"tailscaleIp"`
	FunnelEnabled    bool          `json:"funnelEnabled"`
	FunnelPort       int           `json:"funnelPort"`
	FunnelHost       string        `json:"funnelHost"`
	FunnelURL        string        `json:"funnelUrl"`
	FunnelJDBCURL    string        `json:"funnelJdbcUrl"`
	DatabaseFile     string        `json:"databaseFile"`
	FileSizeBytes    int64         `json:"fileSizeBytes"`
	CreatedAt        string        `json:"createdAt"`
	UpdatedAt        string        `json:"updatedAt"`
	StartedAt        string        `json:"startedAt,omitempty"`
	LastError        string        `json:"lastError"`
	JDBCURL          string        `json:"jdbcUrl"`
	TailscaleURL     string        `json:"tailscaleUrl"`
	LogPath          string        `json:"logPath"`
}

// CreateInstanceRequest is the create-modal payload.
type CreateInstanceRequest struct {
	Name     string `json:"name"`
	Username string `json:"username"`
	Password string `json:"password"`
	Port     int    `json:"port"`
}

// JavaCodegenRequest selects an instance and connection mode.
type JavaCodegenRequest struct {
	ID   string `json:"id"`
	Mode string `json:"mode"`
}

// QueryRequest carries one SQL statement.
type QueryRequest struct {
	ID  string `json:"id"`
	SQL string `json:"sql"`
}

// QueryResult is a tabular SQL result with timing.
type QueryResult struct {
	Columns          []jdbc.Column `json:"columns"`
	Rows             [][]*string   `json:"rows"`
	RowCount         int           `json:"rowCount"`
	UpdateCount      int64         `json:"updateCount"`
	HasResultSet     bool          `json:"hasResultSet"`
	Truncated        bool          `json:"truncated"`
	EngineDurationMS int64         `json:"engineDurationMs"`
	DurationMS       int64         `json:"durationMs"`
	ExecutedAt       string        `json:"executedAt"`
}

// LogRequest asks for the tail of an instance's server log.
type LogRequest struct {
	ID    string `json:"id"`
	Lines int    `json:"lines"`
}

// TableRowsRequest asks for one page of rows in a table.
type TableRowsRequest struct {
	ID         string `json:"id"`
	Schema     string `json:"schema"`
	Table      string `json:"table"`
	Limit      int    `json:"limit"`
	Offset     int    `json:"offset"`
	OrderBy    string `json:"orderBy"`
	Descending bool   `json:"descending"`
}

// TableRowsResult is one page of table data.
type TableRowsResult struct {
	Columns []jdbc.Column `json:"columns"`
	Rows    [][]*string   `json:"rows"`
	HasMore bool          `json:"hasMore"`
	Limit   int           `json:"limit"`
	Offset  int           `json:"offset"`
}

// TableMutationRequest carries values and/or a primary-key selector.
type TableMutationRequest struct {
	ID     string             `json:"id"`
	Schema string             `json:"schema"`
	Table  string             `json:"table"`
	Values map[string]*string `json:"values"`
	Key    map[string]*string `json:"key"`
}

// TableMutationResult reports how many rows a mutation affected.
type TableMutationResult struct {
	UpdateCount int64 `json:"updateCount"`
}

// FunnelRequest selects the public port for Tailscale Funnel.
type FunnelRequest struct {
	ID   string `json:"id"`
	Port int    `json:"port"`
}

// AppErrorEvent is emitted when a background operation fails.
type AppErrorEvent struct {
	InstanceID string `json:"instanceId,omitempty"`
	Message    string `json:"message"`
}
