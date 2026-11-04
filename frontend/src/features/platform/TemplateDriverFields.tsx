import { SavedSourceSummary } from './SavedSourceSummary';
import { deploymentTemplate, serviceTemplate } from "./inlineTemplates";
import { useEffect, useRef, useState } from 'react';
import { api } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { t } from '../../shared/i18n';
import { GitBundleSource, emptyGitSource } from './GitBundleSource';
import { ZIPSource } from './ZIPSource';
import { parseTemplateSource, parseTemplateSources, type TemplateSource } from './metadata';
import { RenderInputsEditor } from './RenderInputsEditor';
import type { JsonObject } from './typedVariables';

export type TemplateDriverDraft = { source: string; inputs: JsonObject; namespace: string; cluster: string; provision: string; references: string; blocked?: boolean };
export const emptyTemplateDriver = (): TemplateDriverDraft => ({ source: '', inputs: {}, namespace: '', cluster: '', provision: '{}', references: '{}' });

const initialFiles = JSON.stringify({
  'bundle.yaml': 'apiVersion: orchestrator.dev/template-source/v1\nkind: TemplateSource\nmetadata:\n  name: custom-workload\nrenderer:\n  type: template\n  version: 1.0.0\ninputContract: orchestrator.dev/template-input/v1\ninputsSchema: inputs.schema.json\ndefaults: defaults.yaml\nresources:\n  - deployment.tpl\n  - service.tpl\n',
  'inputs.schema.json': '{"type":"object","additionalProperties":false,"properties":{}}',
  'defaults.yaml': '{}',
  'deployment.tpl': deploymentTemplate,
  'service.tpl': serviceTemplate,
}, null, 2);

export function TemplateDriverFields({ value, onChange, disabled, editing=false }: { value: TemplateDriverDraft; onChange: (value: TemplateDriverDraft) => void; disabled: boolean; editing?: boolean }) {
  const [changingSource, setChangingSource] = useState(false);
  const [mode, setMode] = useState<'inline' | 'git' | 'zip' | 'registered'>('registered');
  const [sources, setSources] = useState<TemplateSource[]>([]);
  const [reload, setReload] = useState(0);
  const [git, setGit] = useState(emptyGitSource);
  const [files, setFiles] = useState(initialFiles);
 const [newFile, setNewFile] = useState('');
 const fileObject = JSON.parse(files) as Record<string, string>;
 const templateNames = Object.keys(fileObject).filter((name) => name.endsWith('.tpl'));
 const changeFile = (name: string, content: string) => { request.current++; setFiles(JSON.stringify({ ...fileObject, [name]: content }, null, 2)); accept(null); };
  const latest = useRef(value); latest.current = value;
 const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const request = useRef(0);
  const selected = sources.find((source) => source.id === value.source);
  useEffect(() => { let current = true; const tracker = request; api<unknown>('/template-sources').then((response) => { if (current) { setSources(parseTemplateSources(response)); setError(''); } }).catch(() => { if (current) setError(t('templateDriver.loadError')); }); return () => { current = false; tracker.current++; }; }, [reload]);
  const accept = (source: TemplateSource | null) => {
    if (source) setSources((current) => [...current.filter((item) => item.id !== source.id), source]);
    onChange({ ...latest.current, source: source?.id ?? '', blocked: false });
  };
  async function prepare() {
    const current = ++request.current;
    setBusy(true); setError('');
    try {
      const parsed: unknown = JSON.parse(files);
      const response = await api<{ templateSource: unknown }>('/template-sources/inline', { method: 'POST', body: JSON.stringify({ files: parsed }) });
      if (request.current === current) accept(parseTemplateSource(response.templateSource));
    } catch { if (request.current === current) setError(t('templateDriver.prepareError')); }
    finally { if (request.current === current) setBusy(false); }
  }
  return <>
    <section className="definition-card"><div className="definition-card-title"><h3>{t('templateDriver.manifests')}</h3><span className="definition-driver">Template</span></div>
      {selected ? <SavedSourceSummary source={selected.source ?? {kind:'registered',revision:selected.version}} /> : null}
      {editing && !changingSource ? <Button type="button" disabled={disabled || busy} onClick={() => setChangingSource(true)}>{t('sourceSummary.change')}</Button> : null}
      {!editing || changingSource ? <>
      <div className="definition-segmented" role="group" aria-label={t('templateDriver.source')}>
        {(['inline', 'git', 'zip', 'registered'] as const).map((kind) => <Button key={kind} type="button" disabled={disabled || busy} aria-pressed={mode === kind} onClick={() => { request.current++; setMode(kind); setError(''); }}>{kind === 'inline' ? t('templateDriver.inline') : kind === 'git' ? 'Git' : kind === 'zip' ? 'ZIP' : t('sourceSummary.registered')}</Button>)}
      </div>
      {mode === 'inline' ? <>
        {templateNames.map((name) => <label key={name}>{name}<textarea aria-label={name} rows={9} spellCheck={false} disabled={disabled || busy} value={fileObject[name]} onChange={(event) => changeFile(name, event.target.value)} /></label>)}
        <div className="form-actions"><label>{t('templateDriver.fileName')}<input value={newFile} disabled={disabled || busy} onChange={(event) => setNewFile(event.target.value)} placeholder="configmap.tpl" /></label><Button type="button" disabled={disabled || busy || !newFile.endsWith('.tpl') || Object.hasOwn(fileObject, newFile)} onClick={() => { const name = newFile.trim(); const next = { ...fileObject, [name]: '[]' }; next['bundle.yaml'] = `${fileObject['bundle.yaml']}  - ${name}\n`; request.current++; setFiles(JSON.stringify(next, null, 2)); setNewFile(''); accept(null); }}>{t('templateDriver.addFile')}</Button></div>
        <details><summary>{t('templateDriver.schemaDefaults')}</summary><label>{t('templateDriver.schema')}<textarea rows={6} value={fileObject['inputs.schema.json']} onChange={(event) => changeFile('inputs.schema.json', event.target.value)} /></label><label>{t('templateDriver.defaults')}<textarea rows={4} value={fileObject['defaults.yaml']} onChange={(event) => changeFile('defaults.yaml', event.target.value)} /></label></details>
        <Button type="button" disabled={disabled || busy} onClick={() => { void prepare(); }}>{busy ? t('templateDriver.preparing') : t('templateDriver.prepare')}</Button></> : null}
      {mode === 'git' ? <GitBundleSource draft={git} onDraftChange={setGit} disabled={disabled} onResult={accept} /> : null}
      {mode === 'zip' ? <ZIPSource endpoint="/template-sources/upload" disabled={disabled} parseResult={(response) => { if (typeof response !== 'object' || response === null || !('templateSource' in response)) throw new Error('invalid metadata'); return parseTemplateSource(response.templateSource); }} onResult={accept} /> : null}
      <label>{t('templateDriver.verifiedSource')}<select aria-label={t('templateDriver.verifiedSource')} value={value.source} disabled={disabled || busy} onChange={(event) => onChange({ ...value, source: event.target.value, inputs: {}, blocked: false })}><option value="">{t('templateDriver.chooseSource')}</option>{sources.map((source) => <option key={source.id} value={source.id} disabled={source.status !== 'AVAILABLE'}>{source.name ?? source.id} · {source.version}</option>)}</select></label>
      </> : null}
      {selected?.source ? <details><summary>{t('templateDriver.pin')}</summary><code>{selected.source.revision}</code><p><code>{selected.source.contentSha256}</code></p></details> : null}
      {error ? <p role="alert" className="form-error">{error} {error === t('templateDriver.loadError') ? <Button type="button" onClick={() => setReload((value) => value + 1)}>{t('platform.resourceDefinitions.retry')}</Button> : null}</p> : null}
    </section>
    <section className="definition-card"><h3>{t('templateDriver.inputs')}</h3><RenderInputsEditor key={value.source} schema={selected?.inputsSchema} value={value.inputs} onChange={(inputs) => onChange({ ...value, inputs })} onRawProblems={(count) => onChange({ ...value, blocked: count > 0 })} disabled={disabled || !selected} /></section>
    <section className="definition-card"><h3>{t('templateDriver.references')}</h3><label>{t('templateDriver.namespace')}<input aria-label={t('templateDriver.namespace')} value={value.namespace} disabled={disabled} onChange={(event) => onChange({ ...value, namespace: event.target.value })} /></label><label>{t('templateDriver.cluster')}<input aria-label={t('templateDriver.cluster')} value={value.cluster} disabled={disabled} onChange={(event) => onChange({ ...value, cluster: event.target.value })} /></label><label>{t('templateDriver.outputs')}<textarea aria-label={t('templateDriver.outputs')} value={value.references} disabled={disabled} onChange={(event) => onChange({ ...value, references: event.target.value })} placeholder={'{"databaseHost":"${resources[\'postgres.default#shared.database\'].outputs.host}"}'} /></label><details><summary>{t('platform.resourceDefinitions.companionResources')}</summary><label>{t('platform.resourceDefinitions.relatedResourceRulesJSONObject')}<textarea value={value.provision} disabled={disabled} onChange={(event) => onChange({ ...value, provision: event.target.value })} /></label></details></section>
  </>;
}
