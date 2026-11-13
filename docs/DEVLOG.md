# HySQL — Engineering Devlog

Chronological record of v0.3.0 implementation. Newest entries are appended at the bottom.

---

## 2026-11-12 — v0.3.0 Phase 1: Documentation & architecture

**Objective:** Define the v0.3.0 contract before implementation.

**Actions**

- Created `docs/PRD_AND_ROADMAP.md` with v0.3.0 goals, personas, functional scope, acceptance criteria, and phase checkboxes.
- Created `docs/MCP_AND_REST_SPEC.md` documenting MCP transports/methods/tools/resources and every REST route, filter, response envelope, and security rule.
- Created `docs/ARCHITECTURE.md` describing the MCP/REST/snapshot/AI packages, binding rules, preferences schema, and IPC additions.
- Preserved all v0.2.1 behavior and `instances.json` compatibility as hard requirements; new settings live in `~/.hysql/config.json`.

**Decisions**

1. MCP and REST share one backend adapter in package `main` so safe-mode and auto-checkpoint policy are enforced in one place.
2. Preferences live in a separate config file to avoid schema changes to `instances.json`.
3. REST uses a single multiplexed server (`/api/v1/<db>/...`) instead of one port per database.
4. JDBC bridge gains a prepared-statement mode and richer metadata (FKs, indexes, remarks) to serve MCP and REST safely.

**Next:** Phase 2 — snapshots and checkpoints.

---

## 2026-11-12 — v0.3.0 Phase 2: Snapshots & checkpoints

**Implementation**

- `internal/snapshots`: atomic `.mv.db` checkpoint store under `~/.hysql/snapshots/<db>/`, timestamp + sanitized label names, list/delete/prune, zip extraction, and atomic restore (`temp file → fsync → replace`, clearing stale trace/lock files).
- `internal/jdbc/JdbcBridge.java`: extended `tables` metadata with table/column remarks, foreign keys, and indexes; added the framed `query` mode using `PreparedStatement` with Base64 parameters on stdin.
- `internal/jdbc/bridge.go`: `Query` method, query-input framing, relation structs.
- `internal/dbmanager`: `DatabasePath(id)` so restores target the exact `.mv.db`.
- `app_snapshots.go`: manual create, list, delete, restore, retention pruning, and `Pre-Agent DDL` auto-checkpoints.
- Restore orchestration: stop proxy → graceful H2 stop → verify stopped → atomic file replacement → same-port restart → re-enable Tailscale proxy/Funnel when previously active; a “Pre-Restore safety copy” is taken first.
- UI: `CheckpointsDrawer` (create label, list with size/time, restore, delete) wired to a **Checkpoints** button in the database header.

**Verification**

- `go test ./internal/snapshots` passes (lifecycle, zip extraction, prune).
- Real-H2 integration now covers online `BACKUP TO`, extraction, and restore: a table created after the backup disappears after restore while the original table survives.
- Checkpoint creation through the App adapter is covered in the MCP/REST integration test.

---

## 2026-11-12 — v0.3.0 Phase 3: MCP server & Connect to AI

**Implementation**

- `internal/mcp`: JSON-RPC 2.0 server with `POST /mcp`, SSE keepalive `GET /mcp`, legacy `GET /sse` + `POST /messages`, `DELETE /mcp`, and `/health`.
- Bearer authentication (`hysql_mcp_*`), batch requests, notifications (HTTP 202), and MCP error codes.
- Six tools with JSON Schemas: `list_databases`, `inspect_schema`, `read_query`, `write_query`, `execute_ddl`, `get_connection_bundle`.
- Resources: `hysql://databases` and `hysql://databases/<name>/schema`; `prompts/list` compatibility.
- `app_agent.go`: MCP backend adapter enforcing Read-Only mode and auto-checkpointing before `DROP`/`TRUNCATE`/`ALTER`; emits realtime events for workbench/table-editor refresh.
- `cmd/hysql-mcp`: stdio ↔ HTTP bridge.
- UI: new **Connect to AI** tab with MCP toggle, port, Tailnet bind, token rotate/copy, Agent Safe Mode toggles, REST controls, Windsurf/Cursor/Claude configs, env snippet, and a generated live-schema agent prompt.

**Verification**

- `go test ./internal/mcp` covers initialize, tools/list, tool calls, safe-mode blocking, read-statement validation, resources, notifications, and bearer auth.
- End-to-end `TestAgentGatewaysIntegration` calls `initialize`, `list_databases`, `inspect_schema`, `read_query`, and `get_connection_bundle` over real HTTP against a live H2 database.

---

## 2026-11-12 — v0.3.0 Phase 4: Auto-REST gateway

**Implementation**

- `internal/restapi`: multiplexed `/api/v1/<db>/<schema>/<table>` router with GET list/single, POST object or array, PATCH/DELETE by single-column PK, and `/openapi.json`.
- Filters `eq|neq|gt|gte|lt|lte|like|is.null|is.notnull`, ordering, validated pagination, total count.
- Metadata whitelist (case-insensitive) + quoted identifiers + `?` parameters from the new bridge `query` mode.
- Type-aware JSON conversion (numbers, booleans, nulls).
- CORS for all responses; `X-HySQL-Key`/Bearer auth; public unauthenticated GET option.
- `internal/tailscale/funnel_http.go`: HTTPS Funnel enable/disable for the loopback HTTP port.
- App wiring: REST server lifecycle, Tailnet bind, Funnel exposure, API key rotation, `agent:state` events.
- UI: Overview `RestApiCard` (gateway toggle, Tailnet, Funnel, base URL copy) and full REST controls in Connect to AI.

**Verification**

- `go test ./internal/restapi` covers filters/order validation, type conversion, auth (401), list pagination, parameterized insert, OpenAPI, and unknown tables.
- End-to-end integration performs authenticated REST GET, POST insert, OpenAPI fetch, and verifies 401 without the key against real H2.

---

## 2026-11-12 — v0.3.0 Phase 5: AI SQL & mock data

**Implementation**

- `internal/aihelper`: Ollama `/api/chat` and OpenAI-compatible `/chat/completions` clients, SQL fence extraction, JSON row parsing, compact DDL prompts, and a deterministic type-aware faker.
- FK-aware fallback: referenced values are sampled from the parent table; identity/generated columns are skipped; integer non-identity PKs start after existing `MAX`.
- App APIs: `GetAISettings`, `SaveAISettings`, `GenerateSQL`, `SeedTableRows`.
- UI: AI settings modal (provider/base URL/model/key), schema-aware NL→SQL bar and generate button in the SQL Workbench, and a `Seed rows` modal (10/50) in the Table Editor.
- Seeder inserts all generated rows in one multi-row statement and emits a realtime insert event.

**Verification**

- `go test ./internal/aihelper` covers type-aware fallback values, FK reuse, SQL fence extraction, row parsing/filtering, and compact DDL.
- Frontend TypeScript build passes with the new components.

---

## 2026-11-12 — v0.3.0 Phase 6: Verification & build

**Commands and results**

```text
go test ./...                     # pass
go vet ./...                      # pass
HYSQL_INTEGRATION=1 go test ./internal/sqlrunner -run TestTableDataIntegration
  # pass: prepared queries, FK/index metadata, online backup + restore
HYSQL_INTEGRATION=1 go test . -run TestAgentGatewaysIntegration
  # pass: MCP initialize/tools/resources, REST auth/list/insert/OpenAPI, checkpoint creation
npm run build                     # pass (tsc + Vite)
scripts/build.ps1                 # pass; HySQL.exe + HySQL-0.3.0.exe
```

**Build outputs**

- `build/bin/HySQL.exe`
- `build/bin/HySQL-0.3.0.exe`
- Version bumped to `0.3.0` in `app.go`, `wails.json`, `frontend/package.json`, and the NSIS default.

**Notes**

- MCP and REST default to disabled and loopback-only. Enabling Tailnet/Funnel requires explicit UI toggles and Tailscale connectivity.
- The `hysql-mcp` stdio bridge is built by `go build ./cmd/hysql-mcp`.

---

## 2026-11-12 — v0.3.0 fix: seeder vs. identity sequences

**Reported:** Seeding `PUBLIC.USERS` failed with `SQLState 23505` even though the generated `INSERT` omitted `ID`.

**Root cause:** The table used `ID BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY`. Its rows had been inserted with explicit IDs (`1` and `2`), which do not advance an H2 identity sequence for `GENERATED BY DEFAULT`. The sequence still pointed at `1`, so the seeder's identity-generated `ID` collided with the existing row aliases.

**Fix:** `restartIdentityColumns` runs before every seed insert. For each identity column it reads `MAX(column)` and executes `ALTER TABLE <schema>.<table> ALTER COLUMN <column> RESTART WITH max+1`, then the multi-row insert can safely omit the identity column. Failure to align the sequence aborts the seed instead of producing a partial insert.

**Verification**

- Integration test now creates `IDENTITY_ITEMS(ID BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY, NAME ...)`, inserts `ID = 1` explicitly, seeds five rows, and asserts `COUNT(*) = 6`, `COUNT(DISTINCT ID) = 6`, and `MAX(ID) = 6`.
- `go test ./...`, `go vet ./...`, and the MCP/REST integration suite pass.

---

## 2026-11-13 — Rebrand: TailSQL → HySQL

**Scope:** Full product rename across the desktop app, Go module, CLI/MCP bridge, installer, frontend, website, and documentation. External repo and GitHub Pages links now target `github.com/GodfatherTHR/hysql` and `godfatherthr.github.io/hysql/`.

**Implementation**

- Go module renamed from `tailsql` to `hysql`; every internal import updated (`hysql/internal/...`).
- Binaries and install artifacts renamed: `HySQL.exe`, `HySQL-<version>.exe`, `hysql-mcp.exe`, `HySQL-Setup-<version>.exe`, `%ProgramFiles%\HySQL`, Start Menu/registry keys `HySQL`.
- MCP stdio bridge moved from `cmd/tailsql-mcp` to `cmd/hysql-mcp`.
- Token prefixes changed to `hysql_mcp_*` / `hysql_rest_*`; REST header is now `X-HySQL-Key`; MCP server name/resource URIs use `hysql`. Existing persisted token values remain valid.
- Environment variables renamed (`TAILSQL_*` → `HYSQL_*`), including `HYSQL_INTEGRATION`, `HYSQL_DOWNLOAD_TEST`, `HYSQL_WEBVIEW_DATA`, and `HYSQL_MCP_TOKEN`.
- Frontend workbench/table-editor localStorage keys moved to `hysql.*`.
- Runtime data directory renamed to `~/.hysql`; `migrateLegacyDataDir` renames an existing `~/.tailsql` tree on first launch so databases, credentials, and checkpoints are preserved.
- Regenerated `docs/assets/logo.png` with the HySQL wordmark; the gem mark, favicon, app icon, and sidebar artwork were already text-free.
- README, website, PRD, architecture, MCP/REST spec, devlog, and session log updated to the new brand.

**Verification**

```text
scripts/build-bridge.ps1          # pass (Java 11 target)
go build ./...                    # pass
go vet ./...                      # pass
go test ./...                     # pass
(cd frontend && npm run build)    # pass (tsc + Vite)
scripts/build.ps1                 # pass; HySQL.exe + HySQL-0.3.0.exe + hysql-mcp.exe
scripts/build-installer.ps1       # pass; dist/HySQL-Setup-0.3.0.exe
```

**Notes**

- The GitHub repository and Pages site must be renamed to `hysql` for the new links and release download URLs to resolve.
- A pre-existing TailSQL installation lives in its own `%ProgramFiles%\TailSQL` directory and is not upgraded in place; uninstall it after installing HySQL.
- The gitignored local demo projects under `inventory-management/` and `todo-app/` still use the old `com.tailsql` packages and `TAILSQL_*` variables; they are outside the shipped project and were left untouched.


