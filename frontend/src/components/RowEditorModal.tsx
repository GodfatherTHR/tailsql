import { AlertTriangle, Check, Loader2, X } from 'lucide-react';
import { useEffect, useState } from 'react';
import type { TableInfo } from '../types';
import { Modal } from './Modal';

interface RowEditorField {
  name: string;
  type: string;
  nullable: boolean;
  primary: boolean;
  autoIncrement: boolean;
  generated: boolean;
  hasDefault: boolean;
  defaultValue: string;
  included: boolean;
  isNull: boolean;
  value: string;
  originalValue: string;
  originalIsNull: boolean;
  truncated: boolean;
}

interface RowEditorModalProps {
  open: boolean;
  mode: 'insert' | 'edit';
  table: TableInfo | null;
  row: (string | null)[] | null;
  error: string;
  saving: boolean;
  onClose: () => void;
  onSubmit: (values: Record<string, string | null>) => void;
}

export function RowEditorModal({
  open,
  mode,
  table,
  row,
  error,
  saving,
  onClose,
  onSubmit,
}: RowEditorModalProps) {
  const [fields, setFields] = useState<RowEditorField[]>([]);
  const [localError, setLocalError] = useState('');

  useEffect(() => {
    if (!open || !table) {
      return;
    }
    setFields(buildFields(table, row, mode));
    setLocalError('');
  }, [open, mode, table, row]);

  const updateField = (name: string, patch: Partial<RowEditorField>) => {
    setFields((current) =>
      current.map((field) => (field.name === name ? { ...field, ...patch } : field)),
    );
  };

  const handleSubmit = () => {
    if (!table) {
      return;
    }
    const values: Record<string, string | null> = {};
    for (const field of fields) {
      if (mode === 'edit') {
        if (field.primary || field.generated) {
          continue;
        }
        const dirty =
          field.isNull !== field.originalIsNull ||
          (!field.isNull && field.value !== field.originalValue);
        if (!dirty) {
          continue;
        }
        if (field.truncated && !field.isNull) {
          continue;
        }
        values[field.name] = field.isNull ? null : field.value;
        continue;
      }
      if (!field.included) {
        continue;
      }
      values[field.name] = field.isNull ? null : field.value;
    }
    if (mode === 'edit' && Object.keys(values).length === 0) {
      setLocalError('No changes to save.');
      return;
    }
    setLocalError('');
    onSubmit(values);
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={mode === 'insert' ? 'Insert row' : 'Edit row'}
      description={table ? `${table.schema}.${table.name}` : undefined}
      size="lg"
      preventClose={saving}
      footer={
        <>
          <button type="button" className="btn-secondary" onClick={onClose} disabled={saving}>
            Cancel
          </button>
          <button type="button" className="btn-primary" onClick={handleSubmit} disabled={saving}>
            {saving ? <Loader2 size={14} className="animate-spin" /> : <Check size={14} />}
            {saving ? 'Saving…' : mode === 'insert' ? 'Insert row' : 'Save changes'}
          </button>
        </>
      }
    >
      <div className="space-y-4">
        {error || localError ? (
          <div className="flex gap-2.5 rounded-xl border border-rose-500/20 bg-rose-500/[0.06] px-3.5 py-3 text-xs leading-relaxed text-rose-200/90">
            <AlertTriangle size={15} className="mt-0.5 flex-none" />
            <span className="whitespace-pre-wrap">{localError || error}</span>
          </div>
        ) : null}

        {fields.length === 0 ? (
          <p className="text-xs text-slate-500">No columns were reported for this table.</p>
        ) : (
          <div className="max-h-[55vh] space-y-2.5 overflow-y-auto pr-1">
            {fields.map((field) => (
              <RowEditorFieldRow
                key={field.name}
                field={field}
                mode={mode}
                disabled={saving}
                onChange={(patch) => updateField(field.name, patch)}
              />
            ))}
          </div>
        )}

        <p className="text-[10px] leading-relaxed text-slate-600">
          Values are written as SQL literals with identifier quoting. Leave a generated column off
          to let H2 supply the default. Use <span className="font-mono">NULL</span> for an explicit
          SQL NULL.
        </p>
      </div>
    </Modal>
  );
}

function RowEditorFieldRow({
  field,
  mode,
  disabled,
  onChange,
}: {
  field: RowEditorField;
  mode: 'insert' | 'edit';
  disabled: boolean;
  onChange: (patch: Partial<RowEditorField>) => void;
}) {
  const readOnly = mode === 'edit' && (field.primary || field.generated);
  const longText =
    field.type.toUpperCase().includes('CLOB') ||
    field.type.toUpperCase().includes('LARGE OBJECT');

  return (
    <div className="rounded-xl border border-white/[0.06] bg-ink-950/40 p-3">
      <div className="flex flex-wrap items-center gap-1.5">
        <span className="font-mono text-[11.5px] text-slate-200">{field.name}</span>
        <span className="chip px-1.5 py-0.5 text-[9px]">{field.type}</span>
        {field.primary ? (
          <span className="chip border-amber-400/20 px-1.5 py-0.5 text-[9px] text-amber-300/90">
            PK
          </span>
        ) : null}
        {field.autoIncrement ? (
          <span className="chip px-1.5 py-0.5 text-[9px]">identity</span>
        ) : null}
        {field.generated ? (
          <span className="chip px-1.5 py-0.5 text-[9px]">generated</span>
        ) : null}
        {field.hasDefault ? (
          <span
            className="chip px-1.5 py-0.5 text-[9px]"
            title={field.defaultValue ? `DEFAULT ${field.defaultValue}` : 'Database default'}
          >
            default
          </span>
        ) : null}
        {field.truncated ? (
          <span className="chip border-amber-400/20 px-1.5 py-0.5 text-[9px] text-amber-300/90">
            truncated
          </span>
        ) : null}

        <div className="ml-auto flex items-center gap-1">
          {mode === 'insert' && !field.primary && !field.generated ? (
            <button
              type="button"
              className={`btn-ghost btn-xs ${field.included ? 'text-mint-300' : ''}`}
              onClick={() => onChange({ included: !field.included })}
              disabled={disabled}
              aria-pressed={field.included}
              title={field.included ? 'Omit this column and use the database default' : 'Provide a value'}
            >
              {field.included ? <Check size={11} /> : <X size={11} />}
              {field.included ? 'Set' : 'Omit'}
            </button>
          ) : null}
          {mode === 'edit' && (field.primary || field.generated) ? (
            <span className="chip px-1.5 py-0.5 text-[9px] text-slate-500">read only</span>
          ) : field.nullable && field.included ? (
            <button
              type="button"
              className={`btn-ghost btn-xs ${field.isNull ? 'text-amber-300' : ''}`}
              onClick={() => onChange({ isNull: !field.isNull })}
              disabled={disabled}
              aria-pressed={field.isNull}
              title="Toggle an explicit SQL NULL"
            >
              NULL
            </button>
          ) : null}
        </div>
      </div>

      <div className="mt-2">
        {readOnly ? (
          <div className="rounded-lg border border-white/[0.05] bg-white/[0.02] px-3 py-2 font-mono text-[11.5px] text-slate-400">
            {field.value === '' ? <span className="italic text-slate-600">empty</span> : field.value}
          </div>
        ) : !field.included ? (
          <div className="rounded-lg border border-dashed border-white/10 px-3 py-2 text-[11px] text-slate-600">
            {field.generated
              ? 'H2 computes this column; it is never written by the editor.'
              : field.hasDefault
                ? `Uses the database default${field.defaultValue ? ` (${field.defaultValue})` : ''}`
                : 'Omitted from the insert; H2 applies NULL or its default.'}
          </div>
        ) : field.isNull ? (
          <div className="rounded-lg border border-amber-400/15 bg-amber-400/[0.04] px-3 py-2 font-mono text-[11.5px] italic text-amber-300/80">
            NULL
          </div>
        ) : field.truncated ? (
          <div className="rounded-lg border border-amber-400/15 bg-amber-400/[0.04] px-3 py-2">
            <div className="max-h-24 overflow-auto whitespace-pre-wrap break-words font-mono text-[11px] text-slate-400">
              {field.value}
            </div>
            <p className="mt-1 text-[10px] leading-relaxed text-amber-300/70">
              This value is longer than the bridge&apos;s 10,000-character display cap and cannot be
              edited safely here. Use the SQL Workbench to change it.
            </p>
          </div>
        ) : longText ? (
          <textarea
            className="input min-h-[76px] resize-y font-mono text-[11.5px]"
            value={field.value}
            onChange={(event) => onChange({ value: event.target.value })}
            disabled={disabled}
            spellCheck={false}
            placeholder={field.nullable ? 'Value (use NULL for SQL NULL)' : 'Value'}
          />
        ) : (
          <ScalarInput
            field={field}
            disabled={disabled}
            onChange={(value) => onChange({ value })}
          />
        )}
      </div>
    </div>
  );
}

function ScalarInput({
  field,
  disabled,
  onChange,
}: {
  field: RowEditorField;
  disabled: boolean;
  onChange: (value: string) => void;
}) {
  if (field.type.toUpperCase().includes('BOOLEAN')) {
    return (
      <select
        className="input font-mono text-[11.5px]"
        value={field.value}
        onChange={(event) => onChange(event.target.value)}
        disabled={disabled}
      >
        <option value="">Choose…</option>
        <option value="true">true</option>
        <option value="false">false</option>
      </select>
    );
  }
  return (
    <input
      className="input font-mono text-[11.5px]"
      value={field.value}
      onChange={(event) => onChange(event.target.value)}
      disabled={disabled}
      spellCheck={false}
      placeholder={field.nullable ? 'Value (or toggle NULL)' : 'Value'}
    />
  );
}

function buildFields(
  table: TableInfo,
  row: (string | null)[] | null,
  mode: 'insert' | 'edit',
): RowEditorField[] {
  return table.columns.map((column, index) => {
    const primary = table.primaryKey.some(
      (name) => name.toLowerCase() === column.name.toLowerCase(),
    );
    const raw = mode === 'edit' ? row?.[index] ?? null : null;
    const rawValue = raw ?? '';
    const value = column.type.toUpperCase().includes('BOOLEAN')
      ? rawValue.toLowerCase()
      : rawValue;
    return {
      name: column.name,
      type: column.type,
      nullable: column.nullable,
      primary,
      autoIncrement: column.autoIncrement,
      generated: column.generated,
      hasDefault: column.default.trim() !== '',
      defaultValue: column.default,
      included:
        mode === 'edit'
          ? !primary && !column.generated
          : !column.autoIncrement && !column.generated && column.default.trim() === '',
      isNull: mode === 'edit' && !primary ? raw === null : false,
      value,
      originalValue: value,
      originalIsNull: raw === null,
      truncated: mode === 'edit' && raw !== null && raw.endsWith('… [truncated]'),
    };
  });
}

export function tableRowKey(
  table: TableInfo,
  columns: { name: string }[],
  row: (string | null)[],
): Record<string, string | null> | null {
  const key: Record<string, string | null> = {};
  for (const primaryKey of table.primaryKey) {
    const index = columns.findIndex(
      (column) => column.name.toLowerCase() === primaryKey.toLowerCase(),
    );
    if (index < 0) {
      return null;
    }
    key[primaryKey] = row[index];
  }
  return Object.keys(key).length > 0 ? key : null;
}

export function isEditableTable(table: TableInfo): boolean {
  return table.type.toUpperCase().includes('TABLE') && table.primaryKey.length > 0;
}
