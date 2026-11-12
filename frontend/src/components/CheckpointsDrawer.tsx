import { History, Loader2, Plus, RotateCcw, ShieldAlert, Trash2 } from 'lucide-react';
import { useEffect, useState } from 'react';
import type { Instance, SnapshotInfo } from '../types';
import { api, errorMessage } from '../lib/api';
import { formatBytes, formatDateTime } from '../lib/format';
import type { ToastKind } from '../hooks/useToasts';
import { ConfirmDialog } from './ConfirmDialog';
import { Modal } from './Modal';

interface CheckpointsDrawerProps {
  open: boolean;
  instance: Instance;
  onClose: () => void;
  onToast: (kind: ToastKind, message: string) => void;
}

interface PendingAction {
  kind: 'restore' | 'delete';
  snapshot: SnapshotInfo;
}

export function CheckpointsDrawer({ open, instance, onClose, onToast }: CheckpointsDrawerProps) {
  const [snapshots, setSnapshots] = useState<SnapshotInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState('');
  const [label, setLabel] = useState('');
  const [pending, setPending] = useState<PendingAction | null>(null);

  const load = async () => {
    setLoading(true);
    setError('');
    try {
      setSnapshots(await api.listSnapshots(instance.id));
    } catch (loadError) {
      setError(errorMessage(loadError));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (!open) {
      return;
    }
    setLabel('');
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, instance.id]);

  const create = async () => {
    setBusy('create');
    setError('');
    try {
      const snapshot = await api.createSnapshot(
        instance.id,
        label.trim() || 'Manual checkpoint',
      );
      onToast('success', `Checkpoint ${snapshot.label} created (${formatBytes(snapshot.sizeBytes)}).`);
      setLabel('');
      await load();
    } catch (createError) {
      setError(errorMessage(createError));
      onToast('error', errorMessage(createError));
    } finally {
      setBusy(null);
    }
  };

  const runPending = async () => {
    if (!pending) {
      return;
    }
    const action = pending;
    setBusy(action.kind);
    setError('');
    try {
      if (action.kind === 'restore') {
        await api.restoreSnapshot(instance.id, action.snapshot.id);
        onToast('success', `Restored ${action.snapshot.label}. H2 restarted on port ${instance.port}.`);
      } else {
        await api.deleteSnapshot(instance.id, action.snapshot.id);
        onToast('info', `Deleted checkpoint ${action.snapshot.label}.`);
      }
      setPending(null);
      await load();
    } catch (actionError) {
      setError(errorMessage(actionError));
      onToast('error', errorMessage(actionError));
    } finally {
      setBusy(null);
    }
  };

  return (
    <>
      <Modal
        open={open}
        onClose={onClose}
        title={`Checkpoints — ${instance.name}`}
        description="Consistent .mv.db snapshots stored under ~/.tailsql/snapshots. Restore stops H2, replaces the file, and restarts on the same port."
        size="lg"
        preventClose={Boolean(busy)}
        footer={
          <button type="button" className="btn-secondary" onClick={onClose} disabled={Boolean(busy)}>
            Close
          </button>
        }
      >
        <div className="space-y-4">
          <div className="flex flex-wrap items-end gap-2 rounded-xl border border-white/[0.06] bg-ink-950/40 p-3">
            <label className="min-w-[180px] flex-1">
              <span className="label mb-1.5 block">Checkpoint label</span>
              <input
                className="input"
                value={label}
                onChange={(event) => setLabel(event.target.value)}
                placeholder="Manual checkpoint"
                maxLength={48}
              />
            </label>
            <button
              type="button"
              className="btn-primary"
              onClick={() => void create()}
              disabled={Boolean(busy)}
            >
              {busy === 'create' ? (
                <Loader2 size={14} className="animate-spin" />
              ) : (
                <Plus size={14} />
              )}
              Create checkpoint
            </button>
          </div>

          <div className="flex items-center gap-2 text-[11px] text-slate-500">
            <ShieldAlert size={13} className="text-amber-300" />
            Agent DDL checkpoints are created automatically when “Auto-snapshot before Agent DDL” is on.
          </div>

          {error ? (
            <div className="rounded-xl border border-rose-500/20 bg-rose-500/[0.05] px-3.5 py-3 text-[11px] leading-relaxed text-rose-200/80">
              {error}
            </div>
          ) : null}

          {loading ? (
            <div className="flex items-center gap-2 px-2 py-6 text-xs text-slate-500">
              <Loader2 size={14} className="animate-spin" /> Loading checkpoints…
            </div>
          ) : snapshots.length === 0 ? (
            <div className="flex flex-col items-center justify-center rounded-xl border border-white/[0.06] px-6 py-10 text-center">
              <History size={22} className="mb-2 text-slate-700" />
              <p className="text-xs text-slate-500">No checkpoints for this database yet.</p>
              <p className="mt-1 text-[10px] text-slate-700">
                Create one before letting an agent change the schema.
              </p>
            </div>
          ) : (
            <div className="overflow-hidden rounded-xl border border-white/[0.06]">
              <table className="w-full">
                <thead>
                  <tr>
                    <th className="table-head">Created</th>
                    <th className="table-head">Label</th>
                    <th className="table-head">Size</th>
                    <th className="table-head text-right">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {snapshots.map((snapshot) => (
                    <tr key={snapshot.id} className="group hover:bg-white/[0.02]">
                      <td className="table-cell whitespace-nowrap text-[11px] text-slate-400">
                        {formatDateTime(snapshot.createdAt)}
                      </td>
                      <td className="table-cell">
                        <span className="block max-w-[260px] truncate text-slate-300" title={snapshot.id}>
                          {snapshot.label}
                        </span>
                      </td>
                      <td className="table-cell whitespace-nowrap text-[11px] text-slate-500">
                        {formatBytes(snapshot.sizeBytes)}
                      </td>
                      <td className="table-cell text-right">
                        <div className="flex items-center justify-end gap-1">
                          <button
                            type="button"
                            className="btn-secondary btn-xs"
                            onClick={() => setPending({ kind: 'restore', snapshot })}
                            disabled={Boolean(busy)}
                          >
                            <RotateCcw size={11} />
                            Restore
                          </button>
                          <button
                            type="button"
                            className="icon-btn h-6 w-6 hover:bg-rose-500/10 hover:text-rose-300"
                            onClick={() => setPending({ kind: 'delete', snapshot })}
                            disabled={Boolean(busy)}
                            title="Delete checkpoint"
                            aria-label="Delete checkpoint"
                          >
                            <Trash2 size={11} />
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </Modal>

      <ConfirmDialog
        open={Boolean(pending)}
        title={pending?.kind === 'restore' ? 'Restore checkpoint' : 'Delete checkpoint'}
        description={
          pending
            ? pending.kind === 'restore'
              ? `Restore “${pending.snapshot.label}” into ${instance.name}? TailSQL stops H2, replaces the .mv.db file, and restarts on port ${instance.port}. The current state is snapshotted first.`
              : `Delete the checkpoint “${pending.snapshot.label}”? This only removes the snapshot file.`
            : ''
        }
        confirmLabel={pending?.kind === 'restore' ? 'Restore checkpoint' : 'Delete checkpoint'}
        busy={Boolean(busy)}
        onConfirm={() => void runPending()}
        onCancel={() => {
          if (!busy) {
            setPending(null);
          }
        }}
      />
    </>
  );
}
