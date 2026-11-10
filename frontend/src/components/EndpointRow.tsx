import type { ReactNode } from 'react';
import { CopyButton } from './CopyButton';

interface EndpointRowProps {
  label: string;
  value: string;
  hint?: ReactNode;
  onError?: (message: string) => void;
}

export function EndpointRow({ label, value, hint, onError }: EndpointRowProps) {
  return (
    <div className="flex items-center gap-3 rounded-xl border border-white/[0.06] bg-ink-950/50 px-4 py-3">
      <div className="min-w-0 flex-1">
        <div className="label mb-1">{label}</div>
        <div className="truncate font-mono text-xs text-slate-300" title={value}>
          {value}
        </div>
        {hint ? <div className="mt-1 text-[11px] text-slate-500">{hint}</div> : null}
      </div>
      <CopyButton value={value} iconOnly onError={onError} />
    </div>
  );
}
