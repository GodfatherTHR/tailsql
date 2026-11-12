# TailSQL — Architecture (v0.3.0)

> Local-first H2 database manager with Desktop UI, MCP agent server, Auto-REST gateway, checkpoints, and optional LLM assistance.
>
> Last updated: 2026-11-12

## 1. Runtime Topology

```
┌────────────────────────────── Desktop host ──────────────────────────────┐
│                                                                          │
│  Wails WebView (React 18 + TS)                                           │
│        │ window.go.main.App.* IPC                                        │
│        ▼                                                                 │
│  Go host process (App facade)                                            │
│   ├─ internal/dbmanager ──► H2 TCP server 127.0.0.1:<port>               │
│   ├─ internal/jdbc ───────► short-lived JVM + JdbcBridge                 │
│   ├─ internal/sqlrunner ──► SQL + metadata                               │
│   ├─ internal/snapshots ──► ~/.tailsql/snapshots/<db>/*.mv.db            │
│   ├─ internal/mcp ────────► HTTP 127.0.0.1:9099 / Tailnet IP             │
│   ├─ internal/restapi ────► HTTP 127.0.0.1:9199 / Tailnet IP / Funnel    │
│   ├─ internal/aihelper ───► Ollama / OpenAI-compatible APIs              │
│   ├─ internal/config ─────► instances.json + config.json                 │
│   └─ internal/tailscale ──► L4 proxy, Funnel TCP/HTTPS                   │
│                                                                          │
└──────────────────────────────────────────────────────────────────────────┘
        ▲                          ▲                        ▲
   AI coding agents          Web frontends            Tailnet peers
   (MCP over HTTP)           (Auto-REST HTTPS)         (JDBC via proxy)
```

## 2. Trust Boundaries & Binding Rules

1. H2 always binds loopback (`-tcp` + `-Dh2.bindAddress=127.0.0.1`); remote access only through the Tailscale proxy.
2. MCP and REST servers default to `127.0.0.1`. Tailnet binding requires an explicit user toggle and a validated CGNAT IPv4 (`100.64.0.0/10`).
3. `0.0.0.0`, `::`, LAN, and arbitrary host binds are rejected before `net.Listen`.
4. Tokens (`tsql_mcp_*`, `tsql_rest_*`) are required whenever a server is exposed beyond loopback.
5. Agent Read-Only mode is enforced in the backend adapter for MCP writes/DDL, independent of the UI.
6. REST identifiers are validated against live `INFORMATION_SCHEMA` metadata and quoted; all values use JDBC `?` parameters.

## 3. Package Layout (new in v0.3.0)

```
internal/
  mcp/            JSON-RPC 2.0 MCP server: tools, resources, SSE transports, auth
  restapi/        Dynamic CRUD router, OpenAPI generator, CORS/API-key middleware
  snapshots/      .mv.db checkpoint store (list/create/extract/restore/delete)
  aihelper/       Ollama + OpenAI-compatible clients, NL→SQL, deterministic faker
  config/         preferences.go — ~/.tailsql/config.json (v0.3 prefs)
cmd/
  tailsql-mcp/    stdio ↔ HTTP MCP bridge for local agents
```

App wiring lives in new files:

- `app_agent.go` — implements the MCP/REST backend adapter using `dbmanager`, `sqlrunner`, `snapshots`, `codegen`.
- `app_features.go` — MCP/REST/AI preference APIs, server lifecycle, snippets.
- `app_snapshots.go` — checkpoint APIs and restore orchestration.

## 4. Preferences (`~/.tailsql/config.json`)

Atomic JSON store, `0600`, defaults filled on load:

```json
{
  "mcpEnabled": false,
  "mcpPort": 9099,
  "mcpTailscaleEnabled": false,
  "mcpToken": "tsql_mcp_...",
  "restEnabled": false,
  "restPort": 9199,
  "restTailscaleEnabled": false,
  "restFunnelEnabled": false,
  "restFunnelPort": 443,
  "restApiKey": "tsql_rest_...",
  "restAllowPublicRead": false,
  "agentReadOnly": false,
  "autoSnapshotOnAgentDDL": true,
  "aiProvider": "ollama",
  "aiBaseUrl": "http://127.0.0.1:11434",
  "aiModel": "qwen2.5-coder:7b",
  "aiApiKey": "",
  "snapshotRetention": 30
}
```

`instances.json` is untouched, preserving v0.2.1 state compatibility.

## 5. Snapshot Manager

- Root: `~/.tailsql/snapshots/<db_name>/`.
- File: `<UTC timestamp>_<sanitized label>.mv.db`.
- Online creation: SQL `BACKUP TO '<temp>.zip'` via the JDBC bridge (consistent H2 backup), then extract the `.mv.db` member with `archive/zip`. Stopped databases fall back to a direct file copy.
- Restore: stop Tailscale proxy → graceful H2 stop → verify stopped → replace `.mv.db` (atomic copy via temp + rename, removing stale trace files) → restart H2 on the same port → restore Tailscale proxy/Funnel if previously enabled.
- Labels: `Manual checkpoint`, `Pre-Agent DDL`, `Pre-Restore safety copy`.

## 6. MCP Server

- `net/http` server with `http.Server`, graceful shutdown.
- JSON-RPC 2.0 dispatch (`initialize`, `ping`, `tools/*`, `resources/*`, `prompts/list`).
- Transports: Streamable HTTP `POST /mcp`, SSE keepalive `GET /mcp`, legacy SSE `GET /sse` + `POST /messages?sessionId=`.
- Bearer auth middleware for every route except `/health`.
- Backend interface implemented by `app_agent.go`; safe mode and auto-checkpoint live there.
- Errors are returned as JSON-RPC errors with MCP tool errors additionally carrying `isError: true`.

## 7. Auto-REST Server

- One shared `http.Server` for all databases; routing resolves `<db>` case-insensitively by name or ID.
- Per-request schema snapshot (short TTL cache) provides the whitelist and metadata.
- Query builder: quoted identifiers, `?` params, filters, ordering, pagination, `COUNT(*)`.
- JSON encoding converts H2 string values into numbers/booleans/null based on column type.
- OpenAPI 3.0 document generated from live metadata.
- Exposure: loopback bind, optional Tailnet bind, optional HTTPS Funnel mapping to the loopback port.

## 8. AI Helper

- Provider abstraction: `ollama` (`POST /api/chat`) and OpenAI-compatible (`POST /chat/completions`).
- NL→SQL prompt includes compact DDL (tables, columns, PK/FK) and must return SQL only; backtick fences are stripped.
- Seed rows: LLM asked for a JSON array of objects; each row is validated against column names/types. Without a provider, `FallbackRows` fills type-aware deterministic data, skips identities/defaults, and samples referenced FK values.
- Insertion uses the existing table-editor SQL builder with quoted identifiers and literal escaping; failures roll back nothing else and are surfaced as errors.

## 9. Wails IPC Additions

| Method | Purpose |
|---|---|
| `GetAgentConfig` / `SetMCPConfig` / `SetRESTConfig` / `SetAgentSafety` | Live feature settings, tokens, snippets |
| `RegenerateAgentToken` | Rotate MCP token or REST key |
| `ListSnapshots` / `CreateSnapshot` / `RestoreSnapshot` / `DeleteSnapshot` | Checkpoints |
| `GetAISettings` / `SaveAISettings` | Local LLM configuration |
| `GenerateSQL` / `SeedTableRows` | AI-assisted workbench and table editor |

New events: `agent:state` (MCP/REST state), `snapshots:changed`.

## 10. Frontend Additions

- Nav view `ai` → `AgentConnectView`: MCP toggle/port/tailnet/token, Agent Safe Mode, REST controls, Windsurf/Cursor/Claude config snippets, copy-ready system prompt.
- Overview: REST exposure card + `Checkpoints` drawer button.
- SQL Workbench: AI prompt bar + settings modal.
- Table Editor: `Seed rows` action (10/50) with modal.

## 11. Verification Strategy

- Unit tests: preferences defaults/round-trip, snapshot naming/list/restore, REST filter/query builder + auth, MCP initialize/tools/list/tools/call + auth, AI fallback rows + SQL extraction.
- Integration: real-H2 table editor suite (existing) plus snapshot/list checks where feasible.
- Builds: `go build ./...`, `go vet ./...`, `go test ./...`, `npm run build`, Wails production build.
- Live smoke: start app, enable MCP/REST, curl initialize and REST endpoints against a local test database, create/restore a checkpoint.
