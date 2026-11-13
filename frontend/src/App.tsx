import {
  AlertTriangle,
  Bot,
  Code2,
  Database,
  Globe2,
  LayoutDashboard,
  Loader2,
  ShieldCheck,
  Table2,
  Terminal,
} from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import type {
  AppErrorEvent,
  CreateInstanceRequest,
  FunnelState,
  Instance,
  ProxySnapshot,
  RealtimeChangeEvent,
  SystemHealth,
  TableInfo,
  TailscaleStatus,
  ViewKey,
} from './types';
import { api, errorMessage } from './lib/api';
import { formatBytes } from './lib/format';
import { EventsOff, EventsOn } from '../wailsjs/runtime/runtime';
import { useToasts } from './hooks/useToasts';
import { AgentConnectView } from './components/AgentConnectView';
import { AISettingsModal } from './components/AISettingsModal';
import { ConfirmDialog } from './components/ConfirmDialog';
import { CreateDatabaseModal } from './components/CreateDatabaseModal';
import { EmptyState } from './components/EmptyState';
import { HealthBanner } from './components/HealthBanner';
import { JavaConnectView } from './components/JavaConnectView';
import { OverviewView } from './components/OverviewView';
import { Sidebar } from './components/Sidebar';
import { SqlWorkbenchView } from './components/SqlWorkbenchView';
import { StatusBadge } from './components/StatusBadge';
import { TableEditorView } from './components/TableEditorView';
import { ToastViewport } from './components/ToastViewport';

const navItems = [
  { key: 'overview', label: 'Overview', icon: LayoutDashboard },
  { key: 'table', label: 'Table Editor', icon: Table2 },
  { key: 'java', label: 'Connect to Java', icon: Code2 },
  { key: 'sql', label: 'SQL Workbench', icon: Terminal },
  { key: 'ai', label: 'Connect to AI', icon: Bot },
] as const;

export default function App() {
  const { toasts, push, dismiss } = useToasts();
  const [health, setHealth] = useState<SystemHealth | null>(null);
  const [instances, setInstances] = useState<Instance[]>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [view, setView] = useState<ViewKey>('overview');
  const [proxies, setProxies] = useState<Record<string, ProxySnapshot>>({});
  const [funnels, setFunnels] = useState<Record<string, FunnelState>>({});
  const [realtimeEvent, setRealtimeEvent] = useState<RealtimeChangeEvent | null>(null);
  const [funnelLoading, setFunnelLoading] = useState(false);
  const [funnelBusy, setFunnelBusy] = useState(false);
  const [funnelToken, setFunnelToken] = useState(0);
  const [tableTarget, setTableTarget] = useState<{ schema: string; name: string } | null>(null);
  const [loading, setLoading] = useState(true);
  const [fatal, setFatal] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);

  const [createOpen, setCreateOpen] = useState(false);
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState('');
  const [deleteTarget, setDeleteTarget] = useState<Instance | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [aiSettingsOpen, setAiSettingsOpen] = useState(false);

  const [refreshingTailscale, setRefreshingTailscale] = useState(false);
  const [refreshingHealth, setRefreshingHealth] = useState(false);
  const [installingH2, setInstallingH2] = useState(false);

  const selectedInstance = instances.find((instance) => instance.id === selectedId) ?? null;
  const selectedProxy = selectedInstance ? proxies[selectedInstance.id] : undefined;
  const selectedFunnel = selectedInstance ? funnels[selectedInstance.id] ?? null : null;

  const bootstrap = useCallback(async () => {
    setLoading(true);
    setFatal(null);
    try {
      if (!window.go?.main?.App) {
        throw new Error(
          'HySQL desktop runtime was not detected. Launch the app through Wails (`wails dev`) or the packaged binary.',
        );
      }
      const systemHealth = await api.getSystemHealth();
      setHealth(systemHealth);
      if (!systemHealth.initialized) {
        setFatal(systemHealth.error || 'The HySQL backend failed to initialize.');
        return;
      }
      const list = await api.listInstances();
      setInstances(list);
      setSelectedId((current) =>
        current && list.some((instance) => instance.id === current)
          ? current
          : list[0]?.id ?? null,
      );
    } catch (error) {
      setFatal(errorMessage(error));
    } finally {
      setLoading(false);
    }
  }, []);

  const refreshInstances = useCallback(async () => {
    try {
      const list = await api.listInstances();
      setInstances(list);
      setSelectedId((current) =>
        current && list.some((instance) => instance.id === current)
          ? current
          : list[0]?.id ?? null,
      );
    } catch {
      // The event stream and the originating action already surface failures.
    }
  }, []);

  useEffect(() => {
    void bootstrap();
  }, [bootstrap]);

  useEffect(() => {
    if (!window.runtime) {
      return;
    }
    const handleInstances = (...args: unknown[]) => {
      const list = Array.isArray(args[0]) ? (args[0] as Instance[]) : [];
      setInstances(list);
      setSelectedId((current) =>
        current && list.some((instance) => instance.id === current)
          ? current
          : list[0]?.id ?? null,
      );
    };
    const handleTailscale = (...args: unknown[]) => {
      const status = args[0] as TailscaleStatus;
      if (!status) {
        return;
      }
      setHealth((current) => (current ? { ...current, tailscale: status } : current));
    };
    const handleProxy = (...args: unknown[]) => {
      const snapshot = args[0] as ProxySnapshot;
      if (!snapshot?.instanceId) {
        return;
      }
      setProxies((current) => ({ ...current, [snapshot.instanceId]: snapshot }));
    };
    const handleFunnel = (...args: unknown[]) => {
      const state = args[0] as FunnelState;
      if (!state?.instanceId) {
        return;
      }
      setFunnels((current) => ({ ...current, [state.instanceId]: state }));
    };
    const handleError = (...args: unknown[]) => {
      const event = args[0] as AppErrorEvent;
      if (event?.message) {
        push('error', event.message);
      }
    };
    const handleRealtime = (...args: unknown[]) => {
      const event = args[0] as RealtimeChangeEvent;
      if (event?.instanceId) {
        setRealtimeEvent(event);
      }
    };

    EventsOn('instances:changed', handleInstances);
    EventsOn('tailscale:status', handleTailscale);
    EventsOn('proxy:snapshot', handleProxy);
    EventsOn('funnel:state', handleFunnel);
    EventsOn('app:error', handleError);
    EventsOn('realtime:change', handleRealtime);
    return () => {
      EventsOff('instances:changed');
      EventsOff('tailscale:status');
      EventsOff('proxy:snapshot');
      EventsOff('funnel:state');
      EventsOff('app:error');
      EventsOff('realtime:change');
    };
  }, [push]);

  useEffect(() => {
    if (!selectedId && instances.length > 0) {
      setSelectedId(instances[0].id);
    }
  }, [instances, selectedId]);

  useEffect(() => {
    if (!selectedId) {
      return;
    }
    let cancelled = false;
    api
      .getProxySnapshot(selectedId)
      .then((snapshot) => {
        if (!cancelled) {
          setProxies((current) => ({ ...current, [snapshot.instanceId || selectedId]: snapshot }));
        }
      })
      .catch(() => {
        // Snapshot hydration is best-effort; the card renders its inactive state.
      });
    return () => {
      cancelled = true;
    };
  }, [selectedId, selectedInstance?.status, selectedInstance?.tailscaleEnabled, selectedInstance?.tailscalePort]);

  useEffect(() => {
    if (!selectedId) {
      setFunnelLoading(false);
      return;
    }
    let cancelled = false;
    setFunnelLoading(true);
    api
      .getFunnelState(selectedId)
      .then((state) => {
        if (!cancelled) {
          setFunnels((current) => ({ ...current, [selectedId]: state }));
        }
      })
      .catch((loadError) => {
        if (!cancelled) {
          setFunnels((current) => ({
            ...current,
            [selectedId]: {
              instanceId: selectedId,
              available: false,
              enabled: false,
              port: 0,
              host: '',
              publicUrl: '',
              jdbcUrl: '',
              allowedPorts: [443, 8443, 10000],
              error: errorMessage(loadError),
              checkedAt: new Date().toISOString(),
            },
          }));
        }
      })
      .finally(() => {
        if (!cancelled) {
          setFunnelLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [
    selectedId,
    selectedInstance?.status,
    selectedInstance?.funnelEnabled,
    selectedInstance?.funnelPort,
    health?.tailscale.checkedAt,
    funnelToken,
  ]);

  useEffect(() => {
    if (view !== 'table') {
      setTableTarget(null);
    }
  }, [view]);

  const handleCopyError = useCallback(
    (message: string) => {
      push('error', message);
    },
    [push],
  );

  const handleRefreshTailscale = async () => {
    setRefreshingTailscale(true);
    try {
      const status = await api.refreshTailscale();
      setHealth((current) => (current ? { ...current, tailscale: status } : current));
      if (!status.running) {
        push('info', status.error || 'Tailscale is not connected.');
      }
    } catch (error) {
      push('error', errorMessage(error));
    } finally {
      setRefreshingTailscale(false);
    }
  };

  const handleRefreshFunnel = async () => {
    try {
      const status = await api.refreshTailscale();
      setHealth((current) => (current ? { ...current, tailscale: status } : current));
    } catch {
      // The Funnel state request below surfaces the actionable error.
    }
    setFunnelToken((value) => value + 1);
  };

  const handleRefreshHealth = async () => {
    setRefreshingHealth(true);
    try {
      const systemHealth = await api.refreshSystemHealth();
      setHealth(systemHealth);
      if (!systemHealth.java.supported) {
        push('error', systemHealth.java.error || 'Java 11+ is required.');
      } else if (!systemHealth.h2.available) {
        push('info', systemHealth.h2.error || 'The H2 engine is not installed.');
      } else if (!systemHealth.tailscale.running) {
        push('info', systemHealth.tailscale.error || 'Tailscale is not connected.');
      } else {
        push('success', 'System checks passed.');
      }
    } catch (error) {
      push('error', errorMessage(error));
    } finally {
      setRefreshingHealth(false);
    }
  };

  const handleInstallH2 = async () => {
    setInstallingH2(true);
    try {
      const systemHealth = await api.installH2Engine();
      setHealth(systemHealth);
      push('success', `H2 ${systemHealth.h2.version} installed and verified.`);
    } catch (error) {
      push('error', errorMessage(error));
    } finally {
      setInstallingH2(false);
    }
  };

  const handleCreate = async (request: CreateInstanceRequest) => {
    setCreating(true);
    setCreateError('');
    try {
      const created = await api.createInstance(request);
      push('success', `${created.name} is running on 127.0.0.1:${created.port}.`);
      setCreateOpen(false);
      setSelectedId(created.id);
      setView('overview');
      await refreshInstances();
    } catch (error) {
      const message = errorMessage(error);
      setCreateError(message);
      push('error', message);
      await refreshInstances();
    } finally {
      setCreating(false);
    }
  };

  const runLifecycle = async (label: string, action: () => Promise<unknown>, success: string) => {
    setBusy(label);
    try {
      await action();
      push('success', success);
    } catch (error) {
      push('error', errorMessage(error));
    } finally {
      await refreshInstances();
      setBusy(null);
    }
  };

  const handleStart = () => {
    if (!selectedInstance) {
      return;
    }
    void runLifecycle(
      'Starting database…',
      () => api.startInstance(selectedInstance.id),
      `${selectedInstance.name} started.`,
    );
  };

  const handleStop = () => {
    if (!selectedInstance) {
      return;
    }
    void runLifecycle(
      'Stopping database…',
      () => api.stopInstance(selectedInstance.id),
      `${selectedInstance.name} stopped cleanly.`,
    );
  };

  const handleRestart = () => {
    if (!selectedInstance) {
      return;
    }
    void runLifecycle(
      'Restarting database…',
      () => api.restartInstance(selectedInstance.id),
      `${selectedInstance.name} restarted.`,
    );
  };

  const handleDelete = async () => {
    if (!deleteTarget) {
      return;
    }
    setDeleting(true);
    try {
      await api.deleteInstance(deleteTarget.id);
      push('success', `Removed ${deleteTarget.name}. Database files were kept on disk.`);
      setDeleteTarget(null);
    } catch (error) {
      push('error', errorMessage(error));
    } finally {
      setDeleting(false);
      await refreshInstances();
    }
  };

  const handleToggleHosting = async (enabled: boolean) => {
    if (!selectedInstance) {
      return;
    }
    const instance = selectedInstance;
    setBusy(enabled ? 'Enabling Tailscale hosting…' : 'Disabling Tailscale hosting…');
    try {
      if (enabled) {
        const snapshot = await api.enableTailscaleHosting(instance.id);
        setProxies((current) => ({ ...current, [instance.id]: snapshot }));
        push('success', `${instance.name} is now reachable on your tailnet.`);
      } else {
        await api.disableTailscaleHosting(instance.id);
        setProxies((current) => ({ ...current, [instance.id]: inactiveProxy(instance.id) }));
        push('info', 'Tailscale hosting disabled.');
      }
    } catch (error) {
      push('error', errorMessage(error));
      try {
        const status = await api.refreshTailscale();
        setHealth((current) => (current ? { ...current, tailscale: status } : current));
      } catch {
        // The original failure is already surfaced.
      }
    } finally {
      await refreshInstances();
      setBusy(null);
    }
  };

  const handleEnableFunnel = async (port: number) => {
    if (!selectedInstance) {
      return;
    }
    const instance = selectedInstance;
    setFunnelBusy(true);
    try {
      const state = await api.enableFunnel(instance.id, port);
      setFunnels((current) => ({ ...current, [instance.id]: state }));
      if (state.error) {
        push('info', state.error);
      } else {
        push('success', `${instance.name} is now public on port ${state.port}.`);
      }
      await refreshInstances();
    } catch (error) {
      push('error', errorMessage(error));
      setFunnelToken((value) => value + 1);
    } finally {
      setFunnelBusy(false);
    }
  };

  const handleDisableFunnel = async () => {
    if (!selectedInstance) {
      return;
    }
    const instance = selectedInstance;
    setFunnelBusy(true);
    try {
      const state = await api.disableFunnel(instance.id);
      setFunnels((current) => ({ ...current, [instance.id]: state }));
      push('info', 'Public Funnel hosting disabled.');
      await refreshInstances();
    } catch (error) {
      push('error', errorMessage(error));
      setFunnelToken((value) => value + 1);
    } finally {
      setFunnelBusy(false);
    }
  };

  const openTableEditor = (table: TableInfo) => {
    setTableTarget({ schema: table.schema, name: table.name });
    setView('table');
  };

  const handleSelect = (id: string) => {
    setSelectedId(id);
    setTableTarget(null);
    setView('overview');
  };

  const openCreateModal = () => {
    setCreateError('');
    setCreateOpen(true);
  };

  const canCreate = Boolean(health?.initialized && health.java.supported);
  const javaIssue = Boolean(health && !health.java.supported);

  return (
    <div className="flex h-screen overflow-hidden">
      <Sidebar
        instances={instances}
        selectedId={selectedId}
        health={health}
        refreshingTailscale={refreshingTailscale}
        onSelect={handleSelect}
        onCreate={openCreateModal}
        onRefreshTailscale={() => void handleRefreshTailscale()}
      />

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-[76px] flex-none items-center justify-between gap-4 border-b border-white/[0.06] bg-ink-950/30 px-6">
          <div className="flex min-w-0 items-center gap-3">
            <div className="flex h-9 w-9 flex-none items-center justify-center rounded-lg border border-white/[0.07] bg-white/[0.03]">
              <Database size={16} className="text-slate-400" />
            </div>
            <div className="min-w-0">
              <div className="flex items-center gap-2">
                <span className="truncate text-sm font-semibold text-slate-100">
                  {selectedInstance?.name ?? 'No database selected'}
                </span>
                {selectedInstance ? <StatusBadge status={selectedInstance.status} /> : null}
              </div>
              <div className="truncate text-[11px] text-slate-500">
                {selectedInstance
                  ? `${selectedInstance.databaseFile} · ${formatBytes(selectedInstance.fileSizeBytes)}`
                  : 'Create a database to begin'}
              </div>
            </div>
          </div>

          <nav className="flex items-center gap-1 rounded-xl border border-white/[0.06] bg-ink-900/60 p-1">
            {navItems.map((item) => {
              const Icon = item.icon;
              const active = view === item.key;
              return (
                <button
                  key={item.key}
                  type="button"
                  disabled={!selectedInstance}
                  onClick={() => setView(item.key)}
                  aria-current={active ? 'page' : undefined}
                  className={`flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-[11px] font-medium transition-colors disabled:opacity-35 ${
                    active
                      ? 'bg-white/[0.08] text-slate-100'
                      : 'text-slate-500 hover:bg-white/[0.03] hover:text-slate-300'
                  }`}
                >
                  <Icon size={13} />
                  {item.label}
                </button>
              );
            })}
          </nav>

          <div className="flex items-center gap-2">
            {busy ? (
              <span className="chip border-mint-400/20 text-mint-300">
                <Loader2 size={11} className="animate-spin" />
                {busy}
              </span>
            ) : null}
            {selectedInstance?.funnelEnabled ? (
              <span className="chip border-rose-400/20 text-rose-300">
                <Globe2 size={11} />
                Public
              </span>
            ) : (
              <span className="chip hidden lg:inline-flex">
                <ShieldCheck size={11} />
                Local-first
              </span>
            )}
          </div>
        </header>

        {!fatal ? (
          <HealthBanner
            health={health}
            installingH2={installingH2}
            refreshing={refreshingHealth}
            onInstallH2={() => void handleInstallH2()}
            onRefresh={() => void handleRefreshHealth()}
          />
        ) : null}

        <main className="min-h-0 flex-1 overflow-y-auto">
          {loading ? (
            <LoadingSkeleton />
          ) : fatal ? (
            <FatalPanel message={fatal} onRetry={() => void bootstrap()} />
          ) : !selectedInstance ? (
            <EmptyState canCreate={canCreate} javaIssue={javaIssue} onCreate={openCreateModal} />
          ) : health ? (
            <div className="px-6 py-6">
              {view === 'overview' ? (
                <OverviewView
                  instance={selectedInstance}
                  proxy={selectedProxy}
                  funnel={selectedFunnel}
                  health={health}
                  busy={busy}
                  refreshingTailscale={refreshingTailscale}
                  funnelBusy={funnelBusy}
                  funnelLoading={funnelLoading}
                  onStart={handleStart}
                  onStop={handleStop}
                  onRestart={handleRestart}
                  onDelete={() => setDeleteTarget(selectedInstance)}
                  onToggleHosting={(enabled) => void handleToggleHosting(enabled)}
                  onEnableFunnel={(port) => void handleEnableFunnel(port)}
                  onDisableFunnel={() => void handleDisableFunnel()}
                  onRefreshFunnel={() => void handleRefreshFunnel()}
                  onRefreshTailscale={() => void handleRefreshTailscale()}
                  onOpenJava={() => setView('java')}
                  onCopyError={handleCopyError}
                  onToast={push}
                />
              ) : view === 'table' ? (
                <TableEditorView
                  instance={selectedInstance}
                  busy={busy}
                  onStart={handleStart}
                  initialTable={tableTarget}
                  realtimeEvent={realtimeEvent}
                  onToast={push}
                />
              ) : view === 'java' ? (
                <JavaConnectView
                  instance={selectedInstance}
                  health={health}
                  onOpenOverview={() => setView('overview')}
                  onToast={push}
                />
              ) : view === 'sql' ? (
                <SqlWorkbenchView
                  instance={selectedInstance}
                  busy={busy}
                  onStart={handleStart}
                  onOpenTable={openTableEditor}
                  realtimeEvent={realtimeEvent}
                  onOpenAISettings={() => setAiSettingsOpen(true)}
                />
              ) : (
                <AgentConnectView
                  instance={selectedInstance}
                  onOpenAISettings={() => setAiSettingsOpen(true)}
                  onToast={push}
                />
              )}
            </div>
          ) : (
            <FatalPanel
              message="Backend state is unavailable. Recheck the application health."
              onRetry={() => void bootstrap()}
            />
          )}
        </main>
      </div>

      <CreateDatabaseModal
        open={createOpen}
        creating={creating}
        javaSupported={Boolean(health?.java.supported)}
        error={createError}
        onClose={() => {
          if (!creating) {
            setCreateOpen(false);
          }
        }}
        onCreate={(request) => void handleCreate(request)}
      />

      <ConfirmDialog
        open={Boolean(deleteTarget)}
        title="Remove database"
        description={
          deleteTarget
            ? `Remove "${deleteTarget.name}" from HySQL? The H2 server is stopped first, but the database file ${deleteTarget.databaseFile} stays in ~/.hysql/dbs.`
            : ''
        }
        confirmLabel="Remove database"
        busy={deleting}
        onConfirm={() => void handleDelete()}
        onCancel={() => {
          if (!deleting) {
            setDeleteTarget(null);
          }
        }}
      />

      <AISettingsModal
        open={aiSettingsOpen}
        onClose={() => setAiSettingsOpen(false)}
        onSaved={() => undefined}
        onToast={push}
      />

      <ToastViewport toasts={toasts} onDismiss={dismiss} />
    </div>
  );
}

function LoadingSkeleton() {
  return (
    <div className="space-y-5 px-6 py-6">
      <div className="flex items-center gap-4">
        <div className="h-12 w-12 animate-pulse rounded-xl bg-white/[0.05]" />
        <div className="space-y-2">
          <div className="h-4 w-44 animate-pulse rounded bg-white/[0.06]" />
          <div className="h-3 w-64 animate-pulse rounded bg-white/[0.04]" />
        </div>
      </div>
      <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-4">
        {[0, 1, 2, 3].map((item) => (
          <div key={item} className="h-24 animate-pulse rounded-xl bg-white/[0.035]" />
        ))}
      </div>
      <div className="h-48 animate-pulse rounded-2xl bg-white/[0.03]" />
      <div className="grid gap-5 xl:grid-cols-2">
        <div className="h-72 animate-pulse rounded-2xl bg-white/[0.03]" />
        <div className="h-72 animate-pulse rounded-2xl bg-white/[0.03]" />
      </div>
    </div>
  );
}

function FatalPanel({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <div className="flex min-h-full items-center justify-center px-6 py-16">
      <div className="card max-w-xl p-8 text-center">
        <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-xl border border-rose-500/20 bg-rose-500/[0.08]">
          <AlertTriangle size={22} className="text-rose-300" />
        </div>
        <h1 className="text-lg font-semibold text-slate-100">HySQL could not start</h1>
        <p className="mt-2 whitespace-pre-wrap text-xs leading-relaxed text-slate-500">{message}</p>
        <p className="mt-3 text-[11px] leading-relaxed text-slate-600">
          Check permissions on <span className="font-mono">~/.hysql</span>, then retry. Local
          features require Java 11+ and the H2 engine jar.
        </p>
        <button type="button" className="btn-primary mt-6" onClick={onRetry}>
          Retry initialization
        </button>
      </div>
    </div>
  );
}

function inactiveProxy(instanceId: string): ProxySnapshot {
  return {
    instanceId,
    running: false,
    bindAddress: '',
    port: 0,
    targetAddress: '',
    activeClients: 0,
    totalConnections: 0,
    bytesIn: 0,
    bytesOut: 0,
    clients: [],
  };
}
