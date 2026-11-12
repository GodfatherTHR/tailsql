package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tailsql/internal/jdbc"
)

type fakeBackend struct {
	readOnly bool
	writes   int
	ddls     int
}

func (f *fakeBackend) ListDatabases() []Database {
	return []Database{{ID: "db1", Name: "inven_db", Status: "running", LocalJDBCURL: "jdbc:h2:tcp://127.0.0.1:9092/./inven_db"}}
}

func (f *fakeBackend) ResolveDatabase(name string) (Database, error) {
	if strings.EqualFold(name, "inven_db") {
		return Database{ID: "db1", Name: "inven_db", Status: "running"}, nil
	}
	return Database{}, errors.New("database not found")
}

func (f *fakeBackend) InspectSchema(context.Context, string, string) (jdbc.SchemaSnapshot, error) {
	return jdbc.SchemaSnapshot{Tables: []jdbc.TableInfo{{
		Schema: "INVENTORY", Name: "PRODUCTS", Type: "TABLE", PrimaryKey: []string{"ID"},
		Columns: []jdbc.ColumnInfo{{Name: "ID", Type: "INTEGER"}},
	}}}, nil
}

func (f *fakeBackend) ReadQuery(context.Context, string, string) (jdbc.ExecResult, error) {
	value := "1"
	return jdbc.ExecResult{
		Columns:      []jdbc.Column{{Name: "ONE", Type: "INTEGER"}},
		Rows:         [][]*string{{&value}},
		RowCount:     1,
		HasResultSet: true,
	}, nil
}

func (f *fakeBackend) WriteQuery(context.Context, string, string) (jdbc.ExecResult, error) {
	f.writes++
	return jdbc.ExecResult{UpdateCount: 1}, nil
}

func (f *fakeBackend) ExecuteDDL(context.Context, string, string) (DDLResult, error) {
	f.ddls++
	return DDLResult{UpdateCount: 0, Checkpoint: "snapshot.mv.db"}, nil
}

func (f *fakeBackend) ConnectionBundle(context.Context, string) (ConnectionBundle, error) {
	return ConnectionBundle{Database: "inven_db", LocalJDBCURL: "jdbc:h2:tcp://127.0.0.1:9092/./inven_db"}, nil
}

func (f *fakeBackend) AgentReadOnly() bool { return f.readOnly }

func newTestServer(backend Backend, token string) *Server {
	return New(Config{BindAddress: "127.0.0.1", Port: 9099, Token: token, Version: "0.3.0"}, backend)
}

func postRPC(t *testing.T, server *Server, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	return recorder
}

func decodeResult(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var payload struct {
		Result map[string]any `json:"result"`
		Error  any            `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode JSON-RPC response: %v (%s)", err, recorder.Body.String())
	}
	if payload.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %+v", payload.Error)
	}
	return payload.Result
}

func TestInitializeAndToolList(t *testing.T) {
	server := newTestServer(&fakeBackend{}, "secret")
	if recorder := postRPC(t, server, "", `{"jsonrpc":"2.0","id":1,"method":"ping"}`); recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", recorder.Code)
	}

	initRecorder := postRPC(t, server, "secret",
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`)
	if initRecorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", initRecorder.Code)
	}
	result := decodeResult(t, initRecorder)
	serverInfo, ok := result["serverInfo"].(map[string]any)
	if !ok || serverInfo["name"] != "tailsql" {
		t.Fatalf("unexpected serverInfo: %+v", result)
	}

	listRecorder := postRPC(t, server, "secret", `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	tools, ok := decodeResult(t, listRecorder)["tools"].([]any)
	if !ok || len(tools) != 6 {
		t.Fatalf("expected 6 tools, got %+v", tools)
	}
}

func TestToolCallsAndSafeMode(t *testing.T) {
	backend := &fakeBackend{}
	server := newTestServer(backend, "")

	readRecorder := postRPC(t, server, "", `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"read_query","arguments":{"database_name":"inven_db","sql":"SELECT 1"}}}`)
	result := decodeResult(t, readRecorder)
	if isError, _ := result["isError"].(bool); isError {
		t.Fatalf("read_query failed: %+v", result)
	}
	structured, ok := result["structuredContent"].(map[string]any)
	if !ok {
		t.Fatalf("missing structured content: %+v", result)
	}
	payload, ok := structured["result"].(map[string]any)
	if !ok || payload["rowCount"].(float64) != 1 {
		t.Fatalf("unexpected read payload: %+v", structured)
	}

	writeRecorder := postRPC(t, server, "", `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"write_query","arguments":{"database_name":"inven_db","sql":"UPDATE t SET x = 1"}}}`)
	if isError, _ := decodeResult(t, writeRecorder)["isError"].(bool); isError {
		t.Fatal("write_query should work when safe mode is off")
	}
	if backend.writes != 1 {
		t.Fatalf("expected 1 write, got %d", backend.writes)
	}

	backend.readOnly = true
	blocked := postRPC(t, server, "", `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"execute_ddl","arguments":{"database_name":"inven_db","sql":"DROP TABLE X"}}}`)
	if isError, _ := decodeResult(t, blocked)["isError"].(bool); !isError {
		t.Fatal("execute_ddl must be blocked in read-only mode")
	}
	if backend.ddls != 0 {
		t.Fatal("read-only mode must not reach the backend")
	}
}

func TestRejectsNonReadStatementInReadQuery(t *testing.T) {
	server := newTestServer(&fakeBackend{}, "")
	recorder := postRPC(t, server, "", `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"read_query","arguments":{"database_name":"inven_db","sql":"DELETE FROM t"}}}`)
	if isError, _ := decodeResult(t, recorder)["isError"].(bool); !isError {
		t.Fatal("read_query must reject DELETE")
	}
}

func TestResourcesAndNotifications(t *testing.T) {
	server := newTestServer(&fakeBackend{}, "")
	recorder := postRPC(t, server, "", `{"jsonrpc":"2.0","id":7,"method":"resources/list"}`)
	resources, ok := decodeResult(t, recorder)["resources"].([]any)
	if !ok || len(resources) < 2 {
		t.Fatalf("unexpected resources: %+v", resources)
	}

	notification := postRPC(t, server, "", `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if notification.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for notification, got %d", notification.Code)
	}
}
