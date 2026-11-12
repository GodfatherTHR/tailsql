package sqlrunner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"tailsql/internal/dbmanager"
	"tailsql/internal/jdbc"
)

type fixedProvider struct {
	url      string
	username string
	password string
}

func (p fixedProvider) ConnectionInfo(string) (string, string, string, error) {
	return p.url, p.username, p.password, nil
}

func TestTableDataIntegration(t *testing.T) {
	if os.Getenv("TAILSQL_INTEGRATION") != "1" {
		t.Skip("set TAILSQL_INTEGRATION=1 to run real-H2 table editor tests")
	}
	java := dbmanager.DetectJava("")
	if !java.Supported {
		t.Skipf("Java 11+ not available: %s", java.Error)
	}
	assets, err := filepath.Abs(filepath.Join("..", "..", "assets"))
	if err != nil {
		t.Fatalf("resolve assets: %v", err)
	}
	h2 := dbmanager.ResolveH2Jar("", []string{filepath.Join(assets, dbmanager.H2FileName)})
	if !h2.Available {
		t.Skipf("H2 jar not available: %s", h2.Error)
	}
	classDir, err := jdbc.EnsureClass(filepath.Join(t.TempDir(), "bridge"))
	if err != nil {
		t.Fatalf("materialize bridge: %v", err)
	}

	url := "jdbc:h2:file:" + filepath.ToSlash(filepath.Join(t.TempDir(), "tableeditor"))
	provider := fixedProvider{url: url, username: "sa"}
	bridge := &jdbc.Bridge{JavaPath: java.Path, JarPath: h2.Path, ClassDir: classDir}
	runner := New(provider, bridge)

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if _, err := bridge.Execute(ctx, url, "sa", "",
		"CREATE TABLE items(id INT AUTO_INCREMENT PRIMARY KEY, name VARCHAR(100) NOT NULL DEFAULT 'unnamed', qty INT, note VARCHAR(200))"); err != nil {
		t.Fatalf("create table: %v", err)
	}

	inserted, err := runner.Insert(ctx, RowMutation{
		Schema: "PUBLIC",
		Table:  "ITEMS",
		Values: map[string]*string{
			"NAME": stringPtr("alpha"),
			"QTY":  stringPtr("3"),
		},
	})
	if err != nil {
		t.Fatalf("insert row: %v", err)
	}
	if inserted != 1 {
		t.Fatalf("insert affected %d rows", inserted)
	}

	page, err := runner.Rows(ctx, TableRowsRequest{
		Schema:  "PUBLIC",
		Table:   "ITEMS",
		Limit:   10,
		Offset:  0,
		OrderBy: "ID",
	})
	if err != nil {
		t.Fatalf("read rows: %v", err)
	}
	if len(page.Rows) != 1 || page.HasMore {
		t.Fatalf("unexpected first page: %+v", page)
	}
	if page.Rows[0][1] == nil || *page.Rows[0][1] != "alpha" {
		t.Fatalf("unexpected inserted value: %+v", page.Rows[0])
	}

	updated, err := runner.Update(ctx, RowMutation{
		Schema: "PUBLIC",
		Table:  "ITEMS",
		Key:    map[string]*string{"ID": stringPtr("1")},
		Values: map[string]*string{
			"QTY":  nil,
			"NOTE": stringPtr("O'Reilly"),
		},
	})
	if err != nil {
		t.Fatalf("update row: %v", err)
	}
	if updated != 1 {
		t.Fatalf("update affected %d rows", updated)
	}

	page, err = runner.Rows(ctx, TableRowsRequest{Schema: "PUBLIC", Table: "ITEMS", Limit: 10, OrderBy: "ID"})
	if err != nil {
		t.Fatalf("read updated rows: %v", err)
	}
	columns := map[string]int{}
	for index, column := range page.Columns {
		columns[column.Name] = index
	}
	if page.Rows[0][columns["QTY"]] != nil {
		t.Fatalf("expected QTY to be NULL: %+v", page.Rows[0])
	}
	if page.Rows[0][columns["NOTE"]] == nil || *page.Rows[0][columns["NOTE"]] != "O'Reilly" {
		t.Fatalf("expected escaped quote value: %+v", page.Rows[0])
	}

	defaulted, err := runner.Insert(ctx, RowMutation{Schema: "PUBLIC", Table: "ITEMS"})
	if err != nil {
		t.Fatalf("insert default row: %v", err)
	}
	if defaulted != 1 {
		t.Fatalf("default insert affected %d rows", defaulted)
	}
	page, err = runner.Rows(ctx, TableRowsRequest{Schema: "PUBLIC", Table: "ITEMS", Limit: 10, OrderBy: "ID"})
	if err != nil {
		t.Fatalf("read default rows: %v", err)
	}
	if len(page.Rows) != 2 || page.Rows[1][columns["NAME"]] == nil || *page.Rows[1][columns["NAME"]] != "unnamed" {
		t.Fatalf("database default was not applied: %+v", page.Rows)
	}

	injection := "x'); DROP TABLE items; --"
	if _, err := runner.Insert(ctx, RowMutation{
		Schema: "PUBLIC",
		Table:  "ITEMS",
		Values: map[string]*string{"NAME": stringPtr(injection)},
	}); err != nil {
		t.Fatalf("insert literal payload: %v", err)
	}
	schema, err := bridge.Tables(ctx, url, "sa", "")
	if err != nil {
		t.Fatalf("read schema after literal payload: %v", err)
	}
	found := false
	for _, table := range schema.Tables {
		if table.Name == "ITEMS" {
			found = true
			if len(table.PrimaryKey) != 1 || table.PrimaryKey[0] != "ID" {
				t.Fatalf("unexpected primary key metadata: %+v", table.PrimaryKey)
			}
			for _, column := range table.Columns {
				if column.Name == "ID" && !column.AutoIncrement {
					t.Fatal("ID should be flagged as an identity column")
				}
				if column.Name == "NAME" && column.Default == "" {
					t.Fatal("NAME default was not reported")
				}
			}
		}
	}
	if !found {
		t.Fatal("ITEMS disappeared after a value containing SQL metacharacters")
	}

	deleted, err := runner.Delete(ctx, RowMutation{
		Schema: "PUBLIC",
		Table:  "ITEMS",
		Key:    map[string]*string{"ID": stringPtr("1")},
	})
	if err != nil {
		t.Fatalf("delete row: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("delete affected %d rows", deleted)
	}

	if _, err := runner.Delete(ctx, RowMutation{Schema: "PUBLIC", Table: "ITEMS"}); err == nil {
		t.Fatal("expected delete without a key to fail")
	}

	for index := 0; index < 4; index++ {
		if _, err := runner.Insert(ctx, RowMutation{
			Schema: "PUBLIC",
			Table:  "ITEMS",
			Values: map[string]*string{"NAME": stringPtr("page")},
		}); err != nil {
			t.Fatalf("insert page row %d: %v", index, err)
		}
	}
	firstPage, err := runner.Rows(ctx, TableRowsRequest{Schema: "PUBLIC", Table: "ITEMS", Limit: 2, Offset: 0, OrderBy: "ID"})
	if err != nil {
		t.Fatalf("read first page: %v", err)
	}
	if len(firstPage.Rows) != 2 || !firstPage.HasMore {
		t.Fatalf("unexpected first page: rows=%d hasMore=%v", len(firstPage.Rows), firstPage.HasMore)
	}
	secondPage, err := runner.Rows(ctx, TableRowsRequest{Schema: "PUBLIC", Table: "ITEMS", Limit: 2, Offset: 4, OrderBy: "ID"})
	if err != nil {
		t.Fatalf("read second page: %v", err)
	}
	if len(secondPage.Rows) != 2 || secondPage.HasMore {
		t.Fatalf("unexpected second page: rows=%d hasMore=%v", len(secondPage.Rows), secondPage.HasMore)
	}

	if err := runner.DropTable(ctx, "", "PUBLIC", "ITEMS"); err != nil {
		t.Fatalf("drop table: %v", err)
	}
	schema, err = bridge.Tables(ctx, url, "sa", "")
	if err != nil {
		t.Fatalf("read schema after drop: %v", err)
	}
	for _, table := range schema.Tables {
		if table.Name == "ITEMS" {
			t.Fatal("ITEMS is still present after DropTable")
		}
	}
}
