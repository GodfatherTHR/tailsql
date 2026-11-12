# TailSQL — Product Requirements & Roadmap (v0.3.0)

> **Product:** TailSQL — a local-first H2 database manager for Java developers and AI coding agents, with one-click Tailscale sharing, generated JDBC code, an embedded MCP server, and an auto-generated REST gateway.
>
> **Status:** v0.2.1 shipped. v0.3.0 in implementation.
> **Last updated:** 2026-11-12

---

## 1. Problem Statement

H2 is the fastest way to get a real relational database running locally, but it is a poor fit for modern AI-assisted development:

1. AI coding agents (Windsurf, Cursor, Claude Code, OpenCode) cannot speak H2's raw TCP protocol. They need an MCP endpoint or a JSON API.
2. Browser frontends cannot open H2 TCP sockets. They need a REST/JSON gateway.
3. Agents and remote teammates mutate schemas. Users need one-click checkpoints and rollback.
4. Writing correct H2 SQL and realistic seed data is repetitive. Users want optional local/remote LLM assistance.

TailSQL v0.3.0 closes all four gaps while preserving the v0.2.1 desktop experience (H2 lifecycle, Tailscale L4 proxy, Funnel TCP, Java codegen, SQL workbench, table editor).

## 2. Personas

| Persona | Need | Success signal |
|---|---|---|
| AI-assisted developer | Point Cursor/Windsurf/Claude Code at TailSQL and let the agent inspect schema, query, and seed data | Agent lists databases and runs a `SELECT` on the first attempt using the copied MCP config |
| Frontend developer | Build a React/Next.js app against a local H2 database | Copies the Auto-REST base URL and fetches `GET /api/v1/inven_db/INVENTORY/PRODUCTS` successfully in minutes |
| Backend Java developer | A real H2 TCP endpoint + generated Spring Boot code | Copies generated JDBC URL and pool code and connects on the first try |
| Team lead | Share a database through Tailscale/Funnel safely | Enables Tailnet/Funnel REST with an API key, sends only an HTTPS URL + key |
| Experimental user | Let an agent change schemas without fear | Creates a checkpoint, lets the agent run DDL, restores in one click |

## 3. v0.3.0 Goals

- **G1 — MCP server:** Built-in JSON-RPC 2.0 MCP server on `127.0.0.1:9099` (configurable) with optional Tailnet bind and Bearer token, exposing six tools and resources.
- **G2 — Connect to AI UI:** One tab with server toggles, Agent Safe Mode, copy-ready Windsurf/Cursor/Claude configs, and a paste-ready agent system prompt containing live schema/JDBC/REST details.
- **G3 — Auto-REST gateway:** `/api/v1/<db>/<schema>/<table>` CRUD for tables and read access for views, streaming-json responses, filters, pagination, CORS, API key, OpenAPI 3.0, Tailnet and Funnel exposure.
- **G4 — Checkpoints:** List/create/restore/delete `.mv.db` checkpoints under `~/.tailsql/snapshots/<db>/`, automatic pre-agent-DDL snapshots, 1-click rollback with the same H2 port.
- **G5 — AI SQL assistant:** Schema-aware NL→H2 SQL in the workbench and a 10/50-row mock-data seeder with Ollama/OpenRouter/OpenAI-compatible providers and a deterministic offline fallback.
- **G6 — Safety:** no `0.0.0.0` binds, read-only agent mode, optional auth on every exposed endpoint, identifier whitelisting + parameterized SQL in REST.

### Non-goals

- In-app application/code scaffolding ("AI App Builder"). External agents use MCP/JDBC/REST and build in their own workspace.
- Multi-tenant user accounts, cloud sync, or replacing Tailscale ACLs.
- Encrypted credential vault (settings file remains `0600`).
- H2 Web Console embedding.

## 4. Functional Scope

### 4.1 MCP server (`internal/mcp`)

- JSON-RPC 2.0 over HTTP: `POST /mcp` (streamable HTTP), `GET /mcp` (SSE keepalive), legacy `GET /sse` + `POST /messages`.
- Optional CLI stdio bridge (`cmd/tailsql-mcp`) that forwards JSON-RPC lines to the HTTP endpoint.
- Tools: `list_databases`, `inspect_schema`, `read_query`, `write_query`, `execute_ddl`, `get_connection_bundle`.
- Resources: `tailsql://databases`, `tailsql://databases/<name>/schema`.
- Safety: Bearer `tsql_mcp_*` token; Read-Only mode blocks write/ddl; auto-checkpoint before destructive DDL when enabled.
- Bind default `127.0.0.1`; optional Tailscale IPv4 bind.

### 4.2 Auto-REST gateway (`internal/restapi`)

- `GET /api/v1/<db>/<schema>/<table>` with `limit`, `offset`, `order=col.asc|desc`, filters `<col>=eq|neq|gt|gte|lt|lte|like|is.<value>`.
- `GET /api/v1/<db>/<schema>/<table>/<pk>`.
- `POST /api/v1/<db>/<schema>/<table>` single object or array.
- `PATCH` / `DELETE` by primary key.
- Views readable through the same GET routes; writes rejected.
- `GET /api/v1/<db>/openapi.json`.
- CORS `*`; `X-TailSQL-Key` or Bearer; optional unauthenticated GET for public demos.
- Identifier whitelist from `INFORMATION_SCHEMA`; values via JDBC `?` parameters.
- Local/Tailnet binding + optional HTTPS Funnel.

### 4.3 Snapshots & safe mode (`internal/snapshots`)

- Snapshots under `~/.tailsql/snapshots/<db_name>/<timestamp>_<label>.mv.db` plus metadata.
- Online consistent backup via H2 `BACKUP TO '<path>.zip'`; extraction to `.mv.db`. File copy fallback when stopped.
- Restore: stop H2/proxies, replace `.mv.db`, restart on the same port, re-enable prior Tailscale hosting/Funnel.
- Auto-snapshot on agent `DROP`/`TRUNCATE`/`ALTER` when enabled.

### 4.4 AI helper (`internal/aihelper`)

- Providers: Ollama (`/api/chat`), OpenRouter/OpenAI-compatible (`/chat/completions`).
- Settings in `~/.tailsql/config.json` (provider, base URL, model, key).
- NL→SQL with compact live DDL in the system prompt.
- Seed rows 10/50: LLM JSON rows validated against the table, or deterministic faker fallback; FK-aware sampling.

### 4.5 UI

- Top nav `Connect to AI` with MCP/REST controls, tokens, snippets.
- `Checkpoints` drawer in the database header.
- Overview REST card with Tailnet/Funnel toggles.
- SQL Workbench AI prompt bar and AI settings modal.
- Table Editor `Seed rows` action.

## 5. Roadmap & Progress

### Phase 1 — Documentation & Architecture
- [x] v0.3 PRD, architecture update, MCP/REST spec, devlog scaffold
- [x] Backward-compatibility and security baseline documented

### Phase 2 — Snapshot / Checkpoint Engine
- [x] `internal/snapshots` store, online backup, restore
- [x] Auto-checkpoint hook + same-port restart
- [x] Checkpoints drawer UI

### Phase 3 — Built-in MCP Server
- [x] JSON-RPC 2.0 HTTP+SSE server with Bearer auth
- [x] Six tools + resources + safe mode + auto-checkpoint
- [x] Connect to AI tab and copy-ready snippets/system prompt
- [x] Optional stdio bridge (`cmd/tailsql-mcp`)

### Phase 4 — Auto-REST API Gateway
- [x] CRUD router with parameterized queries and whitelist validation
- [x] OpenAPI 3.0 generator, CORS, API key, public-read option
- [x] Overview REST card, Tailnet/Funnel exposure

### Phase 5 — AI SQL & Seeder
- [x] Ollama/OpenAI-compatible client
- [x] NL→SQL bar + AI settings modal
- [x] Seed rows (10/50) with offline fallback

### Phase 6 — Verification
- [x] `go test ./...`, `go vet ./...`, `npm run build`, Wails build
- [x] Docs audit and release notes

## 6. Acceptance Criteria

- v0.2.1 behavior and `instances.json` state remain compatible; existing views unchanged except additive controls.
- MCP `initialize` + `tools/list` + `tools/call` works over `http://127.0.0.1:9099/mcp`; tool schemas validate.
- REST CRUD works on `INVENTORY.PRODUCTS` and read-only on `INVENTORY.V_LOW_STOCK`; SQL injection attempts through names/filters/body fail safely.
- Checkpoint create → destructive DDL → restore returns the database to the checkpointed rows on the same port.
- With no AI configured, seeding produces valid rows deterministically; with Ollama/OpenAI configured, NL→SQL uses the live schema.
- No listener binds `0.0.0.0`; exposure requires explicit Tailnet/Funnel toggles.
- `go test ./...`, `go vet ./...`, `npm run build`, and the Wails production build pass.

## 7. Constraints & Risks

| Risk | Mitigation |
|---|---|
| v0.2.1 state compatibility | New preferences in `~/.tailsql/config.json`; `instances.json` schema untouched |
| AI agents corrupt data | Read-Only mode, auto-checkpoint before destructive DDL, manual checkpoints |
| SQL injection in dynamic REST paths | Schema whitelist + quoted identifiers + `?` parameter binding |
| H2 backup while running | Use native `BACKUP TO` for consistent online snapshots |
| Funnel port collisions | Use Tailscale Funnel allowed ports and explicit per-feature mapping |
| Windows process/lock issues during restore | Stop H2 gracefully, verify exit, atomic file replace, restart same port |
| LLM output invalid | Fast validation, deterministic fallback for seed data, generated SQL is shown before execution |
| Third-party API keys | Stored only in local `~/.tailsql/config.json` (`0600`), never logged |

## 8. Definition of Done

Code complete with no placeholders, tests green, docs updated (`PRD_AND_ROADMAP.md`, `ARCHITECTURE.md`, `MCP_AND_REST_SPEC.md`, `DEVLOG.md`), frontend and Wails production builds pass.
