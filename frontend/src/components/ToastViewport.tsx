import { AlertTriangle, CheckCircle2, Info, X } from 'lucide-react';
import type { Toast } from '../hooks/useToasts';

const styles = {
  success: {
    box: 'border-mint-400/20 bg-[#0d1a17]/95',
    icon: 'text-mint-300',
    Icon: CheckCircle2,
  },
  error: {
    box: 'border-rose-500/25 bg-[#1c1115]/95',
    icon: 'text-rose-300',
    Icon: AlertTriangle,
  },
  info: {
    box: 'border-sky-400/20 bg-[#0d1620]/95',
    icon: 'text-sky-300',
    Icon: Info,
  },
};

interface ToastViewportProps {
  toasts: Toast[];
  onDismiss: (id: number) => void;
}

export function ToastViewport({ toasts, onDismiss }: ToastViewportProps) {
  if (toasts.length === 0) {
    return null;
  }
  return (
    <div className="pointer-events-none fixed bottom-6 right-6 z-[60] flex w-[min(430px,calc(100vw-3rem))] flex-col gap-2">
      {toasts.map((toast) => {
        const style = styles[toast.kind];
        const Icon = style.Icon;
        return (
          <div
            key={toast.id}
            className={`pointer-events-auto flex animate-slide-in items-start gap-3 rounded-xl border px-4 py-3 shadow-panel backdrop-blur ${style.box}`}
            role="status"
          >
            <Icon size={17} className={`mt-0.5 flex-none ${style.icon}`} />
            <p className="flex-1 text-sm leading-relaxed text-slate-300">{toast.message}</p>
            <button
              type="button"
              className="icon-btn -mr-1 -mt-1 h-6 w-6"
              onClick={() => onDismiss(toast.id)}
              aria-label="Dismiss notification"
            >
              <X size={13} />
            </button>
          </div>
        );
      })}
    </div>
  );
}
