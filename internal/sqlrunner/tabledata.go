package sqlrunner

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"hysql/internal/jdbc"
)

// Row page bounds protect the IPC boundary and the JVM bridge.
const (
	DefaultRowLimit = 50
	MaxRowLimit     = 200
	MaxRowOffset    = 10_000_000
)

// TableRowsRequest asks for one page of rows from a table.
type TableRowsRequest struct {
	InstanceID string
	Schema     string
	Table      string
	Limit      int
	Offset     int
	OrderBy    string
	Descending bool
}

// TableRows is one page of table data.
type TableRows struct {
	Columns []jdbc.Column
	Rows    [][]*string
	HasMore bool
	Limit   int
	Offset  int
}

// RowMutation identifies a row to update or delete, or carries insert values.
type RowMutation struct {
	InstanceID string
	Schema     string
	Table      string
	Values     map[string]*string
	Key        map[string]*string
}

// Rows reads one page using a single SELECT. One extra row is fetched so the
// UI can offer a Next page without a second COUNT query.
func (r *Runner) Rows(ctx context.Context, request TableRowsRequest) (TableRows, error) {
	limit := request.Limit
	if limit <= 0 {
		limit = DefaultRowLimit
	}
	if limit > MaxRowLimit {
		limit = MaxRowLimit
	}
	offset := request.Offset
	if offset < 0 {
		offset = 0
	}
	if offset > MaxRowOffset {
		offset = MaxRowOffset
	}

	statement, err := buildRowsSQL(request.Schema, request.Table, request.OrderBy, request.Descending, limit, offset)
	if err != nil {
		return TableRows{}, err
	}
	url, username, password, err := r.provider.ConnectionInfo(request.InstanceID)
	if err != nil {
		return TableRows{}, err
	}
	result, err := r.bridge.Execute(ctx, url, username, password, statement)
	if err != nil {
		return TableRows{}, err
	}

	rows := result.Rows
	hasMore := false
	if len(rows) > limit {
		rows = rows[:limit]
		hasMore = true
	}
	if rows == nil {
		rows = [][]*string{}
	}
	columns := result.Columns
	if columns == nil {
		columns = []jdbc.Column{}
	}
	return TableRows{
		Columns: columns,
		Rows:    rows,
		HasMore: hasMore,
		Limit:   limit,
		Offset:  offset,
	}, nil
}

// Insert adds one row and returns the number of rows affected.
func (r *Runner) Insert(ctx context.Context, request RowMutation) (int64, error) {
	statement, err := buildInsertSQL(request.Schema, request.Table, request.Values)
	if err != nil {
		return 0, err
	}
	return r.mutate(ctx, request.InstanceID, statement)
}

// Update changes the columns in Values on the row identified by Key.
func (r *Runner) Update(ctx context.Context, request RowMutation) (int64, error) {
	statement, err := buildUpdateSQL(request.Schema, request.Table, request.Key, request.Values)
	if err != nil {
		return 0, err
	}
	return r.mutate(ctx, request.InstanceID, statement)
}

// Delete removes the row identified by Key.
func (r *Runner) Delete(ctx context.Context, request RowMutation) (int64, error) {
	statement, err := buildDeleteSQL(request.Schema, request.Table, request.Key)
	if err != nil {
		return 0, err
	}
	return r.mutate(ctx, request.InstanceID, statement)
}

// DropTable removes a table together with all of its rows.
func (r *Runner) DropTable(ctx context.Context, instanceID, schema, table string) error {
	statement, err := buildDropTableSQL(schema, table)
	if err != nil {
		return err
	}
	_, err = r.mutate(ctx, instanceID, statement)
	return err
}

func (r *Runner) mutate(ctx context.Context, instanceID, statement string) (int64, error) {
	url, username, password, err := r.provider.ConnectionInfo(instanceID)
	if err != nil {
		return 0, err
	}
	result, err := r.bridge.Execute(ctx, url, username, password, statement)
	if err != nil {
		return 0, err
	}
	return result.UpdateCount, nil
}

func buildRowsSQL(schema, table, orderBy string, descending bool, limit, offset int) (string, error) {
	schemaSQL, err := quoteIdentifier(schema)
	if err != nil {
		return "", fmt.Errorf("invalid schema: %w", err)
	}
	tableSQL, err := quoteIdentifier(table)
	if err != nil {
		return "", fmt.Errorf("invalid table: %w", err)
	}
	statement := "SELECT * FROM " + schemaSQL + "." + tableSQL
	if strings.TrimSpace(orderBy) != "" {
		orderSQL, orderErr := quoteIdentifier(orderBy)
		if orderErr != nil {
			return "", fmt.Errorf("invalid sort column: %w", orderErr)
		}
		direction := "ASC"
		if descending {
			direction = "DESC"
		}
		statement += " ORDER BY " + orderSQL + " " + direction
	}
	statement += fmt.Sprintf(" LIMIT %d OFFSET %d", limit+1, offset)
	return statement, nil
}

func buildInsertSQL(schema, table string, values map[string]*string) (string, error) {
	target, err := quoteTarget(schema, table)
	if err != nil {
		return "", err
	}
	if len(values) == 0 {
		return "INSERT INTO " + target + " DEFAULT VALUES", nil
	}
	columns := sortedKeys(values)
	quotedColumns := make([]string, 0, len(columns))
	literals := make([]string, 0, len(columns))
	for _, column := range columns {
		quoted, quoteErr := quoteIdentifier(column)
		if quoteErr != nil {
			return "", fmt.Errorf("invalid insert column: %w", quoteErr)
		}
		quotedColumns = append(quotedColumns, quoted)
		literals = append(literals, sqlLiteral(values[column]))
	}
	return "INSERT INTO " + target + " (" + strings.Join(quotedColumns, ", ") + ") VALUES (" +
		strings.Join(literals, ", ") + ")", nil
}

func buildUpdateSQL(schema, table string, key, values map[string]*string) (string, error) {
	target, err := quoteTarget(schema, table)
	if err != nil {
		return "", err
	}
	if len(values) == 0 {
		return "", errors.New("no columns to update")
	}
	where, err := buildWhere(key)
	if err != nil {
		return "", err
	}

	columns := sortedKeys(values)
	assignments := make([]string, 0, len(columns))
	for _, column := range columns {
		quoted, quoteErr := quoteIdentifier(column)
		if quoteErr != nil {
			return "", fmt.Errorf("invalid update column: %w", quoteErr)
		}
		assignments = append(assignments, quoted+" = "+sqlLiteral(values[column]))
	}
	return "UPDATE " + target + " SET " + strings.Join(assignments, ", ") + " WHERE " + where, nil
}

func buildDeleteSQL(schema, table string, key map[string]*string) (string, error) {
	target, err := quoteTarget(schema, table)
	if err != nil {
		return "", err
	}
	where, err := buildWhere(key)
	if err != nil {
		return "", err
	}
	return "DELETE FROM " + target + " WHERE " + where, nil
}

func buildDropTableSQL(schema, table string) (string, error) {
	target, err := quoteTarget(schema, table)
	if err != nil {
		return "", err
	}
	return "DROP TABLE " + target, nil
}

func buildWhere(key map[string]*string) (string, error) {
	if len(key) == 0 {
		return "", errors.New("a primary key is required for row changes")
	}
	columns := sortedKeys(key)
	parts := make([]string, 0, len(columns))
	for _, column := range columns {
		quoted, err := quoteIdentifier(column)
		if err != nil {
			return "", fmt.Errorf("invalid key column: %w", err)
		}
		parts = append(parts, quoted+" IS NOT DISTINCT FROM "+sqlLiteral(key[column]))
	}
	return strings.Join(parts, " AND "), nil
}

func quoteTarget(schema, table string) (string, error) {
	schemaSQL, err := quoteIdentifier(schema)
	if err != nil {
		return "", fmt.Errorf("invalid schema: %w", err)
	}
	tableSQL, err := quoteIdentifier(table)
	if err != nil {
		return "", fmt.Errorf("invalid table: %w", err)
	}
	return schemaSQL + "." + tableSQL, nil
}

func quoteIdentifier(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", errors.New("identifier cannot be empty")
	}
	for _, char := range value {
		if char == 0 || char == '\r' || char == '\n' {
			return "", errors.New("identifier contains an unsupported control character")
		}
	}
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`, nil
}

func sqlLiteral(value *string) string {
	if value == nil {
		return "NULL"
	}
	return "'" + strings.ReplaceAll(*value, "'", "''") + "'"
}

func sortedKeys(values map[string]*string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
