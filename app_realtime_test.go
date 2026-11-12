package main

import "testing"

func TestMutationTarget(t *testing.T) {
	cases := []struct {
		sql       string
		operation string
		schema    string
		table     string
	}{
		{"INSERT INTO PUBLIC.USERS (name) VALUES ('x')", "insert", "PUBLIC", "USERS"},
		{`insert into "inventory"."products" (name) values ('x')`, "insert", "inventory", "products"},
		{"INSERT INTO users (name) VALUES ('x')", "insert", "", "users"},
		{"UPDATE users SET name='x'", "update", "", "users"},
		{"DELETE FROM inventory.audit_log WHERE id=1", "delete", "inventory", "audit_log"},
		{"MERGE INTO t KEY(id) VALUES (1)", "merge", "", "t"},
		{`INSERT INTO "my""schema"."my""table" VALUES (1)`, "insert", `my"schema`, `my"table`},
		{"CREATE TABLE IF NOT EXISTS \"INVENTORY\".\"FOO\" (id int)", "ddl", "INVENTORY", "FOO"},
		{"ALTER TABLE PUBLIC.USERS ADD COLUMN x INT", "ddl", "PUBLIC", "USERS"},
		{"CREATE SCHEMA IF NOT EXISTS demo", "ddl", "demo", ""},
		{"DROP VIEW inventory.v_low_stock", "ddl", "inventory", "v_low_stock"},
		{"-- note\nINSERT INTO a.b VALUES (1)", "insert", "a", "b"},
		{"/* note */ UPDATE \"a\".\"b\" SET x=1", "update", "a", "b"},
		{"SELECT * FROM users", "", "", ""},
		{"", "", "", ""},
	}
	for _, testCase := range cases {
		operation, schema, table := mutationTarget(testCase.sql)
		if operation != testCase.operation || schema != testCase.schema || table != testCase.table {
			t.Fatalf("mutationTarget(%q) = (%q, %q, %q), want (%q, %q, %q)",
				testCase.sql, operation, schema, table, testCase.operation, testCase.schema, testCase.table)
		}
	}
}
