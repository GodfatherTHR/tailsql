import { ArrowRight, Database, Share2, Terminal } from 'lucide-react';

interface EmptyStateProps {
  canCreate: boolean;
  javaIssue: boolean;
  onCreate: () => void;
}

const steps = [
  {
    icon: Database,
    title: 'Create an H2 database',
    description: 'A real TCP server, generated credentials, and a local .mv.db file.',
  },
  {
    icon: Share2,
    title: 'Share it on your tailnet',
    description: 'Bind a proxy to your 100.x address. No firewall changes or wildcard listeners.',
  },
  {
    icon: Terminal,
    title: 'Connect from Java',
    description: 'Copy a matching JDBC URL, HikariCP config, and dependency snippet.',
  },
];

export function EmptyState({ canCreate, javaIssue, onCreate }: EmptyStateProps) {
  return (
    <div className="flex min-h-full items-center justify-center px-6 py-10">
      <div className="w-full max-w-3xl animate-fade-in">
        <div className="relative overflow-hidden rounded-3xl border border-white/[0.07] bg-ink-900/60 px-8 py-12 shadow-panel sm:px-12">
          <div className="bg-grid pointer-events-none absolute inset-0 opacity-40" />
          <div className="pointer-events-none absolute -right-24 -top-24 h-64 w-64 rounded-full bg-mint-400/[0.07] blur-3xl" />
          <div className="relative">
            <div className="mb-6 flex h-14 w-14 items-center justify-center rounded-2xl border border-mint-400/20 bg-gradient-to-br from-mint-400/20 to-cyan-400/5">
              <Database size={26} className="text-mint-300" />
            </div>
            <p className="label mb-2">Local-first database workspace</p>
            <h1 className="max-w-xl text-balance text-3xl font-semibold tracking-tight text-slate-100">
              Your local H2 lab, one toggle from your team.
            </h1>
            <p className="mt-3 max-w-xl text-sm leading-relaxed text-slate-400">
              TailSQL runs H2 as a loopback-only TCP server, exposes it over your Tailscale
              interface when you ask, and generates the Java code your service actually needs.
            </p>

            <div className="mt-8 grid gap-3 sm:grid-cols-3">
              {steps.map((step, index) => {
                const Icon = step.icon;
                return (
                  <div
                    key={step.title}
                    className="rounded-xl border border-white/[0.06] bg-ink-950/40 p-4"
                  >
                    <div className="mb-3 flex items-center justify-between">
                      <Icon size={16} className="text-slate-400" />
                      <span className="font-mono text-[10px] text-slate-600">0{index + 1}</span>
                    </div>
                    <div className="text-xs font-semibold text-slate-200">{step.title}</div>
                    <p className="mt-1 text-[11px] leading-relaxed text-slate-500">{step.description}</p>
                  </div>
                );
              })}
            </div>

            <div className="mt-8 flex flex-wrap items-center gap-3">
              <button type="button" className="btn-primary" onClick={onCreate} disabled={!canCreate}>
                Create your first database
                <ArrowRight size={15} />
              </button>
              {!canCreate ? (
                <span className="text-xs text-amber-300/80">
                  {javaIssue ? 'Install Java 11+ to start databases.' : 'Backend is still initializing.'}
                </span>
              ) : (
                <span className="text-xs text-slate-600">Auto-assigned port and password.</span>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
