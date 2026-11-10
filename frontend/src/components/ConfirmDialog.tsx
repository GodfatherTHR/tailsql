import { AlertTriangle, Loader2 } from 'lucide-react';
import { Modal } from './Modal';

interface ConfirmDialogProps {
  open: boolean;
  title: string;
  description: string;
  confirmLabel: string;
  busy?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}

export function ConfirmDialog({
  open,
  title,
  description,
  confirmLabel,
  busy = false,
  onConfirm,
  onCancel,
}: ConfirmDialogProps) {
  return (
    <Modal
      open={open}
      onClose={onCancel}
      title={title}
      size="sm"
      preventClose={busy}
      footer={
        <>
          <button type="button" className="btn-secondary" onClick={onCancel} disabled={busy}>
            Cancel
          </button>
          <button type="button" className="btn-danger" onClick={onConfirm} disabled={busy}>
            {busy ? <Loader2 size={14} className="animate-spin" /> : <AlertTriangle size={14} />}
            {confirmLabel}
          </button>
        </>
      }
    >
      <div className="flex gap-3 rounded-xl border border-rose-500/15 bg-rose-500/[0.06] px-4 py-3.5">
        <AlertTriangle size={18} className="mt-0.5 flex-none text-rose-300" />
        <p className="text-sm leading-relaxed text-slate-400">{description}</p>
      </div>
    </Modal>
  );
}
