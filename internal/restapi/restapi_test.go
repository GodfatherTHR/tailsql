package restapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hysql/internal/jdbc"
)

type fakeBackend struct {
	databases []Database
	schema    jdbc.SchemaSnapshot
	results   []jdbc.ExecResult
	sqls      []string
	lastSQL   string
	lastArgs  []*string
}

func (f *fakeBackend) Databases() []Database { return f.databases }

func (f *fakeBackend) ResolveDatabase(name string) (Database, error) {
	for _, database := range f.databases {
		if strings.EqualFold(database.Name, name) || database.ID == name {
			return database, nil
		}
	}
	return Database{}, context.Canceled
}

func (f *fakeBackend) Schema(context.Context, string) (jdbc.SchemaSnapshot, error) {
	return f.schema, nil
}

func (f *fakeBackend) Query(_ context.Context, _ string, sql string, params []*string) (jdbc.ExecResult, error) {
	f.lastSQL = sql
	f.sqls = append(f.sqls, sql)
	f.lastArgs = params
	if len(f.results) > 0 {
		result := f.results[0]
		f.results = f.results[1:]
		return result, nil
	}
	return jdbc.ExecResult{}, nil
}

func testSchema() jdbc.SchemaSnapshot {
	return jdbc.SchemaSnapshot{Tables: []jdbc.TableInfo{{
		Schema:     "INVENTORY",
		Name:       "PRODUCTS",
		Type:       "TABLE",
		PrimaryKey: []string{"ID"},
		Columns: []jdbc.ColumnInfo{
			{Name: "ID", Type: "INTEGER", Ordinal: 1},
			{Name: "NAME", Type: "VARCHAR", Ordinal: 2},
		},
	}}}
}

func newTestServer(backend *fakeBackend, requireAuth bool) *Server {
	return New(Config{
		BindAddress: "127.0.0.1",
		Port:        9199,
		APIKey:      "hysql_rest_test",
		RequireAuth: requireAuth,
		Version:     "0.3.0",
	}, backend)
}

func TestBuildFiltersAndOrder(t *testing.T) {
	table := testSchema().Tables[0]
	where, params, err := buildFilters(map[string][]string{
		"ID":   {"gte.5"},
		"NAME": {"like.Wid%"},
	}, table)
	if err != nil {
		t.Fatalf("build filters: %v", err)
	}
	if !strings.Contains(where, `"ID" >= ?`) || !strings.Contains(where, `"NAME" LIKE ?`) {
		t.Fatalf("unexpected where clause: %s", where)
	}
	if len(params) != 2 || *params[0] != "5" || *params[1] != "Wid%" {
		t.Fatalf("unexpected params: %+v", params)
	}
	if _, _, err := buildFilters(map[string][]string{"MISSING": {"eq.x"}}, table); err == nil {
		t.Fatal("unknown filter column must fail")
	}
	if order, err := buildOrder("name.desc", table); err != nil || order != ` ORDER BY "NAME" DESC` {
		t.Fatalf("unexpected order: %q %v", order, err)
	}
	if _, err := buildOrder("missing.asc", table); err == nil {
		t.Fatal("unknown order column must fail")
	}
}

func TestRowsToMapsConvertsTypes(t *testing.T) {
	value := "42"
	flag := "true"
	rows := rowsToMaps(jdbc.ExecResult{
		Columns: []jdbc.Column{{Name: "ID", Type: "INTEGER"}, {Name: "ACTIVE", Type: "BOOLEAN"}, {Name: "NAME", Type: "VARCHAR"}},
		Rows:    [][]*string{{&value, &flag, nil}},
	})
	if len(rows) != 1 {
		t.Fatalf("unexpected rows: %+v", rows)
	}
	if number, ok := rows[0]["ID"].(json.Number); !ok || number.String() != "42" {
		t.Fatalf("ID was not converted to a number: %#v", rows[0]["ID"])
	}
	if active, ok := rows[0]["ACTIVE"].(bool); !ok || !active {
		t.Fatalf("ACTIVE was not converted to a bool: %#v", rows[0]["ACTIVE"])
	}
	if rows[0]["NAME"] != nil {
		t.Fatalf("NULL was not preserved: %#v", rows[0]["NAME"])
	}
}

func TestAuthAndListRoute(t *testing.T) {
	backend := &fakeBackend{
		databases: []Database{{ID: "db1", Name: "inven_db", Status: "running"}},
		schema:    testSchema(),
		results: []jdbc.ExecResult{
			{Columns: []jdbc.Column{{Name: "ID", Type: "INTEGER"}, {Name: "NAME", Type: "VARCHAR"}},
				Rows: [][]*string{{stringPtr("1"), stringPtr("Widget")}}, HasResultSet: true},
			{Columns: []jdbc.Column{{Name: "COUNT", Type: "BIGINT"}},
				Rows: [][]*string{{stringPtr("1")}}, HasResultSet: true},
		},
	}
	server := newTestServer(backend, true)

	unauthorized := httptest.NewRequest(http.MethodGet, "/api/v1/inven_db/INVENTORY/PRODUCTS", nil)
	unauthRecorder := httptest.NewRecorder()
	server.ServeHTTP(unauthRecorder, unauthorized)
	if unauthRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", unauthRecorder.Code)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/inven_db/INVENTORY/PRODUCTS?limit=10", nil)
	request.Header.Set("X-HySQL-Key", "hysql_rest_test")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Data  []map[string]any `json:"data"`
		Total int64            `json:"total"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Data) != 1 || payload.Total != 1 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if len(backend.sqls) == 0 || !strings.Contains(backend.sqls[0], `LIMIT 10 OFFSET 0`) {
		t.Fatalf("pagination was not applied: %+v", backend.sqls)
	}
}

func TestInsertUsesParameters(t *testing.T) {
	backend := &fakeBackend{
		databases: []Database{{ID: "db1", Name: "inven_db", Status: "running"}},
		schema:    testSchema(),
		results:   []jdbc.ExecResult{{UpdateCount: 1}},
	}
	server := newTestServer(backend, false)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/inven_db/INVENTORY/PRODUCTS",
		strings.NewReader(`{"NAME":"Widget"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(backend.lastSQL, `"NAME"`) || !strings.Contains(backend.lastSQL, "?") {
		t.Fatalf("insert was not parameterized: %s", backend.lastSQL)
	}
	if len(backend.lastArgs) != 1 || backend.lastArgs[0] == nil || *backend.lastArgs[0] != "Widget" {
		t.Fatalf("unexpected insert params: %+v", backend.lastArgs)
	}
}

func TestOpenAPIAndUnknownTable(t *testing.T) {
	backend := &fakeBackend{
		databases: []Database{{ID: "db1", Name: "inven_db", Status: "running"}},
		schema:    testSchema(),
	}
	server := newTestServer(backend, false)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/inven_db/openapi.json", nil)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "3.0.3") {
		t.Fatalf("unexpected OpenAPI response: %d %s", recorder.Code, recorder.Body.String())
	}

	missing := httptest.NewRequest(http.MethodGet, "/api/v1/inven_db/INVENTORY/MISSING", nil)
	missingRecorder := httptest.NewRecorder()
	server.ServeHTTP(missingRecorder, missing)
	if missingRecorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown table, got %d", missingRecorder.Code)
	}
}

func stringPtr(value string) *string { return &value }
