export namespace codegen {
	
	export class Artifacts {
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
	
	    static createFrom(source: any = {}) {
	        return new Artifacts(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.instanceId = source["instanceId"];
	        this.mode = source["mode"];
	        this.available = source["available"];
	        this.reason = source["reason"];
	        this.jdbcUrl = source["jdbcUrl"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.database = source["database"];
	        this.username = source["username"];
	        this.password = source["password"];
	        this.magicDns = source["magicDns"];
	        this.maven = source["maven"];
	        this.gradle = source["gradle"];
	        this.hikari = source["hikari"];
	        this.springProperties = source["springProperties"];
	        this.springYaml = source["springYaml"];
	        this.jdbcTest = source["jdbcTest"];
	    }
	}

}

export namespace dbmanager {
	
	export class H2Info {
	    available: boolean;
	    path: string;
	    version: string;
	    source: string;
	    expectedVersion: string;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new H2Info(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.path = source["path"];
	        this.version = source["version"];
	        this.source = source["source"];
	        this.expectedVersion = source["expectedVersion"];
	        this.error = source["error"];
	    }
	}
	export class JavaInfo {
	    available: boolean;
	    supported: boolean;
	    path: string;
	    version: string;
	    major: number;
	    minimum: number;
	    error: string;
	    checkedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new JavaInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.supported = source["supported"];
	        this.path = source["path"];
	        this.version = source["version"];
	        this.major = source["major"];
	        this.minimum = source["minimum"];
	        this.error = source["error"];
	        this.checkedAt = source["checkedAt"];
	    }
	}

}

export namespace jdbc {
	
	export class Column {
	    name: string;
	    type: string;
	    nullable: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Column(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.nullable = source["nullable"];
	    }
	}
	export class ColumnInfo {
	    name: string;
	    type: string;
	    size: number;
	    nullable: boolean;
	    ordinal: number;
	    default: string;
	    autoIncrement: boolean;
	    generated: boolean;
	    remarks: string;
	
	    static createFrom(source: any = {}) {
	        return new ColumnInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.size = source["size"];
	        this.nullable = source["nullable"];
	        this.ordinal = source["ordinal"];
	        this.default = source["default"];
	        this.autoIncrement = source["autoIncrement"];
	        this.generated = source["generated"];
	        this.remarks = source["remarks"];
	    }
	}
	export class ForeignKeyInfo {
	    name: string;
	    column: string;
	    referencedSchema: string;
	    referencedTable: string;
	    referencedColumn: string;
	    onUpdate: string;
	    onDelete: string;
	
	    static createFrom(source: any = {}) {
	        return new ForeignKeyInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.column = source["column"];
	        this.referencedSchema = source["referencedSchema"];
	        this.referencedTable = source["referencedTable"];
	        this.referencedColumn = source["referencedColumn"];
	        this.onUpdate = source["onUpdate"];
	        this.onDelete = source["onDelete"];
	    }
	}
	export class IndexInfo {
	    name: string;
	    unique: boolean;
	    columns: string[];
	
	    static createFrom(source: any = {}) {
	        return new IndexInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.unique = source["unique"];
	        this.columns = source["columns"];
	    }
	}
	export class TableInfo {
	    schema: string;
	    name: string;
	    type: string;
	    remarks: string;
	    primaryKey: string[];
	    columns: ColumnInfo[];
	    foreignKeys: ForeignKeyInfo[];
	    indexes: IndexInfo[];
	
	    static createFrom(source: any = {}) {
	        return new TableInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.schema = source["schema"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.remarks = source["remarks"];
	        this.primaryKey = source["primaryKey"];
	        this.columns = this.convertValues(source["columns"], ColumnInfo);
	        this.foreignKeys = this.convertValues(source["foreignKeys"], ForeignKeyInfo);
	        this.indexes = this.convertValues(source["indexes"], IndexInfo);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SchemaSnapshot {
	    tables: TableInfo[];
	
	    static createFrom(source: any = {}) {
	        return new SchemaSnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tables = this.convertValues(source["tables"], TableInfo);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace main {
	
	export class AIProviderSettings {
	    provider: string;
	    baseUrl: string;
	    model: string;
	    apiKey: string;
	    clearApiKey: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AIProviderSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.baseUrl = source["baseUrl"];
	        this.model = source["model"];
	        this.apiKey = source["apiKey"];
	        this.clearApiKey = source["clearApiKey"];
	    }
	}
	export class AISettingsView {
	    provider: string;
	    baseUrl: string;
	    model: string;
	    hasApiKey: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AISettingsView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.baseUrl = source["baseUrl"];
	        this.model = source["model"];
	        this.hasApiKey = source["hasApiKey"];
	    }
	}
	export class AgentSafetyView {
	    readOnly: boolean;
	    autoSnapshotOnAgentDdl: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AgentSafetyView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.readOnly = source["readOnly"];
	        this.autoSnapshotOnAgentDdl = source["autoSnapshotOnAgentDdl"];
	    }
	}
	export class RESTStateView {
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
	
	    static createFrom(source: any = {}) {
	        return new RESTStateView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.enabled = source["enabled"];
	        this.port = source["port"];
	        this.bindAddress = source["bindAddress"];
	        this.baseUrl = source["baseUrl"];
	        this.localUrl = source["localUrl"];
	        this.publicBaseUrl = source["publicBaseUrl"];
	        this.apiKey = source["apiKey"];
	        this.tailscaleEnabled = source["tailscaleEnabled"];
	        this.funnelEnabled = source["funnelEnabled"];
	        this.funnelPort = source["funnelPort"];
	        this.allowPublicRead = source["allowPublicRead"];
	        this.databaseCount = source["databaseCount"];
	        this.error = source["error"];
	    }
	}
	export class MCPStateView {
	    running: boolean;
	    enabled: boolean;
	    port: number;
	    bindAddress: string;
	    endpoint: string;
	    token: string;
	    tailscaleEnabled: boolean;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new MCPStateView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.enabled = source["enabled"];
	        this.port = source["port"];
	        this.bindAddress = source["bindAddress"];
	        this.endpoint = source["endpoint"];
	        this.token = source["token"];
	        this.tailscaleEnabled = source["tailscaleEnabled"];
	        this.error = source["error"];
	    }
	}
	export class AgentConfigView {
	    mcp: MCPStateView;
	    rest: RESTStateView;
	    safety: AgentSafetyView;
	    ai: AISettingsView;
	    snippets: Record<string, string>;
	    systemPrompt: string;
	    tailscaleIp: string;
	    tailscaleAvailable: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AgentConfigView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mcp = this.convertValues(source["mcp"], MCPStateView);
	        this.rest = this.convertValues(source["rest"], RESTStateView);
	        this.safety = this.convertValues(source["safety"], AgentSafetyView);
	        this.ai = this.convertValues(source["ai"], AISettingsView);
	        this.snippets = source["snippets"];
	        this.systemPrompt = source["systemPrompt"];
	        this.tailscaleIp = source["tailscaleIp"];
	        this.tailscaleAvailable = source["tailscaleAvailable"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class AgentSafetyRequest {
	    readOnly: boolean;
	    autoSnapshotOnAgentDdl: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AgentSafetyRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.readOnly = source["readOnly"];
	        this.autoSnapshotOnAgentDdl = source["autoSnapshotOnAgentDdl"];
	    }
	}
	
	export class CreateInstanceRequest {
	    name: string;
	    username: string;
	    password: string;
	    port: number;
	
	    static createFrom(source: any = {}) {
	        return new CreateInstanceRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.username = source["username"];
	        this.password = source["password"];
	        this.port = source["port"];
	    }
	}
	export class FunnelRequest {
	    id: string;
	    port: number;
	
	    static createFrom(source: any = {}) {
	        return new FunnelRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.port = source["port"];
	    }
	}
	export class GenerateSQLRequest {
	    id: string;
	    prompt: string;
	
	    static createFrom(source: any = {}) {
	        return new GenerateSQLRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.prompt = source["prompt"];
	    }
	}
	export class GenerateSQLResult {
	    sql: string;
	    provider: string;
	    model: string;
	
	    static createFrom(source: any = {}) {
	        return new GenerateSQLResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sql = source["sql"];
	        this.provider = source["provider"];
	        this.model = source["model"];
	    }
	}
	export class InstanceView {
	    id: string;
	    name: string;
	    username: string;
	    password: string;
	    port: number;
	    status: string;
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
	    startedAt?: string;
	    lastError: string;
	    jdbcUrl: string;
	    tailscaleUrl: string;
	    logPath: string;
	
	    static createFrom(source: any = {}) {
	        return new InstanceView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.username = source["username"];
	        this.password = source["password"];
	        this.port = source["port"];
	        this.status = source["status"];
	        this.pid = source["pid"];
	        this.tailscalePort = source["tailscalePort"];
	        this.tailscaleEnabled = source["tailscaleEnabled"];
	        this.tailscaleIp = source["tailscaleIp"];
	        this.funnelEnabled = source["funnelEnabled"];
	        this.funnelPort = source["funnelPort"];
	        this.funnelHost = source["funnelHost"];
	        this.funnelUrl = source["funnelUrl"];
	        this.funnelJdbcUrl = source["funnelJdbcUrl"];
	        this.databaseFile = source["databaseFile"];
	        this.fileSizeBytes = source["fileSizeBytes"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	        this.startedAt = source["startedAt"];
	        this.lastError = source["lastError"];
	        this.jdbcUrl = source["jdbcUrl"];
	        this.tailscaleUrl = source["tailscaleUrl"];
	        this.logPath = source["logPath"];
	    }
	}
	export class JavaCodegenRequest {
	    id: string;
	    mode: string;
	
	    static createFrom(source: any = {}) {
	        return new JavaCodegenRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.mode = source["mode"];
	    }
	}
	export class LogRequest {
	    id: string;
	    lines: number;
	
	    static createFrom(source: any = {}) {
	        return new LogRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.lines = source["lines"];
	    }
	}
	export class MCPConfigRequest {
	    enabled: boolean;
	    port: number;
	    tailscaleEnabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new MCPConfigRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.port = source["port"];
	        this.tailscaleEnabled = source["tailscaleEnabled"];
	    }
	}
	
	export class QueryRequest {
	    id: string;
	    sql: string;
	
	    static createFrom(source: any = {}) {
	        return new QueryRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.sql = source["sql"];
	    }
	}
	export class QueryResult {
	    columns: jdbc.Column[];
	    rows: string[][];
	    rowCount: number;
	    updateCount: number;
	    hasResultSet: boolean;
	    truncated: boolean;
	    engineDurationMs: number;
	    durationMs: number;
	    executedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new QueryResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.columns = this.convertValues(source["columns"], jdbc.Column);
	        this.rows = source["rows"];
	        this.rowCount = source["rowCount"];
	        this.updateCount = source["updateCount"];
	        this.hasResultSet = source["hasResultSet"];
	        this.truncated = source["truncated"];
	        this.engineDurationMs = source["engineDurationMs"];
	        this.durationMs = source["durationMs"];
	        this.executedAt = source["executedAt"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RESTConfigRequest {
	    enabled: boolean;
	    port: number;
	    tailscaleEnabled: boolean;
	    funnelEnabled: boolean;
	    funnelPort: number;
	    allowPublicRead: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RESTConfigRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.port = source["port"];
	        this.tailscaleEnabled = source["tailscaleEnabled"];
	        this.funnelEnabled = source["funnelEnabled"];
	        this.funnelPort = source["funnelPort"];
	        this.allowPublicRead = source["allowPublicRead"];
	    }
	}
	
	export class SeedRowsRequest {
	    id: string;
	    schema: string;
	    table: string;
	    count: number;
	
	    static createFrom(source: any = {}) {
	        return new SeedRowsRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.schema = source["schema"];
	        this.table = source["table"];
	        this.count = source["count"];
	    }
	}
	export class SeedRowsResult {
	    inserted: number;
	    generatedBy: string;
	
	    static createFrom(source: any = {}) {
	        return new SeedRowsResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.inserted = source["inserted"];
	        this.generatedBy = source["generatedBy"];
	    }
	}
	export class SnapshotRequest {
	    id: string;
	    snapshotId: string;
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new SnapshotRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.snapshotId = source["snapshotId"];
	        this.label = source["label"];
	    }
	}
	export class SnapshotView {
	    id: string;
	    label: string;
	    sizeBytes: number;
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new SnapshotView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.sizeBytes = source["sizeBytes"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class SystemHealth {
	    appVersion: string;
	    dataDir: string;
	    java: dbmanager.JavaInfo;
	    h2: dbmanager.H2Info;
	    tailscale: tailscale.Status;
	    warnings: string[];
	    initialized: boolean;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new SystemHealth(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.appVersion = source["appVersion"];
	        this.dataDir = source["dataDir"];
	        this.java = this.convertValues(source["java"], dbmanager.JavaInfo);
	        this.h2 = this.convertValues(source["h2"], dbmanager.H2Info);
	        this.tailscale = this.convertValues(source["tailscale"], tailscale.Status);
	        this.warnings = source["warnings"];
	        this.initialized = source["initialized"];
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TableDropRequest {
	    id: string;
	    schema: string;
	    table: string;
	
	    static createFrom(source: any = {}) {
	        return new TableDropRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.schema = source["schema"];
	        this.table = source["table"];
	    }
	}
	export class string {
	
	
	    static createFrom(source: any = {}) {
	        return new string(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	
	    }
	}
	export class TableMutationRequest {
	    id: string;
	    schema: string;
	    table: string;
	    values: Record<string, string>;
	    key: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new TableMutationRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.schema = source["schema"];
	        this.table = source["table"];
	        this.values = source["values"];
	        this.key = source["key"];
	    }
	}
	export class TableMutationResult {
	    updateCount: number;
	
	    static createFrom(source: any = {}) {
	        return new TableMutationResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.updateCount = source["updateCount"];
	    }
	}
	export class TableRowsRequest {
	    id: string;
	    schema: string;
	    table: string;
	    limit: number;
	    offset: number;
	    orderBy: string;
	    descending: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TableRowsRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.schema = source["schema"];
	        this.table = source["table"];
	        this.limit = source["limit"];
	        this.offset = source["offset"];
	        this.orderBy = source["orderBy"];
	        this.descending = source["descending"];
	    }
	}
	export class TableRowsResult {
	    columns: jdbc.Column[];
	    rows: string[][];
	    hasMore: boolean;
	    limit: number;
	    offset: number;
	
	    static createFrom(source: any = {}) {
	        return new TableRowsResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.columns = this.convertValues(source["columns"], jdbc.Column);
	        this.rows = source["rows"];
	        this.hasMore = source["hasMore"];
	        this.limit = source["limit"];
	        this.offset = source["offset"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TokenRotateRequest {
	    target: string;
	
	    static createFrom(source: any = {}) {
	        return new TokenRotateRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.target = source["target"];
	    }
	}

}

export namespace tailscale {
	
	export class ClientSnapshot {
	    id: string;
	    peerIp: string;
	    connectedAt: string;
	    bytesIn: number;
	    bytesOut: number;
	
	    static createFrom(source: any = {}) {
	        return new ClientSnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.peerIp = source["peerIp"];
	        this.connectedAt = source["connectedAt"];
	        this.bytesIn = source["bytesIn"];
	        this.bytesOut = source["bytesOut"];
	    }
	}
	export class FunnelState {
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
	
	    static createFrom(source: any = {}) {
	        return new FunnelState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.instanceId = source["instanceId"];
	        this.available = source["available"];
	        this.enabled = source["enabled"];
	        this.port = source["port"];
	        this.host = source["host"];
	        this.publicUrl = source["publicUrl"];
	        this.jdbcUrl = source["jdbcUrl"];
	        this.allowedPorts = source["allowedPorts"];
	        this.error = source["error"];
	        this.checkedAt = source["checkedAt"];
	    }
	}
	export class Snapshot {
	    instanceId: string;
	    running: boolean;
	    bindAddress: string;
	    port: number;
	    targetAddress: string;
	    activeClients: number;
	    totalConnections: number;
	    bytesIn: number;
	    bytesOut: number;
	    startedAt?: string;
	    clients: ClientSnapshot[];
	    lastError?: string;
	
	    static createFrom(source: any = {}) {
	        return new Snapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.instanceId = source["instanceId"];
	        this.running = source["running"];
	        this.bindAddress = source["bindAddress"];
	        this.port = source["port"];
	        this.targetAddress = source["targetAddress"];
	        this.activeClients = source["activeClients"];
	        this.totalConnections = source["totalConnections"];
	        this.bytesIn = source["bytesIn"];
	        this.bytesOut = source["bytesOut"];
	        this.startedAt = source["startedAt"];
	        this.clients = this.convertValues(source["clients"], ClientSnapshot);
	        this.lastError = source["lastError"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Status {
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
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.installed = source["installed"];
	        this.running = source["running"];
	        this.backendState = source["backendState"];
	        this.ipv4 = source["ipv4"];
	        this.magicDns = source["magicDns"];
	        this.hostname = source["hostname"];
	        this.peerCount = source["peerCount"];
	        this.binaryPath = source["binaryPath"];
	        this.error = source["error"];
	        this.checkedAt = source["checkedAt"];
	    }
	}

}

