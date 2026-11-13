import { AlertTriangle, Database, Eye, EyeOff, Loader2, RefreshCw } from 'lucide-react';
import { useEffect, useState, type FormEvent } from 'react';
import type { CreateInstanceRequest } from '../types';
import { generatePassword, isValidSlug, makeSlug } from '../lib/format';
import { Modal } from './Modal';

interface CreateDatabaseModalProps {
  open: boolean;
  creating: boolean;
  javaSupported: boolean;
  error: string;
  onClose: () => void;
  onCreate: (request: CreateInstanceRequest) => void;
}

export function CreateDatabaseModal({
  open,
  creating,
  javaSupported,
  error,
  onClose,
  onCreate,
}: CreateDatabaseModalProps) {
  const [name, setName] = useState('');
  const [username, setUsername] = useState('sa');
  const [password, setPassword] = useState('');
  const [port, setPort] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [localError, setLocalError] = useState('');

  useEffect(() => {
    if (!open) {
      return;
    }
    setName('');
    setUsername('sa');
    setPassword(generatePassword());
    setPort('');
    setShowPassword(false);
    setLocalError('');
  }, [open]);

  const normalizedName = makeSlug(name);
  const nameValid = isValidSlug(normalizedName);

  const handleSubmit = (event: FormEvent) => {
    event.preventDefault();
    const trimmedUsername = username.trim();
    if (!nameValid) {
      setLocalError('Use a slug such as inventory_db (lowercase letters, digits, underscores).');
      return;
    }
    if (password.length < 8) {
      setLocalError('The password must be at least 8 characters.');
      return;
    }
    let parsedPort = 0;
    if (port.trim() !== '') {
      parsedPort = Number(port);
      if (!Number.isInteger(parsedPort) || parsedPort < 1024 || parsedPort > 65535) {
        setLocalError('Port must be an integer between 1024 and 65535.');
        return;
      }
    }
    setLocalError('');
    onCreate({
      name: normalizedName,
      username: trimmedUsername || 'sa',
      password,
      port: parsedPort,
    });
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="New H2 database"
      description="HySQL starts a loopback-only TCP server with generated credentials."
      preventClose={creating}
      footer={
        <>
          <button type="button" className="btn-secondary" onClick={onClose} disabled={creating}>
            Cancel
          </button>
          <button type="submit" form="create-database-form" className="btn-primary" disabled={creating || !javaSupported}>
            {creating ? <Loader2 size={14} className="animate-spin" /> : <Database size={14} />}
            {creating ? 'Creating…' : 'Create & start'}
          </button>
        </>
      }
    >
      <form id="create-database-form" className="space-y-5" onSubmit={handleSubmit}>
        {!javaSupported ? (
          <div className="flex gap-2.5 rounded-xl border border-amber-400/20 bg-amber-400/[0.06] px-3.5 py-3 text-xs leading-relaxed text-amber-200/90">
            <AlertTriangle size={15} className="mt-0.5 flex-none" />
            Install Java 11 or newer before creating a database. The engine jar alone is not enough
            to start an H2 server.
          </div>
        ) : null}

        {error || localError ? (
          <div className="flex gap-2.5 rounded-xl border border-rose-500/20 bg-rose-500/[0.06] px-3.5 py-3 text-xs leading-relaxed text-rose-200/90">
            <AlertTriangle size={15} className="mt-0.5 flex-none" />
            <span className="whitespace-pre-wrap">{localError || error}</span>
          </div>
        ) : null}

        <div>
          <label className="label mb-1.5 block" htmlFor="database-name">
            Database name
          </label>
          <input
            id="database-name"
            className="input font-mono"
            value={name}
            onChange={(event) => setName(event.target.value)}
            onBlur={() => setName(normalizedName)}
            placeholder="inventory_db"
            autoFocus
            spellCheck={false}
            disabled={creating}
          />
          <div className="mt-1.5 flex items-center justify-between gap-3 text-[11px]">
            <span className={name && !nameValid ? 'text-rose-300/90' : 'text-slate-600'}>
              Lowercase letters, digits, underscores; starts with a letter.
            </span>
            {name && normalizedName !== name ? (
              <span className="font-mono text-slate-500">{normalizedName}</span>
            ) : null}
          </div>
        </div>

        <div className="grid gap-4 sm:grid-cols-2">
          <div>
            <label className="label mb-1.5 block" htmlFor="database-user">
              Username
            </label>
            <input
              id="database-user"
              className="input font-mono"
              value={username}
              onChange={(event) => setUsername(event.target.value)}
              placeholder="sa"
              spellCheck={false}
              disabled={creating}
            />
          </div>
          <div>
            <label className="label mb-1.5 block" htmlFor="database-port">
              Local port
            </label>
            <input
              id="database-port"
              className="input font-mono"
              value={port}
              onChange={(event) => setPort(event.target.value.replace(/[^\d]/g, ''))}
              placeholder="Auto (9092+)"
              inputMode="numeric"
              disabled={creating}
            />
          </div>
        </div>

        <div>
          <div className="mb-1.5 flex items-center justify-between">
            <label className="label" htmlFor="database-password">
              Password
            </label>
            <button
              type="button"
              className="btn-ghost btn-xs"
              onClick={() => setPassword(generatePassword())}
              disabled={creating}
            >
              <RefreshCw size={12} />
              Generate
            </button>
          </div>
          <div className="relative">
            <input
              id="database-password"
              className={`input pr-10 font-mono ${showPassword ? '' : 'text-slate-400'}`}
              type={showPassword ? 'text' : 'password'}
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              spellCheck={false}
              autoComplete="new-password"
              disabled={creating}
            />
            <button
              type="button"
              className="icon-btn absolute right-1.5 top-1/2 h-7 w-7 -translate-y-1/2"
              onClick={() => setShowPassword((visible) => !visible)}
              aria-label={showPassword ? 'Hide password' : 'Show password'}
            >
              {showPassword ? <EyeOff size={14} /> : <Eye size={14} />}
            </button>
          </div>
          <p className="mt-1.5 text-[11px] text-slate-600">
            Stored locally in <span className="font-mono">~/.hysql/instances.json</span>.
          </p>
        </div>

        <div className="rounded-xl border border-white/[0.06] bg-ink-950/40 px-4 py-3 text-[11px] leading-relaxed text-slate-500">
          The database is created on first connection, then started immediately. The H2 file lands
          in <span className="font-mono text-slate-400">~/.hysql/dbs/{normalizedName || 'name'}.mv.db</span>.
        </div>
      </form>
    </Modal>
  );
}
