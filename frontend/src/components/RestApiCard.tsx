import { Globe2, Loader2, RefreshCw, Server } from 'lucide-react';
import { useEffect, useState } from 'react';
import type { AgentConfig, RESTConfigRequest } from '../types';
import { api, errorMessage } from '../lib/api';
import type { ToastKind } from '../hooks/useToasts';
import { CopyButton } from './CopyButton';

interface RestApiCardProps {
  onToast: (kind: ToastKind, message: string) => void;
}

export function RestApiCard({ onToast }: RestApiCardProps) {
  const [config, setConfig] = useState<AgentConfig | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  const load = async () => {
    try {
      setConfig(await api.getAgentConfig());
      setError('');
    } catch (loadError) {
      setError(errorMessage(loadError));
    }
  };

  useEffect(() => {
    void load();
  }, []);

  const update = async (overrides: Partial<RESTConfigRequest>) => {
    if (!config) {
      return;
    }
    setBusy(true);
    setError('');
    try {
      const next = await api.setRESTConfig({
        enabled: config.rest.enabled,
        port: config.rest.port,
        tailscaleEnabled: config.rest.tailscaleEnabled,
        funnelEnabled: config.rest.funnelEnabled,
        funnelPort: config.rest.funnelPort,
        allowPublicRead: config.rest.allowPublicRead,
        ...overrides,
      });
      setConfig(next);
      onToast('success', 'Auto-REST updated.');
    } catch (updateError) {
      setError(errorMessage(updateError));
      onToast('error', errorMessage(updateError));
    } finally {
      setBusy(false);
    }
  };

  if (!config) {
    return (
      <div className="card flex items-center gap-2 px-5 py-4 text-[11px] text-slate-500">
        {error ? error : <><Loader2 size={13} className="animate-spin" /> Loading REST gateway…</>}
      </div>
    );
  }

  const rest = config.rest;
  const base = rest.publicBaseUrl || rest.baseUrl || `http://127.0.0.1:${rest.port}`;

  return (
    <div className="card overflow-hidden">
      <div className="card-header">
        <div>
          <h2 className="flex items-center gap-2 text-sm font-semibold text-slate-100">
            <Server size={15} className="text-mint-300" />
            Auto-REST gateway
          </h2>
          <p className="mt-0.5 text-[11px] text-slate-500">
            JSON CRUD for tables and views, generated from live H2 metadata.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <span
            className={`chip ${rest.running ? 'border-mint-400/20 text-mint-300' : 'border-amber-400/20 text-amber-300'}`}
          >
            {rest.running ? 'Running' : 'Stopped'}
          </span>
          <button
            type="button"
            className="icon-btn"
            onClick={() => void load()}
            title="Refresh REST state"
            aria-label="Refresh REST state"
          >
            <RefreshCw size={13} />
          </button>
        </div>
      </div>
      <div className="space-y-3 p-5">
        <div className="flex items-center justify-between gap-3 rounded-xl border border-white/[0.06] bg-ink-950/40 px-3.5 py-2.5">
          <div className="min-w-0">
            <div className="text-[10px] font-semibold uppercase tracking-wider text-slate-600">
              Base URL
            </div>
            <code className="mt-0.5 block truncate font-mono text-[11px] text-slate-300" title={base}>
              {base}/api/v1/&lt;database&gt;/&lt;schema&gt;/&lt;table&gt;
            </code>
          </div>
          <CopyButton value={`${base}/api/v1`} iconOnly onError={(message) => onToast('error', message)} />
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <button
            type="button"
            className={rest.enabled ? 'btn-primary btn-xs' : 'btn-secondary btn-xs'}
            onClick={() => void update({ enabled: !rest.enabled })}
            disabled={busy}
          >
            {rest.enabled ? 'Disable gateway' : 'Enable gateway'}
          </button>
          <button
            type="button"
            className={`btn-xs ${rest.tailscaleEnabled ? 'btn-primary' : 'btn-secondary'}`}
            onClick={() => void update({ tailscaleEnabled: !rest.tailscaleEnabled })}
            disabled={busy || !config.tailscaleAvailable || rest.funnelEnabled}
            title={
              config.tailscaleAvailable
                ? 'Bind the Tailscale IPv4 and require the API key'
                : 'Tailscale is not connected'
            }
          >
            <Globe2 size={11} />
            {rest.tailscaleEnabled ? 'Tailnet on' : 'Expose on Tailnet'}
          </button>
          <button
            type="button"
            className={`btn-xs ${rest.funnelEnabled ? 'btn-primary' : 'btn-secondary'}`}
            onClick={() => void update({ funnelEnabled: !rest.funnelEnabled })}
            disabled={busy || !config.tailscaleAvailable}
            title="Publish the REST API over public HTTPS Funnel"
          >
            <Globe2 size={11} />
            {rest.funnelEnabled ? 'Funnel on' : 'Publish via Funnel'}
          </button>
        </div>

        {error ? <p className="text-[10px] leading-relaxed text-rose-300">{error}</p> : null}
        {rest.error ? <p className="text-[10px] leading-relaxed text-amber-300/80">{rest.error}</p> : null}
      </div>
    </div>
  );
}
