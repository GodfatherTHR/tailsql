package main

import (
	"hysql/internal/config"
	"hysql/internal/dbmanager"
	"hysql/internal/jdbc"
	"hysql/internal/tailscale"
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

// TableDropRequest removes one table and all of its rows.
type TableDropRequest struct {
	ID     string `json:"id"`
	Schema string `json:"schema"`
	Table  string `json:"table"`
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

// MCPConfigRequest updates the embedded MCP server.
type MCPConfigRequest struct {
	Enabled          bool `json:"enabled"`
	Port             int  `json:"port"`
	TailscaleEnabled bool `json:"tailscaleEnabled"`
}

// RESTConfigRequest updates the Auto-REST gateway.
type RESTConfigRequest struct {
	Enabled          bool `json:"enabled"`
	Port             int  `json:"port"`
	TailscaleEnabled bool `json:"tailscaleEnabled"`
	FunnelEnabled    bool `json:"funnelEnabled"`
	FunnelPort       int  `json:"funnelPort"`
	AllowPublicRead  bool `json:"allowPublicRead"`
}

// AgentSafetyRequest toggles agent guardrails.
type AgentSafetyRequest struct {
	ReadOnly               bool `json:"readOnly"`
	AutoSnapshotOnAgentDDL bool `json:"autoSnapshotOnAgentDdl"`
}

// TokenRotateRequest selects which agent token to rotate.
type TokenRotateRequest struct {
	Target string `json:"target"`
}

// AgentSafetyView is the safety state shown in the UI.
type AgentSafetyView struct {
	ReadOnly               bool `json:"readOnly"`
	AutoSnapshotOnAgentDDL bool `json:"autoSnapshotOnAgentDdl"`
}

// MCPStateView describes the MCP server for the frontend.
type MCPStateView struct {
	Running          bool   `json:"running"`
	Enabled          bool   `json:"enabled"`
	Port             int    `json:"port"`
	BindAddress      string `json:"bindAddress"`
	Endpoint         string `json:"endpoint"`
	Token            string `json:"token"`
	TailscaleEnabled bool   `json:"tailscaleEnabled"`
	Error            string `json:"error"`
}

// RESTStateView describes the Auto-REST gateway for the frontend.
type RESTStateView struct {
	Running          bool   `json:"running"`
	Enabled          bool   `json:"enabled"`
	Port             int    `json:"port"`
	BindAddress      string `json:"bindAddress"`
	BaseURL          string `json:"baseUrl"`
	LocalURL         string `json:"localUrl"`
	PublicBaseURL    string `json:"publicBaseUrl"`
	APIKey           string `json:"apiKey"`
	TailscaleEnabled bool   `json:"tailscaleEnabled"`
	FunnelEnabled    bool   `json:"funnelEnabled"`
	FunnelPort       int    `json:"funnelPort"`
	AllowPublicRead  bool   `json:"allowPublicRead"`
	DatabaseCount    int    `json:"databaseCount"`
	Error            string `json:"error"`
}

// AISettingsView is the sanitized AI configuration for the frontend.
type AISettingsView struct {
	Provider  string `json:"provider"`
	BaseURL   string `json:"baseUrl"`
	Model     string `json:"model"`
	HasAPIKey bool   `json:"hasApiKey"`
}

// AIProviderSettings updates the provider configuration. An empty APIKey keeps
// the stored key unless ClearAPIKey is set.
type AIProviderSettings struct {
	Provider    string `json:"provider"`
	BaseURL     string `json:"baseUrl"`
	Model       string `json:"model"`
	APIKey      string `json:"apiKey"`
	ClearAPIKey bool   `json:"clearApiKey"`
}

// AgentConfigView is the complete Connect-to-AI payload.
type AgentConfigView struct {
	MCP                MCPStateView      `json:"mcp"`
	REST               RESTStateView     `json:"rest"`
	Safety             AgentSafetyView   `json:"safety"`
	AI                 AISettingsView    `json:"ai"`
	Snippets           map[string]string `json:"snippets"`
	SystemPrompt       string            `json:"systemPrompt"`
	TailscaleIP        string            `json:"tailscaleIp"`
	TailscaleAvailable bool              `json:"tailscaleAvailable"`
}

// SnapshotView is one checkpoint shown in the drawer.
type SnapshotView struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	SizeBytes int64  `json:"sizeBytes"`
	CreatedAt string `json:"createdAt"`
}

// SnapshotRequest selects a database and optionally a checkpoint.
type SnapshotRequest struct {
	ID         string `json:"id"`
	SnapshotID string `json:"snapshotId"`
	Label      string `json:"label"`
}

// GenerateSQLRequest asks the LLM for one H2 statement.
type GenerateSQLRequest struct {
	ID     string `json:"id"`
	Prompt string `json:"prompt"`
}

// GenerateSQLResult returns generated SQL and provider metadata.
type GenerateSQLResult struct {
	SQL      string `json:"sql"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// SeedRowsRequest generates realistic rows for one table.
type SeedRowsRequest struct {
	ID     string `json:"id"`
	Schema string `json:"schema"`
	Table  string `json:"table"`
	Count  int    `json:"count"`
}

// SeedRowsResult reports how many rows were inserted.
type SeedRowsResult struct {
	Inserted    int    `json:"inserted"`
	GeneratedBy string `json:"generatedBy"`
}
