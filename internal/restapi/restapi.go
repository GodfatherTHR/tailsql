// Package restapi exposes Auto-REST endpoints for running H2 databases.
package restapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"hysql/internal/jdbc"
)

const (
	defaultLimit   = 50
	maxLimit       = 500
	schemaCacheTTL = 3 * time.Second
	maxBodyBytes   = 4 << 20
)

// Database is the minimal identity the gateway needs.
type Database struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// Backend resolves databases and executes SQL against them.
type Backend interface {
	Databases() []Database
	ResolveDatabase(name string) (Database, error)
	Schema(ctx context.Context, databaseID string) (jdbc.SchemaSnapshot, error)
	Query(ctx context.Context, databaseID, sql string, params []*string) (jdbc.ExecResult, error)
}

// Config controls one HTTP listener.
type Config struct {
	BindAddress     string
	Port            int
	APIKey          string
	RequireAuth     bool
	AllowPublicRead bool
	PublicBaseURL   string
	Version         string
}

// State is the UI-facing snapshot of the gateway.
type State struct {
	Running       bool   `json:"running"`
	BindAddress   string `json:"bindAddress"`
	BaseURL       string `json:"baseUrl"`
	LocalURL      string `json:"localUrl"`
	PublicBaseURL string `json:"publicBaseUrl"`
	DatabaseCount int    `json:"databaseCount"`
	Error         string `json:"error"`
	CheckedAt     string `json:"checkedAt"`
}

// Server is the embedded Auto-REST HTTP gateway.
type Server struct {
	mu       sync.Mutex
	cfg      Config
	backend  Backend
	http     *http.Server
	listener net.Listener
	state    State

	cacheMu sync.Mutex
	cache   map[string]schemaCacheEntry
}

type schemaCacheEntry struct {
	snapshot jdbc.SchemaSnapshot
	at       time.Time
}

// New builds a stopped server.
func New(cfg Config, backend Backend) *Server {
	server := &Server{
		cfg:     cfg,
		backend: backend,
		cache:   map[string]schemaCacheEntry{},
	}
	server.state = State{BindAddress: bindAddress(cfg), CheckedAt: now()}
	return server
}

// Start binds the listener. It is safe to call Stop then Start with a new config.
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.http != nil {
		return nil
	}
	if s.backend == nil {
		s.state.Error = "REST backend is not available"
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
		return fmt.Errorf("start REST API on %s: %w", address, err)
	}
	s.listener = listener
	s.http = &http.Server{
		Handler:           s,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	s.state = State{
		Running:       true,
		BindAddress:   listener.Addr().String(),
		BaseURL:       s.baseURL(),
		LocalURL:      fmt.Sprintf("http://127.0.0.1:%d", s.cfg.Port),
		PublicBaseURL: strings.TrimRight(s.cfg.PublicBaseURL, "/"),
		DatabaseCount: len(s.backend.Databases()),
		CheckedAt:     now(),
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

// Stop shuts the listener down.
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
	s.mu.Lock()
	s.state.Running = false
	s.state.BindAddress = bindAddress(s.cfg)
	s.state.CheckedAt = now()
	s.mu.Unlock()
}

// UpdateConfig replaces the configuration; the caller restarts the server.
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
	if s.backend != nil {
		state.DatabaseCount = len(s.backend.Databases())
	}
	state.CheckedAt = now()
	return state
}

func (s *Server) config() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

func (s *Server) baseURL() string {
	if strings.TrimSpace(s.cfg.PublicBaseURL) != "" {
		return strings.TrimRight(s.cfg.PublicBaseURL, "/")
	}
	host := s.cfg.BindAddress
	if host == "" {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("http://%s:%d", host, s.cfg.Port)
}

func bindAddress(cfg Config) string {
	host := strings.TrimSpace(cfg.BindAddress)
	if host == "" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, strconv.Itoa(cfg.Port))
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

func (s *Server) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	setCORS(response)
	if request.Method == http.MethodOptions {
		response.WriteHeader(http.StatusNoContent)
		return
	}
	path := request.URL.Path
	if path == "/health" || path == "/api/v1/health" {
		writeJSON(response, http.StatusOK, map[string]any{"ok": true, "server": "hysql-rest"})
		return
	}
	if path == "/" || path == "/api/v1" {
		writeJSON(response, http.StatusOK, map[string]any{
			"name":    "HySQL Auto-REST",
			"version": s.config().Version,
			"routes":  "/api/v1/<database>/<schema>/<table>",
		})
		return
	}
	if !s.authorized(request) {
		writeError(response, http.StatusUnauthorized, "unauthorized", "A valid X-HySQL-Key or Bearer token is required.")
		return
	}
	if !strings.HasPrefix(path, "/api/v1/") {
		writeError(response, http.StatusNotFound, "not_found", "Unknown route.")
		return
	}
	segments := splitPath(strings.TrimPrefix(path, "/api/v1/"))
	if len(segments) < 2 {
		writeError(response, http.StatusBadRequest, "bad_request", "Use /api/v1/<database>/openapi.json or /api/v1/<database>/<schema>/<table>.")
		return
	}
	database, err := s.backend.ResolveDatabase(segments[0])
	if err != nil {
		writeError(response, http.StatusNotFound, "not_found", errorText(err))
		return
	}
	if strings.EqualFold(database.Status, "stopped") || strings.EqualFold(database.Status, "error") {
		writeError(response, http.StatusConflict, "conflict", "Database "+database.Name+" is "+database.Status+"; start it first.")
		return
	}

	if len(segments) == 2 && strings.EqualFold(segments[1], "openapi.json") {
		s.serveOpenAPI(response, request, database)
		return
	}
	if len(segments) < 3 {
		writeError(response, http.StatusBadRequest, "bad_request", "A schema and table are required.")
		return
	}
	schemaName := segments[1]
	tableName := segments[2]
	snapshot, err := s.schema(request.Context(), database.ID)
	if err != nil {
		writeError(response, http.StatusBadGateway, "internal", errorText(err))
		return
	}
	table, ok := findTable(snapshot, schemaName, tableName)
	if !ok {
		writeError(response, http.StatusNotFound, "not_found",
			fmt.Sprintf("Table or view %s.%s was not found.", schemaName, tableName))
		return
	}
	writable := strings.Contains(strings.ToUpper(table.Type), "TABLE") &&
		!strings.Contains(strings.ToUpper(table.Type), "SYSTEM")

	switch request.Method {
	case http.MethodGet:
		if len(segments) >= 4 {
			s.getOne(response, request, database, table, segments[3])
			return
		}
		s.listRows(response, request, database, table)
	case http.MethodPost:
		if len(segments) >= 4 {
			writeError(response, http.StatusMethodNotAllowed, "bad_request", "POST does not take a primary-key path segment.")
			return
		}
		if !writable {
			writeError(response, http.StatusForbidden, "forbidden", "This object is read-only.")
			return
		}
		s.insertRows(response, request, database, table)
	case http.MethodPatch:
		if len(segments) < 4 {
			writeError(response, http.StatusBadRequest, "bad_request", "PATCH requires the primary-key value in the path.")
			return
		}
		if !writable {
			writeError(response, http.StatusForbidden, "forbidden", "This object is read-only.")
			return
		}
		s.updateRow(response, request, database, table, segments[3])
	case http.MethodDelete:
		if len(segments) < 4 {
			writeError(response, http.StatusBadRequest, "bad_request", "DELETE requires the primary-key value in the path.")
			return
		}
		if !writable {
			writeError(response, http.StatusForbidden, "forbidden", "This object is read-only.")
			return
		}
		s.deleteRow(response, request, database, table, segments[3])
	default:
		writeError(response, http.StatusMethodNotAllowed, "bad_request", "Unsupported method.")
	}
}

func (s *Server) authorized(request *http.Request) bool {
	cfg := s.config()
	if !cfg.RequireAuth {
		return true
	}
	if cfg.AllowPublicRead && request.Method == http.MethodGet {
		return true
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return true
	}
	provided := strings.TrimSpace(request.Header.Get("X-HySQL-Key"))
	if provided == "" {
		authorization := strings.TrimSpace(request.Header.Get("Authorization"))
		if strings.HasPrefix(strings.ToLower(authorization), "bearer ") {
			provided = strings.TrimSpace(authorization[7:])
		}
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(cfg.APIKey)) == 1
}

func (s *Server) schema(ctx context.Context, databaseID string) (jdbc.SchemaSnapshot, error) {
	s.cacheMu.Lock()
	if entry, ok := s.cache[databaseID]; ok && time.Since(entry.at) < schemaCacheTTL {
		s.cacheMu.Unlock()
		return entry.snapshot, nil
	}
	s.cacheMu.Unlock()
	snapshot, err := s.backend.Schema(ctx, databaseID)
	if err != nil {
		return jdbc.SchemaSnapshot{}, err
	}
	s.cacheMu.Lock()
	s.cache[databaseID] = schemaCacheEntry{snapshot: snapshot, at: time.Now()}
	s.cacheMu.Unlock()
	return snapshot, nil
}

func (s *Server) invalidate(databaseID string) {
	s.cacheMu.Lock()
	delete(s.cache, databaseID)
	s.cacheMu.Unlock()
}

func (s *Server) listRows(response http.ResponseWriter, request *http.Request, database Database, table jdbc.TableInfo) {
	ctx, cancel := context.WithTimeout(request.Context(), 30*time.Second)
	defer cancel()
	query := request.URL.Query()
	limit, offset, err := pagination(query)
	if err != nil {
		writeError(response, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	where, params, err := buildFilters(query, table)
	if err != nil {
		writeError(response, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	order, err := buildOrder(query.Get("order"), table)
	if err != nil {
		writeError(response, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	target, err := quotedTarget(table)
	if err != nil {
		writeError(response, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	sql := "SELECT * FROM " + target + where + order +
		fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)
	result, err := s.backend.Query(ctx, database.ID, sql, params)
	if err != nil {
		writeError(response, http.StatusBadRequest, "bad_request", errorText(err))
		return
	}
	total := int64(-1)
	countSQL := "SELECT COUNT(*) FROM " + target + where
	if count, countErr := s.backend.Query(ctx, database.ID, countSQL, params); countErr == nil &&
		count.HasResultSet && len(count.Rows) > 0 && len(count.Rows[0]) > 0 && count.Rows[0][0] != nil {
		total, _ = strconv.ParseInt(*count.Rows[0][0], 10, 64)
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"data":   rowsToMaps(result),
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

func (s *Server) getOne(response http.ResponseWriter, request *http.Request, database Database, table jdbc.TableInfo, key string) {
	pk, ok := singlePrimaryKey(table)
	if !ok {
		writeError(response, http.StatusBadRequest, "bad_request",
			"Single-row routes require a table with exactly one primary-key column.")
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 30*time.Second)
	defer cancel()
	target, err := quotedTarget(table)
	if err != nil {
		writeError(response, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	value := key
	sql := "SELECT * FROM " + target + " WHERE " + quoteIdentifier(pk) + " = ? LIMIT 1"
	result, err := s.backend.Query(ctx, database.ID, sql, []*string{&value})
	if err != nil {
		writeError(response, http.StatusBadRequest, "bad_request", errorText(err))
		return
	}
	rows := rowsToMaps(result)
	if len(rows) == 0 {
		writeError(response, http.StatusNotFound, "not_found", "No row matches that primary key.")
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"data": rows[0]})
}

func (s *Server) insertRows(response http.ResponseWriter, request *http.Request, database Database, table jdbc.TableInfo) {
	rows, err := decodeObjects(request)
	if err != nil {
		writeError(response, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if len(rows) == 0 {
		writeError(response, http.StatusBadRequest, "bad_request", "Send a JSON object or array of objects.")
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 30*time.Second)
	defer cancel()
	target, err := quotedTarget(table)
	if err != nil {
		writeError(response, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	columnsByName := columnMap(table)
	inserted := int64(0)
	for _, object := range rows {
		type assignment struct {
			column string
			value  *string
		}
		pairs := make([]assignment, 0, len(object))
		for name, raw := range object {
			column, ok := columnsByName[strings.ToUpper(name)]
			if !ok {
				writeError(response, http.StatusBadRequest, "bad_request", "Unknown column "+name+".")
				return
			}
			value, valueErr := rawToParam(raw)
			if valueErr != nil {
				writeError(response, http.StatusBadRequest, "bad_request", valueErr.Error())
				return
			}
			pairs = append(pairs, assignment{column: column.Name, value: value})
		}
		sort.Slice(pairs, func(i, j int) bool { return pairs[i].column < pairs[j].column })
		columns := make([]string, 0, len(pairs))
		placeholders := make([]string, 0, len(pairs))
		params := make([]*string, 0, len(pairs))
		for _, pair := range pairs {
			columns = append(columns, quoteIdentifier(pair.column))
			placeholders = append(placeholders, "?")
			params = append(params, pair.value)
		}
		sql := "INSERT INTO " + target
		if len(columns) == 0 {
			sql += " DEFAULT VALUES"
		} else {
			sql += " (" + strings.Join(columns, ", ") + ") VALUES (" + strings.Join(placeholders, ", ") + ")"
		}
		result, runErr := s.backend.Query(ctx, database.ID, sql, params)
		if runErr != nil {
			writeError(response, http.StatusConflict, "conflict", errorText(runErr))
			return
		}
		inserted += result.UpdateCount
	}
	s.invalidate(database.ID)
	writeJSON(response, http.StatusCreated, map[string]any{"inserted": inserted})
}

func (s *Server) updateRow(response http.ResponseWriter, request *http.Request, database Database, table jdbc.TableInfo, key string) {
	pk, ok := singlePrimaryKey(table)
	if !ok {
		writeError(response, http.StatusBadRequest, "bad_request",
			"Updates require a table with exactly one primary-key column.")
		return
	}
	object, err := decodeObject(request)
	if err != nil {
		writeError(response, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	columnsByName := columnMap(table)
	type assignment struct {
		sql   string
		value *string
	}
	pairs := make([]assignment, 0, len(object))
	for name, raw := range object {
		column, exists := columnsByName[strings.ToUpper(name)]
		if !exists {
			writeError(response, http.StatusBadRequest, "bad_request", "Unknown column "+name+".")
			return
		}
		value, valueErr := rawToParam(raw)
		if valueErr != nil {
			writeError(response, http.StatusBadRequest, "bad_request", valueErr.Error())
			return
		}
		pairs = append(pairs, assignment{
			sql:   quoteIdentifier(column.Name) + " = ?",
			value: value,
		})
	}
	if len(pairs) == 0 {
		writeError(response, http.StatusBadRequest, "bad_request", "Send at least one column to update.")
		return
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].sql < pairs[j].sql })
	assignments := make([]string, 0, len(pairs))
	params := make([]*string, 0, len(pairs)+1)
	for _, pair := range pairs {
		assignments = append(assignments, pair.sql)
		params = append(params, pair.value)
	}
	params = append(params, &key)
	target, err := quotedTarget(table)
	if err != nil {
		writeError(response, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 30*time.Second)
	defer cancel()
	sql := "UPDATE " + target + " SET " + strings.Join(assignments, ", ") +
		" WHERE " + quoteIdentifier(pk) + " = ?"
	result, runErr := s.backend.Query(ctx, database.ID, sql, params)
	if runErr != nil {
		writeError(response, http.StatusConflict, "conflict", errorText(runErr))
		return
	}
	s.invalidate(database.ID)
	writeJSON(response, http.StatusOK, map[string]any{"updated": result.UpdateCount})
}

func (s *Server) deleteRow(response http.ResponseWriter, request *http.Request, database Database, table jdbc.TableInfo, key string) {
	pk, ok := singlePrimaryKey(table)
	if !ok {
		writeError(response, http.StatusBadRequest, "bad_request",
			"Deletes require a table with exactly one primary-key column.")
		return
	}
	target, err := quotedTarget(table)
	if err != nil {
		writeError(response, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 30*time.Second)
	defer cancel()
	sql := "DELETE FROM " + target + " WHERE " + quoteIdentifier(pk) + " = ?"
	result, runErr := s.backend.Query(ctx, database.ID, sql, []*string{&key})
	if runErr != nil {
		writeError(response, http.StatusConflict, "conflict", errorText(runErr))
		return
	}
	s.invalidate(database.ID)
	writeJSON(response, http.StatusOK, map[string]any{"deleted": result.UpdateCount})
}

func (s *Server) serveOpenAPI(response http.ResponseWriter, request *http.Request, database Database) {
	snapshot, err := s.schema(request.Context(), database.ID)
	if err != nil {
		writeError(response, http.StatusBadGateway, "internal", errorText(err))
		return
	}
	base := s.baseURL() + "/api/v1/" + url.PathEscape(database.Name)
	writeJSON(response, http.StatusOK, buildOpenAPI(database, snapshot, base, s.config().Version))
}

func pagination(query url.Values) (int, int, error) {
	limit := defaultLimit
	if raw := query.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			return 0, 0, errors.New("limit must be a positive integer")
		}
		if parsed > maxLimit {
			parsed = maxLimit
		}
		limit = parsed
	}
	offset := 0
	if raw := query.Get("offset"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			return 0, 0, errors.New("offset must be a non-negative integer")
		}
		offset = parsed
	}
	return limit, offset, nil
}

func buildFilters(query url.Values, table jdbc.TableInfo) (string, []*string, error) {
	columns := columnMap(table)
	reserved := map[string]bool{
		"limit": true, "offset": true, "order": true, "select": true,
	}
	keys := make([]string, 0, len(query))
	for key := range query {
		if !reserved[strings.ToLower(key)] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	params := make([]*string, 0, len(keys))
	for _, key := range keys {
		column, ok := columns[strings.ToUpper(key)]
		if !ok {
			return "", nil, fmt.Errorf("unknown filter column %s", key)
		}
		raw := query.Get(key)
		identifier := quoteIdentifier(column.Name)
		lower := strings.ToLower(raw)
		if strings.HasPrefix(lower, "is.") {
			switch strings.TrimSpace(lower[3:]) {
			case "null":
				parts = append(parts, identifier+" IS NULL")
				continue
			case "notnull", "not-null", "not null":
				parts = append(parts, identifier+" IS NOT NULL")
				continue
			default:
				return "", nil, fmt.Errorf("unsupported is-filter %q for %s", raw, key)
			}
		}
		operator := "="
		value := raw
		if index := strings.Index(raw, "."); index > 0 {
			candidate := strings.ToLower(raw[:index])
			if op, exists := filterOperators[candidate]; exists {
				operator = op
				value = raw[index+1:]
			}
		}
		copied := value
		params = append(params, &copied)
		parts = append(parts, identifier+" "+operator+" ?")
	}
	if len(parts) == 0 {
		return "", params, nil
	}
	return " WHERE " + strings.Join(parts, " AND "), params, nil
}

var filterOperators = map[string]string{
	"eq":   "=",
	"neq":  "<>",
	"gt":   ">",
	"gte":  ">=",
	"lt":   "<",
	"lte":  "<=",
	"like": "LIKE",
}

func buildOrder(order string, table jdbc.TableInfo) (string, error) {
	order = strings.TrimSpace(order)
	if order == "" {
		return "", nil
	}
	columns := columnMap(table)
	name := order
	direction := "ASC"
	if index := strings.LastIndex(order, "."); index > 0 {
		candidate := strings.ToLower(order[index+1:])
		if candidate == "asc" || candidate == "desc" {
			direction = strings.ToUpper(candidate)
			name = order[:index]
		}
	}
	column, ok := columns[strings.ToUpper(name)]
	if !ok {
		return "", fmt.Errorf("unknown order column %s", name)
	}
	return " ORDER BY " + quoteIdentifier(column.Name) + " " + direction, nil
}

func findTable(snapshot jdbc.SchemaSnapshot, schema, table string) (jdbc.TableInfo, bool) {
	for _, candidate := range snapshot.Tables {
		if strings.EqualFold(candidate.Schema, schema) && strings.EqualFold(candidate.Name, table) {
			return candidate, true
		}
	}
	return jdbc.TableInfo{}, false
}

func columnMap(table jdbc.TableInfo) map[string]jdbc.ColumnInfo {
	columns := make(map[string]jdbc.ColumnInfo, len(table.Columns))
	for _, column := range table.Columns {
		columns[strings.ToUpper(column.Name)] = column
	}
	return columns
}

func singlePrimaryKey(table jdbc.TableInfo) (string, bool) {
	if len(table.PrimaryKey) != 1 {
		return "", false
	}
	return table.PrimaryKey[0], true
}

func quotedTarget(table jdbc.TableInfo) (string, error) {
	schemaSQL, err := quoteIdentifierStrict(table.Schema)
	if err != nil {
		return "", fmt.Errorf("invalid schema: %w", err)
	}
	tableSQL, err := quoteIdentifierStrict(table.Name)
	if err != nil {
		return "", fmt.Errorf("invalid table: %w", err)
	}
	return schemaSQL + "." + tableSQL, nil
}

func quoteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func quoteIdentifierStrict(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", errors.New("identifier cannot be empty")
	}
	return quoteIdentifier(value), nil
}

func rowsToMaps(result jdbc.ExecResult) []map[string]any {
	out := make([]map[string]any, 0, len(result.Rows))
	for _, row := range result.Rows {
		object := make(map[string]any, len(result.Columns))
		for index, column := range result.Columns {
			if index >= len(row) {
				object[column.Name] = nil
				continue
			}
			object[column.Name] = convertValue(column.Type, row[index])
		}
		out = append(out, object)
	}
	return out
}

func convertValue(typeName string, value *string) any {
	if value == nil {
		return nil
	}
	upper := strings.ToUpper(typeName)
	raw := *value
	switch {
	case strings.Contains(upper, "BOOL"):
		return strings.EqualFold(strings.TrimSpace(raw), "true")
	case strings.Contains(upper, "INT") || strings.Contains(upper, "DECIMAL") ||
		strings.Contains(upper, "NUMERIC") || strings.Contains(upper, "DOUBLE") ||
		strings.Contains(upper, "REAL") || strings.Contains(upper, "FLOAT"):
		if _, err := strconv.ParseFloat(raw, 64); err == nil {
			return json.Number(raw)
		}
	}
	return raw
}

func decodeObjects(request *http.Request) ([]map[string]json.RawMessage, error) {
	body, err := readBody(request)
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return nil, errors.New("request body is empty")
	}
	if strings.HasPrefix(trimmed, "[") {
		var rows []map[string]json.RawMessage
		if err := json.Unmarshal(body, &rows); err != nil {
			return nil, fmt.Errorf("invalid JSON array: %w", err)
		}
		return rows, nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(body, &object); err != nil {
		return nil, fmt.Errorf("invalid JSON object: %w", err)
	}
	return []map[string]json.RawMessage{object}, nil
}

func decodeObject(request *http.Request) (map[string]json.RawMessage, error) {
	rows, err := decodeObjects(request)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errors.New("request body is empty")
	}
	return rows[0], nil
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

func rawToParam(raw json.RawMessage) (*string, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	if strings.HasPrefix(trimmed, `"`) {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, fmt.Errorf("invalid string value: %w", err)
		}
		return &value, nil
	}
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return nil, errors.New("objects and arrays are not valid column values")
	}
	value := trimmed
	return &value, nil
}

func buildOpenAPI(database Database, snapshot jdbc.SchemaSnapshot, base, version string) map[string]any {
	tables := append([]jdbc.TableInfo(nil), snapshot.Tables...)
	sort.Slice(tables, func(i, j int) bool {
		if tables[i].Schema == tables[j].Schema {
			return tables[i].Name < tables[j].Name
		}
		return tables[i].Schema < tables[j].Schema
	})
	paths := map[string]any{}
	components := map[string]any{}
	for _, table := range tables {
		writable := strings.Contains(strings.ToUpper(table.Type), "TABLE")
		path := "/" + url.PathEscape(table.Schema) + "/" + url.PathEscape(table.Name)
		schemaRef := "#/components/schemas/" + table.Schema + "_" + table.Name
		properties := map[string]any{}
		for _, column := range table.Columns {
			properties[column.Name] = map[string]any{
				"type":        openAPIType(column.Type),
				"description": strings.TrimSpace(column.Remarks + " " + column.Type),
				"nullable":    column.Nullable,
			}
		}
		components[table.Schema+"_"+table.Name] = map[string]any{
			"type":       "object",
			"properties": properties,
		}
		listOperation := map[string]any{
			"summary":     "List rows from " + table.Schema + "." + table.Name,
			"operationId": "list_" + table.Schema + "_" + table.Name,
			"parameters": []map[string]any{
				{"name": "limit", "in": "query", "schema": map[string]any{"type": "integer", "maximum": maxLimit}},
				{"name": "offset", "in": "query", "schema": map[string]any{"type": "integer"}},
				{"name": "order", "in": "query", "schema": map[string]any{"type": "string"}},
			},
			"responses": map[string]any{"200": map[string]any{"description": "Rows"}},
		}
		itemPath := path + "/{pk}"
		itemParameter := map[string]any{
			"name": "pk", "in": "path", "required": true,
			"schema": map[string]any{"type": "string"},
		}
		itemOperations := map[string]any{
			"get": map[string]any{
				"summary":     "Fetch one row by primary key",
				"operationId": "get_" + table.Schema + "_" + table.Name,
				"parameters":  []map[string]any{itemParameter},
				"responses":   map[string]any{"200": map[string]any{"description": "Row"}},
			},
		}
		if writable {
			listOperation["requestBody"] = map[string]any{
				"required": true,
				"content": map[string]any{"application/json": map[string]any{
					"schema": map[string]any{"$ref": schemaRef},
				}},
			}
			listOperation["responses"] = map[string]any{
				"201": map[string]any{"description": "Inserted"},
			}
			itemOperations["patch"] = map[string]any{
				"summary":     "Update columns by primary key",
				"operationId": "update_" + table.Schema + "_" + table.Name,
				"parameters":  []map[string]any{itemParameter},
				"requestBody": map[string]any{
					"required": true,
					"content": map[string]any{"application/json": map[string]any{
						"schema": map[string]any{"$ref": schemaRef},
					}},
				},
				"responses": map[string]any{"200": map[string]any{"description": "Updated"}},
			}
			itemOperations["delete"] = map[string]any{
				"summary":     "Delete by primary key",
				"operationId": "delete_" + table.Schema + "_" + table.Name,
				"parameters":  []map[string]any{itemParameter},
				"responses":   map[string]any{"200": map[string]any{"description": "Deleted"}},
			}
		}
		paths[path] = map[string]any{
			"get": listOperation,
			"post": map[string]any{
				"summary":     "Insert one row or an array of rows",
				"operationId": "insert_" + table.Schema + "_" + table.Name,
				"requestBody": listOperation["requestBody"],
				"responses":   map[string]any{"201": map[string]any{"description": "Inserted"}},
			},
		}
		if !writable {
			delete(paths[path].(map[string]any), "post")
		}
		if len(table.PrimaryKey) == 1 {
			paths[itemPath] = itemOperations
		}
	}
	return map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":       "HySQL Auto-REST — " + database.Name,
			"version":     version,
			"description": "Generated from live H2 INFORMATION_SCHEMA metadata.",
		},
		"servers":    []map[string]any{{"url": base}},
		"paths":      paths,
		"components": map[string]any{"schemas": components},
	}
}

func openAPIType(typeName string) string {
	upper := strings.ToUpper(typeName)
	switch {
	case strings.Contains(upper, "BOOL"):
		return "boolean"
	case strings.Contains(upper, "INT") || strings.Contains(upper, "BIGINT"):
		return "integer"
	case strings.Contains(upper, "DECIMAL") || strings.Contains(upper, "NUMERIC") ||
		strings.Contains(upper, "DOUBLE") || strings.Contains(upper, "REAL") || strings.Contains(upper, "FLOAT"):
		return "number"
	default:
		return "string"
	}
}

func splitPath(path string) []string {
	raw := strings.Split(strings.Trim(path, "/"), "/")
	out := make([]string, 0, len(raw))
	for _, segment := range raw {
		decoded, err := url.PathUnescape(segment)
		if err != nil {
			decoded = segment
		}
		if segment != "" {
			out = append(out, decoded)
		}
	}
	return out
}

func setCORS(response http.ResponseWriter) {
	headers := response.Header()
	headers.Set("Access-Control-Allow-Origin", "*")
	headers.Set("Access-Control-Allow-Methods", "GET,POST,PATCH,DELETE,OPTIONS")
	headers.Set("Access-Control-Allow-Headers", "Content-Type,Authorization,X-HySQL-Key")
	headers.Set("Access-Control-Max-Age", "86400")
}

func writeJSON(response http.ResponseWriter, status int, payload any) {
	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	response.WriteHeader(status)
	encoder := json.NewEncoder(response)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(payload)
}

func writeError(response http.ResponseWriter, status int, code, message string) {
	writeJSON(response, status, map[string]any{
		"error": map[string]any{"code": code, "message": message},
	})
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func now() string {
	return time.Now().UTC().Format(time.RFC3339)
}
