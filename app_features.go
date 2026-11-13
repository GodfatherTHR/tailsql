package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"hysql/internal/config"
	"hysql/internal/mcp"
	"hysql/internal/restapi"
	"hysql/internal/sqlrunner"
	"hysql/internal/tailscale"
)

func (a *App) preferences() (*config.PreferencesStore, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.prefs, a.prefs != nil
}

func (a *App) featureServers() (*mcp.Server, *restapi.Server) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.mcp, a.rest
}

// applyAgentServers restarts MCP and REST according to current preferences.
func (a *App) applyAgentServers() {
	prefs, ok := a.preferences()
	if !ok {
		return
	}
	current := prefs.Get()
	_ = a.applyMCPServer(current)
	_ = a.applyRESTServer(current)
}

func (a *App) applyMCPServer(prefs config.Preferences) error {
	mcpServer, _ := a.featureServers()
	if mcpServer == nil {
		return errors.New("MCP server is not initialized")
	}
	cfg := mcp.Config{
		BindAddress: "127.0.0.1",
		Port:        prefs.MCPPort,
		Token:       prefs.MCPToken,
		Version:     a.version,
	}
	if prefs.MCPTailscaleEnabled {
		status := a.tailscaleStatus()
		if !status.Running || strings.TrimSpace(status.IPv4) == "" {
			mcpServer.Stop()
			mcpServer.UpdateConfig(cfg)
			return errors.New("Tailscale is not connected; MCP stays on loopback")
		}
		cfg.BindAddress = status.IPv4
	}
	mcpServer.Stop()
	mcpServer.UpdateConfig(cfg)
	if !prefs.MCPEnabled {
		return nil
	}
	return mcpServer.Start()
}

func (a *App) applyRESTServer(prefs config.Preferences) error {
	_, restServer := a.featureServers()
	if restServer == nil {
		return errors.New("REST gateway is not initialized")
	}
	a.disableRESTFunnel()
	status := a.tailscaleStatus()
	cfg := restapi.Config{
		BindAddress:     "127.0.0.1",
		Port:            prefs.RESTPort,
		APIKey:          prefs.RESTAPIKey,
		AllowPublicRead: prefs.RESTAllowPublicRead,
		Version:         a.version,
	}
	if prefs.RESTFunnelEnabled {
		cfg.RequireAuth = true
		if status.Running && status.MagicDNS != "" {
			cfg.PublicBaseURL = tailscale.HTTPFunnelURL(status.MagicDNS, prefs.RESTFunnelPort)
		}
	} else if prefs.RESTTailscaleEnabled {
		if !status.Running || strings.TrimSpace(status.IPv4) == "" {
			restServer.Stop()
			restServer.UpdateConfig(cfg)
			return errors.New("Tailscale is not connected; REST stays on loopback")
		}
		cfg.BindAddress = status.IPv4
		cfg.RequireAuth = true
		cfg.PublicBaseURL = fmt.Sprintf("http://%s:%d", status.IPv4, prefs.RESTPort)
	}
	restServer.Stop()
	restServer.UpdateConfig(cfg)
	if !prefs.RESTEnabled {
		return nil
	}
	if err := restServer.Start(); err != nil {
		return err
	}
	if !prefs.RESTFunnelEnabled {
		return nil
	}
	if !status.Running || status.BinaryPath == "" || status.MagicDNS == "" {
		message := status.Error
		if message == "" {
			message = "Tailscale is not ready; cannot publish the REST API through Funnel"
		}
		a.setRESTFunnelError(message)
		return errors.New(message)
	}
	ctx, cancel := context.WithTimeout(a.baseContext(), 45*time.Second)
	defer cancel()
	funnelState, err := tailscale.EnableHTTPFunnel(
		ctx, status.BinaryPath, status.MagicDNS, prefs.RESTPort, prefs.RESTFunnelPort,
	)
	if err != nil {
		a.setRESTFunnelError(err.Error())
		return err
	}
	a.mu.Lock()
	a.restFunnelActive = true
	a.restFunnelPort = prefs.RESTFunnelPort
	a.restFunnelError = ""
	a.mu.Unlock()
	_ = funnelState
	return nil
}

func (a *App) setRESTFunnelError(message string) {
	a.mu.Lock()
	a.restFunnelError = message
	a.mu.Unlock()
}

func (a *App) disableRESTFunnel() {
	a.mu.Lock()
	active := a.restFunnelActive
	port := a.restFunnelPort
	a.mu.Unlock()
	if !active || port <= 0 {
		a.mu.Lock()
		a.restFunnelError = ""
		a.mu.Unlock()
		return
	}
	status := a.currentTailscaleForFunnel()
	if strings.TrimSpace(status.BinaryPath) != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		_ = tailscale.DisableHTTPFunnel(ctx, status.BinaryPath, port)
		cancel()
	}
	a.mu.Lock()
	a.restFunnelActive = false
	a.restFunnelPort = 0
	a.restFunnelError = ""
	a.mu.Unlock()
}

func (a *App) stopAgentServers() {
	a.disableRESTFunnel()
	mcpServer, restServer := a.featureServers()
	if mcpServer != nil {
		mcpServer.Stop()
	}
	if restServer != nil {
		restServer.Stop()
	}
}

func (a *App) emitAgentState() {
	mcpView, restView, safety := a.agentState()
	a.emit("agent:state", map[string]any{
		"mcp":    mcpView,
		"rest":   restView,
		"safety": safety,
	})
}

func (a *App) agentState() (MCPStateView, RESTStateView, AgentSafetyView) {
	prefs, ok := a.preferences()
	if !ok {
		return MCPStateView{}, RESTStateView{}, AgentSafetyView{}
	}
	current := prefs.Get()
	mcpView := MCPStateView{
		Enabled:          current.MCPEnabled,
		Port:             current.MCPPort,
		Token:            current.MCPToken,
		TailscaleEnabled: current.MCPTailscaleEnabled,
	}
	mcpServer, restServer := a.featureServers()
	if mcpServer != nil {
		state := mcpServer.State()
		mcpView.Running = state.Running
		mcpView.BindAddress = state.BindAddress
		mcpView.Endpoint = state.Endpoint
		mcpView.Error = state.Error
	}
	restView := RESTStateView{
		Enabled:          current.RESTEnabled,
		Port:             current.RESTPort,
		APIKey:           current.RESTAPIKey,
		TailscaleEnabled: current.RESTTailscaleEnabled,
		FunnelEnabled:    current.RESTFunnelEnabled,
		FunnelPort:       current.RESTFunnelPort,
		AllowPublicRead:  current.RESTAllowPublicRead,
	}
	if restServer != nil {
		state := restServer.State()
		restView.Running = state.Running
		restView.BindAddress = state.BindAddress
		restView.BaseURL = state.BaseURL
		restView.LocalURL = state.LocalURL
		restView.PublicBaseURL = state.PublicBaseURL
		restView.DatabaseCount = state.DatabaseCount
		restView.Error = state.Error
	}
	a.mu.RLock()
	if restView.Error == "" {
		restView.Error = a.restFunnelError
	}
	a.mu.RUnlock()
	safety := AgentSafetyView{
		ReadOnly:               current.AgentReadOnly,
		AutoSnapshotOnAgentDDL: current.AutoSnapshotOnAgentDDL,
	}
	return mcpView, restView, safety
}

// GetAgentConfig returns the complete Connect-to-AI payload.
func (a *App) GetAgentConfig() AgentConfigView {
	prefs, ok := a.preferences()
	if !ok {
		return AgentConfigView{Snippets: map[string]string{}}
	}
	current := prefs.Get()
	mcpView, restView, safety := a.agentState()
	view := AgentConfigView{
		MCP:    mcpView,
		REST:   restView,
		Safety: safety,
		AI: AISettingsView{
			Provider:  current.AIProvider,
			BaseURL:   current.AIBaseURL,
			Model:     current.AIModel,
			HasAPIKey: current.AIAPIKey != "",
		},
		Snippets: map[string]string{},
	}
	status := a.tailscaleStatus()
	view.TailscaleAvailable = status.Running && status.IPv4 != ""
	view.TailscaleIP = status.IPv4
	view.Snippets = buildAgentSnippets(mcpView, restView)
	view.SystemPrompt = a.buildSystemPrompt(mcpView, restView, safety)
	return view
}

// SetMCPConfig persists MCP settings and restarts the listener.
func (a *App) SetMCPConfig(request MCPConfigRequest) (AgentConfigView, error) {
	prefs, ok := a.preferences()
	if !ok {
		return AgentConfigView{}, errors.New("preferences are not initialized")
	}
	current := prefs.Get()
	current.MCPEnabled = request.Enabled
	current.MCPPort = request.Port
	current.MCPTailscaleEnabled = request.TailscaleEnabled
	if current.MCPPort <= 0 {
		current.MCPPort = config.DefaultMCPPort
	}
	if current.MCPTailscaleEnabled {
		status := a.tailscaleStatus()
		if !status.Running || status.IPv4 == "" {
			return AgentConfigView{}, errors.New("Tailscale is not connected; connect it before exposing MCP on the tailnet")
		}
	}
	if err := prefs.Update(current); err != nil {
		return AgentConfigView{}, err
	}
	updated := prefs.Get()
	if err := a.applyMCPServer(updated); err != nil && updated.MCPEnabled {
		a.emitAgentState()
		return a.GetAgentConfig(), err
	}
	a.emitAgentState()
	return a.GetAgentConfig(), nil
}

// SetRESTConfig persists Auto-REST settings and restarts the listener.
func (a *App) SetRESTConfig(request RESTConfigRequest) (AgentConfigView, error) {
	prefs, ok := a.preferences()
	if !ok {
		return AgentConfigView{}, errors.New("preferences are not initialized")
	}
	current := prefs.Get()
	current.RESTEnabled = request.Enabled
	current.RESTPort = request.Port
	current.RESTTailscaleEnabled = request.TailscaleEnabled
	current.RESTFunnelEnabled = request.FunnelEnabled
	current.RESTFunnelPort = request.FunnelPort
	current.RESTAllowPublicRead = request.AllowPublicRead
	if current.RESTPort <= 0 {
		current.RESTPort = config.DefaultRESTPort
	}
	if current.RESTFunnelPort <= 0 {
		current.RESTFunnelPort = config.DefaultFunnelPort
	}
	if current.RESTFunnelEnabled && !tailscale.IsAllowedFunnelPort(current.RESTFunnelPort) {
		return AgentConfigView{}, errors.New("Funnel only supports ports 443, 8443, and 10000")
	}
	if current.RESTFunnelEnabled || current.RESTTailscaleEnabled {
		status := a.tailscaleStatus()
		if !status.Running || status.IPv4 == "" {
			return AgentConfigView{}, errors.New("Tailscale is not connected; connect it before exposing the REST API")
		}
	}
	if err := prefs.Update(current); err != nil {
		return AgentConfigView{}, err
	}
	updated := prefs.Get()
	if err := a.applyRESTServer(updated); err != nil && updated.RESTEnabled {
		a.emitAgentState()
		return a.GetAgentConfig(), err
	}
	a.emitAgentState()
	return a.GetAgentConfig(), nil
}

// SetAgentSafety persists the agent guardrails.
func (a *App) SetAgentSafety(request AgentSafetyRequest) (AgentConfigView, error) {
	prefs, ok := a.preferences()
	if !ok {
		return AgentConfigView{}, errors.New("preferences are not initialized")
	}
	current := prefs.Get()
	current.AgentReadOnly = request.ReadOnly
	current.AutoSnapshotOnAgentDDL = request.AutoSnapshotOnAgentDDL
	if err := prefs.Update(current); err != nil {
		return AgentConfigView{}, err
	}
	a.emitAgentState()
	return a.GetAgentConfig(), nil
}

// RegenerateAgentToken rotates the MCP bearer token or the REST API key.
func (a *App) RegenerateAgentToken(request TokenRotateRequest) (AgentConfigView, error) {
	prefs, ok := a.preferences()
	if !ok {
		return AgentConfigView{}, errors.New("preferences are not initialized")
	}
	switch strings.ToLower(strings.TrimSpace(request.Target)) {
	case "mcp":
		if _, err := prefs.RotateMCPToken(); err != nil {
			return AgentConfigView{}, err
		}
	case "rest":
		if _, err := prefs.RotateRESTKey(); err != nil {
			return AgentConfigView{}, err
		}
	default:
		return AgentConfigView{}, errors.New("target must be \"mcp\" or \"rest\"")
	}
	if err := a.applyMCPServer(prefs.Get()); err != nil && prefs.Get().MCPEnabled {
		a.emitAgentState()
		return a.GetAgentConfig(), err
	}
	if err := a.applyRESTServer(prefs.Get()); err != nil && prefs.Get().RESTEnabled {
		a.emitAgentState()
		return a.GetAgentConfig(), err
	}
	a.emitAgentState()
	return a.GetAgentConfig(), nil
}

// GetAISettings returns the sanitized AI provider settings.
func (a *App) GetAISettings() AISettingsView {
	prefs, ok := a.preferences()
	if !ok {
		return AISettingsView{}
	}
	current := prefs.Get()
	return AISettingsView{
		Provider:  current.AIProvider,
		BaseURL:   current.AIBaseURL,
		Model:     current.AIModel,
		HasAPIKey: current.AIAPIKey != "",
	}
}

// SaveAISettings persists the local/remote LLM provider settings.
func (a *App) SaveAISettings(settings AIProviderSettings) (AISettingsView, error) {
	prefs, ok := a.preferences()
	if !ok {
		return AISettingsView{}, errors.New("preferences are not initialized")
	}
	current := prefs.Get()
	current.AIProvider = settings.Provider
	current.AIBaseURL = settings.BaseURL
	current.AIModel = settings.Model
	if settings.ClearAPIKey {
		current.AIAPIKey = ""
	} else if strings.TrimSpace(settings.APIKey) != "" {
		current.AIAPIKey = strings.TrimSpace(settings.APIKey)
	}
	if err := prefs.Update(current); err != nil {
		return AISettingsView{}, err
	}
	updated := prefs.Get()
	return AISettingsView{
		Provider:  updated.AIProvider,
		BaseURL:   updated.AIBaseURL,
		Model:     updated.AIModel,
		HasAPIKey: updated.AIAPIKey != "",
	}, nil
}

func (a *App) buildSystemPrompt(mcpView MCPStateView, restView RESTStateView, safety AgentSafetyView) string {
	endpoint := mcpView.Endpoint
	if endpoint == "" {
		host := mcpView.BindAddress
		if host == "" {
			host = fmt.Sprintf("127.0.0.1:%d", mcpView.Port)
		}
		endpoint = "http://" + host + "/mcp"
	}
	restBase := restView.BaseURL
	if restBase == "" {
		restBase = fmt.Sprintf("http://127.0.0.1:%d", restView.Port)
	}
	var builder strings.Builder
	builder.WriteString("# HySQL agent context\n\n")
	builder.WriteString("You have access to live H2 databases through the HySQL MCP server.\n\n")
	builder.WriteString("- MCP endpoint: " + endpoint + "\n")
	builder.WriteString("- Authorization: Bearer " + mcpView.Token + "\n")
	builder.WriteString("- Auto-REST base: " + restBase + "/api/v1/<database>/<schema>/<table>\n")
	builder.WriteString("- REST API key: " + restView.APIKey + "\n")
	builder.WriteString(fmt.Sprintf("- Agent safety: read-only=%v, auto-checkpoint-before-DDL=%v\n\n", safety.ReadOnly, safety.AutoSnapshotOnAgentDDL))
	builder.WriteString("## Workflow\n")
	builder.WriteString("1. Call `list_databases` to see every database and its JDBC/REST URLs.\n")
	builder.WriteString("2. Call `inspect_schema` before writing SQL so you use real tables and columns.\n")
	builder.WriteString("3. Use `read_query` for SELECTs, `write_query` for DML, and `execute_ddl` for schema changes.\n")
	builder.WriteString("4. Call `get_connection_bundle` when building an application; it returns JDBC credentials, HikariCP/Spring snippets, and the REST API key.\n")
	builder.WriteString("5. For browser apps, use the Auto-REST gateway with `X-HySQL-Key`; for Java/Spring, use the JDBC bundle.\n\n")
	builder.WriteString("## Building an app against this database\n")
	builder.WriteString("- REST is CORS-enabled for local and deployed frontends; send `X-HySQL-Key` (or `Authorization: Bearer <rest key>`).\n")
	builder.WriteString("- Read the generated contract first: `GET <rest base>/api/v1/<database>/openapi.json`.\n")
	builder.WriteString("- List rows: `GET <rest base>/api/v1/<database>/<schema>/<table>?limit=50&offset=0&order=<pk>.desc`.\n")
	builder.WriteString("- Insert: `POST <rest base>/api/v1/<database>/<schema>/<table>` with a JSON object (or array).\n")
	builder.WriteString("- Update/delete by primary key: `PATCH|DELETE <rest base>/api/v1/<database>/<schema>/<table>/<pk>`.\n")
	builder.WriteString("- Example: `curl -s -H \"X-HySQL-Key: " + restView.APIKey + "\" \"" + restBase + "/api/v1/<database>/<schema>/<table>?limit=5\"`.\n")
	builder.WriteString("- Use the exact schema-qualified names from the schema section below; identifiers are case-insensitive in the REST path but SQL must quote names when needed.\n\n")
	builder.WriteString("## Live databases and schema\n")
	builder.WriteString(a.schemaSummary())
	builder.WriteString("\n")
	return builder.String()
}

func (a *App) schemaSummary() string {
	manager, _, _ := a.components()
	if manager == nil {
		return "No databases are available."
	}
	instances := manager.List()
	if len(instances) == 0 {
		return "No databases are available. Create one in HySQL first."
	}
	var builder strings.Builder
	builder.WriteString("Use the exact schema-qualified names below; never invent columns.\n\n")
	for _, instance := range instances {
		localURL := fmt.Sprintf("jdbc:h2:tcp://127.0.0.1:%d/./%s", instance.Port, instance.Name)
		builder.WriteString(fmt.Sprintf("### %s (%s)\n", instance.Name, instance.Status))
		builder.WriteString("- JDBC: " + localURL + "\n")
		if instance.TailscaleEnabled && instance.TailscaleIP != "" {
			builder.WriteString(fmt.Sprintf("- Tailscale JDBC: jdbc:h2:tcp://%s:%d/./%s\n",
				instance.TailscaleIP, instance.TailscalePort, instance.Name))
		}
		if instance.Status != config.StatusRunning {
			builder.WriteString("- Schema unavailable (database is not running).\n\n")
			continue
		}
		ctx, cancel := context.WithTimeout(a.baseContext(), 10*time.Second)
		snapshot, err := sqlrunner.New(manager, manager.Bridge()).Tables(ctx, instance.ID)
		cancel()
		if err != nil {
			builder.WriteString("- Schema unavailable: " + err.Error() + "\n\n")
			continue
		}
		tableCount := 0
		for _, table := range snapshot.Tables {
			if tableCount >= 40 {
				builder.WriteString("- … more tables omitted\n")
				break
			}
			tableCount++
			builder.WriteString("  " + table.Schema + "." + table.Name + " (" + table.Type + "): ")
			columns := make([]string, 0, len(table.Columns))
			for index, column := range table.Columns {
				if index >= 25 {
					columns = append(columns, "…")
					break
				}
				part := column.Name + " " + column.Type
				if !column.Nullable {
					part += " NOT NULL"
				}
				if containsString(table.PrimaryKey, column.Name) {
					part += " PK"
				}
				columns = append(columns, part)
			}
			builder.WriteString(strings.Join(columns, ", ") + ")\n")
		}
		builder.WriteString("\n")
	}
	return builder.String()
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}

func buildAgentSnippets(mcpView MCPStateView, restView RESTStateView) map[string]string {
	endpoint := mcpView.Endpoint
	if endpoint == "" {
		host := mcpView.BindAddress
		if host == "" {
			host = fmt.Sprintf("127.0.0.1:%d", mcpView.Port)
		}
		endpoint = "http://" + host + "/mcp"
	}
	token := mcpView.Token
	windsurf := fmt.Sprintf(`{
  "mcpServers": {
    "hysql": {
      "serverUrl": %q,
      "headers": {
        "Authorization": "Bearer %s"
      }
    }
  }
}`, endpoint, token)
	cursor := fmt.Sprintf(`{
  "mcpServers": {
    "hysql": {
      "url": %q,
      "headers": {
        "Authorization": "Bearer %s"
      }
    }
  }
}`, endpoint, token)
	claude := fmt.Sprintf(`{
  "mcpServers": {
    "hysql": {
      "command": "npx",
      "args": [
        "-y",
        "mcp-remote",
        %q,
        "--header",
        "Authorization: Bearer %s"
      ]
    }
  }
}`, endpoint, token)
	restBase := restView.BaseURL
	if restBase == "" {
		restBase = fmt.Sprintf("http://127.0.0.1:%d", restView.Port)
	}
	env := fmt.Sprintf("# HySQL\nHYSQL_MCP_URL=%s\nHYSQL_MCP_TOKEN=%s\nHYSQL_REST_BASE=%s/api/v1\nHYSQL_REST_KEY=%s",
		endpoint, token, restBase, restView.APIKey)
	return map[string]string{
		"windsurf":    windsurf,
		"cursor":      cursor,
		"claude":      claude,
		"environment": env,
	}
}
