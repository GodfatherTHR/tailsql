import {
  AlertTriangle,
  CheckCircle2,
  Database,
  Eraser,
  Loader2,
  Play,
  RefreshCw,
  Table2,
  Terminal,
} from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import type { Instance, QueryResult, SchemaSnapshot, TableInfo } from '../types';
import { api, errorMessage } from '../lib/api';
import { formatDuration, formatTime } from '../lib/format';
import { SchemaBrowser } from './SchemaBrowser';

interface SqlWorkbenchViewProps {
  instance: Instance;
  busy: string | null;
  onStart: () => void;
  onOpenTable: (table: TableInfo) => void;
}

const sampleQueries = [
  { label: 'SELECT 1', sql: 'SELECT 1;' },
  { label: 'SHOW TABLES', sql: 'SHOW TABLES;' },
  { label: 'INFORMATION_SCHEMA', sql: 'SELECT * FROM INFORMATION_SCHEMA.TABLES LIMIT 20;' },
];

export function SqlWorkbenchView({ instance, busy, onStart, onOpenTable }: SqlWorkbenchViewProps) {
  const [query, setQuery] = useState('SELECT 1;');
  const [result, setResult] = useState<QueryResult | null>(null);
  const [error, setError] = useState('');
  const [running, setRunning] = useState(false);
  const [schema, setSchema] = useState<SchemaSnapshot | null>(null);
  const [schemaError, setSchemaError] = useState('');
  const [schemaLoading, setSchemaLoading] = useState(false);
  const [schemaToken, setSchemaToken] = useState(0);
  const editorRef = useRef<HTMLTextAreaElement>(null);

  const live = instance.status === 'running';
  const starting = instance.status === 'starting';

  useEffect(() => {
    setQuery('SELECT 1;');
    setResult(null);
    setError('');
  }, [instance.id]);

  useEffect(() => {
    let cancelled = false;
    if (!live) {
      setSchema(null);
      setSchemaError('');
      setSchemaLoading(false);
      return;
    }
    setSchemaLoading(true);
    setSchemaError('');
    api
      .listTables(instance.id)
      .then((snapshot) => {
        if (!cancelled) {
          setSchema(snapshot);
        }
      })
      .catch((loadError) => {
        if (!cancelled) {
          setSchemaError(errorMessage(loadError));
        }
      })
      .finally(() => {
        if (!cancelled) {
          setSchemaLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [instance.id, instance.status, live, schemaToken]);

  const runQuery = async () => {
    if (!query.trim() || running || !live) {
      return;
    }
    setRunning(true);
    setError('');
    try {
      const response = await api.executeQuery(instance.id, query);
      setResult(response);
      if (/^\s*(create|alter|drop|truncate|rename)\b/i.test(query)) {
        setSchemaToken((value) => value + 1);
      }
    } catch (queryError) {
      setError(errorMessage(queryError));
      setResult(null);
    } finally {
      setRunning(false);
    }
  };

  const selectTable = (table: TableInfo) => {
    const escapedSchema = table.schema.replace(/"/g, '""');
    const escapedName = table.name.replace(/"/g, '""');
    setQuery(`SELECT * FROM "${escapedSchema}"."${escapedName}" LIMIT 100;`);
    editorRef.current?.focus();
  };

  return (
    <div className="space-y-5 animate-fade-in">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <p className="label mb-1">Scratchpad</p>
          <h1 className="text-xl font-semibold tracking-tight text-slate-100">SQL Workbench</h1>
          <p className="mt-1 text-xs text-slate-500">
            Run one statement at a time against <span className="font-mono text-slate-400">{instance.name}</span>.
            Results are capped at 500 rows.
          </p>
        </div>
        <button
          type="button"
          className="btn-secondary"
          onClick={() => setSchemaToken((value) => value + 1)}
          disabled={!live || schemaLoading}
        >
          <RefreshCw size={14} className={schemaLoading ? 'animate-spin' : ''} />
          Refresh schema
        </button>
      </div>

      {!live ? (
        <div className="card flex flex-col items-center justify-center px-6 py-16 text-center">
          <div className="mb-4 flex h-12 w-12 items-center justify-center rounded-xl border border-white/[0.07] bg-white/[0.03]">
            {starting ? (
              <Loader2 size={20} className="animate-spin text-mint-300" />
            ) : (
              <Database size={20} className="text-slate-400" />
            )}
          </div>
          <h2 className="text-sm font-semibold text-slate-200">
            {starting ? 'Database is starting…' : 'Start the database to run SQL'}
          </h2>
          <p className="mt-1 max-w-sm text-xs leading-relaxed text-slate-500">
            The workbench connects over loopback with the instance credentials and reads its schema
            through JDBC metadata.
          </p>
          {!starting ? (
            <button
              type="button"
              className="btn-primary mt-5"
              onClick={onStart}
              disabled={Boolean(busy) || instance.status === 'stopping'}
            >
              {busy ? <Loader2 size={14} className="animate-spin" /> : <Play size={14} />}
              Start database
            </button>
          ) : null}
        </div>
      ) : (
        <div className="grid items-start gap-5 xl:grid-cols-[280px_minmax(0,1fr)]">
          <div className="card flex max-h-[780px] min-h-[420px] flex-col overflow-hidden">
            <div className="flex items-center justify-between border-b border-white/[0.06] px-3.5 py-2.5">
              <div className="flex items-center gap-2 text-xs font-semibold text-slate-300">
                <Table2 size={13} className="text-slate-500" />
                Schema
              </div>
              <span className="text-[10px] text-slate-600">{schema?.tables.length ?? 0} tables</span>
            </div>
            <div className="flex-1 overflow-y-auto p-2">
              <SchemaBrowser
                schema={schema}
                loading={schemaLoading}
                error={schemaError}
                onOpenTable={onOpenTable}
                onInsertSelect={selectTable}
              />
            </div>
          </div>

          <div className="min-w-0 space-y-4">
            <div className="card overflow-hidden">
              <div className="flex items-center justify-between border-b border-white/[0.06] px-4 py-2.5">
                <div className="flex items-center gap-2 text-xs font-semibold text-slate-300">
                  <Terminal size={13} className="text-slate-500" />
                  Query
                </div>
                <span className="text-[10px] text-slate-600">
                  <kbd className="rounded border border-white/10 bg-white/[0.03] px-1 py-0.5 font-mono text-[9px]">Ctrl</kbd>
                  <span className="mx-1">+</span>
                  <kbd className="rounded border border-white/10 bg-white/[0.03] px-1 py-0.5 font-mono text-[9px]">Enter</kbd>
                  <span className="ml-1.5">to run</span>
                </span>
              </div>
              <textarea
                ref={editorRef}
                className="h-[190px] w-full resize-y bg-transparent px-4 py-3 font-mono text-[12.5px] leading-relaxed text-slate-200 placeholder:text-slate-700 focus:outline-none"
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                onKeyDown={(event) => {
                  if ((event.ctrlKey || event.metaKey) && event.key === 'Enter') {
                    event.preventDefault();
                    void runQuery();
                  }
                }}
                placeholder="SELECT * FROM ..."
                spellCheck={false}
                aria-label="SQL query"
              />
              <div className="flex flex-wrap items-center justify-between gap-2 border-t border-white/[0.06] px-3 py-2.5">
                <div className="flex flex-wrap items-center gap-1.5">
                  {sampleQueries.map((sample) => (
                    <button
                      key={sample.label}
                      type="button"
                      className="btn-ghost btn-xs"
                      onClick={() => setQuery(sample.sql)}
                    >
                      {sample.label}
                    </button>
                  ))}
                </div>
                <div className="flex items-center gap-2">
                  <button
                    type="button"
                    className="btn-ghost btn-xs"
                    onClick={() => {
                      setQuery('');
                      setResult(null);
                      setError('');
                      editorRef.current?.focus();
                    }}
                    disabled={running}
                  >
                    <Eraser size={12} />
                    Clear
                  </button>
                  <button
                    type="button"
                    className="btn-primary"
                    onClick={() => void runQuery()}
                    disabled={running || !query.trim()}
                  >
                    {running ? <Loader2 size={14} className="animate-spin" /> : <Play size={14} />}
                    {running ? 'Running…' : 'Run query'}
                  </button>
                </div>
              </div>
            </div>

            <div className="card overflow-hidden">
              <div className="flex flex-wrap items-center justify-between gap-2 border-b border-white/[0.06] px-4 py-2.5">
                <div className="flex items-center gap-2 text-xs font-semibold text-slate-300">
                  <Table2 size={13} className="text-slate-500" />
                  Results
                </div>
                {result ? (
                  <div className="flex flex-wrap items-center gap-2 text-[10px] text-slate-500">
                    <span>{formatDuration(result.durationMs)}</span>
                    <span className="text-slate-700">/</span>
                    <span>engine {formatDuration(result.engineDurationMs)}</span>
                    <span className="text-slate-700">/</span>
                    <span>{formatTime(result.executedAt)}</span>
                  </div>
                ) : null}
              </div>

              {error ? (
                <div className="flex gap-3 border-b border-rose-500/10 bg-rose-500/[0.04] px-4 py-3.5">
                  <AlertTriangle size={16} className="mt-0.5 flex-none text-rose-300" />
                  <pre className="max-h-48 flex-1 overflow-auto whitespace-pre-wrap font-mono text-[11px] leading-relaxed text-rose-200/80">
                    {error}
                  </pre>
                </div>
              ) : null}

              {result ? (
                result.hasResultSet ? (
                  <>
                    <div className="flex flex-wrap items-center gap-2 border-b border-white/[0.04] px-4 py-2 text-[10px] text-slate-500">
                      <span className="chip">{result.rowCount} rows</span>
                      <span className="chip">{result.columns.length} columns</span>
                      {result.truncated ? (
                        <span className="chip border-amber-400/20 text-amber-300">capped at 500 rows</span>
                      ) : null}
                    </div>
                    <div className="max-h-[460px] overflow-auto">
                      <table className="w-full min-w-max">
                        <thead>
                          <tr>
                            <th className="table-head w-10 text-right">#</th>
                            {result.columns.map((column) => (
                              <th key={column.name} className="table-head">
                                <div className="text-slate-300">{column.name}</div>
                                <div className="mt-0.5 font-mono text-[9px] font-normal normal-case tracking-normal text-slate-600">
                                  {column.type}
                                  {column.nullable ? '' : ' · not null'}
                                </div>
                              </th>
                            ))}
                          </tr>
                        </thead>
                        <tbody>
                          {result.rows.length === 0 ? (
                            <tr>
                              <td
                                className="px-4 py-6 text-center text-[11px] text-slate-600"
                                colSpan={result.columns.length + 1}
                              >
                                The query returned zero rows.
                              </td>
                            </tr>
                          ) : (
                            result.rows.map((row, rowIndex) => (
                              <tr key={rowIndex} className="transition-colors hover:bg-white/[0.02]">
                                <td className="table-cell text-right text-slate-700">{rowIndex + 1}</td>
                                {row.map((value, columnIndex) => (
                                  <td className="table-cell" key={`${rowIndex}-${columnIndex}`}>
                                    {value === null ? (
                                      <span className="font-mono text-[10px] italic text-slate-600">NULL</span>
                                    ) : (
                                      <span
                                        className="block max-w-[360px] truncate"
                                        title={value}
                                      >
                                        {value}
                                      </span>
                                    )}
                                  </td>
                                ))}
                              </tr>
                            ))
                          )}
                        </tbody>
                      </table>
                    </div>
                  </>
                ) : (
                  <div className="flex items-center gap-3 px-4 py-8">
                    <CheckCircle2 size={20} className="flex-none text-mint-300" />
                    <div>
                      <div className="text-xs font-semibold text-slate-200">Statement executed</div>
                      <p className="mt-0.5 text-[11px] text-slate-500">
                        {result.updateCount >= 0
                          ? `${result.updateCount} row${result.updateCount === 1 ? '' : 's'} affected.`
                          : 'The statement completed without a result set.'}
                      </p>
                    </div>
                  </div>
                )
              ) : !error ? (
                <div className="flex flex-col items-center justify-center px-6 py-12 text-center">
                  <Table2 size={24} className="mb-3 text-slate-700" />
                  <p className="text-xs text-slate-500">Run a query to inspect its results here.</p>
                  <p className="mt-1 text-[10px] text-slate-700">
                    Use the schema tree to insert a SELECT for any table.
                  </p>
                </div>
              ) : null}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
