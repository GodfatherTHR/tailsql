import {
  AlertTriangle,
  Clock,
  Code2,
  Database,
  Eye,
  EyeOff,
  HardDrive,
  History,
  KeyRound,
  Loader2,
  Lock,
  Play,
  RotateCw,
  Server,
  Square,
  Trash2,
  User,
} from 'lucide-react';
import { useEffect, useState, type ReactNode } from 'react';
import type { FunnelState, Instance, ProxySnapshot, SystemHealth } from '../types';
import {
  formatBytes,
  formatDateTime,
  formatUptime,
  statusLabel,
} from '../lib/format';
import type { ToastKind } from '../hooks/useToasts';
import { CheckpointsDrawer } from './CheckpointsDrawer';
import { CopyButton } from './CopyButton';
import { EndpointRow } from './EndpointRow';
import { FunnelCard } from './FunnelCard';
import { RestApiCard } from './RestApiCard';
import { ServerLogPanel } from './ServerLogPanel';
import { StatusBadge } from './StatusBadge';
import { TailscaleCard } from './TailscaleCard';

interface OverviewViewProps {
  instance: Instance;
  proxy?: ProxySnapshot;
  funnel: FunnelState | null;
  health: SystemHealth;
  busy: string | null;
  refreshingTailscale: boolean;
  funnelBusy: boolean;
  funnelLoading: boolean;
  onStart: () => void;
  onStop: () => void;
  onRestart: () => void;
  onDelete: () => void;
  onToggleHosting: (enabled: boolean) => void;
  onEnableFunnel: (port: number) => void;
  onDisableFunnel: () => void;
  onRefreshFunnel: () => void;
  onRefreshTailscale: () => void;
  onOpenJava: () => void;
  onCopyError: (message: string) => void;
  onToast: (kind: ToastKind, message: string) => void;
}

export function OverviewView({
  instance,
  proxy,
  funnel,
  health,
  busy,
  refreshingTailscale,
  funnelBusy,
  funnelLoading,
  onStart,
  onStop,
  onRestart,
  onDelete,
  onToggleHosting,
  onEnableFunnel,
  onDisableFunnel,
  onRefreshFunnel,
  onRefreshTailscale,
  onOpenJava,
  onCopyError,
  onToast,
}: OverviewViewProps) {
  const [now, setNow] = useState(Date.now());
  const [revealPassword, setRevealPassword] = useState(false);
  const [checkpointsOpen, setCheckpointsOpen] = useState(false);
  const running = instance.status === 'running';
  const transient = instance.status === 'starting' || instance.status === 'stopping';
  const actionsDisabled = Boolean(busy) || transient;

  useEffect(() => {
    setRevealPassword(false);
  }, [instance.id]);

  useEffect(() => {
    if (!running) {
      return;
    }
    setNow(Date.now());
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [running, instance.startedAt]);

  return (
    <div className="space-y-5 animate-fade-in">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="flex min-w-0 items-start gap-4">
          <div className="flex h-12 w-12 flex-none items-center justify-center rounded-xl border border-white/[0.07] bg-gradient-to-br from-white/[0.06] to-transparent">
            <Database size={22} className="text-mint-300" />
          </div>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2.5">
              <h1 className="truncate text-xl font-semibold tracking-tight text-slate-100">
                {instance.name}
              </h1>
              <StatusBadge status={instance.status} />
              {busy ? (
                <span className="chip border-mint-400/20 text-mint-300">
                  <Loader2 size={11} className="animate-spin" />
                  {busy}
                </span>
              ) : null}
            </div>
            <p className="mt-1 text-xs text-slate-500">
              <span className="font-mono text-slate-400">H2 TCP</span>
              <span className="mx-1.5 text-slate-700">/</span>
              {instance.databaseFile}
              <span className="mx-1.5 text-slate-700">/</span>
              created {formatDateTime(instance.createdAt)}
            </p>
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          {running ? (
            <>
              <button type="button" className="btn-secondary" onClick={onStop} disabled={actionsDisabled}>
                {instance.status === 'stopping' ? (
                  <Loader2 size={14} className="animate-spin" />
                ) : (
                  <Square size={14} />
                )}
                Stop
              </button>
              <button type="button" className="btn-secondary" onClick={onRestart} disabled={actionsDisabled}>
                <RotateCw size={14} />
                Restart
              </button>
            </>
          ) : (
            <button type="button" className="btn-secondary" onClick={onStart} disabled={actionsDisabled}>
              {instance.status === 'starting' ? (
                <Loader2 size={14} className="animate-spin" />
              ) : (
                <Play size={14} />
              )}
              {instance.status === 'starting' ? 'Starting…' : 'Start'}
            </button>
          )}
          <button
            type="button"
            className="btn-secondary"
            onClick={() => setCheckpointsOpen(true)}
            disabled={actionsDisabled}
          >
            <History size={14} />
            Checkpoints
          </button>
          <button type="button" className="btn-primary" onClick={onOpenJava}>
            <Code2 size={14} />
            Connect to Java
          </button>
          <button
            type="button"
            className="icon-btn hover:bg-rose-500/10 hover:text-rose-300"
            onClick={onDelete}
            disabled={actionsDisabled}
            title="Remove database"
            aria-label="Remove database"
          >
            <Trash2 size={15} />
          </button>
        </div>
      </div>

      {instance.lastError ? (
        <div className="flex gap-3 rounded-xl border border-rose-500/20 bg-rose-500/[0.05] px-4 py-3.5">
          <AlertTriangle size={17} className="mt-0.5 flex-none text-rose-300" />
          <div className="min-w-0">
            <div className="text-xs font-semibold text-rose-200">Last error</div>
            <p className="mt-1 whitespace-pre-wrap break-words text-[11px] leading-relaxed text-rose-200/70">
              {instance.lastError}
            </p>
          </div>
        </div>
      ) : null}

      <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard
          icon={<Server size={14} />}
          label="Status"
          value={statusLabel(instance.status)}
          hint={running ? 'Accepting JDBC connections' : 'No active listener'}
        />
        <StatCard
          icon={<HardDrive size={14} />}
          label="Local port"
          value={instance.port.toString()}
          hint="Bound to 127.0.0.1"
          mono
        />
        <StatCard
          icon={<Database size={14} />}
          label="Database file"
          value={formatBytes(instance.fileSizeBytes)}
          hint={instance.databaseFile}
        />
        <StatCard
          icon={<Clock size={14} />}
          label="Uptime"
          value={running ? formatUptime(instance.startedAt, now) : '—'}
          hint={running ? `PID ${instance.pid}` : 'Stopped'}
          mono={running}
        />
      </div>

      <TailscaleCard
        instance={instance}
        status={health.tailscale}
        proxy={proxy}
        busy={Boolean(busy) || transient}
        refreshing={refreshingTailscale}
        onToggle={onToggleHosting}
        onRefresh={onRefreshTailscale}
        onCopyError={onCopyError}
      />

      <FunnelCard
        instance={instance}
        state={funnel}
        status={health.tailscale}
        busy={funnelBusy || Boolean(busy) || transient}
        loading={funnelLoading}
        onEnable={onEnableFunnel}
        onDisable={onDisableFunnel}
        onRefresh={onRefreshFunnel}
        onCopyError={onCopyError}
      />

      <RestApiCard onToast={onToast} />

      <div className="grid gap-5 xl:grid-cols-2">
        <div className="card overflow-hidden">
          <div className="card-header">
            <div>
              <h2 className="text-sm font-semibold text-slate-100">Connection</h2>
              <p className="mt-0.5 text-[11px] text-slate-500">
                Loopback by default, tailnet on demand.
              </p>
            </div>
            <span className="chip">
              <Lock size={11} />
              JDBC
            </span>
          </div>
          <div className="space-y-3 p-5">
            <EndpointRow
              label="Local JDBC URL"
              value={instance.jdbcUrl}
              hint="Works only on this machine."
              onError={onCopyError}
            />
            {instance.tailscaleUrl ? (
              <EndpointRow
                label="Tailscale JDBC URL"
                value={instance.tailscaleUrl}
                hint="Share with devices allowed by your Tailscale ACLs."
                onError={onCopyError}
              />
            ) : null}

            <div className="overflow-hidden rounded-xl border border-white/[0.06]">
              <div className="grid grid-cols-2 divide-x divide-white/[0.06]">
                <div className="p-3.5">
                  <div className="mb-1 flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wider text-slate-600">
                    <User size={11} />
                    Username
                  </div>
                  <div className="flex items-center justify-between gap-2">
                    <span className="truncate font-mono text-xs text-slate-200">{instance.username}</span>
                    <CopyButton value={instance.username} iconOnly onError={onCopyError} />
                  </div>
                </div>
                <div className="p-3.5">
                  <div className="mb-1 flex items-center justify-between gap-1">
                    <span className="flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wider text-slate-600">
                      <KeyRound size={11} />
                      Password
                    </span>
                    <button
                      type="button"
                      className="icon-btn h-6 w-6"
                      onClick={() => setRevealPassword((current) => !current)}
                      title={revealPassword ? 'Hide password' : 'Reveal password'}
                      aria-label={revealPassword ? 'Hide password' : 'Reveal password'}
                    >
                      {revealPassword ? <EyeOff size={12} /> : <Eye size={12} />}
                    </button>
                  </div>
                  <div className="flex items-center justify-between gap-2">
                    <span className="truncate font-mono text-xs text-slate-200">
                      {revealPassword ? instance.password : '••••••••••••'}
                    </span>
                    <CopyButton value={instance.password} iconOnly onError={onCopyError} />
                  </div>
                </div>
              </div>
              <div className="border-t border-white/[0.06] bg-ink-950/30 px-3.5 py-2 text-[10px] text-slate-600">
                Use these credentials in the generated Java code.
              </div>
            </div>

            <button type="button" className="btn-secondary w-full" onClick={onOpenJava}>
              <Code2 size={14} />
              Open Java connectivity
            </button>
          </div>
        </div>

        <div className="space-y-5">
          <div className="card overflow-hidden">
            <div className="card-header">
              <div>
                <h2 className="text-sm font-semibold text-slate-100">Runtime</h2>
                <p className="mt-0.5 text-[11px] text-slate-500">Process and storage details.</p>
              </div>
              <span className={`chip ${running ? 'border-mint-400/20 text-mint-300' : ''}`}>
                {running ? 'Online' : 'Offline'}
              </span>
            </div>
            <div className="divide-y divide-white/[0.04] px-5 py-1">
              <DetailRow label="PID" value={instance.pid > 0 ? String(instance.pid) : '—'} mono />
              <DetailRow label="Started" value={formatDateTime(instance.startedAt)} />
              <DetailRow label="Updated" value={formatDateTime(instance.updatedAt)} />
              <DetailRow label="Database file" value={instance.databaseFile} mono />
              <DetailRow label="File size" value={formatBytes(instance.fileSizeBytes)} mono />
              <DetailRow label="Data directory" value={health.dataDir || '—'} mono title={health.dataDir} />
            </div>
          </div>
          <ServerLogPanel instance={instance} />
        </div>
      </div>

      <CheckpointsDrawer
        open={checkpointsOpen}
        instance={instance}
        onClose={() => setCheckpointsOpen(false)}
        onToast={onToast}
      />
    </div>
  );
}

function StatCard({
  icon,
  label,
  value,
  hint,
  mono = false,
}: {
  icon: ReactNode;
  label: string;
  value: string;
  hint: string;
  mono?: boolean;
}) {
  return (
    <div className="rounded-xl border border-white/[0.06] bg-ink-900/55 px-4 py-3.5">
      <div className="flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wider text-slate-600">
        {icon}
        {label}
      </div>
      <div className={`mt-1.5 truncate text-lg font-semibold text-slate-100 ${mono ? 'font-mono text-base' : ''}`}>
        {value}
      </div>
      <div className="mt-0.5 truncate text-[10px] text-slate-600" title={hint}>
        {hint}
      </div>
    </div>
  );
}

function DetailRow({
  label,
  value,
  mono = false,
  title,
}: {
  label: string;
  value: string;
  mono?: boolean;
  title?: string;
}) {
  return (
    <div className="flex items-start justify-between gap-4 py-2.5">
      <span className="text-[11px] text-slate-500">{label}</span>
      <span
        className={`max-w-[62%] truncate text-right text-[11px] text-slate-300 ${mono ? 'font-mono' : ''}`}
        title={title ?? value}
      >
        {value}
      </span>
    </div>
  );
}
