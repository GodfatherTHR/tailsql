# TailSQL v0.3 — MCP & Auto-REST Specification

> Version: 1.0 — 2026-11-12

## 1. MCP Server

### 1.1 Transport

| Endpoint | Method | Purpose |
|---|---|---|
| `/mcp` | `POST` | JSON-RPC 2.0 request/response (Streamable HTTP) |
| `/mcp` | `GET` | SSE keepalive stream for Streamable HTTP clients |
| `/mcp` | `DELETE` | Close a session (returns 204) |
| `/sse` | `GET` | Legacy SSE transport; first event is `endpoint` |
| `/messages?sessionId=<id>` | `POST` | Legacy SSE request ingress |
| `/health` | `GET` | `{ "ok": true, "server": "tailsql-mcp" }` |

Default bind: `127.0.0.1:9099`. Optional bind: Tailscale IPv4 on the same port.
Authentication: `Authorization: Bearer tsql_mcp_<random>` when a token is configured.

### 1.2 JSON-RPC methods

| Method | Notes |
|---|---|
| `initialize` | Returns protocolVersion, capabilities (`tools`, `resources`), serverInfo (`tailsql`, version) |
| `notifications/initialized` | Notification; HTTP 202, no body |
| `ping` | Returns `{}` |
| `tools/list` | Tool definitions with JSON Schema |
| `tools/call` | `{ name, arguments }` → `{ content: [{type:"text", text}], isError }` + `structuredContent` |
| `resources/list` | `tailsql://databases` and one schema resource per database |
| `resources/read` | Returns `application/json` contents |
| `prompts/list` | Empty list (compatibility) |

Errors: `-32700` parse, `-32600` invalid request, `-32601` unknown method, `-32602` invalid params, `-32603` internal.
Notifications (no `id`) return HTTP 202.

### 1.3 Tools

#### `list_databases`
```json
{ "type": "object", "properties": {}, "additionalProperties": false }
```
Result: array of `{ id, name, status, localJdbcUrl, tailscaleJdbcUrl, restBaseUrl }`.

#### `inspect_schema`
```json
{
  "type": "object",
  "properties": {
    "database_name": { "type": "string" },
    "schema_name": { "type": "string" }
  },
  "required": ["database_name"],
  "additionalProperties": false
}
```
Result: tables/views with columns (type, size, nullability, default, auto-increment, generated), primary key, foreign keys (name, column, referenced table/column, on update/delete), indexes (name, unique, columns), and row count.

#### `read_query`
```json
{
  "type": "object",
  "properties": {
    "database_name": { "type": "string" },
    "sql": { "type": "string" },
    "limit": { "type": "integer", "minimum": 1, "maximum": 500 }
  },
  "required": ["database_name", "sql"],
  "additionalProperties": false
}
```
Only `SELECT`, `WITH`, `SHOW`, `EXPLAIN`, `DESCRIBE`, and `VALUES` are accepted. Rows capped at 500.

#### `write_query`
```json
{
  "type": "object",
  "properties": {
    "database_name": { "type": "string" },
    "sql": { "type": "string" }
  },
  "required": ["database_name", "sql"],
  "additionalProperties": false
}
```
Only `INSERT`, `UPDATE`, `DELETE`, `MERGE`. Blocked in Read-Only agent mode.

#### `execute_ddl`
```json
{
  "type": "object",
  "properties": {
    "database_name": { "type": "string" },
    "sql": { "type": "string" }
  },
  "required": ["database_name", "sql"],
  "additionalProperties": false
}
```
Only `CREATE`, `ALTER`, `DROP`, `TRUNCATE`, `COMMENT`, `RENAME`. Blocked in Read-Only mode. When “Auto-snapshot before Agent DDL” is on, `DROP`/`TRUNCATE`/`ALTER` creates a `Pre-Agent DDL` checkpoint first.

#### `get_connection_bundle`
```json
{
  "type": "object",
  "properties": { "database_name": { "type": "string" } },
  "required": ["database_name"],
  "additionalProperties": false
}
```
Result fields: `database`, `jdbcUrls` (local/tailscale), `username`, `password`, `hikari`, `springProperties`, `restBaseUrl`, `restApiKey`, `openapiUrl`.

### 1.4 Resources

- `tailsql://databases` — JSON array of databases and status.
- `tailsql://databases/<name>/schema` — compact JSON schema for that database.

## 2. Auto-REST Gateway

### 2.1 Base

- Default: `http://127.0.0.1:9199`
- Tailnet: `http://100.x.y.z:9199`
- Funnel: `https://<magicdns>` (port 443) or `https://<magicdns>:<8443|10000>`

All routes are prefixed with `/api/v1/<database>/`.

### 2.2 Routes

| Method | Route | Description |
|---|---|---|
| GET | `/<db>/<schema>/<table>` | List rows: `limit` (≤500), `offset`, `order=col.asc|desc`, filters |
| GET | `/<db>/<schema>/<table>/<pk>` | One row by single-column primary key |
| POST | `/<db>/<schema>/<table>` | Insert object or array; returns inserted rows |
| PATCH | `/<db>/<schema>/<table>/<pk>` | Update supplied columns |
| DELETE | `/<db>/<schema>/<table>/<pk>` | Delete by primary key |
| GET | `/<db>/openapi.json` | OpenAPI 3.0 document for the database |

Views (`TABLE_TYPE = VIEW`) support GET only.

### 2.3 Filters

`?<column>=<op>.<value>` where `<op>` is `eq`, `neq`, `gt`, `gte`, `lt`, `lte`, `like`, or `is` (`is.null`, `is.notnull`). Unknown columns return `400`.

### 2.4 Responses

```json
{ "data": [ { "ID": 1, "NAME": "Widget" } ], "total": 42, "limit": 50, "offset": 0 }
```

Single row: `{ "data": { ... } }`. Mutation: `{ "inserted": 1, "data": [...] }`, `{ "updated": 1, "data": { ... } }`, `{ "deleted": 1 }`.
Errors: `{ "error": { "code": "not_found|bad_request|unauthorized|forbidden|conflict|internal", "message": "..." } }`.

### 2.5 Security

- CORS: `Access-Control-Allow-Origin: *`, methods `GET,POST,PATCH,DELETE,OPTIONS`, headers `Content-Type,Authorization,X-TailSQL-Key`.
- `X-TailSQL-Key: tsql_rest_<random>` or `Authorization: Bearer …` when exposed or when public read is off.
- `AllowPublicRead`: unauthenticated `GET` only.
- Every schema/table name is matched case-insensitively against the live metadata snapshot; identifiers are database-quoted, values use `?` parameters.

### 2.6 OpenAPI

Generated from live metadata: one path per table/view, `get` for all, `post` for tables, and `get/patch/delete` for tables with a single-column primary key. Component schemas reflect column types.

## 3. Shared Security Rules

- Never bind `0.0.0.0`, `::`, or non-loop/non-CGNAT addresses.
- MCP and REST servers default to loopback and stay there unless the user enables Tailnet/Funnel.
- Tokens are generated locally, stored in `~/.tailsql/config.json` with `0600`, and can be rotated from the UI.
- Agent Read-Only mode blocks `write_query` and `execute_ddl` at the backend adapter, not only in the UI.
