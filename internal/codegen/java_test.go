package codegen

import (
	"strings"
	"testing"
)

func localRequest() Request {
	return Request{
		InstanceID: "id-1",
		Mode:       ModeLocal,
		Database:   "inventory_db",
		Username:   "sa",
		Password:   "Secret99pw",
		Host:       "127.0.0.1",
		Port:       9092,
		Available:  true,
	}
}

func TestGenerateLocalArtifacts(t *testing.T) {
	artifacts := Generate(localRequest())
	if !artifacts.Available {
		t.Fatalf("expected available artifacts: %s", artifacts.Reason)
	}
	if artifacts.JDBCURL != "jdbc:h2:tcp://127.0.0.1:9092/./inventory_db" {
		t.Fatalf("unexpected URL: %s", artifacts.JDBCURL)
	}
	if !strings.Contains(artifacts.Hikari, `"Secret99pw"`) {
		t.Fatal("Hikari class does not include the password")
	}
	if !strings.Contains(artifacts.Hikari, "setMaximumPoolSize(5)") ||
		!strings.Contains(artifacts.Hikari, "setConnectionTimeout(10000)") ||
		!strings.Contains(artifacts.Hikari, "setValidationTimeout(5_000)") ||
		!strings.Contains(artifacts.Hikari, "setInitializationFailTimeout(5_000)") {
		t.Fatal("Hikari pool settings missing")
	}
	if !strings.Contains(artifacts.Maven, "com.h2database") || !strings.Contains(artifacts.Maven, "HikariCP") {
		t.Fatal("Maven dependencies missing")
	}
	if !strings.Contains(artifacts.Gradle, "com.zaxxer:HikariCP:6.2.1") {
		t.Fatal("Gradle dependency missing")
	}
	if !strings.Contains(artifacts.SpringProperties, "jdbc:h2:tcp://127.0.0.1:9092/./inventory_db") {
		t.Fatal("Spring properties URL missing")
	}
	if !strings.Contains(artifacts.SpringYAML, "driver-class-name: org.h2.Driver") {
		t.Fatal("Spring YAML driver missing")
	}
	if !strings.Contains(artifacts.JDBCTest, "DriverManager.getConnection") {
		t.Fatal("raw JDBC snippet missing")
	}
	if strings.Contains(artifacts.Hikari, "0.0.0.0") || strings.Contains(artifacts.SpringProperties, "0.0.0.0") {
		t.Fatal("generated artifacts must never contain a wildcard host")
	}
}

func TestGenerateTailscaleArtifacts(t *testing.T) {
	request := localRequest()
	request.Mode = ModeTailscale
	request.Host = "100.76.208.54"
	request.Port = 19092
	request.MagicDNS = "workstation.tailnet-abc.ts.net."
	artifacts := Generate(request)
	if !artifacts.Available {
		t.Fatalf("expected available artifacts: %s", artifacts.Reason)
	}
	if artifacts.JDBCURL != "jdbc:h2:tcp://100.76.208.54:19092/./inventory_db" {
		t.Fatalf("unexpected URL: %s", artifacts.JDBCURL)
	}
	if artifacts.MagicDNS != "workstation.tailnet-abc.ts.net" {
		t.Fatalf("MagicDNS not normalized: %q", artifacts.MagicDNS)
	}
	if !strings.Contains(artifacts.Hikari, "MagicDNS alternative") {
		t.Fatal("MagicDNS comment missing from Hikari artifact")
	}
	if !strings.Contains(artifacts.SpringYAML, "MagicDNS alternative") {
		t.Fatal("MagicDNS comment missing from YAML artifact")
	}
}

func TestGenerateRejectsUnsafeHosts(t *testing.T) {
	cases := []Request{
		func() Request { r := localRequest(); r.Host = "0.0.0.0"; return r }(),
		func() Request { r := localRequest(); r.Host = "192.168.1.10"; return r }(),
		func() Request { r := localRequest(); r.Mode = ModeTailscale; r.Host = "192.168.1.10"; return r }(),
		func() Request { r := localRequest(); r.Database = "../escape"; return r }(),
		func() Request { r := localRequest(); r.Port = 0; return r }(),
	}
	for _, request := range cases {
		artifacts := Generate(request)
		if artifacts.Available {
			t.Errorf("unsafe request was accepted: %+v", request)
		}
		if artifacts.Reason == "" {
			t.Errorf("unsafe request has no reason: %+v", request)
		}
	}
}

func TestGenerateUnavailableReasonIsPassedThrough(t *testing.T) {
	request := localRequest()
	request.Available = false
	request.Reason = "Enable Tailscale hosting first."
	artifacts := Generate(request)
	if artifacts.Available || artifacts.Reason != request.Reason {
		t.Fatalf("unexpected unavailable result: %+v", artifacts)
	}
	if artifacts.Hikari != "" || artifacts.Maven != "" {
		t.Fatal("unavailable artifacts should not include code")
	}
}

func TestJavaEscaping(t *testing.T) {
	if got, want := javaQuote("a\"b\\c\n"), `"a\"b\\c\n"`; got != want {
		t.Fatalf("javaQuote = %s, want %s", got, want)
	}
}

func TestPropertiesEscaping(t *testing.T) {
	got := propertyEscape(" key=va:l#ue!")
	want := `\ key=va:l#ue!`
	if got != want {
		t.Fatalf("propertyEscape = %q, want %q", got, want)
	}
	if escaped := propertyEscape(`back\slash`); escaped != `back\\slash` {
		t.Fatalf("backslash escaping = %q", escaped)
	}
}

func TestYAMLEscaping(t *testing.T) {
	got := yamlQuote("a\"b\\c")
	want := `"a\"b\\c"`
	if got != want {
		t.Fatalf("yamlQuote = %s, want %s", got, want)
	}
}

func TestGenerateEscapesCredentials(t *testing.T) {
	request := localRequest()
	request.Password = `p@ss"w\ord#1`
	artifacts := Generate(request)
	if !artifacts.Available {
		t.Fatalf("expected available: %s", artifacts.Reason)
	}
	if !strings.Contains(artifacts.Hikari, `p@ss\"w\\ord#1`) {
		t.Fatal("Java password escaping incorrect")
	}
	if !strings.Contains(artifacts.SpringProperties, `p@ss"w\\ord#1`) {
		t.Fatal("properties password escaping incorrect")
	}
}
