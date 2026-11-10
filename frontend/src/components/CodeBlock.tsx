import { FileCode2 } from 'lucide-react';
import type { CodeLanguage } from '../types';
import { highlightLine } from '../lib/highlight';
import { CopyButton } from './CopyButton';

interface CodeBlockProps {
  code: string;
  language: CodeLanguage;
  filename: string;
  onError?: (message: string) => void;
}

export function CodeBlock({ code, language, filename, onError }: CodeBlockProps) {
  const lines = code.split('\n');
  if (lines.length > 1 && lines[lines.length - 1] === '') {
    lines.pop();
  }

  return (
    <div className="overflow-hidden rounded-xl border border-white/[0.06] bg-[#080d13]">
      <div className="flex items-center justify-between gap-3 border-b border-white/[0.06] bg-white/[0.015] px-4 py-2.5">
        <div className="flex min-w-0 items-center gap-2">
          <FileCode2 size={14} className="flex-none text-slate-500" />
          <span className="truncate font-mono text-xs text-slate-400">{filename}</span>
        </div>
        <CopyButton value={code} onError={onError} />
      </div>
      <div className="code-scroll max-h-[520px]">
        <div className="min-w-max py-3">
          {lines.map((line, index) => (
            <div className="code-line" key={`${index}-${line.slice(0, 8)}`}>
              <span className="line-num">{index + 1}</span>
              <span
                className="line-content"
                dangerouslySetInnerHTML={{ __html: highlightLine(line, language) }}
              />
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
