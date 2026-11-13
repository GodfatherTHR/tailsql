import { Loader2, Save, Sparkles } from 'lucide-react';
import { useEffect, useState } from 'react';
import type { AISettings } from '../types';
import { api, errorMessage } from '../lib/api';
import type { ToastKind } from '../hooks/useToasts';
import { Modal } from './Modal';

interface AISettingsModalProps {
  open: boolean;
  onClose: () => void;
  onSaved: (settings: AISettings) => void;
  onToast: (kind: ToastKind, message: string) => void;
}

const providers = [
  { value: 'ollama', label: 'Ollama (local)' },
  { value: 'openrouter', label: 'OpenRouter' },
  { value: 'openai', label: 'OpenAI-compatible' },
];

const defaults: Record<string, { baseUrl: string; model: string }> = {
  ollama: { baseUrl: 'http://127.0.0.1:11434', model: 'qwen2.5-coder:7b' },
  openrouter: { baseUrl: 'https://openrouter.ai/api/v1', model: 'deepseek/deepseek-chat' },
  openai: { baseUrl: 'https://api.openai.com/v1', model: 'gpt-4o-mini' },
};

export function AISettingsModal({ open, onClose, onSaved, onToast }: AISettingsModalProps) {
  const [provider, setProvider] = useState('ollama');
  const [baseUrl, setBaseUrl] = useState(defaults.ollama.baseUrl);
  const [model, setModel] = useState(defaults.ollama.model);
  const [apiKey, setApiKey] = useState('');
  const [clearKey, setClearKey] = useState(false);
  const [hasKey, setHasKey] = useState(false);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!open) {
      return;
    }
    let cancelled = false;
    setLoading(true);
    setError('');
    setApiKey('');
    setClearKey(false);
    api
      .getAISettings()
      .then((settings) => {
        if (cancelled) {
          return;
        }
        setProvider(settings.provider || 'ollama');
        setBaseUrl(settings.baseUrl || defaults.ollama.baseUrl);
        setModel(settings.model || defaults.ollama.model);
        setHasKey(settings.hasApiKey);
      })
      .catch((loadError) => {
        if (!cancelled) {
          setError(errorMessage(loadError));
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [open]);

  const handleProvider = (value: string) => {
    setProvider(value);
    const preset = defaults[value];
    const known = Object.values(defaults).some((entry) => entry.baseUrl === baseUrl);
    if (preset && (baseUrl.trim() === '' || known)) {
      setBaseUrl(preset.baseUrl);
      setModel(preset.model);
    }
  };

  const save = async () => {
    setSaving(true);
    setError('');
    try {
      const settings = await api.saveAISettings({
        provider,
        baseUrl: baseUrl.trim(),
        model: model.trim(),
        apiKey: apiKey.trim(),
        clearApiKey: clearKey,
      });
      setHasKey(settings.hasApiKey);
      onSaved(settings);
      onToast('success', 'AI settings saved.');
      onClose();
    } catch (saveError) {
      setError(errorMessage(saveError));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="AI settings"
      description="Used by the SQL assistant and the mock-data seeder. Keys stay in ~/.hysql/config.json."
      preventClose={saving}
      footer={
        <>
          <button type="button" className="btn-secondary" onClick={onClose} disabled={saving}>
            Cancel
          </button>
          <button type="button" className="btn-primary" onClick={() => void save()} disabled={saving || loading}>
            {saving ? <Loader2 size={14} className="animate-spin" /> : <Save size={14} />}
            Save settings
          </button>
        </>
      }
    >
      {loading ? (
        <div className="flex items-center gap-2 text-xs text-slate-500">
          <Loader2 size={14} className="animate-spin" /> Loading settings…
        </div>
      ) : (
        <div className="space-y-4">
          <div className="flex items-start gap-2.5 rounded-xl border border-mint-400/15 bg-mint-400/[0.04] px-3.5 py-3 text-[11px] leading-relaxed text-slate-400">
            <Sparkles size={14} className="mt-0.5 flex-none text-mint-300" />
            Ollama runs locally and needs no key. OpenRouter and OpenAI-compatible providers use the
            API key below. The seeder falls back to deterministic offline data when the provider is
            unreachable.
          </div>

          <label className="block">
            <span className="label mb-1.5 block">Provider</span>
            <select
              className="input"
              value={provider}
              onChange={(event) => handleProvider(event.target.value)}
            >
              {providers.map((entry) => (
                <option key={entry.value} value={entry.value}>
                  {entry.label}
                </option>
              ))}
            </select>
          </label>

          <label className="block">
            <span className="label mb-1.5 block">Base URL</span>
            <input
              className="input font-mono text-xs"
              value={baseUrl}
              onChange={(event) => setBaseUrl(event.target.value)}
              placeholder="http://127.0.0.1:11434"
            />
          </label>

          <label className="block">
            <span className="label mb-1.5 block">Model</span>
            <input
              className="input font-mono text-xs"
              value={model}
              onChange={(event) => setModel(event.target.value)}
              placeholder="qwen2.5-coder:7b"
            />
          </label>

          {provider !== 'ollama' ? (
            <label className="block">
              <span className="label mb-1.5 block">API key</span>
              <input
                className="input font-mono text-xs"
                type="password"
                value={apiKey}
                onChange={(event) => setApiKey(event.target.value)}
                placeholder={hasKey ? 'Saved — leave blank to keep it' : 'sk-…'}
                autoComplete="off"
              />
              {hasKey ? (
                <label className="mt-2 flex items-center gap-2 text-[11px] text-slate-500">
                  <input
                    type="checkbox"
                    checked={clearKey}
                    onChange={(event) => setClearKey(event.target.checked)}
                  />
                  Clear the stored key
                </label>
              ) : null}
            </label>
          ) : null}

          {error ? <p className="text-[11px] leading-relaxed text-rose-300">{error}</p> : null}
        </div>
      )}
    </Modal>
  );
}
