import {
  AlertTriangle,
  Globe2,
  Loader2,
  Power,
  RefreshCw,
  ShieldAlert,
} from 'lucide-react';
import { useEffect, useState, type ReactNode } from 'react';
import type { FunnelState, Instance, TailscaleStatus } from '../types';
import { formatTime } from '../lib/format';
import { CopyButton } from './CopyButton';
import { EndpointRow } from './EndpointRow';

interface FunnelCardProps {
  instance: Instance;
  state: FunnelState | null;
  status: TailscaleStatus;
  busy: boolean;
  loading: boolean;
  onEnable: (port: number) => void;
  onDisable: () => void;
  onRefresh: () => void;
  onCopyError: (message: string) => void;
}

const fallbackPorts = [443, 8443, 10000];

export function FunnelCard({
  instance,
  state,
  status,
  busy,
  loading,
  onEnable,
  onDisable,
  onRefresh,
  onCopyError,
}: FunnelCardProps) {
  const running = instance.status === 'running';
  const enabled = Boolean(state?.enabled || instance.funnelEnabled);
  const allowedPorts = state?.allowedPorts?.length ? state.allowedPorts : fallbackPorts;
  const activePort = state?.port || instance.funnelPort || 10000;
  const [selectedPort, setSelectedPort] = useState(activePort);
  const endpoint = state?.jdbcUrl || instance.funnelJdbcUrl;
  const publicEndpoint = state?.publicUrl || instance.funnelUrl;
  const host = state?.host || instance.funnelHost;
  const canToggle = status.running && running && !busy && !loading;

  useEffect(() => {
    setSelectedPort(activePort);
  }, [activePort]);

  return (
    <div className="card overflow-hidden">
      <div className="card-header">
        <div className="flex min-w-0 items-center gap-3">
          <div className="flex h-9 w-9 flex-none items-center justify-center rounded-lg border border-rose-400/20 bg-rose-400/[0.05]">
            <Globe2 size={16} className="text-rose-300" />
          </div>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="text-sm font-semibold text-slate-100">Public internet (Funnel)</h2>
              {enabled ? (
                <span className="inline-flex items-center gap-1 rounded-full border border-rose-400/25 bg-rose-400/[0.08] px-2 py-0.5 text-[10px] font-semibold text-rose-300">
                  <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-rose-400" />
                  Public
                </span>
              ) : null}
            </div>
            <p className="mt-0.5 text-[11px] text-slate-500">
              Tailscale Funnel exposes the H2 TCP port to the entire internet.
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <button
            type="button"
            className="icon-btn"
            onClick={onRefresh}
            disabled={loading || busy}
            title="Refresh Funnel status"
            aria-label="Refresh Funnel status"
          >
            <RefreshCw size={14} className={loading ? 'animate-spin' : ''} />
          </button>
          <button
            type="button"
            role="switch"
            aria-checked={enabled}
            aria-label="Publish this database on the public internet"
            className="switch"
            data-on={enabled ? 'true' : 'false'}
            disabled={!canToggle}
            onClick={() => {
              if (enabled) {
                onDisable();
              } else {
                onEnable(selectedPort);
              }
            }}
          >
            <span className="switch-knob" />
          </button>
        </div>
      </div>

      <div className="space-y-4 p-5">
        <Notice tone="danger" icon={<ShieldAlert size={15} />}>
          <strong className="font-semibold text-rose-200">Public exposure.</strong> Anyone on the
          internet can reach this H2 TCP port. The database username and password are the only
          access control, and raw Funnel TCP is not encrypted in transit. Use a strong password and
          turn Funnel off when you are done.
        </Notice>

        {!status.installed ? (
          <Notice tone="warning" icon={<AlertTriangle size={15} />}>
            The Tailscale CLI was not found. Install Tailscale and sign in to use Funnel.
          </Notice>
        ) : !status.running ? (
          <Notice tone="warning" icon={<AlertTriangle size={15} />}>
            {status.error || 'Tailscale is not connected. Connect before publishing this database.'}
          </Notice>
        ) : !running ? (
          <Notice tone="warning" icon={<AlertTriangle size={15} />}>
            Start the database before enabling Funnel.
          </Notice>
        ) : null}

        {state?.error ? (
          <Notice tone="danger" icon={<AlertTriangle size={15} />}>
            {state.error}
          </Notice>
        ) : null}

        {enabled ? (
          <>
            <EndpointRow
              label="Public JDBC endpoint"
              value={endpoint || 'Waiting for the Funnel mapping…'}
              hint={
                publicEndpoint
                  ? `Raw TCP: ${publicEndpoint} — reachable from anywhere on the internet.`
                  : 'Tailscale has not reported the public mapping yet. Click refresh.'
              }
              onError={onCopyError}
            />
            <div className="grid gap-2 sm:grid-cols-3">
              <StatusTile label="Public host" value={host || '—'} mono onCopyError={onCopyError} />
              <StatusTile
                label="Funnel port"
                value={String(activePort || '—')}
                mono
                onCopyError={onCopyError}
              />
              <StatusTile
                label="Access"
                value={
                  <span className="flex items-center gap-1.5 text-rose-300">
                    <span className="h-1.5 w-1.5 rounded-full bg-rose-400" />
                    Entire internet
                  </span>
                }
              />
            </div>
            <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-rose-400/15 bg-rose-400/[0.04] px-4 py-3">
              <div>
                <div className="text-xs font-semibold text-slate-200">Funnel is live</div>
                <p className="mt-0.5 text-[11px] text-slate-500">
                  Disable it to remove the public mapping immediately.
                </p>
              </div>
              <button
                type="button"
                className="btn-danger btn-xs"
                onClick={onDisable}
                disabled={busy}
              >
                {busy ? <Loader2 size={12} className="animate-spin" /> : <Power size={12} />}
                Disable public access
              </button>
            </div>
            {state?.checkedAt ? (
              <p className="text-[10px] text-slate-600">Checked {formatTime(state.checkedAt)}</p>
            ) : null}
          </>
        ) : (
          <>
            <div className="grid gap-3 sm:grid-cols-[180px_minmax(0,1fr)] sm:items-end">
              <div>
                <label className="label mb-1.5 block" htmlFor="funnel-port">
                  Public Funnel port
                </label>
                <select
                  id="funnel-port"
                  className="input font-mono text-xs"
                  value={selectedPort}
                  onChange={(event) => setSelectedPort(Number(event.target.value))}
                  disabled={busy || !canToggle}
                >
                  {allowedPorts.map((port) => (
                    <option key={port} value={port}>
                      {port} {port === 10000 ? '(recommended)' : ''}
                    </option>
                  ))}
                </select>
              </div>
              <button
                type="button"
                className="btn-primary"
                onClick={() => onEnable(selectedPort)}
                disabled={!canToggle}
              >
                {busy ? <Loader2 size={14} className="animate-spin" /> : <Globe2 size={14} />}
                Publish database
              </button>
            </div>
            <p className="text-[10px] leading-relaxed text-slate-600">
              Tailscale Funnel only allows ports 443, 8443, and 10000. Ports 443 and 8443 are often
              already used by HTTPS services, so 10000 is the safe default.
            </p>
            <div className="flex items-start gap-3 rounded-xl border border-dashed border-white/10 bg-ink-950/30 px-4 py-4">
              <Globe2 size={17} className="mt-0.5 flex-none text-slate-500" />
              <div>
                <div className="text-xs font-semibold text-slate-300">Currently private</div>
                <p className="mt-1 text-[11px] leading-relaxed text-slate-500">
                  H2 stays on <span className="font-mono">127.0.0.1</span>. Enabling Funnel makes
                  Tailscale create a public TLS-terminated TCP mapping from{' '}
                  <span className="font-mono">{status.magicDns || 'your-node.ts.net'}</span> to this
                  database.
                </p>
              </div>
            </div>
          </>
        )}
      </div>
    </div>
  );
}

function StatusTile({
  label,
  value,
  mono = false,
  onCopyError,
}: {
  label: string;
  value: ReactNode;
  mono?: boolean;
  onCopyError?: (message: string) => void;
}) {
  return (
    <div className="flex items-center gap-2 rounded-xl border border-white/[0.06] bg-ink-950/40 px-3 py-2.5">
      <div className="min-w-0 flex-1">
        <div className="text-[10px] font-semibold uppercase tracking-wider text-slate-600">
          {label}
        </div>
        <div className={`mt-0.5 truncate text-xs text-slate-300 ${mono ? 'font-mono' : ''}`}>
          {value}
        </div>
      </div>
      {typeof value === 'string' && value !== '—' && onCopyError ? (
        <CopyButton value={value} iconOnly onError={onCopyError} />
      ) : null}
    </div>
  );
}

function Notice({
  tone,
  icon,
  children,
}: {
  tone: 'warning' | 'danger';
  icon: ReactNode;
  children: ReactNode;
}) {
  const classes =
    tone === 'danger'
      ? 'border-rose-500/20 bg-rose-500/[0.05] text-rose-200/80'
      : 'border-amber-400/20 bg-amber-400/[0.05] text-amber-200/90';
  return (
    <div className={`flex gap-2.5 rounded-xl border px-3.5 py-3 text-[11px] leading-relaxed ${classes}`}>
      <span className="mt-0.5 flex-none">{icon}</span>
      <span>{children}</span>
    </div>
  );
}
