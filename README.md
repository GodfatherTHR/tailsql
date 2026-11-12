# TailSQL

<p align="center">
  <a href="https://github.com/GodfatherTHR/tailsql/releases/latest"><strong>Download for Windows</strong></a>
  &middot;
  <a href="https://godfatherthr.github.io/tailsql/">Website</a>
  &middot;
  <a href="https://godfatherthr.github.io/tailsql/docs.html">Documentation</a>
</p>

Local-first H2 database manager with a table editor, SQL workbench, Tailscale hosting, public Funnel support, generated Java (JDBC) code, a built-in MCP server for AI coding agents, an Auto-REST gateway, checkpoints, and optional LLM SQL assistance.

TailSQL runs H2 as a loopback-only TCP server, exposes it to your tailnet through a Go L4 proxy when you opt in, and produces copy-paste Maven, Gradle, HikariCP, Spring Boot, and raw JDBC snippets matching the selected connection mode. In v0.3.0 it also lets Windsurf, Cursor, Claude Code, or any MCP client inspect schemas, run queries, seed data, and fetch connection bundles — while browser apps talk to the same database through a generated REST/JSON API.

> **Verification status:** v0.3.0 is implemented and locally verified. Go unit and real-H2 integration tests cover the lifecycle, table editor, prepared statements, foreign-key/index metadata, online backup/restore, MCP tools over HTTP, REST auth/list/insert/OpenAPI, and checkpoint creation. The frontend production build and Wails build pass. The only unexercised path is a live remote Tailscale peer and a live public Funnel mapping; proxy forwarding/metrics are covered by a loopback test, and Funnel status/URL logic is unit tested.

## Install on Windows

Download `TailSQL-Setup-0.2.1.exe` from [GitHub Releases](https://github.com/GodfatherTHR/tailsql/releases/latest).

1. Run the installer. Windows SmartScreen may ask you to confirm because the installer is unsigned.
2. Launch TailSQL from the Start Menu or the desktop shortcut.
3. Select **New database** and follow the setup.

The installer includes the pinned H2 engine. Java 11 or newer is required. Tailscale is optional and is needed only for tailnet or public Funnel hosting.

## What it does

- Create, start, stop, restart, inspect, and remove local H2 databases without an external installer.
- Browse, insert, edit, and delete rows with a Supabase-style table editor. Tables are selected from the schema tree; updates and deletes require a primary key, while views and keyless tables stay read-only.
- H2 always binds `127.0.0.1`; the Tailscale proxy always binds the detected `100.x.y.z` interface. No `0.0.0.0`.
- Toggle Tailscale exposure and watch active peers plus byte throughput live.
- Publish a database to the public internet with Tailscale Funnel on an allowed public port (443, 8443, or 10000), with explicit exposure warnings and automatic Funnel cleanup when the database stops.
- Render Java connectivity code with active credentials, ports, and MagicDNS context.
- Run SQL and inspect tables/columns with a built-in workbench (500-row result cap). Keep query drafts per database across views and restarts.
- **Connect to AI:** one-click MCP server on `127.0.0.1:9099` with Bearer auth, optional Tailnet bind, copy-ready Windsurf/Cursor/Claude configs, and a generated system prompt containing live schema, JDBC URLs, and REST keys. Tools: `list_databases`, `inspect_schema`, `read_query`, `write_query`, `execute_ddl`, `get_connection_bundle`.
- **Agent safe mode:** read-only mode for agents and automatic `Pre-Agent DDL` checkpoints before `DROP`/`TRUNCATE`/`ALTER`.
- **Auto-REST gateway:** `/api/v1/<db>/<schema>/<table>` CRUD for tables, read-only views, filters, pagination, CORS, API key/Bearer auth, and OpenAPI 3.0 at `/api/v1/<db>/openapi.json`; bind loopback, Tailnet, or public HTTPS Funnel.
- **Checkpoints:** consistent `.mv.db` snapshots under `~/.tailsql/snapshots`, create/restore/delete from the database header, same-port restart with Tailscale/Funnel re-enabled.
- **AI SQL + mock data:** schema-aware natural-language-to-H2 SQL in the workbench and a 10/50-row seeder in the table editor using local Ollama or OpenRouter/OpenAI-compatible APIs, with a deterministic offline fallback.
- Recover cleanly from app crashes by validating and adopting (or cleaning) persisted H2 PIDs. Orphan H2 servers that still hold a database file but were never persisted are found by command line, adopted if healthy, or terminated if wedged.

## Requirements

| Component | Version | Notes |
|---|---|---|
| Go | 1.22+ | Build |
| Node.js | 18+ | Build |
| Wails CLI | v2.17.1 | `go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.1` |
| Java | 11+ | Runtime; 17, 21, or 24 recommended |
| JDK | 11+ | Only needed to rebuild the embedded JDBC bridge |
| NSIS | 3.x | Optional, builds the Windows setup installer |
| Tailscale | optional | Required only for remote hosting |
| WebView2 | Windows runtime | Installed on current Windows 10/11 |

## Build and run from source

Windows PowerShell:

```powershell
scripts/fetch-h2.ps1
scripts/build-bridge.ps1
cd frontend
npm install --include=dev
npm run build
cd ..
wails dev
```

Production build (copies `h2.jar` next to the executable and creates a versioned copy):

```powershell
scripts/build.ps1
build/bin/TailSQL.exe
build/bin/TailSQL-0.2.1.exe
```

Windows installer (requires NSIS):

```powershell
scripts/build-installer.ps1
# Or point at a specific makensis:
scripts/build-installer.ps1 -Makensis "C:\Program Files (x86)\NSIS\makensis.exe"
```

The installer is written to `dist/TailSQL-Setup-<version>.exe`.

macOS/Linux:

```bash
bash scripts/fetch-h2.sh
bash scripts/build-bridge.sh
(cd frontend && npm install --include=dev && npm run build)
wails dev
```

`bash scripts/build.sh` performs a full production build.

`npm install` must include dev dependencies (TypeScript/Vite). Some shells export `NODE_ENV=production`, which omits them; the scripts and `wails.json` use `--include=dev` to avoid a `'tsc' is not recognized` build failure.

If you run `wails build` directly instead of the scripts, copy `assets/h2.jar` into `build/bin/assets/` afterward, or use the in-app **Download H2** action.

If the H2 jar is not present at runtime, the UI offers a **Download H2** action that installs the pinned engine into `~/.tailsql/assets/h2.jar`.

## Runtime data

```
~/.tailsql/
├── instances.json        # schema-versioned instance state (0600)
├── dbs/<name>.mv.db      # H2 database files
├── logs/<id>.log         # H2 server output
├── bridge/               # embedded JDBC bridge classes
└── assets/h2.jar         # optional runtime download
```

Database metadata includes credentials in local JSON for the MVP. Tailscale ACLs are the authorization boundary for remote peers; H2 remains the authentication layer.

## How it works

- **Wails v2** embeds the compiled React bundle and binds the Go `App` methods.
- **H2** runs as `java -cp h2.jar org.h2.tools.Server -tcp -tcpPort <port> -tcpPassword <secret> -baseDir <dbs> -ifNotExists` with `-Dh2.bindAddress=127.0.0.1`.
- **JDBC bridge** is a precompiled Java 11 class executed with the H2 jar. Credentials travel through environment variables, SQL through stdin, and results return as JSON. This avoids requiring a Go H2 driver or `javac` at runtime.
- **Tailscale detector** parses `tailscale status --json`, validates the CGNAT address, and the proxy forwards TCP bidirectionally with byte/client tracking.
- **Graceful shutdown** uses H2's password-authenticated `-tcpShutdown` tool before any process kill, protecting `.mv.db` integrity.
- **Crash recovery** compares persisted PIDs against Java process images, the stored H2 command line (when available), and listening ports before adopting or cleaning them.
- **IPC** exposes typed Go DTOs through Wails; the React app consumes them through a typed facade in `src/lib/api.ts`. Generated bindings live in `frontend/wailsjs/`.

See the [TailSQL documentation](https://godfatherthr.github.io/tailsql/docs.html) for setup, usage, sharing, security, and troubleshooting.

## Tests

```bash
go test ./...
go vet ./...
(cd frontend && npm run build)
```

Real-H2 integration tests are opt-in:

```powershell
$env:TAILSQL_INTEGRATION = "1"
go test ./internal/dbmanager -run Integration -v
```

They cover create → start → SQL → metadata → stop, persisted-PID orphan adoption, and untracked H2 orphan adoption. The table editor also has an opt-in real-H2 test:

```powershell
$env:TAILSQL_INTEGRATION = "1"
go test ./internal/sqlrunner -run TestTableDataIntegration -v
```

It exercises insert, pagination, NULL handling, escaped literals, primary-key updates/deletes, and metadata defaults against a real H2 engine.

The H2 engine downloader is also opt-in:

```powershell
$env:TAILSQL_DOWNLOAD_TEST = "1"
go test ./internal/dbmanager -run TestDownloadH2Integration -v
```

It downloads from Maven Central, validates the archive, writes a SHA-256 sidecar, and resolves the jar.

### Generated Java verification

The generated HikariCP and raw JDBC snippets were compiled with:

```text
javac --release 11 -cp h2.jar;HikariCP.jar;slf4j-api.jar;slf4j-simple.jar -d classes DatabaseConfig.java JdbcTest.java
```

and executed against a live H2 server. Both printed `SELECT 1 -> 1`.

### Isolated UI debugging

`TAILSQL_WEBVIEW_DATA` can point WebView2 at a separate user-data directory, which is useful for running a second isolated instance or UI automation without disturbing the primary profile. It has no effect when unset.

## MVP limitations

- Credentials are immutable; recreate a database to rotate them.
- Deleting an instance removes metadata only; database files are retained.
- The table editor requires a primary key to update or delete a row (H2 2.x has no universal `ROWID`). Keyless tables and views can still be browsed, and keyless tables accept inserts.
- Tailscale Funnel raw TCP is **not encrypted in transit** and is reachable by anyone on the internet; H2 credentials are the only access control. Funnel is disabled automatically when the database stops, restarts, is deleted, or the app exits cleanly.
- One SQL statement per execution, 500-row response cap.
- IPv4 Tailscale addresses only; IPv6 is detected but not used in generated URLs.
- Tailscale exposure requires the CLI to be installed and signed in; local features do not.
- A live remote-peer connection was not available during verification; the proxy data path is tested over loopback, but a second tailnet device is a manual follow-up.
- Tailscale hosting is intentionally not auto-restored after an app restart; re-enable it from the overview. A Funnel mapping left behind by a crash is reconciled on the next launch.
