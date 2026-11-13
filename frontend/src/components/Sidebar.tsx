import {
  Globe2,
  Network,
  Plus,
  RefreshCw,
  Share2,
} from 'lucide-react';
import type { Instance, SystemHealth } from '../types';
import { statusLabel } from '../lib/format';
import { statusDotClass } from './StatusBadge';
import logoMark from '../assets/logo-mark.png';

interface SidebarProps {
  instances: Instance[];
  selectedId: string | null;
  health: SystemHealth | null;
  refreshingTailscale: boolean;
  onSelect: (id: string) => void;
  onCreate: () => void;
  onRefreshTailscale: () => void;
}

export function Sidebar({
  instances,
  selectedId,
  health,
  refreshingTailscale,
  onSelect,
  onCreate,
  onRefreshTailscale,
}: SidebarProps) {
  const tailscale = health?.tailscale;
  const tailscaleRunning = Boolean(tailscale?.running);
  const tailscaleLabel = !tailscale
    ? 'Checking…'
    : !tailscale.installed
      ? 'Not installed'
      : tailscaleRunning
        ? 'Connected'
        : tailscale.backendState || 'Not connected';
  const tailscaleDot = tailscaleRunning
    ? 'bg-mint-400'
    : tailscale?.installed
      ? 'bg-amber-400'
      : 'bg-slate-600';

  return (
    <aside className="flex h-full w-[290px] flex-none flex-col border-r border-white/[0.06] bg-ink-900/55">
      <div className="flex h-[76px] items-center gap-3 border-b border-white/[0.06] px-5">
        <div className="flex h-10 w-10 flex-none overflow-hidden rounded-xl border border-mint-400/20 shadow-[0_0_24px_-8px_rgba(52,211,153,0.5)]">
          <img src={logoMark} alt="HySQL" className="h-full w-full object-cover" />
        </div>
        <div className="min-w-0">
          <div className="flex items-baseline gap-2">
            <span className="text-[15px] font-semibold tracking-tight text-slate-100">HySQL</span>
            <span className="text-[10px] font-medium text-slate-600">
              v{health?.appVersion ?? '0.1.0'}
            </span>
          </div>
          <p className="text-[11px] text-slate-500">H2 over Tailscale</p>
        </div>
      </div>

      <div className="px-4 pt-4">
        <button type="button" className="btn-primary w-full" onClick={onCreate}>
          <Plus size={16} />
          New database
        </button>
      </div>

      <div className="mt-6 flex min-h-0 flex-1 flex-col">
        <div className="flex items-center justify-between px-5 pb-2">
          <span className="label">Databases</span>
          <span className="rounded-full border border-white/[0.07] px-1.5 py-0.5 text-[10px] text-slate-500">
            {instances.length}
          </span>
        </div>
        <div className="min-h-0 flex-1 space-y-1 overflow-y-auto px-3 pb-4">
          {instances.length === 0 ? (
            <p className="px-2 py-3 text-xs leading-relaxed text-slate-600">
              No databases yet. Create one to start a local H2 TCP server.
            </p>
          ) : (
            instances.map((instance) => {
              const selected = instance.id === selectedId;
              return (
                <button
                  key={instance.id}
                  type="button"
                  aria-current={selected ? 'page' : undefined}
                  onClick={() => onSelect(instance.id)}
                  className={`group w-full rounded-xl border px-3 py-2.5 text-left transition-all ${
                    selected
                      ? 'border-mint-400/20 bg-mint-400/[0.07]'
                      : 'border-transparent hover:border-white/[0.06] hover:bg-white/[0.03]'
                  }`}
                >
                  <div className="flex items-center gap-2">
                    <span className={`h-1.5 w-1.5 flex-none rounded-full ${statusDotClass(instance.status)}`} />
                    <span
                      className={`min-w-0 flex-1 truncate text-[13px] font-medium ${
                        selected ? 'text-slate-100' : 'text-slate-300'
                      }`}
                    >
                      {instance.name}
                    </span>
                    {instance.tailscaleEnabled ? (
                      <Share2 size={12} className="flex-none text-mint-300/80" aria-label="Hosted on Tailscale" />
                    ) : null}
                    {instance.funnelEnabled ? (
                      <Globe2
                        size={12}
                        className="flex-none text-rose-300/80"
                        aria-label="Public on the internet via Tailscale Funnel"
                      />
                    ) : null}
                  </div>
                  <div className="mt-1 flex items-center gap-1.5 pl-3.5 text-[11px] text-slate-500">
                    <span className="font-mono">:{instance.port}</span>
                    <span className="text-slate-700">/</span>
                    <span>{statusLabel(instance.status)}</span>
                  </div>
                </button>
              );
            })
          )}
        </div>
      </div>

      <div className="border-t border-white/[0.06] p-4">
        <div className="rounded-xl border border-white/[0.06] bg-ink-950/40 px-3.5 py-3">
          <div className="flex items-center gap-2">
            <Network size={14} className="flex-none text-slate-500" />
            <span className="min-w-0 flex-1 text-xs font-medium text-slate-300">Tailscale</span>
            <span className={`h-1.5 w-1.5 flex-none rounded-full ${tailscaleDot}`} />
            <button
              type="button"
              className="icon-btn h-6 w-6"
              onClick={onRefreshTailscale}
              disabled={refreshingTailscale}
              title="Refresh Tailscale status"
              aria-label="Refresh Tailscale status"
            >
              <RefreshCw size={12} className={refreshingTailscale ? 'animate-spin' : ''} />
            </button>
          </div>
          <div className="mt-1.5 pl-[22px]">
            <div className="text-[11px] text-slate-400">{tailscaleLabel}</div>
            {tailscale?.running && tailscale.magicDns ? (
              <div className="mt-0.5 truncate font-mono text-[10px] text-slate-600" title={tailscale.magicDns}>
                {tailscale.magicDns}
              </div>
            ) : null}
            {!tailscaleRunning ? (
              <div className="mt-1 text-[10px] leading-relaxed text-slate-600">
                Local databases work without Tailscale.
              </div>
            ) : null}
          </div>
        </div>
        <p className="mt-3 px-1 text-[10px] leading-relaxed text-slate-700">
          Data is stored in <span className="font-mono text-slate-600">~/.hysql</span>
        </p>
      </div>
    </aside>
  );
}
