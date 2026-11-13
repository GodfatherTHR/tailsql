package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreferencesDefaultsAndTokens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	store, err := OpenPreferences(path)
	if err != nil {
		t.Fatalf("open preferences: %v", err)
	}
	prefs := store.Get()
	if prefs.MCPPort != DefaultMCPPort || prefs.RESTPort != DefaultRESTPort {
		t.Fatalf("unexpected default ports: %+v", prefs)
	}
	if !strings.HasPrefix(prefs.MCPToken, "hysql_mcp_") {
		t.Fatalf("unexpected MCP token: %q", prefs.MCPToken)
	}
	if !strings.HasPrefix(prefs.RESTAPIKey, "hysql_rest_") {
		t.Fatalf("unexpected REST key: %q", prefs.RESTAPIKey)
	}
	if !prefs.AutoSnapshotOnAgentDDL {
		t.Fatal("auto snapshot should default to true")
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("preferences file was not created: %v", statErr)
	}

	reloaded, err := OpenPreferences(path)
	if err != nil {
		t.Fatalf("reload preferences: %v", err)
	}
	if reloaded.Get().MCPToken != prefs.MCPToken {
		t.Fatal("MCP token should persist across reloads")
	}
}

func TestPreferencesNormalizeAndRotate(t *testing.T) {
	store, err := OpenPreferences(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatalf("open preferences: %v", err)
	}
	prefs := store.Get()
	prefs.MCPPort = 10
	prefs.RESTPort = 10
	prefs.RESTFunnelEnabled = true
	prefs.RESTTailscaleEnabled = true
	prefs.AIProvider = "custom"
	prefs.AIBaseURL = ""
	prefs.SnapshotRetention = -5
	if err := store.Update(prefs); err != nil {
		t.Fatalf("update preferences: %v", err)
	}
	updated := store.Get()
	if updated.MCPPort != DefaultMCPPort {
		t.Fatalf("invalid MCP port was not reset: %d", updated.MCPPort)
	}
	if updated.RESTPort == updated.MCPPort {
		t.Fatal("REST and MCP ports must not collide")
	}
	if updated.RESTTailscaleEnabled {
		t.Fatal("Tailnet binding must be disabled while Funnel is enabled")
	}
	if updated.AIProvider != ProviderOpenAICompat {
		t.Fatalf("provider alias was not normalized: %q", updated.AIProvider)
	}
	if updated.AIBaseURL == "" || updated.SnapshotRetention != DefaultSnapshotMax {
		t.Fatalf("defaults were not applied: %+v", updated)
	}

	oldToken := updated.MCPToken
	rotated, err := store.RotateMCPToken()
	if err != nil {
		t.Fatalf("rotate token: %v", err)
	}
	if rotated.MCPToken == oldToken || !strings.HasPrefix(rotated.MCPToken, "hysql_mcp_") {
		t.Fatal("MCP token was not rotated")
	}
}
