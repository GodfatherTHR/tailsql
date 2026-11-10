import {
  AlertTriangle,
  ArrowRight,
  Check,
  Code2,
  HardDrive,
  Network,
  RefreshCw,
  ShieldCheck,
} from 'lucide-react';
import { useEffect, useState } from 'react';
import type { ReactNode } from 'react';
import type { CodeLanguage, Instance, JavaCodegen, JavaMode, SystemHealth } from '../types';
import { api, errorMessage } from '../lib/api';
import { CopyButton } from './CopyButton';
import { CodeBlock } from './CodeBlock';
import { EndpointRow } from './EndpointRow';

type TabKey = 'hikari' | 'jdbc' | 'maven' | 'gradle' | 'properties' | 'yaml';

interface TabDefinition {
  key: TabKey;
  label: string;
  filename: string;
  language: CodeLanguage;
  description: string;
  code: (codegen: JavaCodegen) => string;
}

const tabs: TabDefinition[] = [
  {
    key: 'hikari',
    label: 'HikariCP',
    filename: 'DatabaseConfig.java',
    language: 'java',
    description: 'Pooled connection with a 10s timeout and five connections.',
    code: (codegen) => codegen.hikari,
  },
  {
    key: 'jdbc',
    label: 'JDBC test',
    filename: 'JdbcTest.java',
    language: 'java',
    description: 'Minimal DriverManager check for a first connection.',
    code: (codegen) => codegen.jdbcTest,
  },
  {
    key: 'maven',
    label: 'Maven',
    filename: 'pom.xml',
    language: 'xml',
    description: 'Dependency block to add to pom.xml.',
    code: (codegen) => codegen.maven,
  },
  {
    key: 'gradle',
    label: 'Gradle',
    filename: 'build.gradle',
    language: 'groovy',
    description: 'Dependency block for build.gradle.',
    code: (codegen) => codegen.gradle,
  },
  {
    key: 'properties',
    label: 'Spring properties',
    filename: 'application.properties',
    language: 'properties',
    description: 'Spring Boot datasource configuration.',
    code: (codegen) => codegen.springProperties,
  },
  {
    key: 'yaml',
    label: 'Spring YAML',
    filename: 'application.yml',
    language: 'yaml',
    description: 'The same datasource configuration in YAML.',
    code: (codegen) => codegen.springYaml,
  },
];

interface JavaConnectViewProps {
  instance: Instance;
  health: SystemHealth;
  onOpenOverview: () => void;
  onToast: (kind: 'success' | 'error' | 'info', message: string) => void;
}

export function JavaConnectView({ instance, health, onOpenOverview, onToast }: JavaConnectViewProps) {
  const [mode, setMode] = useState<JavaMode>('local');
  const [tab, setTab] = useState<TabKey>('hikari');
  const [codegen, setCodegen] = useState<JavaCodegen | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [reloadToken, setReloadToken] = useState(0);

  useEffect(() => {
    setMode('local');
    setTab('hikari');
  }, [instance.id]);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError('');
    api
      .getJavaCodegen(instance.id, mode)
      .then((result) => {
        if (!cancelled) {
          setCodegen(result);
        }
      })
      .catch((loadError) => {
        if (!cancelled) {
          setError(errorMessage(loadError));
          setCodegen(null);
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [
    instance.id,
    instance.status,
    instance.tailscaleEnabled,
    instance.tailscalePort,
    mode,
    health.tailscale.checkedAt,
    reloadToken,
  ]);

  const activeTab = tabs.find((candidate) => candidate.key === tab) ?? tabs[0];
  const code = codegen ? activeTab.code(codegen) : '';
  const copyError = (message: string) => onToast('error', message);

  return (
    <div className="space-y-5 animate-fade-in">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <p className="label mb-1">Code generator</p>
          <h1 className="text-xl font-semibold tracking-tight text-slate-100">Connect to Java</h1>
          <p className="mt-1 max-w-xl text-xs leading-relaxed text-slate-500">
            Live snippets for <span className="font-mono text-slate-400">{instance.name}</span>,
            populated with the credentials stored on this machine.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <span className="chip">
            <ShieldCheck size={11} />
            Java 11+
          </span>
          <button
            type="button"
            className="icon-btn"
            onClick={() => setReloadToken((value) => value + 1)}
            disabled={loading}
            title="Regenerate snippets"
            aria-label="Regenerate snippets"
          >
            <RefreshCw size={14} className={loading ? 'animate-spin' : ''} />
          </button>
        </div>
      </div>

      <div className="grid gap-2 sm:grid-cols-2">
        <ModeCard
          active={mode === 'local'}
          icon={<HardDrive size={16} />}
          title="Local machine"
          subtitle={`127.0.0.1:${instance.port}`}
          note="Loopback-only endpoint"
          onClick={() => setMode('local')}
        />
        <ModeCard
          active={mode === 'tailscale'}
          icon={<Network size={16} />}
          title="Tailscale tailnet"
          subtitle={
            instance.tailscaleEnabled
              ? `${instance.tailscaleIp || '100.x.y.z'}:${instance.tailscalePort}`
              : 'Hosting is off'
          }
          note={instance.tailscaleEnabled ? 'Reachable by allowed peers' : 'Enable hosting first'}
          warning={!instance.tailscaleEnabled}
          onClick={() => setMode('tailscale')}
        />
      </div>

      {loading ? (
        <div className="card space-y-3 p-5">
          <div className="h-3 w-56 animate-pulse rounded bg-white/[0.06]" />
          <div className="h-48 animate-pulse rounded-xl bg-white/[0.03]" />
        </div>
      ) : error ? (
        <div className="card p-6">
          <div className="flex gap-3 rounded-xl border border-rose-500/20 bg-rose-500/[0.05] px-4 py-3.5 text-xs leading-relaxed text-rose-200/90">
            <AlertTriangle size={16} className="mt-0.5 flex-none" />
            <span className="whitespace-pre-wrap">{error}</span>
          </div>
        </div>
      ) : codegen && !codegen.available ? (
        <div className="card p-6">
          <div className="flex flex-col items-start gap-4 rounded-xl border border-amber-400/20 bg-amber-400/[0.05] px-5 py-5 sm:flex-row sm:items-center">
            <div className="flex h-10 w-10 flex-none items-center justify-center rounded-xl border border-amber-400/20 bg-amber-400/[0.08]">
              <AlertTriangle size={18} className="text-amber-300" />
            </div>
            <div className="min-w-0 flex-1">
              <div className="text-sm font-semibold text-amber-200">Remote mode is not available</div>
              <p className="mt-1 text-xs leading-relaxed text-amber-200/70">
                {codegen.reason || 'Enable Tailscale hosting for this database first.'}
              </p>
            </div>
            <button type="button" className="btn-secondary btn-xs" onClick={onOpenOverview}>
              Open overview
              <ArrowRight size={12} />
            </button>
          </div>
        </div>
      ) : codegen ? (
        <>
          <div className="space-y-2.5">
            <EndpointRow
              label={`${mode === 'local' ? 'Local' : 'Tailscale'} JDBC URL`}
              value={codegen.jdbcUrl}
              hint={
                codegen.magicDns
                  ? `MagicDNS alternative: ${codegen.magicDns}:${codegen.port}`
                  : `Database ${codegen.database} · user ${codegen.username}`
              }
              onError={copyError}
            />
            <div className="flex flex-wrap items-center gap-2 text-[11px] text-slate-600">
              <span className="chip">
                <Code2 size={11} />
                {activeTab.description}
              </span>
              <span className="font-mono">H2 {codegen.mode === 'tailscale' ? 'remote' : 'local'}</span>
              <span className="text-slate-700">/</span>
              <span>{codegen.database}</span>
              <CopyButton value={codegen.password} label="Copy password" onError={copyError} />
            </div>
          </div>

          <div className="flex gap-1 overflow-x-auto rounded-xl border border-white/[0.06] bg-ink-900/50 p-1 scrollbar-none">
            {tabs.map((candidate) => {
              const active = candidate.key === tab;
              return (
                <button
                  key={candidate.key}
                  type="button"
                  onClick={() => setTab(candidate.key)}
                  className={`flex-none rounded-lg px-3 py-1.5 text-[11px] font-medium transition-colors ${
                    active
                      ? 'bg-white/[0.08] text-slate-100'
                      : 'text-slate-500 hover:bg-white/[0.03] hover:text-slate-300'
                  }`}
                >
                  {candidate.label}
                </button>
              );
            })}
          </div>

          <CodeBlock
            code={code}
            language={activeTab.language}
            filename={activeTab.filename}
            onError={copyError}
          />

          <div className="flex items-start gap-2 text-[11px] leading-relaxed text-slate-600">
            <ShieldCheck size={13} className="mt-0.5 flex-none text-slate-500" />
            <span>
              Dependencies are pinned to H2 2.3.232 and HikariCP 6.2.1. Credentials are immutable in
              this MVP; recreate the database to rotate them.
            </span>
          </div>
        </>
      ) : null}
    </div>
  );
}

function ModeCard({
  active,
  icon,
  title,
  subtitle,
  note,
  warning = false,
  onClick,
}: {
  active: boolean;
  icon: ReactNode;
  title: string;
  subtitle: string;
  note: string;
  warning?: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`group relative rounded-xl border px-4 py-3.5 text-left transition-all ${
        active
          ? 'border-mint-400/30 bg-mint-400/[0.06] shadow-[0_0_30px_-18px_rgba(52,211,153,0.8)]'
          : 'border-white/[0.07] bg-ink-900/45 hover:border-white/[0.12] hover:bg-white/[0.02]'
      }`}
    >
      <div className="flex items-center gap-2">
        <span className={active ? 'text-mint-300' : 'text-slate-500'}>{icon}</span>
        <span className="text-xs font-semibold text-slate-200">{title}</span>
        {active ? (
          <span className="ml-auto flex h-4 w-4 items-center justify-center rounded-full bg-mint-400 text-ink-950">
            <Check size={11} strokeWidth={3} />
          </span>
        ) : null}
        {!active && warning ? (
          <span className="ml-auto h-1.5 w-1.5 rounded-full bg-amber-400/80" />
        ) : null}
      </div>
      <div className="mt-1.5 font-mono text-[11px] text-slate-400">{subtitle}</div>
      <div className="mt-0.5 text-[10px] text-slate-600">{note}</div>
    </button>
  );
}

