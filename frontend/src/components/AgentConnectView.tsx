import {
  Bot,
  Globe2,
  KeyRound,
  Loader2,
  RefreshCw,
  Server,
  Settings2,
  ShieldCheck,
  Sparkles,
  Terminal,
} from 'lucide-react';
import { useEffect, useState, type ReactNode } from 'react';
import type {
  AgentConfig,
  AgentSafetyRequest,
  Instance,
  MCPConfigRequest,
  RESTConfigRequest,
} from '../types';
import { api, errorMessage } from '../lib/api';
import type { ToastKind } from '../hooks/useToasts';
import { CodeBlock } from './CodeBlock';
import { CopyButton } from './CopyButton';

interface AgentConnectViewProps {
  instance: Instance;
  onOpenAISettings: () => void;
  onToast: (kind: ToastKind, message: string) => void;
}

const snippetTabs = [
  { key: 'windsurf', label: 'Windsurf', file: 'mcp_config.json' },
  { key: 'cursor', label: 'Cursor', file: '.cursor/mcp.json' },
  { key: 'claude', label: 'Claude Code', file: 'claude_desktop_config.json' },
  { key: 'environment', label: 'Env vars', file: '.env' },
  { key: 'prompt', label: 'Agent prompt', file: 'tailsql-agent-prompt.md' },
] as const;

type SnippetKey = (typeof snippetTabs)[number]['key'];

export function AgentConnectView({ instance, onOpenAISettings, onToast }: AgentConnectViewProps) {
  const [config, setConfig] = useState<AgentConfig | null>(null);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState('');
  const [mcpPort, setMcpPort] = useState('9099');
  const [restPort, setRestPort] = useState('9199');
  const [funnelPort, setFunnelPort] = useState('443');
  const [tab, setTab] = useState<SnippetKey>('prompt');

  const apply = (next: AgentConfig) => {
    setConfig(next);
    setMcpPort(String(next.mcp.port));
    setRestPort(String(next.rest.port));
    setFunnelPort(String(next.rest.funnelPort));
  };

  const load = async () => {
    setLoading(true);
    setError('');
    try {
      apply(await api.getAgentConfig());
    } catch (loadError) {
      setError(errorMessage(loadError));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const saveMCP = async (overrides: Partial<MCPConfigRequest>) => {
    if (!config) {
      return;
    }
    setBusy('mcp');
    setError('');
    try {
      const next = await api.setMCPConfig({
        enabled: config.mcp.enabled,
        port: Number(mcpPort) || config.mcp.port,
        tailscaleEnabled: config.mcp.tailscaleEnabled,
        ...overrides,
      });
      apply(next);
      onToast('success', 'MCP server settings applied.');
    } catch (saveError) {
      setError(errorMessage(saveError));
      onToast('error', errorMessage(saveError));
    } finally {
      setBusy(null);
    }
  };

  const saveREST = async (overrides: Partial<RESTConfigRequest>) => {
    if (!config) {
      return;
    }
    setBusy('rest');
    setError('');
    try {
      const next = await api.setRESTConfig({
        enabled: config.rest.enabled,
        port: Number(restPort) || config.rest.port,
        tailscaleEnabled: config.rest.tailscaleEnabled,
        funnelEnabled: config.rest.funnelEnabled,
        funnelPort: Number(funnelPort) || config.rest.funnelPort,
        allowPublicRead: config.rest.allowPublicRead,
        ...overrides,
      });
      apply(next);
      onToast('success', 'Auto-REST settings applied.');
    } catch (saveError) {
      setError(errorMessage(saveError));
      onToast('error', errorMessage(saveError));
    } finally {
      setBusy(null);
    }
  };

  const saveSafety = async (overrides: Partial<AgentSafetyRequest>) => {
    if (!config) {
      return;
    }
    setBusy('safety');
    setError('');
    try {
      const next = await api.setAgentSafety({
        readOnly: config.safety.readOnly,
        autoSnapshotOnAgentDdl: config.safety.autoSnapshotOnAgentDdl,
        ...overrides,
      });
      apply(next);
      onToast('success', 'Agent safety updated.');
    } catch (saveError) {
      setError(errorMessage(saveError));
      onToast('error', errorMessage(saveError));
    } finally {
      setBusy(null);
    }
  };

  const rotate = async (target: 'mcp' | 'rest') => {
    setBusy(target);
    setError('');
    try {
      apply(await api.regenerateAgentToken(target));
      onToast('success', `${target.toUpperCase()} token rotated. Update any saved agent configs.`);
    } catch (rotateError) {
      setError(errorMessage(rotateError));
      onToast('error', errorMessage(rotateError));
    } finally {
      setBusy(null);
    }
  };

  const prompt = config?.systemPrompt ?? '';
  const snippets: Record<SnippetKey, string> = {
    windsurf: config?.snippets.windsurf ?? '',
    cursor: config?.snippets.cursor ?? '',
    claude: config?.snippets.claude ?? '',
    environment: config?.snippets.environment ?? '',
    prompt,
  };
  const activeTab = snippetTabs.find((entry) => entry.key === tab) ?? snippetTabs[0];

  return (
    <div className="space-y-5 animate-fade-in">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <p className="label mb-1">Agent bridge</p>
          <h1 className="text-xl font-semibold tracking-tight text-slate-100">Connect to AI</h1>
          <p className="mt-1 max-w-2xl text-xs leading-relaxed text-slate-500">
            Expose {instance.name} and every other TailSQL database to Windsurf, Cursor, Claude Code,
            or any MCP client. Copy a snippet below, or paste the agent prompt into a chat so the
            agent knows the live schema, JDBC URLs, and REST endpoints.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <button type="button" className="btn-secondary" onClick={onOpenAISettings}>
            <Settings2 size={14} />
            AI settings
          </button>
          <button type="button" className="btn-secondary" onClick={() => void load()} disabled={loading}>
            <RefreshCw size={14} className={loading ? 'animate-spin' : ''} />
            Refresh
          </button>
        </div>
      </div>

      {error ? (
        <div className="rounded-xl border border-rose-500/20 bg-rose-500/[0.05] px-4 py-3 text-[11px] leading-relaxed text-rose-200/80">
          {error}
        </div>
      ) : null}

      {loading && !config ? (
        <div className="card flex items-center gap-2 p-6 text-xs text-slate-500">
          <Loader2 size={14} className="animate-spin" /> Reading agent configuration…
        </div>
      ) : config ? (
        <>
          <div className="grid gap-5 xl:grid-cols-2">
            <div className="card overflow-hidden">
              <div className="card-header">
                <div>
                  <h2 className="flex items-center gap-2 text-sm font-semibold text-slate-100">
                    <Bot size={15} className="text-mint-300" />
                    MCP server
                  </h2>
                  <p className="mt-0.5 text-[11px] text-slate-500">
                    JSON-RPC 2.0 over HTTP/SSE for AI coding agents.
                  </p>
                </div>
                <button
                  type="button"
                  className="switch"
                  data-on={config.mcp.enabled}
                  onClick={() => void saveMCP({ enabled: !config.mcp.enabled })}
                  disabled={Boolean(busy)}
                  aria-label="Toggle MCP server"
                  aria-pressed={config.mcp.enabled}
                >
                  <span className="switch-knob" />
                </button>
              </div>
              <div className="space-y-3.5 p-5">
                <div className="flex flex-wrap items-center gap-2">
                  <span
                    className={`chip ${
                      config.mcp.running
                        ? 'border-mint-400/20 text-mint-300'
                        : 'border-amber-400/20 text-amber-300'
                    }`}
                  >
                    {config.mcp.running ? 'Running' : 'Stopped'}
                  </span>
                  {config.mcp.endpoint ? (
                    <span className="chip font-mono text-[10px]">{config.mcp.endpoint}</span>
                  ) : null}
                  {config.mcp.tailscaleEnabled ? (
                    <span className="chip">
                      <Globe2 size={10} />
                      Tailnet
                    </span>
                  ) : null}
                </div>

                <div className="flex flex-wrap items-end gap-2">
                  <label className="min-w-[120px] flex-1">
                    <span className="label mb-1.5 block">Port</span>
                    <input
                      className="input font-mono text-xs"
                      value={mcpPort}
                      onChange={(event) => setMcpPort(event.target.value)}
                      inputMode="numeric"
                    />
                  </label>
                  <button
                    type="button"
                    className="btn-secondary"
                    onClick={() => void saveMCP({ port: Number(mcpPort) })}
                    disabled={Boolean(busy)}
                  >
                    Apply port
                  </button>
                </div>

                <ToggleRow
                  icon={<Globe2 size={13} />}
                  title="Expose MCP on Tailnet"
                  description={
                    config.tailscaleAvailable
                      ? `Bind ${config.tailscaleIp || 'the Tailscale IP'} instead of loopback. Requires the Bearer token.`
                      : 'Tailscale is not connected. Connect it first to expose MCP remotely.'
                  }
                  on={config.mcp.tailscaleEnabled}
                  disabled={Boolean(busy) || !config.tailscaleAvailable}
                  onToggle={() => void saveMCP({ tailscaleEnabled: !config.mcp.tailscaleEnabled })}
                />

                <div className="rounded-xl border border-white/[0.06] bg-ink-950/40 p-3">
                  <div className="mb-1 flex items-center justify-between gap-2">
                    <span className="flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wider text-slate-600">
                      <KeyRound size={11} />
                      Bearer token
                    </span>
                    <div className="flex items-center gap-1">
                      <button
                        type="button"
                        className="btn-ghost btn-xs"
                        onClick={() => void rotate('mcp')}
                        disabled={Boolean(busy)}
                      >
                        Rotate
                      </button>
                      <CopyButton value={config.mcp.token} iconOnly onError={(message) => onToast('error', message)} />
                    </div>
                  </div>
                  <code className="block break-all font-mono text-[10px] leading-relaxed text-slate-400">
                    {config.mcp.token}
                  </code>
                </div>
                {config.mcp.error ? (
                  <p className="text-[11px] leading-relaxed text-amber-300/80">{config.mcp.error}</p>
                ) : null}
              </div>
            </div>

            <div className="card overflow-hidden">
              <div className="card-header">
                <div>
                  <h2 className="flex items-center gap-2 text-sm font-semibold text-slate-100">
                    <ShieldCheck size={15} className="text-mint-300" />
                    Agent safe mode
                  </h2>
                  <p className="mt-0.5 text-[11px] text-slate-500">
                    Guardrails enforced in the backend, not just the UI.
                  </p>
                </div>
              </div>
              <div className="space-y-1 p-3">
                <ToggleRow
                  icon={<ShieldCheck size={13} />}
                  title="Read-Only mode for AI agents"
                  description="Blocks write_query and execute_ddl for every MCP agent."
                  on={config.safety.readOnly}
                  disabled={Boolean(busy)}
                  onToggle={() => void saveSafety({ readOnly: !config.safety.readOnly })}
                />
                <ToggleRow
                  icon={<Sparkles size={13} />}
                  title="Auto-snapshot before Agent DDL"
                  description="Creates a “Pre-Agent DDL” checkpoint before DROP, TRUNCATE, or ALTER."
                  on={config.safety.autoSnapshotOnAgentDdl}
                  disabled={Boolean(busy)}
                  onToggle={() =>
                    void saveSafety({ autoSnapshotOnAgentDdl: !config.safety.autoSnapshotOnAgentDdl })
                  }
                />
                <div className="px-3 pb-2 pt-1 text-[10px] leading-relaxed text-slate-600">
                  Checkpoints live in ~/.tailsql/snapshots and can be restored from the database
                  header at any time.
                </div>
              </div>
            </div>
          </div>

          <div className="card overflow-hidden">
            <div className="card-header">
              <div>
                <h2 className="flex items-center gap-2 text-sm font-semibold text-slate-100">
                  <Server size={15} className="text-mint-300" />
                  Auto-REST gateway
                </h2>
                <p className="mt-0.5 text-[11px] text-slate-500">
                  Browser-friendly JSON CRUD for every table and view, generated from live metadata.
                </p>
              </div>
              <button
                type="button"
                className="switch"
                data-on={config.rest.enabled}
                onClick={() => void saveREST({ enabled: !config.rest.enabled })}
                disabled={Boolean(busy)}
                aria-label="Toggle Auto-REST gateway"
                aria-pressed={config.rest.enabled}
              >
                <span className="switch-knob" />
              </button>
            </div>
            <div className="grid gap-5 p-5 lg:grid-cols-2">
              <div className="space-y-3.5">
                <div className="flex flex-wrap items-center gap-2">
                  <span
                    className={`chip ${
                      config.rest.running
                        ? 'border-mint-400/20 text-mint-300'
                        : 'border-amber-400/20 text-amber-300'
                    }`}
                  >
                    {config.rest.running ? 'Running' : 'Stopped'}
                  </span>
                  {config.rest.baseUrl ? (
                    <span className="chip font-mono text-[10px]">{config.rest.baseUrl}</span>
                  ) : null}
                </div>

                <div className="flex flex-wrap items-end gap-2">
                  <label className="min-w-[110px] flex-1">
                    <span className="label mb-1.5 block">Port</span>
                    <input
                      className="input font-mono text-xs"
                      value={restPort}
                      onChange={(event) => setRestPort(event.target.value)}
                      inputMode="numeric"
                    />
                  </label>
                  <label className="min-w-[110px] flex-1">
                    <span className="label mb-1.5 block">Funnel port</span>
                    <select
                      className="input font-mono text-xs"
                      value={funnelPort}
                      onChange={(event) => setFunnelPort(event.target.value)}
                    >
                      <option value="443">443</option>
                      <option value="8443">8443</option>
                      <option value="10000">10000</option>
                    </select>
                  </label>
                  <button
                    type="button"
                    className="btn-secondary"
                    onClick={() => void saveREST({ port: Number(restPort), funnelPort: Number(funnelPort) })}
                    disabled={Boolean(busy)}
                  >
                    Apply
                  </button>
                </div>

                <div className="rounded-xl border border-white/[0.06] bg-ink-950/40 p-3">
                  <div className="mb-1 flex items-center justify-between gap-2">
                    <span className="flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wider text-slate-600">
                      <KeyRound size={11} />
                      X-TailSQL-Key
                    </span>
                    <div className="flex items-center gap-1">
                      <button
                        type="button"
                        className="btn-ghost btn-xs"
                        onClick={() => void rotate('rest')}
                        disabled={Boolean(busy)}
                      >
                        Rotate
                      </button>
                      <CopyButton value={config.rest.apiKey} iconOnly onError={(message) => onToast('error', message)} />
                    </div>
                  </div>
                  <code className="block break-all font-mono text-[10px] leading-relaxed text-slate-400">
                    {config.rest.apiKey}
                  </code>
                </div>
                {config.rest.funnelEnabled && config.rest.publicBaseUrl ? (
                  <div className="flex items-center justify-between gap-2 rounded-xl border border-mint-400/20 bg-mint-400/[0.04] px-3 py-2">
                    <span className="truncate font-mono text-[10px] text-mint-200">
                      {config.rest.publicBaseUrl}/api/v1/{instance.name}
                    </span>
                    <CopyButton
                      value={`${config.rest.publicBaseUrl}/api/v1/${instance.name}`}
                      iconOnly
                      onError={(message) => onToast('error', message)}
                    />
                  </div>
                ) : null}
                {config.rest.error ? (
                  <p className="text-[11px] leading-relaxed text-amber-300/80">{config.rest.error}</p>
                ) : null}
              </div>

              <div className="space-y-1">
                <ToggleRow
                  icon={<Globe2 size={13} />}
                  title="Expose REST on Tailnet"
                  description="Bind the Tailscale IPv4 and require the API key."
                  on={config.rest.tailscaleEnabled}
                  disabled={Boolean(busy) || !config.tailscaleAvailable || config.rest.funnelEnabled}
                  onToggle={() => void saveREST({ tailscaleEnabled: !config.rest.tailscaleEnabled })}
                />
                <ToggleRow
                  icon={<Globe2 size={13} />}
                  title="Publish REST through Funnel"
                  description="Public HTTPS URL for Vercel/Netlify frontends. The API key is required."
                  on={config.rest.funnelEnabled}
                  disabled={Boolean(busy) || !config.tailscaleAvailable}
                  onToggle={() => void saveREST({ funnelEnabled: !config.rest.funnelEnabled })}
                />
                <ToggleRow
                  icon={<ShieldCheck size={13} />}
                  title="Allow public GET requests"
                  description="Keep GET read-only routes open when Funnel or Tailnet auth is enabled."
                  on={config.rest.allowPublicRead}
                  disabled={Boolean(busy)}
                  onToggle={() => void saveREST({ allowPublicRead: !config.rest.allowPublicRead })}
                />
                <div className="px-3 pt-1 text-[10px] leading-relaxed text-slate-600">
                  Try <span className="font-mono">/{`{db}`}/{`{schema}`}/{`{table}`}?limit=10</span>. Views are
                  read-only; tables support POST, PATCH, and DELETE by primary key.
                </div>
              </div>
            </div>
          </div>

          <div className="card overflow-hidden">
            <div className="card-header">
              <div>
                <h2 className="flex items-center gap-2 text-sm font-semibold text-slate-100">
                  <Terminal size={15} className="text-mint-300" />
                  Copy-ready agent setup
                </h2>
                <p className="mt-0.5 text-[11px] text-slate-500">
                  Paste a config into your agent, or give it the prompt and let it discover everything.
                </p>
              </div>
              <span className="chip">
                <Sparkles size={11} />
                {config.ai.provider === 'ollama' ? 'Local AI available' : config.ai.hasApiKey ? 'AI ready' : 'AI key missing'}
              </span>
            </div>
            <div className="flex flex-wrap gap-1.5 border-b border-white/[0.06] px-4 py-2.5">
              {snippetTabs.map((entry) => (
                <button
                  key={entry.key}
                  type="button"
                  className={`btn-xs rounded-md px-2.5 py-1.5 text-[11px] font-medium transition-colors ${
                    tab === entry.key
                      ? 'bg-white/[0.08] text-slate-100'
                      : 'text-slate-500 hover:bg-white/[0.03] hover:text-slate-300'
                  }`}
                  onClick={() => setTab(entry.key)}
                >
                  {entry.label}
                </button>
              ))}
            </div>
            <div className="p-4">
              <CodeBlock
                code={snippets[tab]}
                language="text"
                filename={activeTab.file}
                onError={(message) => onToast('error', message)}
              />
              {tab === 'prompt' ? (
                <p className="mt-2 text-[10px] leading-relaxed text-slate-600">
                  The prompt contains the live MCP endpoint, token, REST key, JDBC credentials, and a
                  compact schema of every running database.
                </p>
              ) : null}
            </div>
          </div>
        </>
      ) : null}
    </div>
  );
}

function ToggleRow({
  icon,
  title,
  description,
  on,
  disabled,
  onToggle,
}: {
  icon: ReactNode;
  title: string;
  description: string;
  on: boolean;
  disabled?: boolean;
  onToggle: () => void;
}) {
  return (
    <div className="flex items-start justify-between gap-3 rounded-xl px-3 py-2.5 transition-colors hover:bg-white/[0.02]">
      <div className="flex min-w-0 items-start gap-2.5">
        <span className="mt-0.5 flex-none text-slate-500">{icon}</span>
        <div className="min-w-0">
          <div className="text-[12px] font-medium text-slate-200">{title}</div>
          <p className="mt-0.5 text-[10px] leading-relaxed text-slate-600">{description}</p>
        </div>
      </div>
      <button
        type="button"
        className="switch"
        data-on={on}
        onClick={onToggle}
        disabled={disabled}
        aria-label={title}
        aria-pressed={on}
      >
        <span className="switch-knob" />
      </button>
    </div>
  );
}
