import { useEffect, useId, useRef, type ReactNode } from 'react';
import { t } from '../../shared/i18n';
import { Button } from '../../shared/ui/Button';
import { SavedSourceSummary } from './SavedSourceSummary';
import type { PreparedEdit } from './useDefinitionCodeEditor';

export function DefinitionEditMethods({ children, enabled, mode, text, busy, held, error, notice, prepared, onMode, onChange, onCheck, onSave, onCancel }: {
  children: ReactNode; enabled: boolean; mode: 'form' | 'code'; text: string; busy: boolean; held: boolean;
  error: string; notice: string; prepared?: PreparedEdit; onMode: (mode: 'form' | 'code') => void;
  onChange: (text: string) => void; onCheck: () => void; onSave: () => void; onCancel: () => void;
}) {
  const id = useId();
  const previousMode = useRef(mode);
  useEffect(() => {
    if (previousMode.current !== mode) document.getElementById(`${id}-${mode}`)?.focus();
    previousMode.current = mode;
  }, [id, mode]);
  if (!enabled) return children;
  return <>
    <div className="tabs" role="tablist" aria-label={t('definitionCode.method')}>
      {(['form', 'code'] as const).map((tab, i) => <button key={tab} type="button" role="tab" id={`${id}-${tab}`} aria-controls={`${id}-${tab}-panel`} aria-selected={mode === tab} tabIndex={mode === tab ? 0 : -1} disabled={busy || held} className={`tab ${mode === tab ? 'tab-active' : ''}`} onClick={() => onMode(tab)} onKeyDown={event => {
        if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
        event.preventDefault();
        const next = event.key === 'Home' ? 'form' : event.key === 'End' ? 'code' : i === 0 ? 'code' : 'form';
        onMode(next);
      }}>{tab === 'form' ? t('catalogDocument.form') : 'YAML/JSON'}</button>)}
    </div>
    <div role="tabpanel" id={`${id}-form-panel`} aria-labelledby={`${id}-form`} hidden={mode !== 'form'}>{children}</div>
    <section className="content-panel definition-code-editor" role="tabpanel" id={`${id}-code-panel`} aria-labelledby={`${id}-code`} hidden={mode !== 'code'}>
      <fieldset className="editor-grid form-fieldset connection-form" disabled={busy} aria-busy={busy}>
      <label>{t('catalogDocument.content')}<textarea aria-label={t('catalogDocument.content')} rows={22} spellCheck={false} disabled={busy || held} value={text} onChange={event => onChange(event.target.value)} /></label>
      <details><summary>{t('catalogDocument.help')}</summary><p>{t('definitionCode.help')}</p></details>
      {prepared?.source ? <SavedSourceSummary source={{ kind: 'git', ...prepared.source }} /> : null}
      {prepared ? <p role="status" className="form-success">{t('catalogDocument.checked')}</p> : null}
      {mode === 'code' && notice ? <p role="status" className="form-success">{notice}</p> : null}
      {busy ? <p role="status">{t('catalogDocument.checking')}</p> : null}
      <div className="form-actions"><Button type="button" disabled={busy || held || !text.trim()} onClick={onCheck}>{t('catalogDocument.check')}</Button><Button type="button" disabled={busy} onClick={onCancel}>{t('platform.resourceDefinitions.cancelEditing')}</Button><Button type="button" tone="primary" disabled={busy || held || !text.trim()} onClick={onSave}>{t('platform.resourceDefinitions.saveChanges')}</Button></div>
      </fieldset>
    </section>
    {error ? <p role="alert" className="form-error">{error}</p> : null}
  </>;
}
