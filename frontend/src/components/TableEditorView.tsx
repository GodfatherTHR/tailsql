import {
  AlertTriangle,
  ArrowDown,
  ArrowUp,
  ArrowUpDown,
  ChevronLeft,
  ChevronRight,
  Database,
  KeyRound,
  Loader2,
  Pencil,
  Play,
  Plus,
  RefreshCw,
  Sparkles,
  Table2,
  Trash2,
} from 'lucide-react';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type {
  Instance,
  RealtimeChangeEvent,
  SchemaSnapshot,
  TableInfo,
  TableRowsResult,
} from '../types';
import { api, errorMessage } from '../lib/api';
import { formatTime } from '../lib/format';
import type { ToastKind } from '../hooks/useToasts';
import { ConfirmDialog } from './ConfirmDialog';
import { Modal } from './Modal';
import { RowEditorModal, isEditableTable, tableRowKey } from './RowEditorModal';
import { SchemaBrowser } from './SchemaBrowser';

interface TableEditorViewProps {
  instance: Instance;
  busy: string | null;
  onStart: () => void;
  initialTable: { schema: string; name: string } | null;
  realtimeEvent: RealtimeChangeEvent | null;
  onToast: (kind: ToastKind, message: string) => void;
}

interface EditorState {
  mode: 'insert' | 'edit';
  row: (string | null)[] | null;
}

interface DeleteTarget {
  key: Record<string, string | null>;
  label: string;
}

const pageSizes = [25, 50, 100];

export function TableEditorView({
  instance,
  busy,
  onStart,
  initialTable,
  realtimeEvent,
  onToast,
}: TableEditorViewProps) {
  const [schema, setSchema] = useState<SchemaSnapshot | null>(null);
  const [schemaError, setSchemaError] = useState('');
  const [schemaLoading, setSchemaLoading] = useState(false);
  const [schemaToken, setSchemaToken] = useState(0);
  const [selectedKey, setSelectedKey] = useState<string | null>(null);

  const [rows, setRows] = useState<TableRowsResult | null>(null);
  const [rowsError, setRowsError] = useState('');
  const [rowsLoading, setRowsLoading] = useState(false);
  const [rowsToken, setRowsToken] = useState(0);
  const [offset, setOffset] = useState(0);
  const [limit, setLimit] = useState(50);
  const [orderBy, setOrderBy] = useState('');
  const [descending, setDescending] = useState(false);

  const [editor, setEditor] = useState<EditorState | null>(null);
  const [editorError, setEditorError] = useState('');
  const [saving, setSaving] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<DeleteTarget | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [dropTarget, setDropTarget] = useState<TableInfo | null>(null);
  const [dropping, setDropping] = useState(false);
  const [seedOpen, setSeedOpen] = useState(false);
  const [seedCount, setSeedCount] = useState(10);
  const [seeding, setSeeding] = useState(false);
  const [seedError, setSeedError] = useState('');
  const [lastRealtimeAt, setLastRealtimeAt] = useState<string | null>(null);

  const sortInitializedFor = useRef<string | null>(null);
  const handledRealtime = useRef('');
  const schemaRefreshAt = useRef(0);

  const live = instance.status === 'running';
  const starting = instance.status === 'starting';

  useEffect(() => {
    setSchema(null);
    setSchemaError('');
    setSelectedKey(null);
    setRows(null);
    setRowsError('');
    setOffset(0);
    setOrderBy('');
    setDescending(false);
    setEditor(null);
    setDeleteTarget(null);
    setDropTarget(null);
    setSeedOpen(false);
    setSeedError('');
    setLastRealtimeAt(null);
    sortInitializedFor.current = null;
    handledRealtime.current = '';
  }, [instance.id]);

  useEffect(() => {
    if (!initialTable) {
      return;
    }
    const nextKey = `${initialTable.schema}.${initialTable.name}`;
    setSelectedKey(nextKey);
    storeTableSelection(instance.id, nextKey);
    sortInitializedFor.current = null;
    setOffset(0);
  }, [initialTable?.schema, initialTable?.name, instance.id]);

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

  useEffect(() => {
    if (!schema) {
      return;
    }
    const selectedExists = selectedKey
      ? schema.tables.some((table) => tableKey(table) === selectedKey)
      : false;
    if (selectedExists) {
      return;
    }
    const stored = readTableSelection(instance.id);
    const restored = stored
      ? schema.tables.find((table) => tableKey(table) === stored) ?? null
      : null;
    const target = restored ?? schema.tables[0] ?? null;
    if (!target) {
      setSelectedKey(null);
      return;
    }
    const nextKey = tableKey(target);
    setSelectedKey(nextKey);
    storeTableSelection(instance.id, nextKey);
    setOffset(0);
  }, [schema, selectedKey, instance.id]);

  const selectedTable = useMemo(() => {
    if (!schema || !selectedKey) {
      return null;
    }
    return schema.tables.find((table) => tableKey(table) === selectedKey) ?? null;
  }, [schema, selectedKey]);

  useEffect(() => {
    if (!selectedKey || !selectedTable) {
      return;
    }
    if (sortInitializedFor.current === selectedKey) {
      return;
    }
    sortInitializedFor.current = selectedKey;
    const primary = selectedTable.primaryKey[0] ?? '';
    setOrderBy(primary);
    setDescending(primary !== '');
    setOffset(0);
  }, [selectedKey, selectedTable]);

  const writable = selectedTable ? selectedTable.type.toUpperCase().includes('TABLE') : false;
  const editable = selectedTable ? isEditableTable(selectedTable) : false;

  useEffect(() => {
    let cancelled = false;
    if (!live || !selectedTable) {
      setRows(null);
      setRowsError('');
      setRowsLoading(false);
      return;
    }
    setRowsLoading(true);
    setRowsError('');
    api
      .getTableRows(instance.id, selectedTable, { limit, offset, orderBy, descending })
      .then((result) => {
        if (!cancelled) {
          setRows(result);
        }
      })
      .catch((loadError) => {
        if (!cancelled) {
          setRows(null);
          setRowsError(errorMessage(loadError));
        }
      })
      .finally(() => {
        if (!cancelled) {
          setRowsLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [
    instance.id,
    instance.status,
    live,
    selectedTable,
    limit,
    offset,
    orderBy,
    descending,
    rowsToken,
  ]);

  useEffect(() => {
    if (!realtimeEvent || realtimeEvent.instanceId !== instance.id) {
      return;
    }
    const eventKey = `${realtimeEvent.at}|${realtimeEvent.operation}|${realtimeEvent.schema ?? ''}|${realtimeEvent.table ?? ''}`;
    if (handledRealtime.current === eventKey) {
      return;
    }
    handledRealtime.current = eventKey;
    setLastRealtimeAt(realtimeEvent.at);

    const changedTable = realtimeEvent.table?.toLowerCase() ?? '';
    const matchesSelection =
      changedTable === '' ||
      (selectedTable !== null &&
        changedTable === selectedTable.name.toLowerCase() &&
        (!realtimeEvent.schema ||
          realtimeEvent.schema.toLowerCase() === selectedTable.schema.toLowerCase()));
    if (matchesSelection) {
      setRowsToken((value) => value + 1);
    }
    if (
      (changedTable === '' || realtimeEvent.operation === 'ddl') &&
      Date.now() - schemaRefreshAt.current > 4000
    ) {
      schemaRefreshAt.current = Date.now();
      setSchemaToken((value) => value + 1);
    }
  }, [realtimeEvent, instance.id, selectedTable]);

  const selectTable = useCallback(
    (table: TableInfo) => {
      const nextKey = tableKey(table);
      setSelectedKey(nextKey);
      storeTableSelection(instance.id, nextKey);
      setOffset(0);
      setRows(null);
      setRowsError('');
    },
    [instance.id],
  );

  const handleSort = (columnName: string) => {
    if (orderBy.toLowerCase() === columnName.toLowerCase()) {
      if (!descending) {
        setDescending(true);
      } else {
        setOrderBy('');
        setDescending(false);
      }
    } else {
      setOrderBy(columnName);
      setDescending(false);
    }
    setOffset(0);
  };

  const openInsert = () => {
    setEditorError('');
    setEditor({ mode: 'insert', row: null });
  };

  const openEdit = (row: (string | null)[]) => {
    if (!selectedTable || !editable) {
      return;
    }
    setEditorError('');
    setEditor({ mode: 'edit', row });
  };

  const submitEditor = async (values: Record<string, string | null>) => {
    if (!selectedTable || !editor) {
      return;
    }
    if (editor.mode === 'edit') {
      if (!rows) {
        setEditorError('Rows are no longer loaded. Refresh the table and try again.');
        return;
      }
      const key = tableRowKey(selectedTable, rows.columns, editor.row ?? []);
      if (!key) {
        setEditorError('This row has no usable primary key and cannot be updated.');
        return;
      }
      setSaving(true);
      setEditorError('');
      try {
        const result = await api.updateTableRow(instance.id, selectedTable, key, values);
        onToast('success', `Updated ${result.updateCount} row${result.updateCount === 1 ? '' : 's'}.`);
        setEditor(null);
        setRowsToken((value) => value + 1);
      } catch (mutationError) {
        setEditorError(errorMessage(mutationError));
      } finally {
        setSaving(false);
      }
      return;
    }

    setSaving(true);
    setEditorError('');
    try {
      const result = await api.insertTableRow(instance.id, selectedTable, values);
      onToast('success', `Inserted ${result.updateCount} row${result.updateCount === 1 ? '' : 's'}.`);
      setEditor(null);
      const primary = selectedTable.primaryKey[0];
      if (primary) {
        setOrderBy(primary);
        setDescending(true);
      }
      setOffset(0);
      setRowsToken((value) => value + 1);
    } catch (mutationError) {
      setEditorError(errorMessage(mutationError));
    } finally {
      setSaving(false);
    }
  };

  const confirmDelete = async () => {
    if (!selectedTable || !deleteTarget) {
      return;
    }
    setDeleting(true);
    try {
      const result = await api.deleteTableRow(instance.id, selectedTable, deleteTarget.key);
      onToast('success', `Deleted ${result.updateCount} row${result.updateCount === 1 ? '' : 's'}.`);
      setDeleteTarget(null);
      if (rows && rows.rows.length === 1 && offset > 0) {
        setOffset(Math.max(0, offset - limit));
      }
      setRowsToken((value) => value + 1);
    } catch (mutationError) {
      onToast('error', errorMessage(mutationError));
    } finally {
      setDeleting(false);
    }
  };

  const confirmDropTable = async () => {
    if (!dropTarget) {
      return;
    }
    setDropping(true);
    try {
      await api.dropTable(instance.id, dropTarget);
      onToast('success', `Dropped ${dropTarget.schema}.${dropTarget.name}.`);
      if (selectedTable && tableKey(selectedTable) === tableKey(dropTarget)) {
        setSelectedKey(null);
        setRows(null);
        setRowsError('');
        setOffset(0);
      }
      setDropTarget(null);
      setSchemaToken((value) => value + 1);
    } catch (dropError) {
      onToast('error', errorMessage(dropError));
    } finally {
      setDropping(false);
    }
  };

  const seedRows = async () => {
    if (!selectedTable) {
      return;
    }
    setSeeding(true);
    setSeedError('');
    try {
      const result = await api.seedTableRows(instance.id, selectedTable, seedCount);
      onToast(
        'success',
        `Inserted ${result.inserted} row${result.inserted === 1 ? '' : 's'} (${result.generatedBy}).`,
      );
      setSeedOpen(false);
      setOffset(0);
      setRowsToken((value) => value + 1);
    } catch (seedRunError) {
      setSeedError(errorMessage(seedRunError));
      onToast('error', errorMessage(seedRunError));
    } finally {
      setSeeding(false);
    }
  };

  const refreshAll = () => {
    setSchemaToken((value) => value + 1);
    setRowsToken((value) => value + 1);
  };

  const rowStart = rows && rows.rows.length > 0 ? offset + 1 : 0;
  const rowEnd = rows ? offset + rows.rows.length : 0;

  return (
    <div className="space-y-5 animate-fade-in">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <p className="label mb-1">Table editor</p>
          <h1 className="text-xl font-semibold tracking-tight text-slate-100">Browse &amp; edit data</h1>
          <p className="mt-1 max-w-xl text-xs leading-relaxed text-slate-500">
            Click a table in the schema tree to inspect its rows. Changes are applied as normal H2
            SQL statements against <span className="font-mono text-slate-400">{instance.name}</span>{' '}
            and committed to disk immediately. Other clients&apos; writes appear here live.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {selectedTable ? (
            <span className="chip">
              <Table2 size={11} />
              {selectedTable.schema}.{selectedTable.name}
            </span>
          ) : null}
          {live ? (
            <span
              className="chip border-mint-400/20 text-mint-300"
              title={
                lastRealtimeAt
                  ? `Last database change at ${formatTime(lastRealtimeAt)}`
                  : 'Watching this database for changes from every connected client'
              }
            >
              <span className="relative flex h-1.5 w-1.5">
                <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-mint-300 opacity-60" />
                <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-mint-300" />
              </span>
              Live
            </span>
          ) : null}
          <button
            type="button"
            className="btn-secondary"
            onClick={refreshAll}
            disabled={!live || schemaLoading || rowsLoading}
          >
            <RefreshCw size={14} className={schemaLoading || rowsLoading ? 'animate-spin' : ''} />
            Refresh
          </button>
          <button
            type="button"
            className="btn-secondary border-rose-500/20 text-rose-300 hover:border-rose-500/40 hover:bg-rose-500/10"
            onClick={() => {
              if (selectedTable) {
                setDropTarget(selectedTable);
              }
            }}
            disabled={!live || !selectedTable || !writable}
            title={
              selectedTable && writable
                ? `Drop ${selectedTable.schema}.${selectedTable.name} and all of its rows`
                : 'Select a base table to drop'
            }
          >
            <Trash2 size={14} />
            Drop table
          </button>
          <button
            type="button"
            className="btn-secondary border-mint-400/20 text-mint-300 hover:border-mint-400/40 hover:bg-mint-400/10"
            onClick={() => {
              setSeedError('');
              setSeedOpen(true);
            }}
            disabled={!live || !selectedTable || !writable}
            title={
              selectedTable && writable
                ? `Generate realistic rows for ${selectedTable.schema}.${selectedTable.name}`
                : 'Select a base table to seed rows'
            }
          >
            <Sparkles size={14} />
            Seed rows
          </button>
          <button
            type="button"
            className="btn-primary"
            onClick={openInsert}
            disabled={!live || !selectedTable || !writable}
          >
            <Plus size={14} />
            Add row
          </button>
        </div>
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
            {starting ? 'Database is starting…' : 'Start the database to browse rows'}
          </h2>
          <p className="mt-1 max-w-sm text-xs leading-relaxed text-slate-500">
            The table editor connects over loopback with the instance credentials and reads table
            metadata through JDBC.
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
          <div className="card flex max-h-[820px] min-h-[460px] flex-col overflow-hidden">
            <div className="flex items-center justify-between border-b border-white/[0.06] px-3.5 py-2.5">
              <div className="flex items-center gap-2 text-xs font-semibold text-slate-300">
                <Table2 size={13} className="text-slate-500" />
                Schema
              </div>
              <span className="text-[10px] text-slate-600">
                {schema?.tables.length ?? 0} tables
              </span>
            </div>
            <div className="flex-1 overflow-y-auto p-2">
              <SchemaBrowser
                schema={schema}
                loading={schemaLoading}
                error={schemaError}
                selectedKey={selectedKey}
                onOpenTable={selectTable}
              />
            </div>
          </div>

          <div className="min-w-0 space-y-4">
            {!selectedTable ? (
              <div className="card flex flex-col items-center justify-center px-6 py-16 text-center">
                <Table2 size={24} className="mb-3 text-slate-700" />
                <p className="text-xs text-slate-500">
                  Select a table from the schema tree to browse its data.
                </p>
              </div>
            ) : (
              <>
                <div className="card overflow-hidden">
                  <div className="flex flex-wrap items-center justify-between gap-3 border-b border-white/[0.06] px-4 py-3">
                    <div className="flex min-w-0 flex-wrap items-center gap-2">
                      <div className="flex h-8 w-8 flex-none items-center justify-center rounded-lg border border-white/[0.07] bg-white/[0.03]">
                        <Table2 size={14} className="text-mint-300" />
                      </div>
                      <span className="truncate font-mono text-xs text-slate-200">
                        {selectedTable.schema}.{selectedTable.name}
                      </span>
                      <span className="chip px-2 py-0.5 text-[9px]">{selectedTable.type}</span>
                      {selectedTable.primaryKey.length > 0 ? (
                        <span className="chip px-2 py-0.5 text-[9px] text-amber-300/90">
                          <KeyRound size={9} />
                          {selectedTable.primaryKey.join(', ')}
                        </span>
                      ) : (
                        <span className="chip border-amber-400/20 px-2 py-0.5 text-[9px] text-amber-300/90">
                          <AlertTriangle size={9} />
                          no primary key
                        </span>
                      )}
                    </div>
                    <div className="flex items-center gap-2 text-[10px] text-slate-500">
                      {rows ? (
                        <>
                          <span>
                            Rows {rowStart}–{rowEnd}
                          </span>
                          {rows.hasMore ? <span className="chip px-1.5 py-0.5 text-[9px]">more</span> : null}
                        </>
                      ) : rowsLoading ? (
                        <Loader2 size={12} className="animate-spin" />
                      ) : null}
                      <select
                        className="rounded-md border border-white/10 bg-ink-950/70 px-2 py-1 text-[10px] text-slate-400 focus:border-mint-400/40 focus:outline-none"
                        value={limit}
                        onChange={(event) => {
                          setLimit(Number(event.target.value));
                          setOffset(0);
                        }}
                        aria-label="Rows per page"
                      >
                        {pageSizes.map((size) => (
                          <option key={size} value={size}>
                            {size} / page
                          </option>
                        ))}
                      </select>
                    </div>
                  </div>

                  {!writable ? (
                    <div className="flex gap-2.5 border-b border-sky-400/15 bg-sky-400/[0.04] px-4 py-3 text-[11px] leading-relaxed text-slate-400">
                      <AlertTriangle size={14} className="mt-0.5 flex-none text-sky-300" />
                      This object is a view or otherwise not a base table. It is read-only in the
                      table editor.
                    </div>
                  ) : !editable ? (
                    <div className="flex gap-2.5 border-b border-amber-400/20 bg-amber-400/[0.05] px-4 py-3 text-[11px] leading-relaxed text-amber-200/80">
                      <AlertTriangle size={14} className="mt-0.5 flex-none text-amber-300" />
                      This table has no primary key. You can insert rows, but updating and deleting
                      rows are disabled because a row cannot be identified safely.
                    </div>
                  ) : null}

                  {rowsError ? (
                    <div className="flex gap-3 border-b border-rose-500/10 bg-rose-500/[0.04] px-4 py-3.5">
                      <AlertTriangle size={16} className="mt-0.5 flex-none text-rose-300" />
                      <pre className="max-h-40 flex-1 overflow-auto whitespace-pre-wrap font-mono text-[11px] leading-relaxed text-rose-200/80">
                        {rowsError}
                      </pre>
                    </div>
                  ) : null}

                  {rowsLoading && !rows ? (
                    <div className="space-y-2 p-4">
                      {[0, 1, 2, 3, 4].map((row) => (
                        <div key={row} className="h-8 animate-pulse rounded bg-white/[0.035]" />
                      ))}
                    </div>
                  ) : rows ? (
                    rows.columns.length === 0 ? (
                      <div className="px-4 py-10 text-center text-[11px] text-slate-600">
                        No columns were returned for this table.
                      </div>
                    ) : (
                      <div className="max-h-[560px] overflow-auto">
                        <table className="w-full min-w-max">
                          <thead>
                            <tr>
                              <th className="table-head w-10 text-right">#</th>
                              {rows.columns.map((column) => {
                                const active = orderBy.toLowerCase() === column.name.toLowerCase();
                                return (
                                  <th key={column.name} className="table-head">
                                    <button
                                      type="button"
                                      className="flex items-center gap-1 text-left"
                                      onClick={() => handleSort(column.name)}
                                      title={`Sort by ${column.name}`}
                                    >
                                      <span className={active ? 'text-slate-200' : 'text-slate-300'}>
                                        {column.name}
                                      </span>
                                      {active ? (
                                        descending ? (
                                          <ArrowDown size={10} className="text-mint-300" />
                                        ) : (
                                          <ArrowUp size={10} className="text-mint-300" />
                                        )
                                      ) : (
                                        <ArrowUpDown size={9} className="text-slate-700" />
                                      )}
                                    </button>
                                    <div className="mt-0.5 font-mono text-[9px] font-normal normal-case tracking-normal text-slate-600">
                                      {column.type}
                                      {column.nullable ? '' : ' · not null'}
                                    </div>
                                  </th>
                                );
                              })}
                              <th className="table-head w-20 text-right">Actions</th>
                            </tr>
                          </thead>
                          <tbody>
                            {rows.rows.length === 0 ? (
                              <tr>
                                <td
                                  className="px-4 py-10 text-center text-[11px] text-slate-600"
                                  colSpan={rows.columns.length + 2}
                                >
                                  This table is empty. {writable ? 'Add the first row.' : ''}
                                </td>
                              </tr>
                            ) : (
                              rows.rows.map((row, rowIndex) => {
                                const key = tableRowKey(selectedTable, rows.columns, row);
                                return (
                                  <tr
                                    key={rowIndex}
                                    className="group transition-colors hover:bg-white/[0.02]"
                                    onDoubleClick={() => {
                                      if (editable) {
                                        openEdit(row);
                                      }
                                    }}
                                  >
                                    <td className="table-cell text-right text-slate-700">
                                      {offset + rowIndex + 1}
                                    </td>
                                    {row.map((value, columnIndex) => (
                                      <td className="table-cell" key={`${rowIndex}-${columnIndex}`}>
                                        {value === null ? (
                                          <span className="font-mono text-[10px] italic text-slate-600">
                                            NULL
                                          </span>
                                        ) : (
                                          <span
                                            className="block max-w-[320px] truncate"
                                            title={value}
                                          >
                                            {value}
                                          </span>
                                        )}
                                      </td>
                                    ))}
                                    <td className="table-cell text-right">
                                      <div className="flex items-center justify-end gap-1 opacity-0 transition-opacity group-hover:opacity-100">
                                        <button
                                          type="button"
                                          className="icon-btn h-6 w-6"
                                          onClick={() => openEdit(row)}
                                          disabled={!editable || !key}
                                          title={
                                            editable && key
                                              ? 'Edit row'
                                              : 'A primary key is required to edit rows'
                                          }
                                          aria-label="Edit row"
                                        >
                                          <Pencil size={11} />
                                        </button>
                                        <button
                                          type="button"
                                          className="icon-btn h-6 w-6 hover:bg-rose-500/10 hover:text-rose-300"
                                          onClick={() => {
                                            if (!key) {
                                              return;
                                            }
                                            setDeleteTarget({
                                              key,
                                              label:
                                                selectedTable.primaryKey
                                                  .map((name) => {
                                                    const index = rows.columns.findIndex(
                                                      (column) =>
                                                        column.name.toLowerCase() ===
                                                        name.toLowerCase(),
                                                    );
                                                    return `${name}=${index >= 0 ? row[index] ?? 'NULL' : '?'}`;
                                                  })
                                                  .join(', ') || `row ${offset + rowIndex + 1}`,
                                            });
                                          }}
                                          disabled={!editable || !key}
                                          title={
                                            editable && key
                                              ? 'Delete row'
                                              : 'A primary key is required to delete rows'
                                          }
                                          aria-label="Delete row"
                                        >
                                          <Trash2 size={11} />
                                        </button>
                                      </div>
                                    </td>
                                  </tr>
                                );
                              })
                            )}
                          </tbody>
                        </table>
                      </div>
                    )
                  ) : (
                    <div className="px-4 py-10 text-center text-[11px] text-slate-600">
                      Select a table to load its rows.
                    </div>
                  )}

                  <div className="flex flex-wrap items-center justify-between gap-2 border-t border-white/[0.06] px-4 py-2.5">
                    <div className="text-[10px] text-slate-600">
                      {rows?.hasMore
                        ? 'More rows are available.'
                        : rows
                          ? 'End of the current result.'
                          : ' '}
                    </div>
                    <div className="flex items-center gap-1.5">
                      <button
                        type="button"
                        className="btn-secondary btn-xs"
                        onClick={() => setOffset(Math.max(0, offset - limit))}
                        disabled={offset === 0 || rowsLoading}
                      >
                        <ChevronLeft size={12} />
                        Previous
                      </button>
                      <button
                        type="button"
                        className="btn-secondary btn-xs"
                        onClick={() => setOffset(offset + limit)}
                        disabled={!rows?.hasMore || rowsLoading}
                      >
                        Next
                        <ChevronRight size={12} />
                      </button>
                    </div>
                  </div>
                </div>
              </>
            )}
          </div>
        </div>
      )}

      <RowEditorModal
        open={Boolean(editor)}
        mode={editor?.mode ?? 'insert'}
        table={selectedTable}
        row={editor?.row ?? null}
        error={editorError}
        saving={saving}
        onClose={() => {
          if (!saving) {
            setEditor(null);
            setEditorError('');
          }
        }}
        onSubmit={(values) => void submitEditor(values)}
      />

      <ConfirmDialog
        open={Boolean(deleteTarget)}
        title="Delete row"
        description={
          deleteTarget && selectedTable
            ? `Delete the row (${deleteTarget.label}) from ${selectedTable.schema}.${selectedTable.name}? This cannot be undone.`
            : ''
        }
        confirmLabel="Delete row"
        busy={deleting}
        onConfirm={() => void confirmDelete()}
        onCancel={() => {
          if (!deleting) {
            setDeleteTarget(null);
          }
        }}
      />

      <ConfirmDialog
        open={Boolean(dropTarget)}
        title="Drop table"
        description={
          dropTarget
            ? `Drop ${dropTarget.schema}.${dropTarget.name} and permanently delete all of its rows? Views and foreign keys that depend on it will make the drop fail. This cannot be undone.`
            : ''
        }
        confirmLabel="Drop table"
        busy={dropping}
        onConfirm={() => void confirmDropTable()}
        onCancel={() => {
          if (!dropping) {
            setDropTarget(null);
          }
        }}
      />

      <Modal
        open={seedOpen}
        onClose={() => {
          if (!seeding) {
            setSeedOpen(false);
          }
        }}
        title="Seed realistic rows"
        description={
          selectedTable
            ? `Generate and insert sample data into ${selectedTable.schema}.${selectedTable.name}. The AI uses the live column types, NOT NULL rules, and foreign keys; without a provider it falls back to deterministic type-aware data.`
            : ''
        }
        size="sm"
        preventClose={seeding}
        footer={
          <>
            <button
              type="button"
              className="btn-secondary"
              onClick={() => setSeedOpen(false)}
              disabled={seeding}
            >
              Cancel
            </button>
            <button
              type="button"
              className="btn-primary"
              onClick={() => void seedRows()}
              disabled={seeding}
            >
              {seeding ? <Loader2 size={14} className="animate-spin" /> : <Sparkles size={14} />}
              {seeding ? 'Generating…' : `Generate ${seedCount} rows`}
            </button>
          </>
        }
      >
        <div className="space-y-3">
          <div className="flex gap-2">
            {[10, 50].map((count) => (
              <button
                key={count}
                type="button"
                className={`btn-xs rounded-lg px-3 py-2 text-xs font-medium transition-colors ${
                  seedCount === count
                    ? 'bg-mint-400/15 text-mint-200 ring-1 ring-mint-400/30'
                    : 'bg-white/[0.03] text-slate-400 hover:bg-white/[0.06]'
                }`}
                onClick={() => setSeedCount(count)}
                disabled={seeding}
              >
                {count} rows
              </button>
            ))}
          </div>
          <p className="text-[11px] leading-relaxed text-slate-500">
            Rows are inserted in one statement. Identity and generated columns are skipped; foreign
            keys reuse existing referenced values when available.
          </p>
          {seedError ? (
            <p className="rounded-lg border border-rose-500/20 bg-rose-500/[0.05] px-3 py-2 text-[11px] leading-relaxed text-rose-200/80">
              {seedError}
            </p>
          ) : null}
        </div>
      </Modal>
    </div>
  );
}

function tableKey(table: TableInfo): string {
  return `${table.schema}.${table.name}`;
}

function selectionStorageKey(instanceId: string): string {
  return `hysql.tableEditor.selection.${instanceId}`;
}

function readTableSelection(instanceId: string): string | null {
  try {
    return window.localStorage.getItem(selectionStorageKey(instanceId));
  } catch {
    return null;
  }
}

function storeTableSelection(instanceId: string, key: string): void {
  try {
    window.localStorage.setItem(selectionStorageKey(instanceId), key);
  } catch {
    // A locked-down WebView may reject storage; selection persistence is optional.
  }
}
