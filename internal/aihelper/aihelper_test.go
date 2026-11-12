package aihelper

import (
	"strings"
	"testing"
)

func sampleTable() Table {
	return Table{
		Schema: "inventory",
		Name:   "products",
		Columns: []Column{
			{Name: "ID", Type: "INTEGER", PrimaryKey: true},
			{Name: "NAME", Type: "VARCHAR", Size: 100},
			{Name: "EMAIL", Type: "VARCHAR", Size: 120},
			{Name: "PRICE", Type: "DECIMAL"},
			{Name: "IN_STOCK", Type: "BOOLEAN"},
			{Name: "CREATED_AT", Type: "TIMESTAMP"},
			{Name: "CATEGORY_ID", Type: "INTEGER"},
			{Name: "SERIAL", Type: "INTEGER", AutoIncrement: true},
			{Name: "TOTAL", Type: "DECIMAL", Generated: true},
		},
		ForeignKeys: []ForeignKey{{
			Column: "CATEGORY_ID", ReferencedSchema: "inventory", ReferencedTable: "categories", ReferencedColumn: "ID",
		}},
	}
}

func TestFallbackRowsAreTypeAware(t *testing.T) {
	rows := FallbackRows(sampleTable(), 3, map[string][]string{"CATEGORY_ID": {"7", "8"}})
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	if _, exists := rows[0]["SERIAL"]; exists {
		t.Fatal("auto-increment column must be skipped")
	}
	if _, exists := rows[0]["TOTAL"]; exists {
		t.Fatal("generated column must be skipped")
	}
	if rows[0]["ID"] == nil || *rows[0]["ID"] != "1" {
		t.Fatalf("unexpected integer value: %+v", rows[0]["ID"])
	}
	if rows[0]["EMAIL"] == nil || !strings.Contains(*rows[0]["EMAIL"], "@example.com") {
		t.Fatalf("unexpected email value: %+v", rows[0]["EMAIL"])
	}
	if rows[1]["IN_STOCK"] == nil || *rows[1]["IN_STOCK"] != "false" {
		t.Fatalf("unexpected boolean value: %+v", rows[1]["IN_STOCK"])
	}
	if rows[2]["CATEGORY_ID"] == nil || *rows[2]["CATEGORY_ID"] != "7" {
		t.Fatalf("foreign key sample was not reused: %+v", rows[2]["CATEGORY_ID"])
	}
}

func TestExtractSQLAndParseRows(t *testing.T) {
	sql := extractSQL("Here you go:\n```sql\nSELECT * FROM INVENTORY.PRODUCTS;\n```\n")
	if sql != "SELECT * FROM INVENTORY.PRODUCTS;" {
		t.Fatalf("unexpected extracted SQL: %q", sql)
	}
	rows, err := parseRows(`[{"NAME":"Widget","PRICE":12.5,"IN_STOCK":true,"UNKNOWN":1}]`, sampleTable())
	if err != nil {
		t.Fatalf("parse rows: %v", err)
	}
	if len(rows) != 1 || rows[0]["NAME"] == nil || *rows[0]["NAME"] != "Widget" {
		t.Fatalf("unexpected parsed rows: %+v", rows)
	}
	if _, exists := rows[0]["UNKNOWN"]; exists {
		t.Fatal("unknown columns must be dropped")
	}
	if rows[0]["PRICE"] == nil || *rows[0]["PRICE"] != "12.5" {
		t.Fatalf("unexpected numeric value: %+v", rows[0]["PRICE"])
	}
}

func TestCompactDDLIncludesColumnsAndRelations(t *testing.T) {
	ddl := CompactDDL([]Table{sampleTable()})
	for _, fragment := range []string{"inventory.products", "ID INTEGER", "FK products.CATEGORY_ID"} {
		if !strings.Contains(ddl, fragment) {
			t.Fatalf("DDL is missing %q:\n%s", fragment, ddl)
		}
	}
}
