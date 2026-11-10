import type { InstanceStatus } from '../types';
import { statusLabel } from '../lib/format';

const styles: Record<InstanceStatus, { box: string; dot: string }> = {
  running: {
    box: 'border-mint-400/20 bg-mint-400/[0.08] text-mint-300',
    dot: 'bg-mint-400',
  },
  starting: {
    box: 'border-amber-400/20 bg-amber-400/[0.08] text-amber-300',
    dot: 'bg-amber-400 animate-pulse',
  },
  stopping: {
    box: 'border-amber-400/20 bg-amber-400/[0.08] text-amber-300',
    dot: 'bg-amber-400 animate-pulse',
  },
  stopped: {
    box: 'border-white/10 bg-white/[0.03] text-slate-400',
    dot: 'bg-slate-500',
  },
  error: {
    box: 'border-rose-500/20 bg-rose-500/[0.08] text-rose-300',
    dot: 'bg-rose-400',
  },
};

export function StatusBadge({ status }: { status: InstanceStatus }) {
  const style = styles[status] ?? styles.stopped;
  return (
    <span
      className={`inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-[11px] font-semibold ${style.box}`}
    >
      <span className={`h-1.5 w-1.5 rounded-full ${style.dot}`} />
      {statusLabel(status)}
    </span>
  );
}

export function statusDotClass(status: InstanceStatus): string {
  return styles[status]?.dot ?? styles.stopped.dot;
}
