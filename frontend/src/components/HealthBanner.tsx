import { AlertTriangle, Download, Info, RefreshCw } from 'lucide-react';
import type { ReactNode } from 'react';
import type { SystemHealth } from '../types';

interface HealthBannerProps {
  health: SystemHealth | null;
  installingH2: boolean;
  refreshing: boolean;
  onInstallH2: () => void;
  onRefresh: () => void;
}

export function HealthBanner({
  health,
  installingH2,
  refreshing,
  onInstallH2,
  onRefresh,
}: HealthBannerProps) {
  if (!health) {
    return null;
  }

  const fatal = !health.initialized;
  const javaIssue = !health.java.available || !health.java.supported;
  const h2Issue = !health.h2.available;
  const tailscaleIssue = !health.tailscale.installed || !health.tailscale.running;
  const warnings = health.warnings ?? [];

  if (!fatal && !javaIssue && !h2Issue && !tailscaleIssue && warnings.length === 0) {
    return null;
  }

  return (
    <div className="space-y-2 border-b border-white/[0.05] bg-ink-950/40 px-6 py-3">
      {fatal ? (
        <Banner tone="error" icon={<AlertTriangle size={16} />} title="Backend unavailable">
          {health.error || 'HySQL could not initialize its local state.'}
          <BannerActions>
            <button type="button" className="btn-secondary btn-xs" onClick={onRefresh} disabled={refreshing}>
              <RefreshCw size={12} className={refreshing ? 'animate-spin' : ''} />
              Recheck
            </button>
          </BannerActions>
        </Banner>
      ) : null}

      {!fatal && javaIssue ? (
        <Banner
          tone="error"
          icon={<AlertTriangle size={16} />}
          title={health.java.available ? 'Unsupported Java runtime' : 'Java runtime not found'}
        >
          {health.java.error || 'Install Java 11 or newer and ensure `java` is available on PATH.'}
          <BannerActions>
            <button type="button" className="btn-secondary btn-xs" onClick={onRefresh} disabled={refreshing}>
              <RefreshCw size={12} className={refreshing ? 'animate-spin' : ''} />
              Recheck
            </button>
          </BannerActions>
        </Banner>
      ) : null}

      {!fatal && h2Issue ? (
        <Banner
          tone="warning"
          icon={<AlertTriangle size={16} />}
          title="H2 engine not installed"
        >
          {health.h2.error || 'The H2 jar is required to start local databases.'}
          <BannerActions>
            <button type="button" className="btn-secondary btn-xs" onClick={onInstallH2} disabled={installingH2}>
              <Download size={12} className={installingH2 ? 'animate-bounce' : ''} />
              {installingH2 ? 'Downloading…' : `Download H2 ${health.h2.expectedVersion}`}
            </button>
            <button type="button" className="btn-ghost btn-xs" onClick={onRefresh} disabled={refreshing}>
              <RefreshCw size={12} className={refreshing ? 'animate-spin' : ''} />
              Recheck
            </button>
          </BannerActions>
        </Banner>
      ) : null}

      {!fatal && tailscaleIssue ? (
        <Banner tone="info" icon={<Info size={16} />} title="Tailscale is not available">
          {health.tailscale.error ||
            'Sign in to Tailscale to host databases on your tailnet. Local databases keep working.'}
          <BannerActions>
            <button type="button" className="btn-ghost btn-xs" onClick={onRefresh} disabled={refreshing}>
              <RefreshCw size={12} className={refreshing ? 'animate-spin' : ''} />
              Recheck
            </button>
          </BannerActions>
        </Banner>
      ) : null}

      {warnings.map((warning) => (
        <Banner key={warning} tone="warning" icon={<Info size={16} />} title="State notice">
          {warning}
        </Banner>
      ))}
    </div>
  );
}

function Banner({
  tone,
  icon,
  title,
  children,
}: {
  tone: 'error' | 'warning' | 'info';
  icon: ReactNode;
  title: string;
  children: ReactNode;
}) {
  const styles = {
    error: 'border-rose-500/20 bg-rose-500/[0.06]',
    warning: 'border-amber-400/20 bg-amber-400/[0.05]',
    info: 'border-sky-400/15 bg-sky-400/[0.04]',
  }[tone];
  const iconStyle = {
    error: 'text-rose-300',
    warning: 'text-amber-300',
    info: 'text-sky-300',
  }[tone];

  return (
    <div className={`flex flex-wrap items-center gap-x-3 gap-y-1 rounded-xl border px-3.5 py-2.5 ${styles}`}>
      <span className={`flex-none ${iconStyle}`}>{icon}</span>
      <span className="text-xs font-semibold text-slate-200">{title}</span>
      <span className="min-w-0 flex-1 text-xs leading-relaxed text-slate-400">{children}</span>
    </div>
  );
}

function BannerActions({ children }: { children: ReactNode }) {
  return <span className="flex flex-none items-center gap-1.5">{children}</span>;
}
