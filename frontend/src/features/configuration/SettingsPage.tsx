import { useCallback, useEffect, useRef, useState } from 'react';
import { navigate, replaceRoute } from '../../app/routes';
import { ApiError, isStaleConflict } from '../../shared/api/client';
import type { Application, EnvironmentKey, EnvironmentTarget } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { environmentTitle } from '../environment/labels';
import { useEnvironmentBusy } from '../environment/useEnvironmentBusy';
import { OperationBanner } from '../environment/OperationBanner';
import { SecretStoreSelection } from '../environment/SecretStoreSelection';
import { fetchEnvironment } from '../environment/api';
import { deleteKey, getConfiguration, putKey, renameKey, type ConfigKey, type Configuration, type KeyKind } from './api';
import { t } from '../../shared/i18n';
import { DeleteApplicationDialog } from '../applications/deletion/DeleteApplicationDialog';
import { DeletionPanel } from '../applications/deletion/DeletionPanel';
import { EnvironmentCatalogPanel } from '../catalog/EnvironmentCatalogPanel';
import { DeliverySelectionPanel } from '../delivery/DeliverySelectionPanel';
import { Dialog } from '../../shared/ui/Dialog';

type Form = { kind: KeyKind; name: string; value: string; editing?: string };
type Action = { kind: 'rename' | 'delete'; key: ConfigKey; newName: string; updateReferences: boolean };
type Notice = { text: string; workloads: string[] };

const unknownOutcome = t('configuration.settings.itIsUnclearWhetherThe');

// Text comes from the stable error code. A network failure never claims the
// change was not saved; reload outcome wording is added only after the GET result is known.
// The guards are only taken from one authoritative GET; a missing version is never replaced by zero.
function referenceGuards(data: Configuration) {
  return Number.isInteger(data.environmentVersion) && Number.isInteger(data.draftVersion)
    ? { environmentVersion: data.environmentVersion as number, draftVersion: data.draftVersion as number } : undefined;
}

function describe(err: unknown): string {
  if (!(err instanceof ApiError)) return t('configuration.settings.noResponseFromTheServer');
  return err.message;
}

export function SettingsPage({ application, initialEnvironment = 'staging', canEditDelivery = false, onTargetChange, onDeletionStarted, onDeleted }: { application: Application; initialEnvironment?: EnvironmentKey; canEditDelivery?: boolean; onTargetChange?(applicationId: string, environment: EnvironmentKey, target: EnvironmentTarget): void; onDeletionStarted?(applicationId: string, operationId: string): void; onDeleted?(applicationId: string): void }) {
  const [deleting, setDeleting] = useState(false);
  const [deletionId, setDeletionId] = useState(application.deletionOperationId);
  const deletionOperation = application.status === 'DELETING' ? deletionId ?? application.deletionOperationId : deletionId;
  const deleted = useCallback(() => onDeleted?.(application.id), [onDeleted, application.id]);
  const [environment, setEnvironment] = useState<EnvironmentKey>(initialEnvironment);
  const [deliveryDirty, setDeliveryDirty] = useState(false);
  const [switchEnvironment, setSwitchEnvironment] = useState<EnvironmentKey>();
  function changeEnvironment(next: EnvironmentKey) {
    if (next === environment) return;
    if (deliveryDirty) { setSwitchEnvironment(next); return; }
    setEnvironment(next); replaceRoute({ name: 'settings', applicationId: application.id, environment: next });
  }
  const deliverySaved = useCallback(() => {
    setReloadCount(value => value + 1);
    void fetchEnvironment(application.id, environment).then(target => { if (target) onTargetChange?.(application.id, environment, target); }).catch(() => undefined);
  }, [application.id, environment, onTargetChange]);
  const [data, setData] = useState<Configuration>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [form, setForm] = useState<Form>();
  const [action, setAction] = useState<Action>();
  const [saving, setSaving] = useState(false);
  const [reloadCount, setReloadCount] = useState(0);
  const [needStore, setNeedStore] = useState(false);
  // Set when a stale conflict could not reload the authoritative configuration: writes stay off until a reload succeeds.
  const [reloadNeeded, setReloadNeeded] = useState(false);
  const [notice, setNotice] = useState<Notice>();
  // scope changes with the Application/Environment; a mutation or reload reply
  // that belongs to an older scope is dropped instead of showing another
  // Environment's data. seq orders replies inside one scope: only the newest
  // data wins, so a late reload cannot undo a newer save.
  const scope = useRef(0);
  const seq = useRef(0);
  // editor changes whenever the user opens, edits or closes a form or confirmation.
  // A mutation that started before such a change must not close what the user is
  // doing now: its reply still updates the data, but only an untouched editor is closed.
  const editor = useRef(0);
  const updateForm = (next: Form | undefined) => { editor.current += 1; setNotice(undefined); setForm(next); };
  const updateAction = (next: Action | undefined) => { editor.current += 1; setNotice(undefined); setAction(next); };
  // An active or interrupted Environment operation holds every configuration write
  // (the server answers 409 ENVIRONMENT_BUSY). The state is also polled while idle, so an
  // operation started elsewhere is noticed. Open editors keep their text and
  // simply cannot submit until the target no longer reports an operation.
  const operationBusy = useEnvironmentBusy(application, environment, onTargetChange);
  // A deleting Application is read-only (UC-18 BR-04).
  const busy = operationBusy || reloadNeeded || Boolean(deletionOperation);

  // Leaving the page invalidates every pending reply.
  useEffect(() => () => { scope.current += 1; seq.current += 1; }, []);

  // Switching Application or Environment resets the editors and loads afresh.
  useEffect(() => {
    scope.current += 1;
    setData(undefined);
    updateForm(undefined);
    updateAction(undefined);
    setSaving(false);
    setError('');
    setNeedStore(false);
    setReloadNeeded(false);
    setNotice(undefined);
    setLoading(true);
  }, [application.id, environment]);

  // A refresh (initial load, after a store change) only replaces the data: the
  // open editor and its text stay exactly as the user left them.
  useEffect(() => {
    const mine = scope.current;
    const order = ++seq.current;
    getConfiguration(application.id, environment)
      .then((result) => { if (mine === scope.current && order === seq.current) { setData(result); setReloadNeeded(false); } })
      .catch(() => { if (mine === scope.current && order === seq.current) setError(t('configuration.settings.couldNotLoadTheConfiguration')); })
      .finally(() => { if (mine === scope.current) setLoading(false); });
  }, [application.id, environment, reloadCount]);

  // Runs one mutation; its reply applies only to the scope it started in.
  async function mutate(run: () => Promise<Configuration>, done: (result: Configuration) => void, guardOutcome = false) {
    const mine = scope.current;
    const touched = editor.current;
    setSaving(true); setError('');
    try {
      const result = await run();
      if (mine !== scope.current) return;
      seq.current += 1;
      setData(result);
      if (touched === editor.current) done(result);
    } catch (err) {
      if (mine !== scope.current) return;
      const base = describe(err);
      setError(base);
      // A reference rename that may have committed must be reloaded before another attempt; never replayed.
      if (guardOutcome && (!(err instanceof ApiError) || err.status >= 500)) { setReloadNeeded(true); setError(`${base} ${unknownOutcome}`); return; }
      const refreshed = await refreshOn409(err, mine);
      if (refreshed === 'ok' && mine === scope.current) setError(t('configuration.settings.baseTheLatestValuesHave', { base }));
      if (refreshed === 'failed' && mine === scope.current) { setReloadNeeded(true); setError(t('configuration.settings.baseCouldNotReloadThe', { base })); }
    } finally {
      if (mine === scope.current) setSaving(false);
    }
  }

  async function saveForm() {
    if (!form || !data || busy || saving) return;
    const current = form;
    await mutate(() => putKey(application.id, environment, current.name.trim(), current.kind, current.value, data.version), () => updateForm(undefined));
  }

  // After a conflict the authoritative configuration replaces the stale copy
  // without clearing the message or the editor; the user reviews it and
  // resubmits explicitly.
  // Reports whether the authoritative GET succeeded: 'skipped' when the error is not a stale conflict.
  async function refreshOn409(err: unknown, mine: number): Promise<'ok' | 'failed' | 'skipped'> {
    if (!isStaleConflict(err)) return 'skipped';
    const order = ++seq.current;
    try {
      const latest = await getConfiguration(application.id, environment);
      if (mine === scope.current && order === seq.current) { setData(latest); setReloadNeeded(false); }
      return 'ok';
    } catch { return 'failed'; }
  }

  async function retryReload() {
    const mine = scope.current;
    const order = ++seq.current;
    try {
      const latest = await getConfiguration(application.id, environment);
      if (mine === scope.current && order === seq.current) { setData(latest); setReloadNeeded(false); setError(''); }
    } catch { if (mine === scope.current) setError(t('configuration.settings.stillCouldNotReloadThe')); }
  }

  async function confirmAction() {
    if (!action || !data || busy || saving) return;
    const current = action;
    const references = current.kind === 'rename' && current.updateReferences ? referenceGuards(data) : undefined;
    if (current.kind === 'rename' && current.updateReferences && !references) return;
    setNotice(undefined);
    await mutate(() => current.kind === 'rename'
      ? renameKey(application.id, environment, current.key.name, current.newName.trim(), data.version, references)
      : deleteKey(application.id, environment, current.key.name, data.version), (result) => {
      updateAction(undefined);
      // The affected workloads come from the authoritative reply for the new key, not from metadata captured when the dialog opened.
      const renamed = current.newName.trim();
      if (references) setNotice({ text: t('configuration.settings.renamedNameToRenamedAnd', { name: current.key.name, renamed }), workloads: result.keys.find((key) => key.name === renamed)?.usedBy ?? [] });
    }, Boolean(references));
  }

  function section(kind: KeyKind, title: string) {
    const keys = data?.keys.filter((key) => key.kind === kind) ?? [];
    return <section className="content-panel" aria-label={title}>
      <div className="section-header"><div><h2>{title}</h2><p>{kind === 'SECRET' ? t('configuration.settings.theValueIsHiddenAfter') : t('configuration.settings.sharedByTheWorkloadsIn')}</p></div><Button disabled={busy} onClick={() => { updateAction(undefined); if (kind === 'SECRET' && !application.environments[environment].secretStoreKey) { updateForm(undefined); setNeedStore(true); return; } setNeedStore(false); updateForm({ kind, name: '', value: '' }); }}>{t('configuration.settings.addValue', { value: kind === 'SECRET' ? t('configuration.settings.secret') : t('configuration.settings.variable') })}</Button></div>
      {keys.length === 0 ? <div className="section-empty">{t('configuration.settings.noToLowerCaseInEnvironmentYet', { toLowerCase: title.toLowerCase(), environment })}</div> : <div className="settings-table"><div className="settings-table-head"><span>{t('configuration.settings.keyName')}</span><span>{kind === 'SECRET' ? t('configuration.settings.status') : t('configuration.settings.value')}</span><span>{t('configuration.settings.usedByWorkloads')}</span><span>{t('configuration.settings.actions')}</span></div>
        {keys.map((key) => <div className="settings-table-row" key={key.name}><strong>{key.name}</strong><span className={kind === 'SECRET' ? 'muted' : ''}>{kind === 'SECRET' ? t('configuration.settings.configured') : key.value}</span><span>{key.usedBy.length ? key.usedBy.join(', ') : '—'}</span><span className="row-actions"><Button tone="quiet" disabled={busy} onClick={() => { updateAction(undefined); updateForm({ kind, name: key.name, value: kind === 'SECRET' ? '' : key.value ?? '', editing: key.name }); }}>{kind === 'SECRET' ? t('configuration.settings.update') : t('configuration.settings.edit')}</Button><Button tone="quiet" disabled={busy} onClick={() => { updateForm(undefined); updateAction({ kind: 'rename', key, newName: key.name, updateReferences: false }); }}>{t('configuration.settings.rename')}</Button><Button tone="danger" disabled={busy} onClick={() => { updateForm(undefined); updateAction({ kind: 'delete', key, newName: '', updateReferences: false }); }}>{t('configuration.settings.delete')}</Button></span></div>)}</div>}
    </section>;
  }

  // The dialog names the consumers of the key in the latest authoritative data (it changes after a stale reload).
  const consumers = action ? (data?.keys.find((key) => key.name === action.key.name && key.kind === action.key.kind)?.usedBy ?? action.key.usedBy) : [];
  const change = onTargetChange ?? (() => undefined);
  const title = environmentTitle(environment);
  const hasStore = Boolean(application.environments[environment].secretStoreKey);
  return <section className="page settings-page">
    <button className="back-link" onClick={() => navigate({ name: 'application', applicationId: application.id })}>← {application.name}</button>
    <header className="page-header application-header"><div><p className="eyebrow">{t('configuration.settings.applicationSettings')}</p><h1>{t('configuration.settings.environmentSettings')}</h1><p>{t('configuration.settings.chooseTheSecretStoreOf')}</p></div>{data && data.version > 0 ? <span className="pending-pill">{t('configuration.settings.configurationVersionVVersion', { version: data.version })}</span> : null}</header>
    <div className="tabs" role="tablist" aria-label={t('configuration.settings.environment')}>{(['staging', 'production'] as const).map((env) => <button key={env} role="tab" aria-selected={env === environment} className={env === environment ? 'tab tab-active' : 'tab'} onClick={() => changeEnvironment(env)}>{environmentTitle(env)}</button>)}</div>
    <OperationBanner application={application} environment={environment} onTargetChange={change} />
    <EnvironmentCatalogPanel key={`catalog:${application.id}:${environment}`} applicationId={application.id} environment={environment} busy={busy} onSaved={deliverySaved} />
    <DeliverySelectionPanel key={`${application.id}:${environment}`} applicationId={application.id} environment={environment} canEdit={canEditDelivery} busy={busy} onDirtyChange={setDeliveryDirty} onSaved={deliverySaved} />
    {switchEnvironment ? <Dialog label={t('delivery.discardTitle')} onClose={() => setSwitchEnvironment(undefined)}><h2>{t('delivery.discardTitle')}</h2><div className="form-actions"><Button onClick={() => setSwitchEnvironment(undefined)}>{t('delivery.keepEditing')}</Button><Button onClick={() => { const next = switchEnvironment; setSwitchEnvironment(undefined); setDeliveryDirty(false); setEnvironment(next); replaceRoute({ name: 'settings', applicationId: application.id, environment: next }); }}>{t('delivery.discard')}</Button></div></Dialog> : null}
    <SecretStoreSelection application={application} environment={environment} configVersion={data?.version ?? 0} onTargetChange={change} onConfigurationChanged={() => setReloadCount((value) => value + 1)} />
    {needStore && !hasStore ? <div className="form-info" role="status">{t('configuration.settings.chooseASecretStoreAbove')}</div> : null}
    {error ? <div className="form-error" role="alert">{error}{reloadNeeded ? <Button type="button" onClick={() => void retryReload()}>{t('configuration.settings.reloadConfiguration')}</Button> : null}{!data && !loading ? <Button type="button" onClick={() => { setError(''); setLoading(true); setReloadCount((value) => value + 1); }}>{t('configuration.settings.retry')}</Button> : null}</div> : null}
    {notice ? <div className="form-info" role="status">{notice.text}{notice.workloads.length ? t('configuration.settings.affectedWorkloadsValue', { value: notice.workloads.join(', ') }) : ''} {' '}{t('configuration.settings.nothingHasBeenDeployedUse')}{' '} <Button type="button" onClick={() => navigate({ name: 'application', applicationId: application.id, environment })}>{t('configuration.settings.goToPreviewChanges')}</Button></div> : null}
    {loading ? <p>{t('configuration.settings.loadingConfiguration')}</p> : null}
    {!loading && data ? <>{section('VARIABLE', t('configuration.settings.variables'))}{section('SECRET', t('configuration.settings.secrets'))}</> : null}
    {form ? <div className="content-panel inline-editor"><div className="section-header"><h2>{form.editing ? (form.kind === 'SECRET' ? t('configuration.settings.replaceSecret') : t('configuration.settings.editVariable')) : (form.kind === 'SECRET' ? t('configuration.settings.addSecret') : t('configuration.settings.addVariable'))}</h2></div><label>{t('configuration.settings.keyName')}<input value={form.name} disabled={Boolean(form.editing)} onChange={(event) => updateForm({ ...form, name: event.target.value })} placeholder="API_URL" /></label><label>{form.kind === 'SECRET' ? t('configuration.settings.newSecretValue') : t('configuration.settings.value')}<input type={form.kind === 'SECRET' ? 'password' : 'text'} autoComplete="off" value={form.value} onChange={(event) => updateForm({ ...form, value: event.target.value })} /></label>{form.kind === 'SECRET' ? <p className="form-info">{t('configuration.settings.theValueIsOnlyEntered')}</p> : null}{operationBusy ? <p className="form-info" role="status">{t('configuration.settings.anEnvironmentOperationIsRunning')}</p> : null}<div className="form-actions"><Button onClick={() => updateForm(undefined)}>{t('configuration.settings.cancel')}</Button><Button tone="primary" disabled={busy || saving || !form.name.trim() || !form.value} onClick={saveForm}>{t('configuration.settings.savePendingChange')}</Button></div></div> : null}
    {deletionOperation ? <DeletionPanel applicationId={application.id} operationId={deletionOperation} onDeleted={deleted} onOperation={(id) => { setDeletionId(id); onDeletionStarted?.(application.id, id); }} /> : <section className="content-panel danger-zone" aria-label={t('applications.deletion.title')}><div className="section-header"><div><h2>{t('applications.deletion.title')}</h2><p>{t('applications.deletion.zoneDetail')}</p></div><Button tone="danger" disabled={operationBusy} onClick={() => setDeleting(true)}>{t('applications.deletion.open')}</Button></div>{operationBusy ? <p className="muted">{t('applications.deletion.busyHint')}</p> : null}</section>}
    {deleting ? <DeleteApplicationDialog applicationId={application.id} onClose={() => setDeleting(false)} onLost={() => setDeleting(false)} onAccepted={(admission) => { setDeleting(false); setDeletionId(admission.operationId); onDeletionStarted?.(application.id, admission.operationId); }} /> : null}
    {action ? <div className="content-panel inline-editor" role="dialog" aria-label={`${action.kind === 'rename' ? t('configuration.settings.rename') : t('configuration.settings.delete')} ${action.key.name}`}><h2>{action.kind === 'rename' ? t('configuration.settings.rename') : t('configuration.settings.delete')} {action.key.name}?</h2>{consumers.length ? <p className="warning-text">{t('configuration.settings.referencedByConsumersValue', { consumers: consumers.join(', '), value: action.updateReferences ? t('configuration.settings.referencesOfTheseWorkloadsIn') : t('configuration.settings.referencesWillNotBeUpdated') })}</p> : <p>{t('configuration.settings.thisChangeAppliesOnlyTo', { title })}</p>}{action.kind === 'rename' ? <label>{t('configuration.settings.newKeyName')}<input value={action.newName} onChange={(event) => updateAction({ ...action, newName: event.target.value })} /></label> : null}{action.kind === 'rename' ? <label className="checkbox-row"><input type="checkbox" checked={action.updateReferences} onChange={(event) => updateAction({ ...action, updateReferences: event.target.checked })} /> {' '}{t('configuration.settings.renameAndUpdateReferences')}</label> : null}{action.kind === 'rename' && action.updateReferences && !referenceGuards(data!) ? <p className="form-info" role="status">{t('configuration.settings.theEnvironmentVersionAndDraft')}</p> : null}{operationBusy ? <p className="form-info" role="status">{t('configuration.settings.anEnvironmentOperationIsRunning2')}</p> : null}<div className="form-actions"><Button onClick={() => updateAction(undefined)}>{t('configuration.settings.cancel')}</Button><Button tone={action.kind === 'delete' ? 'danger' : 'primary'} disabled={busy || saving || (action.kind === 'rename' && (!action.newName.trim() || (action.updateReferences && !referenceGuards(data!))))} onClick={confirmAction}>{t('configuration.settings.continue')}</Button></div></div> : null}
  </section>;
}
