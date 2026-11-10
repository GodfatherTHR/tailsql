import {
  Activity,
  AlertTriangle,
  ArrowDownToLine,
  ArrowUpFromLine,
  Network,
  RefreshCw,
  Share2,
  ShieldOff,
  Users,
} from 'lucide-react';
import type { ReactNode } from 'react';
import type { Instance, ProxySnapshot, TailscaleStatus } from '../types';
import { formatBytes, formatNumber, formatTime } from '../lib/format';
import { CopyButton } from './CopyButton';
import { EndpointRow } from './EndpointRow';

interface TailscaleCardProps {
  instance: Instance;
  status: TailscaleStatus;
  proxy?: ProxySnapshot;
  busy: boolean;
  refreshing: boolean;
  onToggle: (enabled: boolean) => void;
  onRefresh: () => void;
  onCopyError: (message: string) => void;
}

export function TailscaleCard({
  instance,
  status,
  proxy,
  busy,
  refreshing,
  onToggle,
  onRefresh,
  onCopyError,
}: TailscaleCardProps) {
  const running = instance.status === 'running';
  const enabled = instance.tailscaleEnabled;
  const proxyRunning = Boolean(proxy?.running);
  const canToggle = status.running && running && !busy;
  const endpoint =
    instance.tailscaleUrl ||
    (instance.tailscaleIp && instance.tailscalePort
      ? `jdbc:h2:tcp://${instance.tailscaleIp}:${instance.tailscalePort}/./${instance.name}`
      : '');

  return (
    <div className="card overflow-hidden">
      <div className="card-header">
        <div className="flex min-w-0 items-center gap-3">
          <div className="flex h-9 w-9 flex-none items-center justify-center rounded-lg border border-white/[0.07] bg-white/[0.03]">
            <Network size={16} className="text-slate-300" />
          </div>
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <h2 className="text-sm font-semibold text-slate-100">Tailscale hosting</h2>
              {enabled && proxyRunning ? (
                <span className="inline-flex items-center gap-1 rounded-full border border-mint-400/20 bg-mint-400/[0.08] px-2 py-0.5 text-[10px] font-semibold text-mint-300">
                  <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-mint-400" />
                  Live
                </span>
              ) : null}
            </div>
            <p className="mt-0.5 text-[11px] text-slate-500">
              L4 proxy from your tailnet to <span className="font-mono">127.0.0.1:{instance.port}</span>
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <button
            type="button"
            className="icon-btn"
            onClick={onRefresh}
            disabled={refreshing}
            title="Refresh Tailscale status"
            aria-label="Refresh Tailscale status"
          >
            <RefreshCw size={14} className={refreshing ? 'animate-spin' : ''} />
          </button>
          <button
            type="button"
            role="switch"
            aria-checked={enabled}
            aria-label="Host this database on Tailscale"
            className="switch"
            data-on={enabled ? 'true' : 'false'}
            disabled={!canToggle}
            onClick={() => onToggle(!enabled)}
          >
            <span className="switch-knob" />
          </button>
        </div>
      </div>

      <div className="space-y-4 p-5">
        <div className="grid gap-2 sm:grid-cols-3">
          <StatusTile
            label="Tailscale state"
            value={
              <span className="flex items-center gap-1.5">
                <span
                  className={`h-1.5 w-1.5 rounded-full ${
                    status.running ? 'bg-mint-400' : status.installed ? 'bg-amber-400' : 'bg-slate-600'
                  }`}
                />
                {status.running ? 'Connected' : status.installed ? status.backendState || 'Not connected' : 'Not installed'}
              </span>
            }
          />
          <StatusTile
            label="Tailnet IPv4"
            value={status.ipv4 || '—'}
            action={status.ipv4 ? <CopyButton value={status.ipv4} iconOnly onError={onCopyError} /> : null}
          />
          <StatusTile
            label="MagicDNS"
            value={status.magicDns || '—'}
            mono
            action={status.magicDns ? <CopyButton value={status.magicDns} iconOnly onError={onCopyError} /> : null}
          />
        </div>

        {!status.installed ? (
          <Notice tone="warning" icon={<ShieldOff size={15} />}>
            The Tailscale CLI was not found. Install Tailscale, sign in, then refresh. Local databases
            remain fully functional.
          </Notice>
        ) : !status.running ? (
          <Notice tone="warning" icon={<ShieldOff size={15} />}>
            {status.error || 'Tailscale is not connected. Sign in to host this database on your tailnet.'}
          </Notice>
        ) : !running ? (
          <Notice tone="info" icon={<AlertTriangle size={15} />}>
            Start the database before enabling Tailscale hosting.
          </Notice>
        ) : null}

        {enabled && proxyRunning ? (
          <>
            <EndpointRow
              label="Tailscale JDBC endpoint"
              value={endpoint}
              hint={
                status.magicDns
                  ? `MagicDNS: ${status.magicDns}:${proxy?.port ?? instance.tailscalePort}`
                  : 'Visible to devices allowed by your Tailscale ACLs.'
              }
              onError={onCopyError}
            />

            <div className="grid grid-cols-2 gap-2 lg:grid-cols-4">
              <Metric icon={<Users size={13} />} label="Active clients" value={formatNumber(proxy?.activeClients ?? 0)} />
              <Metric icon={<Activity size={13} />} label="Total connections" value={formatNumber(proxy?.totalConnections ?? 0)} />
              <Metric icon={<ArrowDownToLine size={13} />} label="Data to H2" value={formatBytes(proxy?.bytesIn ?? 0)} />
              <Metric icon={<ArrowUpFromLine size={13} />} label="Data to peers" value={formatBytes(proxy?.bytesOut ?? 0)} />
            </div>

            {proxy?.lastError ? (
              <Notice tone="warning" icon={<AlertTriangle size={15} />}>
                {proxy.lastError}
              </Notice>
            ) : null}

            <div className="overflow-hidden rounded-xl border border-white/[0.06]">
              <div className="flex items-center justify-between border-b border-white/[0.06] bg-white/[0.015] px-3.5 py-2">
                <span className="text-[11px] font-semibold text-slate-400">Connected peers</span>
                <span className="text-[10px] text-slate-600">
                  {proxy?.bindAddress || `${status.ipv4}:${proxy?.port ?? instance.tailscalePort}`}
                </span>
              </div>
              {proxy && proxy.clients.length > 0 ? (
                <div className="overflow-x-auto">
                  <table className="w-full min-w-[520px]">
                    <thead>
                      <tr>
                        <th className="table-head">Peer</th>
                        <th className="table-head">Connected</th>
                        <th className="table-head text-right">In</th>
                        <th className="table-head text-right">Out</th>
                      </tr>
                    </thead>
                    <tbody>
                      {proxy.clients.map((client) => (
                        <tr key={client.id} className="transition-colors hover:bg-white/[0.02]">
                          <td className="table-cell">{client.peerIp || 'unknown'}</td>
                          <td className="table-cell text-slate-500">{formatTime(client.connectedAt)}</td>
                          <td className="table-cell text-right text-slate-400">{formatBytes(client.bytesIn)}</td>
                          <td className="table-cell text-right text-slate-400">{formatBytes(client.bytesOut)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ) : (
                <p className="px-3.5 py-4 text-[11px] leading-relaxed text-slate-600">
                  No clients connected. Share the Tailscale JDBC URL with a teammate on your tailnet.
                </p>
              )}
            </div>
          </>
        ) : enabled ? (
          <Notice tone="warning" icon={<AlertTriangle size={15} />}>
            Tailscale hosting is enabled in state, but the proxy is not running. Toggle hosting off
            and on again to rebind it.
          </Notice>
        ) : (
          <div className="flex items-start gap-3 rounded-xl border border-dashed border-white/10 bg-ink-950/30 px-4 py-4">
            <Share2 size={17} className="mt-0.5 flex-none text-slate-500" />
            <div>
              <div className="text-xs font-semibold text-slate-300">Private to this machine</div>
              <p className="mt-1 text-[11px] leading-relaxed text-slate-500">
                H2 listens only on <span className="font-mono">127.0.0.1</span>. Flip the switch to
                bind a proxy on <span className="font-mono">{status.ipv4 || '100.x.y.z'}</span> and
                share the generated JDBC URL.
              </p>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

function StatusTile({
  label,
  value,
  action,
  mono = false,
}: {
  label: string;
  value: ReactNode;
  action?: ReactNode;
  mono?: boolean;
}) {
  return (
    <div className="flex items-center gap-2 rounded-xl border border-white/[0.06] bg-ink-950/40 px-3 py-2.5">
      <div className="min-w-0 flex-1">
        <div className="text-[10px] font-semibold uppercase tracking-wider text-slate-600">{label}</div>
        <div className={`mt-0.5 truncate text-xs text-slate-300 ${mono ? 'font-mono' : ''}`}>{value}</div>
      </div>
      {action}
    </div>
  );
}

function Metric({ icon, label, value }: { icon: ReactNode; label: string; value: string }) {
  return (
    <div className="rounded-xl border border-white/[0.06] bg-ink-950/40 px-3 py-2.5">
      <div className="flex items-center gap-1.5 text-[10px] font-medium uppercase tracking-wider text-slate-600">
        {icon}
        {label}
      </div>
      <div className="mt-1 font-mono text-sm text-slate-200">{value}</div>
    </div>
  );
}

function Notice({
  tone,
  icon,
  children,
}: {
  tone: 'warning' | 'info';
  icon: ReactNode;
  children: ReactNode;
}) {
  const classes =
    tone === 'warning'
      ? 'border-amber-400/20 bg-amber-400/[0.05] text-amber-200/90'
      : 'border-sky-400/15 bg-sky-400/[0.04] text-slate-400';
  return (
    <div className={`flex gap-2.5 rounded-xl border px-3.5 py-3 text-[11px] leading-relaxed ${classes}`}>
      <span className="mt-0.5 flex-none">{icon}</span>
      <span>{children}</span>
    </div>
  );
}
