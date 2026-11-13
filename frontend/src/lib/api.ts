import type {
  AgentConfig,
  AgentSafetyRequest,
  AISettings,
  CreateInstanceRequest,
  FunnelState,
  GenerateSQLResult,
  Instance,
  JavaCodegen,
  LogRequest,
  MCPConfigRequest,
  ProxySnapshot,
  QueryResult,
  RESTConfigRequest,
  SchemaSnapshot,
  SeedRowsResult,
  SnapshotInfo,
  SystemHealth,
  TableMutationResult,
  TableRowsResult,
  TailscaleStatus,
} from '../types';

interface DesktopBridge {
  GetSystemHealth(): Promise<SystemHealth>;
  RefreshSystemHealth(): Promise<SystemHealth>;
  RefreshTailscale(): Promise<TailscaleStatus>;
  InstallH2Engine(): Promise<SystemHealth>;
  ListInstances(): Promise<Instance[]>;
  CreateInstance(request: CreateInstanceRequest): Promise<Instance>;
  StartInstance(id: string): Promise<Instance>;
  StopInstance(id: string): Promise<Instance>;
  RestartInstance(id: string): Promise<Instance>;
  DeleteInstance(id: string): Promise<void>;
  EnableTailscaleHosting(id: string): Promise<ProxySnapshot>;
  DisableTailscaleHosting(id: string): Promise<Instance>;
  GetProxySnapshot(id: string): Promise<ProxySnapshot>;
  GetJavaCodegen(request: { id: string; mode: string }): Promise<JavaCodegen>;
  ExecuteQuery(request: { id: string; sql: string }): Promise<QueryResult>;
  ListTables(id: string): Promise<SchemaSnapshot>;
  GetTableRows(request: {
    id: string;
    schema: string;
    table: string;
    limit: number;
    offset: number;
    orderBy: string;
    descending: boolean;
  }): Promise<TableRowsResult>;
  InsertTableRow(request: {
    id: string;
    schema: string;
    table: string;
    values: Record<string, string | null>;
    key: Record<string, string | null>;
  }): Promise<TableMutationResult>;
  UpdateTableRow(request: {
    id: string;
    schema: string;
    table: string;
    values: Record<string, string | null>;
    key: Record<string, string | null>;
  }): Promise<TableMutationResult>;
  DeleteTableRow(request: {
    id: string;
    schema: string;
    table: string;
    values: Record<string, string | null>;
    key: Record<string, string | null>;
  }): Promise<TableMutationResult>;
  DropTable(request: { id: string; schema: string; table: string }): Promise<void>;
  GetAgentConfig(): Promise<AgentConfig>;
  SetMCPConfig(request: MCPConfigRequest): Promise<AgentConfig>;
  SetRESTConfig(request: RESTConfigRequest): Promise<AgentConfig>;
  SetAgentSafety(request: AgentSafetyRequest): Promise<AgentConfig>;
  RegenerateAgentToken(request: { target: string }): Promise<AgentConfig>;
  GetAISettings(): Promise<AISettings>;
  SaveAISettings(settings: {
    provider: string;
    baseUrl: string;
    model: string;
    apiKey: string;
    clearApiKey: boolean;
  }): Promise<AISettings>;
  ListSnapshots(id: string): Promise<SnapshotInfo[]>;
  CreateSnapshot(request: { id: string; label: string }): Promise<SnapshotInfo>;
  RestoreSnapshot(request: { id: string; snapshotId: string }): Promise<Instance>;
  DeleteSnapshot(request: { id: string; snapshotId: string }): Promise<void>;
  GenerateSQL(request: { id: string; prompt: string }): Promise<GenerateSQLResult>;
  SeedTableRows(request: {
    id: string;
    schema: string;
    table: string;
    count: number;
  }): Promise<SeedRowsResult>;
  GetFunnelState(id: string): Promise<FunnelState>;
  EnableFunnel(request: { id: string; port: number }): Promise<FunnelState>;
  DisableFunnel(id: string): Promise<FunnelState>;
  GetInstanceLog(request: LogRequest): Promise<string>;
}

function desktop(): DesktopBridge {
  const bridge = window.go?.main?.App;
  if (!bridge) {
    throw new Error(
      'HySQL desktop bridge is unavailable. Launch the app through Wails (`wails dev` or the packaged binary).',
    );
  }
  return bridge as unknown as DesktopBridge;
}

export const api = {
  getSystemHealth: (): Promise<SystemHealth> => desktop().GetSystemHealth(),
  refreshSystemHealth: (): Promise<SystemHealth> => desktop().RefreshSystemHealth(),
  refreshTailscale: (): Promise<TailscaleStatus> => desktop().RefreshTailscale(),
  installH2Engine: (): Promise<SystemHealth> => desktop().InstallH2Engine(),
  listInstances: (): Promise<Instance[]> => desktop().ListInstances(),
  createInstance: (request: CreateInstanceRequest): Promise<Instance> =>
    desktop().CreateInstance(request),
  startInstance: (id: string): Promise<Instance> => desktop().StartInstance(id),
  stopInstance: (id: string): Promise<Instance> => desktop().StopInstance(id),
  restartInstance: (id: string): Promise<Instance> => desktop().RestartInstance(id),
  deleteInstance: (id: string): Promise<void> => desktop().DeleteInstance(id),
  enableTailscaleHosting: (id: string): Promise<ProxySnapshot> =>
    desktop().EnableTailscaleHosting(id),
  disableTailscaleHosting: (id: string): Promise<Instance> =>
    desktop().DisableTailscaleHosting(id),
  getProxySnapshot: (id: string): Promise<ProxySnapshot> => desktop().GetProxySnapshot(id),
  getJavaCodegen: (id: string, mode: string): Promise<JavaCodegen> =>
    desktop().GetJavaCodegen({ id, mode }),
  executeQuery: (id: string, sql: string): Promise<QueryResult> =>
    desktop().ExecuteQuery({ id, sql }),
  listTables: (id: string): Promise<SchemaSnapshot> => desktop().ListTables(id),
  getTableRows: (
    id: string,
    table: { schema: string; name: string },
    options: { limit: number; offset: number; orderBy: string; descending: boolean },
  ): Promise<TableRowsResult> =>
    desktop().GetTableRows({
      id,
      schema: table.schema,
      table: table.name,
      limit: options.limit,
      offset: options.offset,
      orderBy: options.orderBy,
      descending: options.descending,
    }),
  insertTableRow: (
    id: string,
    table: { schema: string; name: string },
    values: Record<string, string | null>,
  ): Promise<TableMutationResult> =>
    desktop().InsertTableRow({ id, schema: table.schema, table: table.name, values, key: {} }),
  updateTableRow: (
    id: string,
    table: { schema: string; name: string },
    key: Record<string, string | null>,
    values: Record<string, string | null>,
  ): Promise<TableMutationResult> =>
    desktop().UpdateTableRow({ id, schema: table.schema, table: table.name, values, key }),
  deleteTableRow: (
    id: string,
    table: { schema: string; name: string },
    key: Record<string, string | null>,
  ): Promise<TableMutationResult> =>
    desktop().DeleteTableRow({ id, schema: table.schema, table: table.name, values: {}, key }),
  dropTable: (id: string, table: { schema: string; name: string }): Promise<void> =>
    desktop().DropTable({ id, schema: table.schema, table: table.name }),
  getAgentConfig: (): Promise<AgentConfig> => desktop().GetAgentConfig(),
  setMCPConfig: (request: MCPConfigRequest): Promise<AgentConfig> =>
    desktop().SetMCPConfig(request),
  setRESTConfig: (request: RESTConfigRequest): Promise<AgentConfig> =>
    desktop().SetRESTConfig(request),
  setAgentSafety: (request: AgentSafetyRequest): Promise<AgentConfig> =>
    desktop().SetAgentSafety(request),
  regenerateAgentToken: (target: 'mcp' | 'rest'): Promise<AgentConfig> =>
    desktop().RegenerateAgentToken({ target }),
  getAISettings: (): Promise<AISettings> => desktop().GetAISettings(),
  saveAISettings: (settings: {
    provider: string;
    baseUrl: string;
    model: string;
    apiKey: string;
    clearApiKey: boolean;
  }): Promise<AISettings> => desktop().SaveAISettings(settings),
  listSnapshots: (id: string): Promise<SnapshotInfo[]> => desktop().ListSnapshots(id),
  createSnapshot: (id: string, label: string): Promise<SnapshotInfo> =>
    desktop().CreateSnapshot({ id, label }),
  restoreSnapshot: (id: string, snapshotId: string): Promise<Instance> =>
    desktop().RestoreSnapshot({ id, snapshotId }),
  deleteSnapshot: (id: string, snapshotId: string): Promise<void> =>
    desktop().DeleteSnapshot({ id, snapshotId }),
  generateSQL: (id: string, prompt: string): Promise<GenerateSQLResult> =>
    desktop().GenerateSQL({ id, prompt }),
  seedTableRows: (
    id: string,
    table: { schema: string; name: string },
    count: number,
  ): Promise<SeedRowsResult> =>
    desktop().SeedTableRows({ id, schema: table.schema, table: table.name, count }),
  getFunnelState: (id: string): Promise<FunnelState> => desktop().GetFunnelState(id),
  enableFunnel: (id: string, port: number): Promise<FunnelState> =>
    desktop().EnableFunnel({ id, port }),
  disableFunnel: (id: string): Promise<FunnelState> => desktop().DisableFunnel(id),
  getInstanceLog: (request: LogRequest): Promise<string> => desktop().GetInstanceLog(request),
};

export function errorMessage(error: unknown): string {
  if (typeof error === 'string') {
    return error;
  }
  if (error instanceof Error) {
    return error.message;
  }
  if (error && typeof error === 'object' && 'message' in error) {
    const message = (error as { message?: unknown }).message;
    if (typeof message === 'string') {
      return message;
    }
  }
  return 'An unexpected error occurred.';
}
