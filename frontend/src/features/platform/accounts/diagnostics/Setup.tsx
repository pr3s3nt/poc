import { useEffect, useRef, useState } from 'react';
import { api, describeError } from '../../../../shared/api/client';
import { Button } from '../../../../shared/ui/Button';
import { t } from '../../../../shared/i18n';
import type { Account } from '../accountsApi';
import { Icon } from './Icon';
import { useCaseKeys, type Mode, type Purpose } from './diagnosticsApi';

export type Draft = { purpose: Purpose; mode: Mode; region: string; useCases: string[]; application: string; environment: string };
type AppOption = { key: string; name: string; environments: { key: string; name: string }[] };
const regionShape = /^[a-z]{2}(-[a-z]+)+-[0-9]$/;

export function defaultDraft(account: Account, purpose: Purpose): Draft {
  return { purpose, mode: 'CONDITIONS', region: account.config?.verificationRegions?.[0] ?? '', useCases: [], application: '', environment: '' };
}

async function loadApplications(): Promise<AppOption[]> {
  const response = await api<{ applications: { key: string; name?: string; environments?: { key: string; name?: string }[] }[] }>('/applications');
  return (response.applications ?? []).map((app) => ({ key: app.key, name: app.name ?? app.key, environments: (app.environments ?? []).map((env) => ({ key: env.key, name: env.name ?? env.key })) }));
}

/**
 * Scope of one run. The account is fixed; these controls change only the
 * diagnostic request and never the account's configuration or policy.
 */
export function DiagnosticSetup({ account, draft: initial, onClose, onRun }: { account: Account; draft: Draft; onClose(): void; onRun(draft: Draft): void }) {
  const [draft, setDraft] = useState<Draft>({ ...initial, useCases: [...initial.useCases] });
  const [errors, setErrors] = useState<{ useCases?: boolean; region?: boolean; environment?: boolean }>({});
  const [apps, setApps] = useState<{ state: 'loading' | 'ready' | 'error'; items: AppOption[]; message: string }>({ state: 'loading', items: [], message: '' });
  const ref = useRef<HTMLDialogElement>(null);
  const general = draft.purpose === 'GENERAL';
  const aws = account.type === 'AWS';

  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    const dialog = ref.current;
    if (dialog && typeof dialog.showModal === 'function') dialog.showModal(); else dialog?.setAttribute('open', '');
    return () => previous?.focus();
  }, []);
  useEffect(() => {
    if (general) return;
    let alive = true;
    loadApplications().then((items) => { if (alive) setApps({ state: 'ready', items, message: '' }); })
      .catch((reason) => { if (alive) setApps({ state: 'error', items: [], message: describeError(reason, t('platform.diagnostics.setup.applicationsFailed')) }); });
    return () => { alive = false; };
  }, [general]);

  const selectedApp = apps.items.find((item) => item.key === draft.application);
  function toggle(use: string) {
    setErrors({});
    setDraft({ ...draft, useCases: draft.useCases.includes(use) ? draft.useCases.filter((item) => item !== use) : [...draft.useCases, use] });
  }
  function submit(event: React.FormEvent) {
    event.preventDefault();
    const next = {
      region: aws && !regionShape.test(draft.region.trim()),
      useCases: !general && draft.useCases.length === 0,
      environment: !general && !!draft.application && !draft.environment,
    };
    if (next.region || next.useCases || next.environment) { setErrors(next); return; }
    onRun({ ...draft, region: draft.region.trim() });
  }

  return <dialog ref={ref} className="dg-dialog ac-setup" aria-labelledby="ac-setup-title" onCancel={onClose} onClick={(event) => { if (event.target === event.currentTarget) onClose(); }}>
    <form onSubmit={submit} noValidate>
      <header><div><p className="eyebrow">{t('platform.diagnostics.setup.fixedAccount')}</p><h2 id="ac-setup-title">{general ? t('platform.diagnostics.setup.configureGeneral') : t('platform.diagnostics.setup.configureUseCase')}</h2></div><button type="button" className="dg-close" aria-label={t('platform.diagnostics.cancel')} onClick={onClose}>×</button></header>
      <div className="dg-dialog-body">
        <div className="ac-locked-target"><Icon name={aws ? 'cloud' : 'cluster'} size={22} /><div><strong>{account.name}</strong><small>{account.key}</small></div></div>
        {aws && <label className="ac-region-field">{t('platform.diagnostics.region')}
          <input aria-label={t('platform.diagnostics.region')} list="ac-regions" value={draft.region} onChange={(event) => { setErrors({}); setDraft({ ...draft, region: event.target.value }); }} aria-invalid={errors.region || undefined} />
          <datalist id="ac-regions">{(account.config?.verificationRegions ?? []).map((region) => <option key={region} value={region} />)}</datalist>
          {errors.region && <span className="form-error" role="alert">{t('platform.diagnostics.setup.regionInvalid')}</span>}</label>}
        {!general && <>
          <fieldset className="dg-usecase-picker"><legend>{t('platform.diagnostics.useCases')}</legend>
            <div>{useCaseKeys.map((use) => <label key={use}><input type="checkbox" checked={draft.useCases.includes(use)} onChange={() => toggle(use)} /><span>{t(`platform.diagnostics.useCase.${use}`)}</span></label>)}</div></fieldset>
          {errors.useCases && <p className="form-error" role="alert">{t('platform.diagnostics.validation')}</p>}
          <div className="dg-form-grid ac-application-fields">
            <label>{t('platform.diagnostics.setup.applicationOptional')}
              <select aria-label={t('platform.diagnostics.setup.applicationOptional')} value={draft.application} disabled={apps.state === 'loading'} onChange={(event) => { setErrors({}); setDraft({ ...draft, application: event.target.value, environment: '' }); }}>
                <option value="">{t('platform.diagnostics.setup.noApplication')}</option>
                {apps.items.map((item) => <option key={item.key} value={item.key}>{item.name}</option>)}
              </select></label>
            {draft.application && <label>{t('platform.diagnostics.environment')}
              <select aria-label={t('platform.diagnostics.environment')} value={draft.environment} aria-invalid={errors.environment || undefined} onChange={(event) => { setErrors({}); setDraft({ ...draft, environment: event.target.value }); }}>
                <option value="">{t('platform.diagnostics.setup.chooseEnvironment')}</option>
                {(selectedApp?.environments ?? []).map((env) => <option key={env.key} value={env.key}>{env.name}</option>)}
              </select>{errors.environment && <span className="form-error" role="alert">{t('platform.diagnostics.setup.environmentRequired')}</span>}</label>}
          </div>
          {apps.state === 'error' && <p className="form-error" role="alert">{apps.message}</p>}
          <p className="ac-field-help">{t('platform.diagnostics.setup.applicationHelp')}</p>
          <fieldset className="dg-mode-picker"><legend>{t('platform.diagnostics.mode')}</legend>
            {(['CONDITIONS', 'EXECUTION'] as const).map((mode) => <label key={mode} data-selected={draft.mode === mode}>
              <input type="radio" name="mode" checked={draft.mode === mode} onChange={() => setDraft({ ...draft, mode })} />
              <span><strong>{t(`platform.diagnostics.mode.${mode}`)}</strong><small>{t(`platform.diagnostics.mode.${mode}Help`)}</small></span></label>)}</fieldset>
          {draft.mode === 'EXECUTION' && <p className="dg-mode-note">{aws ? t('platform.diagnostics.setup.executionNoteAws') : t('platform.diagnostics.setup.executionNote')}</p>}
        </>}
        {general && <p className="ac-general-readonly">{t('platform.diagnostics.setup.readOnlyScope')}</p>}
        <p className="dg-runtime-note"><Icon name="diagnose" size={15} />{t('platform.diagnostics.setup.scopeHelp')}</p>
      </div>
      <footer><Button type="button" onClick={onClose}>{t('platform.diagnostics.cancel')}</Button><Button type="submit" tone="primary"><Icon name="play" size={15} />{t('platform.diagnostics.setup.start')}</Button></footer>
    </form>
  </dialog>;
}
