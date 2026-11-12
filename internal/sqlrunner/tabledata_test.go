package sqlrunner

import (
	"strings"
	"testing"
)

func stringPtr(value string) *string {
	return &value
}

func TestBuildRowsSQL(t *testing.T) {
	statement, err := buildRowsSQL("inventory", "products", "price", true, 50, 100)
	if err != nil {
		t.Fatalf("build rows SQL: %v", err)
	}
	want := `SELECT * FROM "inventory"."products" ORDER BY "price" DESC LIMIT 51 OFFSET 100`
	if statement != want {
		t.Fatalf("unexpected SQL:\n got: %s\nwant: %s", statement, want)
	}
}

func TestBuildRowsSQLWithoutOrder(t *testing.T) {
	statement, err := buildRowsSQL("public", "users", "", false, 10, 0)
	if err != nil {
		t.Fatalf("build rows SQL: %v", err)
	}
	want := `SELECT * FROM "public"."users" LIMIT 11 OFFSET 0`
	if statement != want {
		t.Fatalf("unexpected SQL:\n got: %s\nwant: %s", statement, want)
	}
}

func TestBuildInsertSQL(t *testing.T) {
	statement, err := buildInsertSQL("inventory", "products", map[string]*string{
		"name":     stringPtr("O'Reilly"),
		"price":    stringPtr("19.99"),
		"notes":    nil,
		"category": stringPtr(""),
	})
	if err != nil {
		t.Fatalf("build insert SQL: %v", err)
	}
	want := `INSERT INTO "inventory"."products" ("category", "name", "notes", "price") VALUES ('', 'O''Reilly', NULL, '19.99')`
	if statement != want {
		t.Fatalf("unexpected SQL:\n got: %s\nwant: %s", statement, want)
	}
}

func TestBuildInsertSQLDefaults(t *testing.T) {
	statement, err := buildInsertSQL("public", "tasks", nil)
	if err != nil {
		t.Fatalf("build insert SQL: %v", err)
	}
	want := `INSERT INTO "public"."tasks" DEFAULT VALUES`
	if statement != want {
		t.Fatalf("unexpected SQL:\n got: %s\nwant: %s", statement, want)
	}
}

func TestBuildUpdateSQL(t *testing.T) {
	statement, err := buildUpdateSQL("inventory", "products",
		map[string]*string{"id": stringPtr("7")},
		map[string]*string{"name": stringPtr("new"), "price": nil},
	)
	if err != nil {
		t.Fatalf("build update SQL: %v", err)
	}
	want := `UPDATE "inventory"."products" SET "name" = 'new', "price" = NULL WHERE "id" IS NOT DISTINCT FROM '7'`
	if statement != want {
		t.Fatalf("unexpected SQL:\n got: %s\nwant: %s", statement, want)
	}
}

func TestBuildUpdateSQLRequiresValuesAndKey(t *testing.T) {
	if _, err := buildUpdateSQL("public", "tasks", map[string]*string{"id": stringPtr("1")}, nil); err == nil {
		t.Fatal("expected an error when there are no update values")
	}
	if _, err := buildUpdateSQL("public", "tasks", nil, map[string]*string{"name": stringPtr("x")}); err == nil {
		t.Fatal("expected an error when the key is empty")
	}
}

func TestBuildDeleteSQL(t *testing.T) {
	statement, err := buildDeleteSQL("todo", "tasks",
		map[string]*string{"id": stringPtr("1"), "owner": nil},
	)
	if err != nil {
		t.Fatalf("build delete SQL: %v", err)
	}
	want := `DELETE FROM "todo"."tasks" WHERE "id" IS NOT DISTINCT FROM '1' AND "owner" IS NOT DISTINCT FROM NULL`
	if statement != want {
		t.Fatalf("unexpected SQL:\n got: %s\nwant: %s", statement, want)
	}
}

func TestBuildDropTableSQL(t *testing.T) {
	statement, err := buildDropTableSQL("inventory", "products")
	if err != nil {
		t.Fatalf("build drop table SQL: %v", err)
	}
	want := `DROP TABLE "inventory"."products"`
	if statement != want {
		t.Fatalf("unexpected SQL:\n got: %s\nwant: %s", statement, want)
	}
	if _, err := buildDropTableSQL("", "products"); err == nil {
		t.Fatal("expected an error for an empty schema")
	}
}

func TestQuoteIdentifierEscapesInjection(t *testing.T) {
	quoted, err := quoteIdentifier(`x"; DROP TABLE products; --`)
	if err != nil {
		t.Fatalf("quote identifier: %v", err)
	}
	want := `"x""; DROP TABLE products; --"`
	if quoted != want {
		t.Fatalf("unexpected quoted identifier:\n got: %s\nwant: %s", quoted, want)
	}
	if _, err := quoteIdentifier(" \t "); err == nil {
		t.Fatal("expected an error for a blank identifier")
	}
	if _, err := quoteIdentifier("bad\nname"); err == nil {
		t.Fatal("expected an error for a control character")
	}
}

func TestSqlLiteral(t *testing.T) {
	if got := sqlLiteral(nil); got != "NULL" {
		t.Fatalf("nil literal = %s", got)
	}
	if got := sqlLiteral(stringPtr("")); got != "''" {
		t.Fatalf("empty literal = %s", got)
	}
	if got := sqlLiteral(stringPtr("O'Reilly")); got != "'O''Reilly'" {
		t.Fatalf("quoted literal = %s", got)
	}
	if got := sqlLiteral(stringPtr("a'b'c")); got != "'a''b''c'" {
		t.Fatalf("multiple quotes = %s", got)
	}
	if got := sqlLiteral(stringPtr(`back\slash`)); !strings.Contains(got, `\`) {
		t.Fatalf("backslash should be preserved: %s", got)
	}
}
