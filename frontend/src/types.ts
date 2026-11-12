export type InstanceStatus = 'stopped' | 'starting' | 'running' | 'stopping' | 'error';
export type JavaMode = 'local' | 'tailscale';
export type ViewKey = 'overview' | 'table' | 'java' | 'sql' | 'ai';
export type CodeLanguage = 'java' | 'xml' | 'properties' | 'yaml' | 'groovy' | 'sql' | 'text';

export interface JavaInfo {
  available: boolean;
  supported: boolean;
  path: string;
  version: string;
  major: number;
  minimum: number;
  error: string;
  checkedAt: string;
}

export interface H2Info {
  available: boolean;
  path: string;
  version: string;
  source: string;
  expectedVersion: string;
  error: string;
}

export interface TailscaleStatus {
  installed: boolean;
  running: boolean;
  backendState: string;
  ipv4: string;
  magicDns: string;
  hostname: string;
  peerCount: number;
  binaryPath: string;
  error: string;
  checkedAt: string;
}

export interface SystemHealth {
  appVersion: string;
  dataDir: string;
  java: JavaInfo;
  h2: H2Info;
  tailscale: TailscaleStatus;
  warnings: string[];
  initialized: boolean;
  error: string;
}

export interface Instance {
  id: string;
  name: string;
  username: string;
  password: string;
  port: number;
  status: InstanceStatus;
  pid: number;
  tailscalePort: number;
  tailscaleEnabled: boolean;
  tailscaleIp: string;
  funnelEnabled: boolean;
  funnelPort: number;
  funnelHost: string;
  funnelUrl: string;
  funnelJdbcUrl: string;
  databaseFile: string;
  fileSizeBytes: number;
  createdAt: string;
  updatedAt: string;
  startedAt?: string | null;
  lastError: string;
  jdbcUrl: string;
  tailscaleUrl: string;
  logPath: string;
}

export interface CreateInstanceRequest {
  name: string;
  username: string;
  password: string;
  port: number;
}

export interface QueryColumn {
  name: string;
  type: string;
  nullable: boolean;
}

export interface QueryResult {
  columns: QueryColumn[];
  rows: (string | null)[][];
  rowCount: number;
  updateCount: number;
  hasResultSet: boolean;
  truncated: boolean;
  engineDurationMs: number;
  durationMs: number;
  executedAt: string;
}

export interface TableColumn {
  name: string;
  type: string;
  size: number;
  nullable: boolean;
  ordinal: number;
  default: string;
  autoIncrement: boolean;
  generated: boolean;
}

export interface TableInfo {
  schema: string;
  name: string;
  type: string;
  primaryKey: string[];
  columns: TableColumn[];
}

export interface SchemaSnapshot {
  tables: TableInfo[];
}

export interface TableRowsResult {
  columns: QueryColumn[];
  rows: (string | null)[][];
  hasMore: boolean;
  limit: number;
  offset: number;
}

export interface TableMutationResult {
  updateCount: number;
}

export interface FunnelState {
  instanceId: string;
  available: boolean;
  enabled: boolean;
  port: number;
  host: string;
  publicUrl: string;
  jdbcUrl: string;
  allowedPorts: number[];
  error: string;
  checkedAt: string;
}

export interface ProxyClient {
  id: string;
  peerIp: string;
  connectedAt: string;
  bytesIn: number;
  bytesOut: number;
}

export interface ProxySnapshot {
  instanceId: string;
  running: boolean;
  bindAddress: string;
  port: number;
  targetAddress: string;
  activeClients: number;
  totalConnections: number;
  bytesIn: number;
  bytesOut: number;
  startedAt?: string | null;
  clients: ProxyClient[];
  lastError?: string;
}

export interface JavaCodegen {
  instanceId: string;
  mode: string;
  available: boolean;
  reason: string;
  jdbcUrl: string;
  host: string;
  port: number;
  database: string;
  username: string;
  password: string;
  magicDns?: string;
  maven: string;
  gradle: string;
  hikari: string;
  springProperties: string;
  springYaml: string;
  jdbcTest: string;
}

export interface AppErrorEvent {
  instanceId?: string;
  message: string;
}

export interface RealtimeChangeEvent {
  instanceId: string;
  schema?: string;
  table?: string;
  operation: string;
  source: string;
  updateCount?: number;
  at: string;
}

export interface LogRequest {
  id: string;
  lines: number;
}

export interface MCPState {
  running: boolean;
  enabled: boolean;
  port: number;
  bindAddress: string;
  endpoint: string;
  token: string;
  tailscaleEnabled: boolean;
  error: string;
}

export interface RESTState {
  running: boolean;
  enabled: boolean;
  port: number;
  bindAddress: string;
  baseUrl: string;
  localUrl: string;
  publicBaseUrl: string;
  apiKey: string;
  tailscaleEnabled: boolean;
  funnelEnabled: boolean;
  funnelPort: number;
  allowPublicRead: boolean;
  databaseCount: number;
  error: string;
}

export interface AgentSafety {
  readOnly: boolean;
  autoSnapshotOnAgentDdl: boolean;
}

export interface AISettings {
  provider: string;
  baseUrl: string;
  model: string;
  hasApiKey: boolean;
}

export interface AgentConfig {
  mcp: MCPState;
  rest: RESTState;
  safety: AgentSafety;
  ai: AISettings;
  snippets: Record<string, string>;
  systemPrompt: string;
  tailscaleIp: string;
  tailscaleAvailable: boolean;
}

export interface MCPConfigRequest {
  enabled: boolean;
  port: number;
  tailscaleEnabled: boolean;
}

export interface RESTConfigRequest {
  enabled: boolean;
  port: number;
  tailscaleEnabled: boolean;
  funnelEnabled: boolean;
  funnelPort: number;
  allowPublicRead: boolean;
}

export interface AgentSafetyRequest {
  readOnly: boolean;
  autoSnapshotOnAgentDdl: boolean;
}

export interface SnapshotInfo {
  id: string;
  label: string;
  sizeBytes: number;
  createdAt: string;
}

export interface GenerateSQLResult {
  sql: string;
  provider: string;
  model: string;
}

export interface SeedRowsResult {
  inserted: number;
  generatedBy: string;
}
