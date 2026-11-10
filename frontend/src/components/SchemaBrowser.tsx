import { ChevronDown, ChevronRight, KeyRound, Table2, Terminal } from 'lucide-react';
import { useMemo, useState } from 'react';
import type { SchemaSnapshot, TableInfo } from '../types';

interface SchemaBrowserProps {
  schema: SchemaSnapshot | null;
  loading: boolean;
  error: string;
  selectedKey?: string | null;
  onOpenTable: (table: TableInfo) => void;
  onInsertSelect?: (table: TableInfo) => void;
}

export function SchemaBrowser({
  schema,
  loading,
  error,
  selectedKey,
  onOpenTable,
  onInsertSelect,
}: SchemaBrowserProps) {
  const [expanded, setExpanded] = useState<Set<string>>(new Set());

  const groupedTables = useMemo(() => {
    const groups = new Map<string, TableInfo[]>();
    for (const table of schema?.tables ?? []) {
      const bucket = groups.get(table.schema) ?? [];
      bucket.push(table);
      groups.set(table.schema, bucket);
    }
    return Array.from(groups.entries());
  }, [schema]);

  const toggleTable = (key: string) => {
    setExpanded((current) => {
      const next = new Set(current);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
      }
      return next;
    });
  };

  if (loading && !schema) {
    return (
      <div className="space-y-2 p-2">
        {[0, 1, 2, 3, 4].map((row) => (
          <div key={row} className="h-6 animate-pulse rounded bg-white/[0.04]" />
        ))}
      </div>
    );
  }
  if (error) {
    return <div className="p-2 text-[11px] leading-relaxed text-rose-300/80">{error}</div>;
  }
  if (groupedTables.length === 0) {
    return (
      <p className="px-2 py-4 text-[11px] leading-relaxed text-slate-600">
        No user tables yet. Create one with SQL and refresh.
      </p>
    );
  }

  return (
    <div>
      {groupedTables.map(([schemaName, tables]) => (
        <div key={schemaName} className="mb-2">
          <div className="px-2 py-1 text-[9px] font-semibold uppercase tracking-[0.16em] text-slate-600">
            {schemaName}
          </div>
          {tables.map((table) => {
            const key = `${table.schema}.${table.name}`;
            const open = expanded.has(key);
            const selected = selectedKey === key;
            return (
              <div key={key}>
                <div
                  className={`group flex items-center rounded-lg transition-colors ${
                    selected ? 'bg-mint-400/[0.08]' : 'hover:bg-white/[0.03]'
                  }`}
                >
                  <button
                    type="button"
                    className="flex h-7 w-6 flex-none items-center justify-center text-slate-600 hover:text-slate-300"
                    onClick={() => toggleTable(key)}
                    aria-label={open ? 'Collapse table columns' : 'Expand table columns'}
                    aria-expanded={open}
                  >
                    {open ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
                  </button>
                  <button
                    type="button"
                    className="flex min-w-0 flex-1 items-center gap-1.5 py-1.5 text-left"
                    onClick={() => onOpenTable(table)}
                    title={`Open data editor for ${table.schema}.${table.name}`}
                  >
                    <Table2
                      size={12}
                      className={`flex-none ${selected ? 'text-mint-300' : 'text-slate-500'}`}
                    />
                    <span
                      className={`truncate text-[11px] ${
                        selected ? 'text-slate-100' : 'text-slate-300'
                      }`}
                    >
                      {table.name}
                    </span>
                    {table.primaryKey.length > 0 ? (
                      <KeyRound
                        size={9}
                        className="flex-none text-amber-400/50"
                        aria-label="Has a primary key"
                      />
                    ) : null}
                    <span className="ml-auto text-[9px] text-slate-600">{table.columns.length}</span>
                  </button>
                  {onInsertSelect ? (
                    <button
                      type="button"
                      className="icon-btn mr-1 h-6 w-6 opacity-0 group-hover:opacity-100"
                      onClick={() => onInsertSelect(table)}
                      title="Insert SELECT into the SQL workbench"
                      aria-label={`Insert SELECT for ${table.name}`}
                    >
                      <Terminal size={11} />
                    </button>
                  ) : null}
                </div>
                {open ? (
                  <div className="mb-1 ml-[15px] border-l border-white/[0.05] pl-2.5">
                    {table.columns.map((column) => {
                      const primary = table.primaryKey.some(
                        (name) => name.toLowerCase() === column.name.toLowerCase(),
                      );
                      return (
                        <div key={column.name} className="flex items-center gap-1.5 py-1">
                          {primary ? (
                            <KeyRound size={8} className="flex-none text-amber-400/50" />
                          ) : null}
                          <span className="truncate text-[10px] text-slate-400">{column.name}</span>
                          <span className="ml-auto truncate font-mono text-[9px] text-slate-600">
                            {column.type}
                          </span>
                          {!column.nullable ? (
                            <span className="text-[9px] text-amber-400/70" title="NOT NULL">
                              *
                            </span>
                          ) : null}
                        </div>
                      );
                    })}
                  </div>
                ) : null}
              </div>
            );
          })}
        </div>
      ))}
    </div>
  );
}
