package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"hysql/internal/aihelper"
	"hysql/internal/codegen"
	"hysql/internal/config"
	"hysql/internal/jdbc"
	"hysql/internal/mcp"
	"hysql/internal/restapi"
	"hysql/internal/sqlrunner"
)

// mcpBackend adapts the desktop app to the MCP tool contract.
type mcpBackend struct {
	app *App
}

func newMCPBackend(app *App) *mcpBackend { return &mcpBackend{app: app} }

func (b *mcpBackend) ListDatabases() []mcp.Database {
	manager, _, _ := b.app.components()
	if manager == nil {
		return []mcp.Database{}
	}
	restBase := b.app.restBaseURL()
	instances := manager.List()
	out := make([]mcp.Database, 0, len(instances))
	for _, instance := range instances {
		entry := mcp.Database{
			ID:           instance.ID,
			Name:         instance.Name,
			Status:       string(instance.Status),
			LocalJDBCURL: fmt.Sprintf("jdbc:h2:tcp://127.0.0.1:%d/./%s", instance.Port, instance.Name),
		}
		if instance.TailscaleEnabled && instance.TailscaleIP != "" {
			entry.TailscaleJDBCURL = fmt.Sprintf("jdbc:h2:tcp://%s:%d/./%s",
				instance.TailscaleIP, instance.TailscalePort, instance.Name)
		}
		if restBase != "" {
			entry.RESTBaseURL = restBase + "/api/v1/" + instance.Name
		}
		out = append(out, entry)
	}
	return out
}

func (b *mcpBackend) ResolveDatabase(name string) (mcp.Database, error) {
	instance, err := b.app.resolveInstance(name)
	if err != nil {
		return mcp.Database{}, err
	}
	entry := mcp.Database{
		ID:           instance.ID,
		Name:         instance.Name,
		Status:       string(instance.Status),
		LocalJDBCURL: fmt.Sprintf("jdbc:h2:tcp://127.0.0.1:%d/./%s", instance.Port, instance.Name),
	}
	if instance.TailscaleEnabled && instance.TailscaleIP != "" {
		entry.TailscaleJDBCURL = fmt.Sprintf("jdbc:h2:tcp://%s:%d/./%s",
			instance.TailscaleIP, instance.TailscalePort, instance.Name)
	}
	if restBase := b.app.restBaseURL(); restBase != "" {
		entry.RESTBaseURL = restBase + "/api/v1/" + instance.Name
	}
	return entry, nil
}

func (b *mcpBackend) InspectSchema(ctx context.Context, databaseName, schemaName string) (jdbc.SchemaSnapshot, error) {
	instance, err := b.app.resolveInstance(databaseName)
	if err != nil {
		return jdbc.SchemaSnapshot{}, err
	}
	manager, _, _ := b.app.components()
	runner := sqlrunner.New(manager, manager.Bridge())
	snapshot, err := runner.Tables(ctx, instance.ID)
	if err != nil {
		return jdbc.SchemaSnapshot{}, err
	}
	if strings.TrimSpace(schemaName) == "" {
		return snapshot, nil
	}
	filtered := jdbc.SchemaSnapshot{Tables: []jdbc.TableInfo{}}
	for _, table := range snapshot.Tables {
		if strings.EqualFold(table.Schema, schemaName) {
			filtered.Tables = append(filtered.Tables, table)
		}
	}
	return filtered, nil
}

func (b *mcpBackend) ReadQuery(ctx context.Context, databaseName, sql string) (jdbc.ExecResult, error) {
	instance, err := b.app.resolveInstance(databaseName)
	if err != nil {
		return jdbc.ExecResult{}, err
	}
	defer b.app.markDatabaseActivity(instance.ID)
	manager, _, _ := b.app.components()
	runner := sqlrunner.New(manager, manager.Bridge())
	return runner.Execute(ctx, instance.ID, sql)
}

func (b *mcpBackend) WriteQuery(ctx context.Context, databaseName, sql string) (jdbc.ExecResult, error) {
	instance, err := b.app.resolveInstance(databaseName)
	if err != nil {
		return jdbc.ExecResult{}, err
	}
	defer b.app.markDatabaseActivity(instance.ID)
	manager, _, _ := b.app.components()
	runner := sqlrunner.New(manager, manager.Bridge())
	result, err := runner.Execute(ctx, instance.ID, sql)
	if err != nil {
		return jdbc.ExecResult{}, err
	}
	operation, schema, table := mutationTarget(sql)
	if operation == "" {
		operation = "write"
	}
	b.app.emitRealtimeChange(RealtimeChangeEvent{
		InstanceID:  instance.ID,
		Schema:      schema,
		Table:       table,
		Operation:   operation,
		Source:      "mcp-agent",
		UpdateCount: result.UpdateCount,
	})
	return result, nil
}

func (b *mcpBackend) ExecuteDDL(ctx context.Context, databaseName, sql string) (mcp.DDLResult, error) {
	instance, err := b.app.resolveInstance(databaseName)
	if err != nil {
		return mcp.DDLResult{}, err
	}
	prefs, ok := b.app.preferences()
	autoSnapshot := ok && prefs.Get().AutoSnapshotOnAgentDDL
	checkpoint := ""
	keyword := sqlKeyword(sql)
	if autoSnapshot && (keyword == "DROP" || keyword == "TRUNCATE" || keyword == "ALTER") {
		snapshot, snapshotErr := b.app.createCheckpoint(instance, "Pre-Agent DDL")
		if snapshotErr != nil {
			return mcp.DDLResult{}, fmt.Errorf("auto-checkpoint failed; DDL was not executed: %w", snapshotErr)
		}
		checkpoint = snapshot.ID
	}
	defer b.app.markDatabaseActivity(instance.ID)
	manager, _, _ := b.app.components()
	runner := sqlrunner.New(manager, manager.Bridge())
	result, err := runner.Execute(ctx, instance.ID, sql)
	if err != nil {
		return mcp.DDLResult{}, err
	}
	_, schema, table := mutationTarget(sql)
	b.app.emitRealtimeChange(RealtimeChangeEvent{
		InstanceID:  instance.ID,
		Schema:      schema,
		Table:       table,
		Operation:   "ddl",
		Source:      "mcp-agent",
		UpdateCount: result.UpdateCount,
	})
	return mcp.DDLResult{UpdateCount: result.UpdateCount, Checkpoint: checkpoint}, nil
}

func (b *mcpBackend) ConnectionBundle(_ context.Context, databaseName string) (mcp.ConnectionBundle, error) {
	instance, err := b.app.resolveInstance(databaseName)
	if err != nil {
		return mcp.ConnectionBundle{}, err
	}
	artifacts := codegen.Generate(codegen.Request{
		InstanceID: instance.ID,
		Mode:       codegen.ModeLocal,
		Database:   instance.Name,
		Username:   instance.Username,
		Password:   instance.Password,
		Host:       "127.0.0.1",
		Port:       instance.Port,
		Available:  true,
	})
	bundle := mcp.ConnectionBundle{
		Database:         instance.Name,
		LocalJDBCURL:     artifacts.JDBCURL,
		Username:         instance.Username,
		Password:         instance.Password,
		HikariCP:         artifacts.Hikari,
		SpringProperties: artifacts.SpringProperties,
	}
	if instance.TailscaleEnabled && instance.TailscaleIP != "" {
		bundle.TailscaleJDBCURL = fmt.Sprintf("jdbc:h2:tcp://%s:%d/./%s",
			instance.TailscaleIP, instance.TailscalePort, instance.Name)
	}
	if prefs, ok := b.app.preferences(); ok {
		current := prefs.Get()
		bundle.RESTAPIKey = current.RESTAPIKey
	}
	if restBase := b.app.restBaseURL(); restBase != "" {
		bundle.RESTBaseURL = restBase + "/api/v1/" + instance.Name
		bundle.OpenAPIURL = restBase + "/api/v1/" + instance.Name + "/openapi.json"
	}
	return bundle, nil
}

func (b *mcpBackend) AgentReadOnly() bool {
	if prefs, ok := b.app.preferences(); ok {
		return prefs.Get().AgentReadOnly
	}
	return false
}

// restBackend adapts the desktop app to the Auto-REST gateway contract.
type restBackend struct {
	app *App
}

func newRESTBackend(app *App) *restBackend { return &restBackend{app: app} }

func (b *restBackend) Databases() []restapi.Database {
	manager, _, _ := b.app.components()
	if manager == nil {
		return []restapi.Database{}
	}
	instances := manager.List()
	out := make([]restapi.Database, 0, len(instances))
	for _, instance := range instances {
		out = append(out, restapi.Database{
			ID:     instance.ID,
			Name:   instance.Name,
			Status: string(instance.Status),
		})
	}
	return out
}

func (b *restBackend) ResolveDatabase(name string) (restapi.Database, error) {
	instance, err := b.app.resolveInstance(name)
	if err != nil {
		return restapi.Database{}, err
	}
	return restapi.Database{
		ID:     instance.ID,
		Name:   instance.Name,
		Status: string(instance.Status),
	}, nil
}

func (b *restBackend) Schema(ctx context.Context, databaseID string) (jdbc.SchemaSnapshot, error) {
	manager, _, _ := b.app.components()
	runner := sqlrunner.New(manager, manager.Bridge())
	return runner.Tables(ctx, databaseID)
}

func (b *restBackend) Query(ctx context.Context, databaseID, sql string, params []*string) (jdbc.ExecResult, error) {
	defer b.app.markDatabaseActivity(databaseID)
	manager, _, _ := b.app.components()
	url, username, password, err := manager.ConnectionInfo(databaseID)
	if err != nil {
		return jdbc.ExecResult{}, err
	}
	result, err := manager.Bridge().Query(ctx, url, username, password, sql, params)
	if err != nil {
		return jdbc.ExecResult{}, err
	}
	if !result.HasResultSet {
		b.app.emitRealtimeChange(RealtimeChangeEvent{
			InstanceID:  databaseID,
			Operation:   "external",
			Source:      "rest-api",
			UpdateCount: result.UpdateCount,
		})
	}
	return result, nil
}

func (a *App) resolveInstance(name string) (config.Instance, error) {
	manager, _, _ := a.components()
	if manager == nil {
		return config.Instance{}, errors.New("HySQL backend is not initialized")
	}
	target := strings.TrimSpace(name)
	if target == "" {
		return config.Instance{}, errors.New("database name is required")
	}
	for _, instance := range manager.List() {
		if instance.ID == target || strings.EqualFold(instance.Name, target) {
			return instance, nil
		}
	}
	return config.Instance{}, fmt.Errorf("database %q was not found", name)
}

func (a *App) restBaseURL() string {
	_, restServer := a.featureServers()
	if restServer == nil {
		return ""
	}
	state := restServer.State()
	if !state.Running {
		return ""
	}
	return strings.TrimRight(state.BaseURL, "/")
}

func sqlKeyword(sql string) string {
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
	for end < len(remaining) && ((remaining[end] >= 'a' && remaining[end] <= 'z') ||
		(remaining[end] >= 'A' && remaining[end] <= 'Z')) {
		end++
	}
	return strings.ToUpper(remaining[:end])
}

// GenerateSQL produces one H2 statement from a natural-language prompt.
func (a *App) GenerateSQL(request GenerateSQLRequest) (GenerateSQLResult, error) {
	if err := a.ready(); err != nil {
		return GenerateSQLResult{}, err
	}
	instance, err := a.resolveInstance(request.ID)
	if err != nil {
		return GenerateSQLResult{}, err
	}
	if instance.Status != config.StatusRunning {
		return GenerateSQLResult{}, fmt.Errorf("%s is %s; start it before asking the AI for SQL", instance.Name, instance.Status)
	}
	prefs, ok := a.preferences()
	if !ok {
		return GenerateSQLResult{}, errors.New("preferences are not initialized")
	}
	current := prefs.Get()
	if current.AIProvider != config.ProviderOllama && current.AIAPIKey == "" {
		return GenerateSQLResult{}, errors.New("add an API key in AI settings, or switch to a local Ollama model")
	}
	manager, _, _ := a.components()
	runner := sqlrunner.New(manager, manager.Bridge())
	ctx, cancel := context.WithTimeout(a.baseContext(), 20*time.Second)
	snapshot, err := runner.Tables(ctx, instance.ID)
	cancel()
	if err != nil {
		return GenerateSQLResult{}, err
	}
	tables := toAIHelperTables(snapshot)
	ddl := aihelper.CompactDDL(tables)
	aiCtx, aiCancel := context.WithTimeout(a.baseContext(), 100*time.Second)
	defer aiCancel()
	statement, err := aihelper.GenerateSQL(aiCtx, aihelper.Settings{
		Provider: current.AIProvider,
		BaseURL:  current.AIBaseURL,
		Model:    current.AIModel,
		APIKey:   current.AIAPIKey,
	}, ddl, request.Prompt)
	if err != nil {
		return GenerateSQLResult{}, err
	}
	return GenerateSQLResult{SQL: statement, Provider: current.AIProvider, Model: current.AIModel}, nil
}

// SeedTableRows generates and inserts realistic rows into one table.
func (a *App) SeedTableRows(request SeedRowsRequest) (SeedRowsResult, error) {
	if err := a.ready(); err != nil {
		return SeedRowsResult{}, err
	}
	if request.Count <= 0 {
		request.Count = 10
	}
	if request.Count > 200 {
		return SeedRowsResult{}, errors.New("seed at most 200 rows per request")
	}
	instance, err := a.resolveInstance(request.ID)
	if err != nil {
		return SeedRowsResult{}, err
	}
	if instance.Status != config.StatusRunning {
		return SeedRowsResult{}, fmt.Errorf("%s is %s; start it before seeding rows", instance.Name, instance.Status)
	}
	manager, _, _ := a.components()
	runner := sqlrunner.New(manager, manager.Bridge())
	ctx, cancel := context.WithTimeout(a.baseContext(), 30*time.Second)
	defer cancel()
	snapshot, err := runner.Tables(ctx, instance.ID)
	if err != nil {
		return SeedRowsResult{}, err
	}
	table, ok := findSnapshotTable(snapshot, request.Schema, request.Table)
	if !ok {
		return SeedRowsResult{}, fmt.Errorf("table %s.%s was not found", request.Schema, request.Table)
	}
	if !strings.Contains(strings.ToUpper(table.Type), "TABLE") {
		return SeedRowsResult{}, fmt.Errorf("%s.%s is a %s and cannot be seeded", table.Schema, table.Name, table.Type)
	}
	aiTable := toAIHelperTable(table)
	foreignSamples := a.foreignKeySamples(instance.ID, table)

	prefs, _ := a.preferences()
	current := prefs.Get()
	rows := []map[string]*string{}
	generatedBy := "offline-fallback"
	aiConfigured := current.AIProvider == config.ProviderOllama || current.AIAPIKey != ""
	if aiConfigured {
		aiCtx, aiCancel := context.WithTimeout(a.baseContext(), 120*time.Second)
		aiRows, aiErr := aihelper.GenerateRows(aiCtx, aihelper.Settings{
			Provider: current.AIProvider,
			BaseURL:  current.AIBaseURL,
			Model:    current.AIModel,
			APIKey:   current.AIAPIKey,
		}, aiTable, request.Count, foreignSamples)
		aiCancel()
		if aiErr == nil && len(aiRows) > 0 {
			rows = mergeWithFallback(aiTable, aiRows, foreignSamples, request.Count)
			generatedBy = current.AIProvider + ":" + current.AIModel
		}
	}
	if len(rows) == 0 {
		rows = aihelper.FallbackRows(aiTable, request.Count, foreignSamples)
	}
	// H2 identity sequences do not move when rows were inserted with explicit
	// IDs ("GENERATED BY DEFAULT"), so the sequence can still point at 1 while
	// row 1 exists. Align it before inserting generated rows.
	if err := restartIdentityColumns(ctx, runner, instance.ID, table); err != nil {
		return SeedRowsResult{}, err
	}
	if offsets := nextIntegerKeyValues(ctx, runner, instance.ID, table); len(offsets) > 0 {
		for index, row := range rows {
			for column, offset := range offsets {
				value := strconv.FormatInt(offset+int64(index)+1, 10)
				row[column] = &value
			}
		}
	}

	statement, err := buildSeedInsert(table, rows)
	if err != nil {
		return SeedRowsResult{}, err
	}
	defer a.markDatabaseActivity(instance.ID)
	result, err := runner.Execute(ctx, instance.ID, statement)
	if err != nil {
		return SeedRowsResult{}, err
	}
	inserted := int(result.UpdateCount)
	if inserted <= 0 {
		inserted = len(rows)
	}
	a.emitRealtimeChange(RealtimeChangeEvent{
		InstanceID:  instance.ID,
		Schema:      table.Schema,
		Table:       table.Name,
		Operation:   "insert",
		Source:      "ai-seeder",
		UpdateCount: int64(inserted),
	})
	return SeedRowsResult{Inserted: inserted, GeneratedBy: generatedBy}, nil
}

func (a *App) foreignKeySamples(instanceID string, table jdbc.TableInfo) map[string][]string {
	samples := map[string][]string{}
	manager, _, _ := a.components()
	if manager == nil {
		return samples
	}
	runner := sqlrunner.New(manager, manager.Bridge())
	for _, fk := range table.ForeignKeys {
		schema := fk.ReferencedSchema
		if strings.TrimSpace(schema) == "" {
			schema = table.Schema
		}
		statement := fmt.Sprintf("SELECT DISTINCT %s FROM %s.%s WHERE %s IS NOT NULL LIMIT 25",
			quoteSQLIdentifier(fk.ReferencedColumn),
			quoteSQLIdentifier(schema),
			quoteSQLIdentifier(fk.ReferencedTable),
			quoteSQLIdentifier(fk.ReferencedColumn))
		ctx, cancel := context.WithTimeout(a.baseContext(), 10*time.Second)
		result, err := runner.Execute(ctx, instanceID, statement)
		cancel()
		if err != nil {
			continue
		}
		for _, row := range result.Rows {
			if len(row) > 0 && row[0] != nil {
				samples[strings.ToUpper(fk.Column)] = append(samples[strings.ToUpper(fk.Column)], *row[0])
			}
		}
	}
	return samples
}

func toAIHelperTables(snapshot jdbc.SchemaSnapshot) []aihelper.Table {
	tables := make([]aihelper.Table, 0, len(snapshot.Tables))
	for _, table := range snapshot.Tables {
		tables = append(tables, toAIHelperTable(table))
	}
	return tables
}

func toAIHelperTable(table jdbc.TableInfo) aihelper.Table {
	primary := make(map[string]bool, len(table.PrimaryKey))
	for _, name := range table.PrimaryKey {
		primary[strings.ToUpper(name)] = true
	}
	result := aihelper.Table{Schema: table.Schema, Name: table.Name}
	for _, column := range table.Columns {
		result.Columns = append(result.Columns, aihelper.Column{
			Name:          column.Name,
			Type:          column.Type,
			Size:          column.Size,
			Nullable:      column.Nullable,
			PrimaryKey:    primary[strings.ToUpper(column.Name)],
			AutoIncrement: column.AutoIncrement,
			Generated:     column.Generated,
			Default:       column.Default,
		})
	}
	for _, fk := range table.ForeignKeys {
		result.ForeignKeys = append(result.ForeignKeys, aihelper.ForeignKey{
			Column:           fk.Column,
			ReferencedSchema: fk.ReferencedSchema,
			ReferencedTable:  fk.ReferencedTable,
			ReferencedColumn: fk.ReferencedColumn,
		})
	}
	return result
}

func mergeWithFallback(table aihelper.Table, rows []map[string]*string, foreignSamples map[string][]string, count int) []map[string]*string {
	fallback := aihelper.FallbackRows(table, count, foreignSamples)
	merged := make([]map[string]*string, 0, count)
	columnByName := map[string]aihelper.Column{}
	for _, column := range table.Columns {
		columnByName[strings.ToUpper(column.Name)] = column
	}
	for index := 0; index < count; index++ {
		source := map[string]*string{}
		if index < len(rows) {
			source = rows[index]
		}
		row := map[string]*string{}
		for key, value := range source {
			column, ok := columnByName[strings.ToUpper(key)]
			if !ok || column.AutoIncrement || column.Generated {
				continue
			}
			row[column.Name] = value
		}
		for _, column := range table.Columns {
			if column.AutoIncrement || column.Generated {
				continue
			}
			if _, exists := row[column.Name]; exists {
				continue
			}
			if index < len(fallback) {
				if value, ok := fallback[index][column.Name]; ok {
					row[column.Name] = value
				}
			}
		}
		merged = append(merged, row)
	}
	return merged
}

func buildSeedInsert(table jdbc.TableInfo, rows []map[string]*string) (string, error) {
	if len(rows) == 0 {
		return "", errors.New("no rows were generated")
	}
	columnByName := map[string]jdbc.ColumnInfo{}
	for _, column := range table.Columns {
		columnByName[strings.ToUpper(column.Name)] = column
	}
	used := map[string]bool{}
	for _, row := range rows {
		for key := range row {
			column, ok := columnByName[strings.ToUpper(key)]
			if !ok {
				return "", fmt.Errorf("generated row contains unknown column %s", key)
			}
			used[column.Name] = true
		}
	}
	columns := make([]string, 0, len(used))
	for _, column := range table.Columns {
		if column.AutoIncrement || column.Generated {
			continue
		}
		if used[column.Name] {
			columns = append(columns, column.Name)
		}
	}
	if len(columns) == 0 {
		return "", errors.New("no insertable columns were generated")
	}
	quotedColumns := make([]string, 0, len(columns))
	for _, column := range columns {
		quotedColumns = append(quotedColumns, quoteSQLIdentifier(column))
	}
	values := make([]string, 0, len(rows))
	for _, row := range rows {
		literals := make([]string, 0, len(columns))
		for _, column := range columns {
			literals = append(literals, stringLiteral(row[column]))
		}
		values = append(values, "("+strings.Join(literals, ", ")+")")
	}
	statement := "INSERT INTO " + quoteSQLIdentifier(table.Schema) + "." + quoteSQLIdentifier(table.Name) +
		" (" + strings.Join(quotedColumns, ", ") + ") VALUES " + strings.Join(values, ", ")
	if len(statement) > sqlrunner.MaxQueryLength {
		return "", errors.New("generated insert is too large; seed fewer rows")
	}
	return statement, nil
}

func quoteSQLIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func stringLiteral(value *string) string {
	if value == nil {
		return "NULL"
	}
	return "'" + strings.ReplaceAll(*value, "'", "''") + "'"
}

func findSnapshotTable(snapshot jdbc.SchemaSnapshot, schema, table string) (jdbc.TableInfo, bool) {
	for _, candidate := range snapshot.Tables {
		if strings.EqualFold(candidate.Schema, schema) && strings.EqualFold(candidate.Name, table) {
			return candidate, true
		}
	}
	return jdbc.TableInfo{}, false
}

// nextIntegerKeyValues finds the current MAX for integer primary keys that are
// not auto-increment so generated rows cannot collide with existing data.
func nextIntegerKeyValues(ctx context.Context, runner *sqlrunner.Runner, instanceID string, table jdbc.TableInfo) map[string]int64 {
	offsets := map[string]int64{}
	for _, name := range table.PrimaryKey {
		for _, column := range table.Columns {
			if !strings.EqualFold(column.Name, name) || column.AutoIncrement || column.Generated {
				continue
			}
			upper := strings.ToUpper(column.Type)
			if !strings.Contains(upper, "INT") {
				continue
			}
			if value, ok := maxColumnValue(ctx, runner, instanceID, table, column.Name); ok {
				offsets[column.Name] = value
			}
		}
	}
	return offsets
}

// restartIdentityColumns moves each identity sequence past the largest value
// already stored. This fixes GENERATED BY DEFAULT identities whose sequence is
// behind explicit IDs inserted by scripts or previous rows.
func restartIdentityColumns(ctx context.Context, runner *sqlrunner.Runner, instanceID string, table jdbc.TableInfo) error {
	target := quoteSQLIdentifier(table.Schema) + "." + quoteSQLIdentifier(table.Name)
	for _, column := range table.Columns {
		if !column.AutoIncrement || column.Generated {
			continue
		}
		current, ok := maxColumnValue(ctx, runner, instanceID, table, column.Name)
		if !ok {
			current = 0
		}
		next := current + 1
		statement := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s RESTART WITH %d",
			target, quoteSQLIdentifier(column.Name), next)
		if _, err := runner.Execute(ctx, instanceID, statement); err != nil {
			return fmt.Errorf("align identity %s.%s.%s: %w",
				table.Schema, table.Name, column.Name, err)
		}
	}
	return nil
}

func maxColumnValue(ctx context.Context, runner *sqlrunner.Runner, instanceID string, table jdbc.TableInfo, column string) (int64, bool) {
	statement := fmt.Sprintf("SELECT MAX(%s) FROM %s.%s",
		quoteSQLIdentifier(column),
		quoteSQLIdentifier(table.Schema),
		quoteSQLIdentifier(table.Name))
	queryCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	result, err := runner.Execute(queryCtx, instanceID, statement)
	cancel()
	if err != nil || !result.HasResultSet || len(result.Rows) == 0 ||
		len(result.Rows[0]) == 0 || result.Rows[0][0] == nil {
		return 0, false
	}
	value, parseErr := strconv.ParseInt(*result.Rows[0][0], 10, 64)
	if parseErr != nil {
		return 0, false
	}
	return value, true
}
