// Package mcp implements a Model Context Protocol server over HTTP and SSE.
package mcp

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"hysql/internal/jdbc"
)

const (
	protocolVersion = "2024-11-05"
	maxBodyBytes    = 4 << 20
	maxQueryLength  = 200_000
)

// Database is the agent-facing database summary.
type Database struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Status           string `json:"status"`
	LocalJDBCURL     string `json:"localJdbcUrl"`
	TailscaleJDBCURL string `json:"tailscaleJdbcUrl,omitempty"`
	RESTBaseURL      string `json:"restBaseUrl,omitempty"`
}

// ConnectionBundle is everything an external agent needs to build an app.
type ConnectionBundle struct {
	Database         string `json:"database"`
	LocalJDBCURL     string `json:"localJdbcUrl"`
	TailscaleJDBCURL string `json:"tailscaleJdbcUrl,omitempty"`
	Username         string `json:"username"`
	Password         string `json:"password"`
	HikariCP         string `json:"hikariCp"`
	SpringProperties string `json:"springProperties"`
	RESTBaseURL      string `json:"restBaseUrl,omitempty"`
	RESTAPIKey       string `json:"restApiKey,omitempty"`
	OpenAPIURL       string `json:"openapiUrl,omitempty"`
}

// DDLResult reports the outcome of execute_ddl.
type DDLResult struct {
	UpdateCount int64  `json:"updateCount"`
	Checkpoint  string `json:"checkpoint,omitempty"`
}

// Backend is implemented by the desktop app adapter.
type Backend interface {
	ListDatabases() []Database
	ResolveDatabase(name string) (Database, error)
	InspectSchema(ctx context.Context, databaseName, schemaName string) (jdbc.SchemaSnapshot, error)
	ReadQuery(ctx context.Context, databaseName, sql string) (jdbc.ExecResult, error)
	WriteQuery(ctx context.Context, databaseName, sql string) (jdbc.ExecResult, error)
	ExecuteDDL(ctx context.Context, databaseName, sql string) (DDLResult, error)
	ConnectionBundle(ctx context.Context, databaseName string) (ConnectionBundle, error)
	AgentReadOnly() bool
}

// Config controls the MCP listener.
type Config struct {
	BindAddress string
	Port        int
	Token       string
	Version     string
}

// State is the UI-facing server snapshot.
type State struct {
	Running     bool   `json:"running"`
	BindAddress string `json:"bindAddress"`
	Endpoint    string `json:"endpoint"`
	Error       string `json:"error"`
	CheckedAt   string `json:"checkedAt"`
}

// Server is the MCP HTTP/SSE server.
type Server struct {
	mu       sync.Mutex
	cfg      Config
	backend  Backend
	http     *http.Server
	listener net.Listener
	state    State

	sessionsMu sync.Mutex
	sessions   map[string]chan []byte
}

// New builds a stopped MCP server.
func New(cfg Config, backend Backend) *Server {
	server := &Server{
		cfg:      cfg,
		backend:  backend,
		sessions: map[string]chan []byte{},
	}
	server.state = State{BindAddress: bindAddress(cfg), Endpoint: endpoint(cfg), CheckedAt: now()}
	return server
}

// Start binds the listener.
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.http != nil {
		return nil
	}
	if s.backend == nil {
		s.state.Error = "MCP backend is not available"
		s.state.CheckedAt = now()
		return errors.New(s.state.Error)
	}
	address := bindAddress(s.cfg)
	if err := validateBindHost(s.cfg.BindAddress); err != nil {
		s.state.Error = err.Error()
		s.state.CheckedAt = now()
		return err
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		s.state.Error = fmt.Sprintf("listen on %s: %v", address, err)
		s.state.CheckedAt = now()
		return fmt.Errorf("start MCP server on %s: %w", address, err)
	}
	s.listener = listener
	s.http = &http.Server{
		Handler:           s,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	s.state = State{
		Running:     true,
		BindAddress: listener.Addr().String(),
		Endpoint:    endpoint(s.cfg),
		CheckedAt:   now(),
	}
	go func(server *http.Server) {
		err := server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.mu.Lock()
			s.state.Error = err.Error()
			s.state.Running = false
			s.state.CheckedAt = now()
			s.mu.Unlock()
		}
	}(s.http)
	return nil
}

// Stop shuts the server and all SSE sessions down.
func (s *Server) Stop() {
	s.mu.Lock()
	httpServer := s.http
	s.http = nil
	s.listener = nil
	s.mu.Unlock()
	if httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = httpServer.Shutdown(ctx)
		cancel()
	}
	s.sessionsMu.Lock()
	for id, channel := range s.sessions {
		close(channel)
		delete(s.sessions, id)
	}
	s.sessionsMu.Unlock()
	s.mu.Lock()
	s.state.Running = false
	s.state.BindAddress = bindAddress(s.cfg)
	s.state.CheckedAt = now()
	s.mu.Unlock()
}

// UpdateConfig replaces settings; the caller restarts the server.
func (s *Server) UpdateConfig(cfg Config) {
	s.mu.Lock()
	s.cfg = cfg
	s.state.CheckedAt = now()
	s.mu.Unlock()
}

// State returns the current snapshot.
func (s *Server) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.state
	state.CheckedAt = now()
	return state
}

func (s *Server) config() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

func (s *Server) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	setCORS(response)
	if request.Method == http.MethodOptions {
		response.WriteHeader(http.StatusNoContent)
		return
	}
	if request.URL.Path == "/health" {
		writeJSON(response, http.StatusOK, map[string]any{
			"ok": true, "server": "hysql-mcp", "version": s.config().Version,
		})
		return
	}
	if !s.authorized(request) {
		writeJSON(response, http.StatusUnauthorized, map[string]any{
			"jsonrpc": "2.0",
			"error":   map[string]any{"code": -32001, "message": "A valid Bearer token is required."},
			"id":      nil,
		})
		return
	}
	switch {
	case request.URL.Path == "/mcp" && request.Method == http.MethodPost:
		s.handleJSONRPC(response, request)
	case request.URL.Path == "/mcp" && request.Method == http.MethodGet:
		s.handleStream(response, request)
	case request.URL.Path == "/mcp" && request.Method == http.MethodDelete:
		response.WriteHeader(http.StatusNoContent)
	case request.URL.Path == "/sse" && request.Method == http.MethodGet:
		s.handleLegacySSE(response, request)
	case request.URL.Path == "/messages" && request.Method == http.MethodPost:
		s.handleLegacyMessage(response, request)
	default:
		writeJSON(response, http.StatusNotFound, map[string]any{
			"jsonrpc": "2.0",
			"error":   map[string]any{"code": -32601, "message": "Unknown MCP route."},
			"id":      nil,
		})
	}
}

func (s *Server) authorized(request *http.Request) bool {
	token := strings.TrimSpace(s.config().Token)
	if token == "" {
		return true
	}
	provided := strings.TrimSpace(request.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(provided), "bearer ") {
		provided = strings.TrimSpace(provided[7:])
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(token)) == 1
}

func (s *Server) handleJSONRPC(response http.ResponseWriter, request *http.Request) {
	body, err := readBody(request)
	if err != nil {
		writeJSON(response, http.StatusBadRequest, rpcErrorResponse(nil, -32700, err.Error()))
		return
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		writeJSON(response, http.StatusBadRequest, rpcErrorResponse(nil, -32600, "Request body is empty."))
		return
	}
	if strings.HasPrefix(trimmed, "[") {
		var batch []rpcRequest
		if err := json.Unmarshal(body, &batch); err != nil {
			writeJSON(response, http.StatusBadRequest, rpcErrorResponse(nil, -32700, "Invalid JSON."))
			return
		}
		responses := make([]rpcResponse, 0, len(batch))
		for _, entry := range batch {
			if entry.isNotification() {
				s.dispatch(request.Context(), entry)
				continue
			}
			responses = append(responses, s.dispatch(request.Context(), entry))
		}
		if len(responses) == 0 {
			response.WriteHeader(http.StatusAccepted)
			return
		}
		writeJSON(response, http.StatusOK, responses)
		return
	}
	var entry rpcRequest
	if err := json.Unmarshal(body, &entry); err != nil {
		writeJSON(response, http.StatusBadRequest, rpcErrorResponse(nil, -32700, "Invalid JSON."))
		return
	}
	if entry.isNotification() {
		s.dispatch(request.Context(), entry)
		response.WriteHeader(http.StatusAccepted)
		return
	}
	writeJSON(response, http.StatusOK, s.dispatch(request.Context(), entry))
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func (r rpcRequest) isNotification() bool {
	return len(r.ID) == 0 || string(r.ID) == "null"
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func rpcErrorResponse(id json.RawMessage, code int, message string) rpcResponse {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	return rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message}}
}

func (s *Server) dispatch(ctx context.Context, request rpcRequest) rpcResponse {
	id := request.ID
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	switch request.Method {
	case "initialize":
		var params struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(request.Params, &params)
		version := strings.TrimSpace(params.ProtocolVersion)
		if version == "" {
			version = protocolVersion
		}
		return rpcResponse{JSONRPC: "2.0", ID: id, Result: map[string]any{
			"protocolVersion": version,
			"capabilities": map[string]any{
				"tools":     map[string]any{"listChanged": false},
				"resources": map[string]any{"subscribe": false, "listChanged": false},
				"prompts":   map[string]any{"listChanged": false},
			},
			"serverInfo": map[string]any{
				"name":    "hysql",
				"version": s.config().Version,
			},
			"instructions": "HySQL exposes live H2 databases. Call list_databases first, then inspect_schema before writing SQL. Use read_query for SELECT, write_query for DML, execute_ddl for schema changes, and get_connection_bundle to build an app with JDBC or the Auto-REST API.",
		}}
	case "ping":
		return rpcResponse{JSONRPC: "2.0", ID: id, Result: map[string]any{}}
	case "tools/list":
		return rpcResponse{JSONRPC: "2.0", ID: id, Result: map[string]any{"tools": toolDefinitions()}}
	case "tools/call":
		return s.callTool(ctx, id, request.Params)
	case "resources/list":
		return rpcResponse{JSONRPC: "2.0", ID: id, Result: map[string]any{"resources": s.resourceList()}}
	case "resources/read":
		return s.readResource(id, request.Params)
	case "resources/templates/list":
		return rpcResponse{JSONRPC: "2.0", ID: id, Result: map[string]any{"resourceTemplates": []any{}}}
	case "prompts/list":
		return rpcResponse{JSONRPC: "2.0", ID: id, Result: map[string]any{"prompts": []any{}}}
	default:
		return rpcErrorResponse(id, -32601, "Unknown method: "+request.Method)
	}
}

func (s *Server) callTool(ctx context.Context, id json.RawMessage, params json.RawMessage) rpcResponse {
	var call struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &call); err != nil {
		return rpcErrorResponse(id, -32602, "Invalid tool call parameters.")
	}
	var (
		value any
		err   error
	)
	switch call.Name {
	case "list_databases":
		value, err = s.toolListDatabases()
	case "inspect_schema":
		value, err = s.toolInspectSchema(ctx, call.Arguments)
	case "read_query":
		value, err = s.toolQuery(ctx, call.Arguments, false)
	case "write_query":
		value, err = s.toolQuery(ctx, call.Arguments, true)
	case "execute_ddl":
		value, err = s.toolDDL(ctx, call.Arguments)
	case "get_connection_bundle":
		value, err = s.toolBundle(ctx, call.Arguments)
	default:
		return rpcErrorResponse(id, -32602, "Unknown tool: "+call.Name)
	}
	if err != nil {
		return rpcResponse{JSONRPC: "2.0", ID: id, Result: map[string]any{
			"content": []map[string]any{{"type": "text", "text": err.Error()}},
			"isError": true,
		}}
	}
	text, marshalErr := json.Marshal(value)
	if marshalErr != nil {
		return rpcErrorResponse(id, -32603, "Could not encode tool result.")
	}
	structured := map[string]any{"result": value}
	return rpcResponse{JSONRPC: "2.0", ID: id, Result: map[string]any{
		"content":           []map[string]any{{"type": "text", "text": string(text)}},
		"structuredContent": structured,
		"isError":           false,
	}}
}

func (s *Server) toolListDatabases() (any, error) {
	databases := s.backend.ListDatabases()
	if databases == nil {
		databases = []Database{}
	}
	return map[string]any{"databases": databases}, nil
}

func (s *Server) toolInspectSchema(ctx context.Context, raw json.RawMessage) (any, error) {
	var args struct {
		DatabaseName string `json:"database_name"`
		SchemaName   string `json:"schema_name"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, errors.New("invalid inspect_schema arguments")
	}
	if strings.TrimSpace(args.DatabaseName) == "" {
		return nil, errors.New("database_name is required")
	}
	runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	snapshot, err := s.backend.InspectSchema(runCtx, args.DatabaseName, args.SchemaName)
	if err != nil {
		return nil, err
	}
	tables := snapshot.Tables
	if tables == nil {
		tables = []jdbc.TableInfo{}
	}
	return map[string]any{"database": args.DatabaseName, "schema": args.SchemaName, "tables": tables}, nil
}

func (s *Server) toolQuery(ctx context.Context, raw json.RawMessage, write bool) (any, error) {
	var args struct {
		DatabaseName string `json:"database_name"`
		SQL          string `json:"sql"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, errors.New("invalid query arguments")
	}
	if strings.TrimSpace(args.DatabaseName) == "" {
		return nil, errors.New("database_name is required")
	}
	sqlText := strings.TrimSpace(args.SQL)
	if sqlText == "" {
		return nil, errors.New("sql is required")
	}
	if len(sqlText) > maxQueryLength {
		return nil, fmt.Errorf("SQL statement is too large (limit %d characters)", maxQueryLength)
	}
	keyword := firstKeyword(sqlText)
	if write {
		if s.backend.AgentReadOnly() {
			return nil, errors.New("Agent Read-Only mode is enabled. Disable it in Connect to AI to allow writes.")
		}
		if !contains(writeKeywords, keyword) {
			return nil, fmt.Errorf("write_query only accepts INSERT, UPDATE, DELETE, or MERGE statements (got %s)", keyword)
		}
	} else if !contains(readKeywords, keyword) {
		return nil, fmt.Errorf("read_query only accepts SELECT, WITH, SHOW, EXPLAIN, DESCRIBE, or VALUES statements (got %s)", keyword)
	}
	runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var result jdbc.ExecResult
	var err error
	if write {
		result, err = s.backend.WriteQuery(runCtx, args.DatabaseName, sqlText)
	} else {
		result, err = s.backend.ReadQuery(runCtx, args.DatabaseName, sqlText)
	}
	if err != nil {
		return nil, err
	}
	return queryPayload(result), nil
}

func queryPayload(result jdbc.ExecResult) map[string]any {
	columns := result.Columns
	if columns == nil {
		columns = []jdbc.Column{}
	}
	rows := result.Rows
	if rows == nil {
		rows = [][]*string{}
	}
	return map[string]any{
		"columns":      columns,
		"rows":         rows,
		"rowCount":     result.RowCount,
		"updateCount":  result.UpdateCount,
		"hasResultSet": result.HasResultSet,
		"truncated":    result.Truncated,
	}
}

func (s *Server) toolDDL(ctx context.Context, raw json.RawMessage) (any, error) {
	var args struct {
		DatabaseName string `json:"database_name"`
		SQL          string `json:"sql"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, errors.New("invalid execute_ddl arguments")
	}
	if strings.TrimSpace(args.DatabaseName) == "" {
		return nil, errors.New("database_name is required")
	}
	sqlText := strings.TrimSpace(args.SQL)
	if sqlText == "" {
		return nil, errors.New("sql is required")
	}
	if len(sqlText) > maxQueryLength {
		return nil, fmt.Errorf("SQL statement is too large (limit %d characters)", maxQueryLength)
	}
	if s.backend.AgentReadOnly() {
		return nil, errors.New("Agent Read-Only mode is enabled. Disable it in Connect to AI to allow DDL.")
	}
	keyword := firstKeyword(sqlText)
	if !contains(ddlKeywords, keyword) {
		return nil, fmt.Errorf("execute_ddl only accepts CREATE, ALTER, DROP, TRUNCATE, COMMENT, or RENAME statements (got %s)", keyword)
	}
	runCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	result, err := s.backend.ExecuteDDL(runCtx, args.DatabaseName, sqlText)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"updateCount": result.UpdateCount,
		"checkpoint":  result.Checkpoint,
	}, nil
}

func (s *Server) toolBundle(ctx context.Context, raw json.RawMessage) (any, error) {
	var args struct {
		DatabaseName string `json:"database_name"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, errors.New("invalid get_connection_bundle arguments")
	}
	if strings.TrimSpace(args.DatabaseName) == "" {
		return nil, errors.New("database_name is required")
	}
	runCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	bundle, err := s.backend.ConnectionBundle(runCtx, args.DatabaseName)
	if err != nil {
		return nil, err
	}
	return bundle, nil
}

func (s *Server) resourceList() []map[string]any {
	resources := []map[string]any{{
		"uri":         "hysql://databases",
		"name":        "HySQL databases",
		"description": "All databases managed by HySQL with status and connection URLs.",
		"mimeType":    "application/json",
	}}
	for _, database := range s.backend.ListDatabases() {
		resources = append(resources, map[string]any{
			"uri":         "hysql://databases/" + database.Name + "/schema",
			"name":        database.Name + " schema",
			"description": "Live H2 schema for " + database.Name + ".",
			"mimeType":    "application/json",
		})
	}
	return resources
}

func (s *Server) readResource(id json.RawMessage, params json.RawMessage) rpcResponse {
	var request struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(params, &request); err != nil {
		return rpcErrorResponse(id, -32602, "Invalid resource parameters.")
	}
	if request.URI == "hysql://databases" {
		databases := s.backend.ListDatabases()
		if databases == nil {
			databases = []Database{}
		}
		data, _ := json.MarshalIndent(map[string]any{"databases": databases}, "", "  ")
		return rpcResponse{JSONRPC: "2.0", ID: id, Result: map[string]any{
			"contents": []map[string]any{{
				"uri": request.URI, "mimeType": "application/json", "text": string(data),
			}},
		}}
	}
	if strings.HasPrefix(request.URI, "hysql://databases/") && strings.HasSuffix(request.URI, "/schema") {
		name := strings.TrimSuffix(strings.TrimPrefix(request.URI, "hysql://databases/"), "/schema")
		name = strings.Trim(name, "/")
		if name == "" {
			return rpcErrorResponse(id, -32602, "Resource URI is missing a database name.")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		snapshot, err := s.backend.InspectSchema(ctx, name, "")
		if err != nil {
			return rpcErrorResponse(id, -32603, err.Error())
		}
		data, _ := json.MarshalIndent(snapshot, "", "  ")
		return rpcResponse{JSONRPC: "2.0", ID: id, Result: map[string]any{
			"contents": []map[string]any{{
				"uri": request.URI, "mimeType": "application/json", "text": string(data),
			}},
		}}
	}
	return rpcErrorResponse(id, -32602, "Unknown resource URI: "+request.URI)
}

func (s *Server) handleStream(response http.ResponseWriter, request *http.Request) {
	flusher, ok := response.(http.Flusher)
	if !ok {
		response.WriteHeader(http.StatusInternalServerError)
		return
	}
	response.Header().Set("Content-Type", "text/event-stream")
	response.Header().Set("Cache-Control", "no-cache")
	response.Header().Set("Connection", "keep-alive")
	response.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(response, ": hysql-mcp stream ready\n\n")
	flusher.Flush()
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-request.Context().Done():
			return
		case <-ticker.C:
			if _, err := fmt.Fprint(response, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *Server) handleLegacySSE(response http.ResponseWriter, request *http.Request) {
	flusher, ok := response.(http.Flusher)
	if !ok {
		response.WriteHeader(http.StatusInternalServerError)
		return
	}
	sessionID := randomID()
	channel := make(chan []byte, 16)
	s.sessionsMu.Lock()
	s.sessions[sessionID] = channel
	s.sessionsMu.Unlock()
	defer func() {
		s.sessionsMu.Lock()
		if existing, exists := s.sessions[sessionID]; exists {
			delete(s.sessions, sessionID)
			close(existing)
		}
		s.sessionsMu.Unlock()
	}()

	response.Header().Set("Content-Type", "text/event-stream")
	response.Header().Set("Cache-Control", "no-cache")
	response.Header().Set("Connection", "keep-alive")
	response.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(response, "event: endpoint\ndata: /messages?sessionId=%s\n\n", sessionID)
	flusher.Flush()

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-request.Context().Done():
			return
		case message, open := <-channel:
			if !open {
				return
			}
			if _, err := fmt.Fprintf(response, "event: message\ndata: %s\n\n", message); err != nil {
				return
			}
			flusher.Flush()
		case <-ticker.C:
			_, _ = fmt.Fprint(response, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

func (s *Server) handleLegacyMessage(response http.ResponseWriter, request *http.Request) {
	sessionID := strings.TrimSpace(request.URL.Query().Get("sessionId"))
	s.sessionsMu.Lock()
	channel, ok := s.sessions[sessionID]
	s.sessionsMu.Unlock()
	if !ok {
		writeJSON(response, http.StatusNotFound, rpcErrorResponse(nil, -32600, "Unknown or expired MCP session."))
		return
	}
	body, err := readBody(request)
	if err != nil {
		writeJSON(response, http.StatusBadRequest, rpcErrorResponse(nil, -32700, err.Error()))
		return
	}
	var entry rpcRequest
	if err := json.Unmarshal(body, &entry); err != nil {
		writeJSON(response, http.StatusBadRequest, rpcErrorResponse(nil, -32700, "Invalid JSON."))
		return
	}
	if !entry.isNotification() {
		result := s.dispatch(request.Context(), entry)
		encoded, _ := json.Marshal(result)
		select {
		case channel <- encoded:
		default:
		}
	} else {
		s.dispatch(request.Context(), entry)
	}
	response.WriteHeader(http.StatusAccepted)
}

func toolDefinitions() []map[string]any {
	return []map[string]any{
		{
			"name":        "list_databases",
			"description": "List every database managed by HySQL with running state, local/Tailscale JDBC URLs, and the Auto-REST base URL.",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false},
		},
		{
			"name":        "inspect_schema",
			"description": "Inspect live H2 schema metadata: tables, views, columns, primary keys, foreign keys, and indexes.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"database_name": map[string]any{"type": "string", "description": "Database name or id from list_databases."},
					"schema_name":   map[string]any{"type": "string", "description": "Optional schema filter such as INVENTORY or PUBLIC."},
				},
				"required":             []string{"database_name"},
				"additionalProperties": false,
			},
		},
		{
			"name":        "read_query",
			"description": "Run a read-only SELECT/WITH/SHOW/EXPLAIN/DESCRIBE/VALUES statement and return structured rows (max 500).",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"database_name": map[string]any{"type": "string"},
					"sql":           map[string]any{"type": "string", "description": "One H2 SQL statement."},
				},
				"required":             []string{"database_name", "sql"},
				"additionalProperties": false,
			},
		},
		{
			"name":        "write_query",
			"description": "Run INSERT/UPDATE/DELETE/MERGE and return the affected row count. Blocked in Agent Read-Only mode.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"database_name": map[string]any{"type": "string"},
					"sql":           map[string]any{"type": "string"},
				},
				"required":             []string{"database_name", "sql"},
				"additionalProperties": false,
			},
		},
		{
			"name":        "execute_ddl",
			"description": "Run CREATE/ALTER/DROP/TRUNCATE/COMMENT/RENAME. When auto-checkpointing is enabled, destructive DDL is snapshotted first.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"database_name": map[string]any{"type": "string"},
					"sql":           map[string]any{"type": "string"},
				},
				"required":             []string{"database_name", "sql"},
				"additionalProperties": false,
			},
		},
		{
			"name":        "get_connection_bundle",
			"description": "Return JDBC URLs, credentials, HikariCP and Spring snippets, plus the REST base URL/key so an agent can build an app.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"database_name": map[string]any{"type": "string"},
				},
				"required":             []string{"database_name"},
				"additionalProperties": false,
			},
		},
	}
}

var (
	readKeywords  = []string{"SELECT", "WITH", "SHOW", "EXPLAIN", "DESCRIBE", "VALUES"}
	writeKeywords = []string{"INSERT", "UPDATE", "DELETE", "MERGE"}
	ddlKeywords   = []string{"CREATE", "ALTER", "DROP", "TRUNCATE", "COMMENT", "RENAME"}
)

func firstKeyword(sql string) string {
	remaining := strings.TrimSpace(sql)
	for {
		switch {
		case strings.HasPrefix(remaining, "--"):
			if index := strings.IndexByte(remaining, '\n'); index >= 0 {
				remaining = strings.TrimSpace(remaining[index+1:])
				continue
			}
			return ""
		case strings.HasPrefix(remaining, "/*"):
			if index := strings.Index(remaining, "*/"); index >= 0 {
				remaining = strings.TrimSpace(remaining[index+2:])
				continue
			}
			return ""
		}
		break
	}
	end := 0
	for end < len(remaining) {
		char := remaining[end]
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') {
			end++
			continue
		}
		break
	}
	return strings.ToUpper(remaining[:end])
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func readBody(request *http.Request) ([]byte, error) {
	if request.Body == nil {
		return nil, errors.New("request body is empty")
	}
	defer request.Body.Close()
	data, err := io.ReadAll(io.LimitReader(request.Body, maxBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read request body: %w", err)
	}
	if len(data) > maxBodyBytes {
		return nil, fmt.Errorf("request body exceeds %d bytes", maxBodyBytes)
	}
	return data, nil
}

func bindAddress(cfg Config) string {
	host := strings.TrimSpace(cfg.BindAddress)
	if host == "" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, strconv.Itoa(cfg.Port))
}

func endpoint(cfg Config) string {
	host := strings.TrimSpace(cfg.BindAddress)
	if host == "" {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("http://%s:%d/mcp", host, cfg.Port)
}

func validateBindHost(host string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("invalid bind address %q", host)
	}
	if ip.IsLoopback() {
		return nil
	}
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
		return nil
	}
	return fmt.Errorf("refusing to bind %q: only loopback or Tailscale CGNAT addresses are allowed", host)
}

func randomID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(buffer)
}

func setCORS(response http.ResponseWriter) {
	headers := response.Header()
	headers.Set("Access-Control-Allow-Origin", "*")
	headers.Set("Access-Control-Allow-Methods", "GET,POST,DELETE,OPTIONS")
	headers.Set("Access-Control-Allow-Headers", "Content-Type,Authorization")
	headers.Set("Access-Control-Max-Age", "86400")
}

func writeJSON(response http.ResponseWriter, status int, payload any) {
	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	response.WriteHeader(status)
	encoder := json.NewEncoder(response)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(payload)
}

func now() string {
	return time.Now().UTC().Format(time.RFC3339)
}
