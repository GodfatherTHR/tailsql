import { ChevronDown, ChevronRight, RefreshCw, Terminal } from 'lucide-react';
import { useEffect, useState } from 'react';
import type { Instance } from '../types';
import { api, errorMessage } from '../lib/api';

export function ServerLogPanel({ instance }: { instance: Instance }) {
  const [open, setOpen] = useState(false);
  const [log, setLog] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const load = async () => {
    setLoading(true);
    setError('');
    try {
      const content = await api.getInstanceLog({ id: instance.id, lines: 250 });
      setLog(content);
    } catch (loadError) {
      setError(errorMessage(loadError));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    setOpen(false);
    setLog('');
    setError('');
  }, [instance.id]);

  useEffect(() => {
    if (open) {
      void load();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, instance.id]);

  return (
    <div className="card overflow-hidden">
      <button
        type="button"
        className="flex w-full items-center gap-3 px-5 py-3.5 text-left transition-colors hover:bg-white/[0.02]"
        onClick={() => setOpen((current) => !current)}
        aria-expanded={open}
      >
        {open ? <ChevronDown size={14} className="text-slate-500" /> : <ChevronRight size={14} className="text-slate-500" />}
        <Terminal size={14} className="text-slate-500" />
        <span className="flex-1 text-xs font-semibold text-slate-300">Server output</span>
        <span className="truncate font-mono text-[10px] text-slate-600" title={instance.logPath}>
          {instance.logPath ? instance.logPath.split(/[\\/]/).pop() : 'h2 log'}
        </span>
      </button>
      {open ? (
        <div className="border-t border-white/[0.06] bg-[#080d13]">
          <div className="flex items-center justify-between border-b border-white/[0.04] px-4 py-2">
            <span className="text-[10px] text-slate-600">Last 250 lines</span>
            <button
              type="button"
              className="btn-ghost btn-xs"
              onClick={() => void load()}
              disabled={loading}
            >
              <RefreshCw size={11} className={loading ? 'animate-spin' : ''} />
              Refresh
            </button>
          </div>
          {error ? (
            <p className="px-4 py-3 text-xs text-rose-300">{error}</p>
          ) : loading && !log ? (
            <p className="px-4 py-3 text-xs text-slate-500">Reading server log…</p>
          ) : log.trim() ? (
            <pre className="max-h-64 overflow-auto whitespace-pre-wrap px-4 py-3 font-mono text-[11px] leading-relaxed text-slate-400">
              {log}
            </pre>
          ) : (
            <p className="px-4 py-3 text-xs text-slate-600">
              No server output yet. Start the database to capture H2 logs.
            </p>
          )}
        </div>
      ) : null}
    </div>
  );
}
