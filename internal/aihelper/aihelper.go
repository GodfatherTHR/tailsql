// Package aihelper adds optional schema-aware LLM assistance to TailSQL.
package aihelper

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Provider identifiers match config.Preference values.
const (
	ProviderOllama     = "ollama"
	ProviderOpenRouter = "openrouter"
	ProviderOpenAI     = "openai"
)

const requestTimeout = 90 * time.Second

// Settings selects an LLM endpoint.
type Settings struct {
	Provider string
	BaseURL  string
	Model    string
	APIKey   string
}

// Column describes one column for prompt and fallback generation.
type Column struct {
	Name          string
	Type          string
	Size          int
	Nullable      bool
	PrimaryKey    bool
	AutoIncrement bool
	Generated     bool
	Default       string
}

// ForeignKey links a column to a referenced table/column.
type ForeignKey struct {
	Column           string
	ReferencedSchema string
	ReferencedTable  string
	ReferencedColumn string
}

// Table is the seeding context for one table.
type Table struct {
	Schema      string
	Name        string
	Columns     []Column
	ForeignKeys []ForeignKey
}

// ErrNotConfigured indicates that no usable AI provider is configured.
var ErrNotConfigured = errors.New("AI is not configured; open AI settings in Connect to AI")

// GenerateSQL asks the provider for one H2 statement that answers question.
func GenerateSQL(ctx context.Context, settings Settings, ddl, question string) (string, error) {
	question = strings.TrimSpace(question)
	if question == "" {
		return "", errors.New("enter a question or instruction for the AI")
	}
	system := "You are an H2 2.x SQL expert. Use only the tables and columns in the schema below. " +
		"Return exactly one executable H2 SQL statement with no prose and no markdown fences. " +
		"Prefer explicit schema-qualified names. Never invent columns.\n\nSchema:\n" + ddl
	content, err := chat(ctx, settings, system, question)
	if err != nil {
		return "", err
	}
	sql := extractSQL(content)
	if sql == "" {
		return "", errors.New("the AI response did not contain a SQL statement")
	}
	return sql, nil
}

// GenerateRows asks the provider for count realistic rows as a JSON array.
// If the provider is unconfigured or fails, callers should use FallbackRows.
func GenerateRows(ctx context.Context, settings Settings, table Table, count int, foreignSamples map[string][]string) ([]map[string]*string, error) {
	if count <= 0 {
		return nil, errors.New("row count must be positive")
	}
	if count > 200 {
		return nil, errors.New("row count is limited to 200 per request")
	}
	system := "You generate realistic mock data for an H2 database. Reply with a JSON array only, " +
		"no prose and no markdown. Each array element is an object keyed by the exact column names. " +
		"Respect NOT NULL columns, do not include auto-increment or generated columns, and use the given " +
		"foreign-key samples for referential integrity.\n\n" + DescribeTable(table, foreignSamples)
	user := fmt.Sprintf("Generate %d rows for %s.%s. Return JSON only.", count, table.Schema, table.Name)
	content, err := chat(ctx, settings, system, user)
	if err != nil {
		return nil, err
	}
	rows, err := parseRows(content, table)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errors.New("the AI returned no rows")
	}
	if len(rows) < count {
		fallback := FallbackRows(table, count, foreignSamples)
		for index := len(rows); index < count; index++ {
			rows = append(rows, fallback[index])
		}
	}
	return rows[:count], nil
}

// FallbackRows deterministically generates valid rows without an LLM.
func FallbackRows(table Table, count int, foreignSamples map[string][]string) []map[string]*string {
	if count <= 0 {
		return []map[string]*string{}
	}
	rows := make([]map[string]*string, 0, count)
	for index := 0; index < count; index++ {
		row := make(map[string]*string, len(table.Columns))
		for _, column := range table.Columns {
			if column.AutoIncrement || column.Generated {
				continue
			}
			if samples := foreignSamples[strings.ToUpper(column.Name)]; len(samples) > 0 {
				value := samples[index%len(samples)]
				row[column.Name] = &value
				continue
			}
			value, ok := fallbackValue(column, index)
			if !ok {
				continue
			}
			row[column.Name] = &value
		}
		rows = append(rows, row)
	}
	return rows
}

// DescribeTable renders a compact schema for prompts.
func DescribeTable(table Table, foreignSamples map[string][]string) string {
	var builder strings.Builder
	builder.WriteString(table.Schema + "." + table.Name + " (\n")
	for _, column := range table.Columns {
		builder.WriteString("  " + column.Name + " " + column.Type)
		if !column.Nullable {
			builder.WriteString(" NOT NULL")
		}
		if column.PrimaryKey {
			builder.WriteString(" PRIMARY KEY")
		}
		if column.AutoIncrement {
			builder.WriteString(" AUTO_INCREMENT")
		}
		if column.Default != "" {
			builder.WriteString(" DEFAULT " + column.Default)
		}
		if samples := foreignSamples[strings.ToUpper(column.Name)]; len(samples) > 0 {
			builder.WriteString(" -- sample values: " + strings.Join(samples, ", "))
		}
		builder.WriteString("\n")
	}
	builder.WriteString(")")
	return builder.String()
}

// CompactDDL renders schema-level DDL for NL→SQL prompts.
func CompactDDL(tables []Table) string {
	var builder strings.Builder
	for _, table := range tables {
		builder.WriteString("CREATE TABLE " + table.Schema + "." + table.Name + " (")
		for index, column := range table.Columns {
			if index > 0 {
				builder.WriteString(", ")
			}
			builder.WriteString(column.Name + " " + column.Type)
			if !column.Nullable {
				builder.WriteString(" NOT NULL")
			}
			if column.PrimaryKey {
				builder.WriteString(" PRIMARY KEY")
			}
		}
		builder.WriteString(");")
		for _, fk := range table.ForeignKeys {
			builder.WriteString(fmt.Sprintf(" -- FK %s.%s -> %s.%s.%s;",
				table.Name, fk.Column, fk.ReferencedSchema, fk.ReferencedTable, fk.ReferencedColumn))
		}
		builder.WriteString("\n")
	}
	return builder.String()
}

func extractSQL(content string) string {
	content = strings.TrimSpace(content)
	fence := regexp.MustCompile("(?s)```(?:sql|h2|sql2)?\\s*(.*?)```")
	if match := fence.FindStringSubmatch(content); len(match) == 2 {
		content = match[1]
	}
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "SQL:")
	content = strings.TrimSpace(content)
	return strings.TrimSpace(content)
}

func parseRows(content string, table Table) ([]map[string]*string, error) {
	content = strings.TrimSpace(content)
	fence := regexp.MustCompile("(?s)```(?:json)?\\s*(.*?)```")
	if match := fence.FindStringSubmatch(content); len(match) == 2 {
		content = strings.TrimSpace(match[1])
	}
	start := strings.Index(content, "[")
	end := strings.LastIndex(content, "]")
	if start >= 0 && end > start {
		content = content[start : end+1]
	}
	var rawRows []map[string]any
	if err := json.Unmarshal([]byte(content), &rawRows); err != nil {
		return nil, fmt.Errorf("the AI did not return a JSON array of rows: %w", err)
	}
	allowed := make(map[string]string, len(table.Columns))
	for _, column := range table.Columns {
		if column.AutoIncrement || column.Generated {
			continue
		}
		allowed[strings.ToUpper(column.Name)] = column.Name
	}
	out := make([]map[string]*string, 0, len(rawRows))
	for _, raw := range rawRows {
		row := make(map[string]*string, len(raw))
		for key, value := range raw {
			name, ok := allowed[strings.ToUpper(key)]
			if !ok {
				continue
			}
			row[name] = jsonScalar(value)
		}
		if len(row) > 0 {
			out = append(out, row)
		}
	}
	return out, nil
}

func jsonScalar(value any) *string {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		return &typed
	case bool:
		text := "false"
		if typed {
			text = "true"
		}
		return &text
	case float64:
		if typed == float64(int64(typed)) {
			text := strconv.FormatInt(int64(typed), 10)
			return &text
		}
		text := strconv.FormatFloat(typed, 'f', -1, 64)
		return &text
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return nil
		}
		text := string(data)
		return &text
	}
}

func chat(ctx context.Context, settings Settings, system, user string) (string, error) {
	if strings.TrimSpace(settings.Provider) == "" {
		return "", ErrNotConfigured
	}
	if settings.Provider != ProviderOllama && strings.TrimSpace(settings.APIKey) == "" {
		return "", errors.New("AI API key is missing; open AI settings and add a key")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(settings.BaseURL), "/")
	if baseURL == "" {
		if settings.Provider == ProviderOllama {
			baseURL = "http://127.0.0.1:11434"
		} else {
			baseURL = "https://api.openai.com/v1"
		}
	}
	model := strings.TrimSpace(settings.Model)
	if model == "" {
		model = "qwen2.5-coder:7b"
	}

	client := &http.Client{Timeout: requestTimeout}
	var payload any
	var endpoint string
	if settings.Provider == ProviderOllama {
		endpoint = baseURL + "/api/chat"
		payload = map[string]any{
			"model":  model,
			"stream": false,
			"messages": []map[string]string{
				{"role": "system", "content": system},
				{"role": "user", "content": user},
			},
		}
	} else {
		endpoint = baseURL + "/chat/completions"
		payload = map[string]any{
			"model": model,
			"messages": []map[string]string{
				{"role": "system", "content": system},
				{"role": "user", "content": user},
			},
			"temperature": 0.2,
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode AI request: %w", err)
	}
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build AI request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if settings.Provider != ProviderOllama && settings.APIKey != "" {
		request.Header.Set("Authorization", "Bearer "+settings.APIKey)
	}
	if settings.Provider == ProviderOpenRouter {
		request.Header.Set("HTTP-Referer", "https://github.com/GodfatherTHR/tailsql")
		request.Header.Set("X-Title", "TailSQL")
	}

	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("AI request failed: %w", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return "", fmt.Errorf("read AI response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("AI provider returned HTTP %d: %s", response.StatusCode, truncate(string(data), 500))
	}

	if settings.Provider == ProviderOllama {
		var decoded struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal(data, &decoded); err != nil {
			return "", fmt.Errorf("parse Ollama response: %w", err)
		}
		return decoded.Message.Content, nil
	}
	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return "", fmt.Errorf("parse AI response: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return "", errors.New("AI provider returned no choices")
	}
	return decoded.Choices[0].Message.Content, nil
}

func fallbackValue(column Column, index int) (string, bool) {
	name := strings.ToLower(column.Name)
	typeName := strings.ToUpper(column.Type)
	switch {
	case strings.Contains(typeName, "BINARY") || strings.Contains(typeName, "BLOB") ||
		strings.Contains(typeName, "ARRAY") || strings.Contains(typeName, "JSON"):
		return "", false
	case strings.Contains(typeName, "BOOL"):
		if index%2 == 0 {
			return "true", true
		}
		return "false", true
	case strings.Contains(typeName, "TINYINT") || strings.Contains(typeName, "SMALLINT") ||
		strings.Contains(typeName, "INT") || strings.Contains(typeName, "BIGINT"):
		return strconv.Itoa(index + 1), true
	case strings.Contains(typeName, "DECIMAL") || strings.Contains(typeName, "NUMERIC") ||
		strings.Contains(typeName, "DOUBLE") || strings.Contains(typeName, "REAL") ||
		strings.Contains(typeName, "FLOAT"):
		return strconv.FormatFloat(9.99+float64(index)*3.25, 'f', 2, 64), true
	case strings.Contains(typeName, "TIMESTAMP") || strings.Contains(typeName, "DATETIME"):
		return time.Date(2025, 1, 2, 9, 30, 0, 0, time.UTC).Add(time.Duration(index) * 6 * time.Hour).
			Format("2006-01-02 15:04:05"), true
	case strings.Contains(typeName, "DATE"):
		return time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC).AddDate(0, 0, index).
			Format("2006-01-02"), true
	case strings.Contains(typeName, "TIME"):
		return time.Date(2025, 1, 2, 9, 30, 0, 0, time.UTC).Add(time.Duration(index) * 17 * time.Minute).
			Format("15:04:05"), true
	case strings.Contains(typeName, "UUID"):
		return fmt.Sprintf("00000000-0000-4000-8000-%012d", index+1), true
	}

	var value string
	switch {
	case strings.Contains(name, "email"):
		value = fmt.Sprintf("%s%d@example.com", firstName(index), index+1)
	case strings.Contains(name, "first"):
		value = firstName(index)
	case strings.Contains(name, "last"):
		value = lastName(index)
	case strings.Contains(name, "user"):
		value = fmt.Sprintf("%s_%d", strings.ToLower(firstName(index)), index+1)
	case strings.Contains(name, "company") || strings.Contains(name, "vendor") || strings.Contains(name, "supplier"):
		value = companies[index%len(companies)]
	case strings.Contains(name, "city"):
		value = cities[index%len(cities)]
	case strings.Contains(name, "country"):
		value = countries[index%len(countries)]
	case strings.Contains(name, "phone"):
		value = fmt.Sprintf("+1-555-%04d", 1000+index)
	case strings.Contains(name, "status") || strings.Contains(name, "state"):
		value = statuses[index%len(statuses)]
	case strings.Contains(name, "category") || strings.Contains(name, "type") || strings.Contains(name, "kind"):
		value = categories[index%len(categories)]
	case strings.Contains(name, "currency"):
		value = "USD"
	case strings.Contains(name, "url") || strings.Contains(name, "website"):
		value = fmt.Sprintf("https://example.com/%s-%d", strings.ReplaceAll(name, "_", "-"), index+1)
	case strings.Contains(name, "sku") || strings.Contains(name, "code"):
		value = fmt.Sprintf("SKU-%04d", 1000+index)
	case strings.Contains(name, "title"):
		value = fmt.Sprintf("Sample %s %d", strings.ReplaceAll(name, "_", " "), index+1)
	case strings.Contains(name, "description") || strings.Contains(name, "note") || strings.Contains(name, "address"):
		value = fmt.Sprintf("Realistic %s number %d", strings.ReplaceAll(name, "_", " "), index+1)
	default:
		value = fmt.Sprintf("sample-%d", index+1)
		if column.Size > 0 && len(value) > column.Size {
			value = value[:column.Size]
		}
	}
	if column.Size > 0 && len(value) > column.Size {
		value = value[:column.Size]
	}
	return value, true
}

var (
	firstNames = []string{"Ava", "Noah", "Mia", "Liam", "Zoe", "Ethan", "Isla", "Owen", "Nora", "Lucas"}
	lastNames  = []string{"Carter", "Nguyen", "Patel", "Silva", "Kim", "Brooks", "Haddad", "Ibrahim", "Moreau", "Zhang"}
	companies  = []string{"Northwind Supply", "Blue Harbor Trading", "Peak Logistics", "Orbit Retail", "Greenline Foods"}
	cities     = []string{"Austin", "Seattle", "Toronto", "Berlin", "Osaka", "Nairobi", "Lisbon"}
	countries  = []string{"United States", "Canada", "Germany", "Japan", "Kenya", "Portugal"}
	statuses   = []string{"active", "pending", "review", "completed"}
	categories = []string{"electronics", "home", "office", "outdoor", "grocery"}
)

func firstName(index int) string { return firstNames[index%len(firstNames)] }
func lastName(index int) string  { return lastNames[index%len(lastNames)] }

func truncate(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) <= max {
		return value
	}
	return value[:max] + "…"
}
