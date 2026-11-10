import { Check, Copy } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { copyText } from '../lib/format';

interface CopyButtonProps {
  value: string;
  label?: string;
  iconOnly?: boolean;
  className?: string;
  onError?: (message: string) => void;
}

export function CopyButton({ value, label = 'Copy', iconOnly = false, className = '', onError }: CopyButtonProps) {
  const [copied, setCopied] = useState(false);
  const timer = useRef<number | null>(null);

  useEffect(() => {
    return () => {
      if (timer.current !== null) {
        window.clearTimeout(timer.current);
      }
    };
  }, []);

  const handleCopy = async () => {
    try {
      await copyText(value);
      setCopied(true);
      if (timer.current !== null) {
        window.clearTimeout(timer.current);
      }
      timer.current = window.setTimeout(() => setCopied(false), 1600);
    } catch (error) {
      onError?.(error instanceof Error ? error.message : 'Could not copy to clipboard.');
    }
  };

  if (iconOnly) {
    return (
      <button
        type="button"
        className={`icon-btn ${className}`}
        onClick={handleCopy}
        title={copied ? 'Copied' : 'Copy to clipboard'}
        aria-label={copied ? 'Copied' : 'Copy to clipboard'}
      >
        {copied ? <Check size={14} className="text-mint-300" /> : <Copy size={14} />}
      </button>
    );
  }

  return (
    <button type="button" className={`btn-secondary btn-xs ${className}`} onClick={handleCopy}>
      {copied ? <Check size={13} className="text-mint-300" /> : <Copy size={13} />}
      {copied ? 'Copied' : label}
    </button>
  );
}
