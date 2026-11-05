import { useState } from 'react';
import { t } from '../../shared/i18n';
import { Button } from '../../shared/ui/Button';
import { Dialog } from '../../shared/ui/Dialog';
import { VolumeSourceFields } from './VolumeSourceFields';
import { validRuntimeIntent } from './runtimeIntent';
import type { Execution, PodVolume } from './templates';

type Kind = 'storage' | 'emptyDir' | 'configMap' | 'hostPath' | 'secret';
const text = (key: string, params?: Record<string, string | number>) => t(`workloads.containerVolumes.${key}`, params);
const volumeText = (key: string) => t(`workloads.volumes.${key}`);
const sourceKind = (v: PodVolume): Kind => v.hostPath ? 'hostPath' : v.configMap ? 'configMap' : 'emptyDir';
const secretKey = (ref: string) => ref.replace(/^\$\{resources\.env\./, '').replace(/}$/, '');
const options = (items: string[], selected: string) => <>{!items.includes(selected) ? <option value={selected}>{selected}</option> : null}{items.map(item => <option key={item} value={item}>{item}</option>)}</>;
function MountPath({ path, subPath, readOnly, locked, onChange }: { path: string; subPath?: string; readOnly: boolean; locked?: boolean; onChange(patch: { path?: string; subPath?: string; readOnly?: boolean }): void }) {
  return <div className="field-grid">
    <label>{volumeText('path')}<input value={path} onChange={e => onChange({ path: e.target.value })} /></label>
    {subPath !== undefined ? <label>{volumeText('subPath')}<input value={subPath} onChange={e => onChange({ subPath: e.target.value })} /></label> : null}
    <label className="checkbox-row"><input type="checkbox" checked={readOnly} disabled={locked} onChange={e => onChange({ readOnly: e.target.checked })} />{volumeText('readOnly')}</label>
  </div>;
}

type Props = { value: Execution; onChange(value: Execution): void; container: string; storage: string[]; secrets: string[]; disabled: boolean; secretCatalogStatus: 'loading' | 'ready' | 'error'; storageReady: boolean; onAddStorage?: (mount: {path: string; subPath: string; readOnly: boolean}) => void; onCreateSecret?: (mount: {path: string; mode: number}) => void };
export function ContainerVolumeFields(props: Props) {
  const {value, onChange, container, storage, secrets, disabled} = props;
  const [adding, setAdding] = useState(false);
  const count = [...(value.mounts ?? []), ...(value.volumeMounts ?? []), ...(value.secretFiles ?? [])].filter(m => m.container === container).length;
  return <fieldset disabled={disabled} className="editor-grid volume-fields" aria-label={text('sectionOf', {name: container})}>
    <div className="section-header"><h3>{text('title')}</h3><Button type="button" onClick={() => setAdding(true)}>{text('add')}</Button></div>
    {!count ? <small className="muted">{text('empty')}</small> : null}
    {(value.mounts ?? []).map((mount, index) => mount.container !== container ? null : <div className="content-panel editor-grid" key={`storage-${index}`}>
      <div className="section-header"><strong>{text('storage')}</strong><Button type="button" tone="quiet" onClick={() => onChange({...value, mounts: value.mounts!.filter((_, i) => i !== index)})}>{text('remove')}</Button></div>
      <label>{text('source')}<select aria-label={text('source')} value={mount.resource} onChange={e => onChange({...value, mounts: value.mounts!.map((m,i) => i === index ? {...m,resource:e.target.value} : m)})}>{options(storage,mount.resource)}</select></label>
      <MountPath path={mount.path} subPath={mount.subPath ?? ''} readOnly={mount.readOnly} onChange={patch => onChange({...value, mounts:value.mounts!.map((m,i) => i === index ? {...m,...patch} : m)})} />
      <small className="muted">{t('workloads.templates.retain')}</small>
    </div>)}
    {(value.volumeMounts ?? []).map((mount,index) => {
      if (mount.container !== container) return null;
      const volume = value.volumes?.find(v => v.name === mount.volume);
      const users = new Set(value.volumeMounts?.filter(m => m.volume === mount.volume).map(m => m.container));
      return <div className="content-panel editor-grid" key={`volume-${index}`}>
        <div className="section-header"><strong>{volume ? text(sourceKind(volume)) : text('missingSource')}</strong><Button type="button" tone="quiet" onClick={() => onChange({...value,volumeMounts:value.volumeMounts!.filter((_,i) => i !== index)})}>{text('remove')}</Button></div>
        <label>{text('source')}<select aria-label={text('source')} value={mount.volume} onChange={e => {
          const next=value.volumes?.find(v => v.name === e.target.value);
          onChange({...value,volumeMounts:value.volumeMounts!.map((m,i) => i === index ? {...m,volume:e.target.value,readOnly:!!(next?.configMap || next?.hostPath)} : m)});
        }}>{options((value.volumes ?? []).map(v => v.name),mount.volume)}</select></label>
        {!volume ? <p role="alert">{text('missingSource')}</p> : null}
        <MountPath path={mount.path} subPath={mount.subPath ?? ''} readOnly={mount.readOnly} locked={!!volume?.configMap} onChange={patch => onChange({...value,volumeMounts:value.volumeMounts!.map((m,i) => i === index ? {...m,...patch} : m)})} />
        {volume ? <details><summary>{users.size > 1 ? text('sharedSettings', {count:users.size}) : text('settings')}</summary>
          <VolumeSourceFields volume={volume} disabled={disabled} onChange={next => onChange({...value,volumes:value.volumes!.map(v => v.name === volume.name ? next : v)})} />
        </details> : null}
      </div>;
    })}
    {(value.secretFiles ?? []).map((file,index) => file.container !== container ? null : <div className="content-panel editor-grid" key={`secret-${index}`}>
      <div className="section-header"><strong>{text('secret')}</strong><Button type="button" tone="quiet" onClick={() => onChange({...value,secretFiles:value.secretFiles!.filter((_,i) => i !== index)})}>{text('remove')}</Button></div>
      <label>{t('workloads.templates.secret')}<select aria-label={t('workloads.templates.secret')} value={secretKey(file.ref)} onChange={e => onChange({...value,secretFiles:value.secretFiles!.map((m,i) => i === index ? {...m,ref:`\${resources.env.${e.target.value}}`} : m)})}>{options(secrets,secretKey(file.ref))}</select></label>
      <div className="field-grid"><label>{t('workloads.templates.filePath')}<input value={file.path} onChange={e => onChange({...value,secretFiles:value.secretFiles!.map((m,i) => i === index ? {...m,path:e.target.value} : m)})} /></label>
      <label>{t('workloads.templates.fileMode')}<select aria-label={t('workloads.templates.fileMode')} value={file.mode ?? 256} onChange={e => onChange({...value,secretFiles:value.secretFiles!.map((m,i) => i === index ? {...m,mode:Number(e.target.value)} : m)})}><option value={256}>0400</option><option value={288}>0440</option></select></label></div>
    </div>)}
    {adding ? <AddVolumeDialog {...props} onClose={() => setAdding(false)} /> : null}
  </fieldset>;
}

function AddVolumeDialog({ value, onChange, container, storage, secrets, disabled, storageReady, secretCatalogStatus, onAddStorage, onCreateSecret, onClose }: Props & {onClose(): void}) {
  const [kind,setKind]=useState<Kind>('emptyDir');
  const [selected,setSelected]=useState('');
  const uniqueName = () => {let name='volume';for(let n=2;value.volumes?.some(v => v.name === name);n++) name=`volume-${n}`;return name;};
  const makeVolume = (k:Kind):PodVolume => ({name:uniqueName(),...(k === 'hostPath' ? {hostPath:{path:'',type:'Directory'}} : k === 'configMap' ? {configMap:{files:[{name:'app.conf',value:''}]}} : {emptyDir:{}})});
  const [draft,setDraft]=useState<PodVolume>(() => makeVolume('emptyDir'));
  const existingPaths=[...(value.mounts ?? []),...(value.volumeMounts ?? []),...(value.secretFiles ?? [])].filter(m => m.container === container).map(m => m.path);
  const overlaps=(a:string,b:string) => a === b || a.startsWith(`${b}/`) || b.startsWith(`${a}/`);
  const defaultPath=(k:Kind) => {
    const base=k === 'storage' ? '/data' : k === 'secret' ? '/keys/value' : k === 'configMap' ? '/config' : k === 'hostPath' ? '/node-data' : '/shared';
    let candidate=base;for(let i=2;existingPaths.some(p => overlaps(p,candidate));i++)candidate=`${base}-${i}`;
    return candidate;
  };
  const [path,setPath]=useState(() => defaultPath('emptyDir'));
  const [subPath,setSubPath]=useState('');
  const [readOnly,setReadOnly]=useState(false);
  const [mode,setMode]=useState(256);
  const local=kind !== 'storage' && kind !== 'secret';
  const source=local ? selected ? value.volumes?.find(v => v.name === selected) : draft : undefined;
  const choices=kind === 'storage' ? storage : kind === 'secret' ? secrets : (value.volumes ?? []).filter(v => sourceKind(v) === kind).map(v => v.name);
  const candidate:Execution={version:1,kind:'Deployment',...(kind === 'storage' ? {mounts:[{resource:selected,container,path,subPath,readOnly}]} : kind === 'secret' ? {secretFiles:[{ref:`\${resources.env.${selected}}`,container,path,mode}]} : source ? {volumes:[source],volumeMounts:[{volume:source.name,container,path,subPath,readOnly}]} : {})};
  const valid=!!container && !!path && !existingPaths.some(p => overlaps(p,path)) && (local ? !!source && (!!selected || !value.volumes?.some(v => v.name === draft.name)) : choices.includes(selected)) && validRuntimeIntent([{name:container,image:'validation'}],candidate);
  function attach() {
    if (!valid || disabled) return;
    if (kind === 'storage') onChange({...value,mounts:[...(value.mounts ?? []),candidate.mounts![0]!]});
    else if (kind === 'secret') onChange({...value,secretFiles:[...(value.secretFiles ?? []),candidate.secretFiles![0]!]});
    else onChange({...value,...(!selected ? {volumes:[...(value.volumes ?? []),draft]} : {}),volumeMounts:[...(value.volumeMounts ?? []),candidate.volumeMounts![0]!]});
    onClose();
  }
  return <Dialog label={text('add')} busy={disabled} onClose={onClose}>
    <h2>{text('add')}</h2>
    <fieldset disabled={disabled} className="editor-grid volume-fields">
      <label>{text('kind')}<select aria-label={text('kind')} value={kind} onChange={e => {
        const k=e.target.value as Kind;setKind(k);setDraft(makeVolume(k));setSelected(k === 'storage' ? storage[0] ?? '' : k === 'secret' ? secrets[0] ?? '' : '');setReadOnly(k === 'hostPath' || k === 'configMap' || k === 'secret');setSubPath('');setPath(defaultPath(k));
      }}>{(['storage','emptyDir','configMap','hostPath','secret'] as const).map(k => <option key={k} value={k}>{text(k)}</option>)}</select></label>
      <label>{text('source')}<select aria-label={text('source')} value={selected} disabled={kind === 'secret' && secretCatalogStatus !== 'ready' || kind === 'storage' && !storageReady} onChange={e => setSelected(e.target.value)}>
        <option value="">{local ? text('createNew') : text('chooseSource')}</option>{choices.map(name => <option key={name} value={name}>{name}</option>)}
      </select></label>
      {local && !selected ? <VolumeSourceFields volume={draft} creating disabled={disabled} onChange={setDraft} /> : null}
      {kind === 'hostPath' && selected ? <small className="muted">{volumeText('hostPathHelp')}</small> : null}
      {kind === 'storage' ? <>
        {onAddStorage ? <Button type="button" onClick={() => {onClose();onAddStorage({path,subPath,readOnly});}}>{text('createStorage')}</Button> : !storage.length && storageReady ? <small className="muted">{t('workloads.templates.storageUnavailable')}</small> : null}
      </> : null}
      {kind === 'secret' ? <>
        {secretCatalogStatus === 'ready' ? onCreateSecret ? <Button type="button" onClick={() => {onClose();onCreateSecret({path,mode});}}>{volumeText('createSecret')}</Button> : null : <p role="status">{text(secretCatalogStatus === 'loading' ? 'loadingSecrets' : 'unavailableSecrets')}</p>}
        {secretCatalogStatus === 'ready' && !secrets.length ? <small className="muted">{volumeText('noSecrets')}</small> : null}
      </> : null}
      {kind === 'secret' ? <div className="field-grid"><label>{t('workloads.templates.filePath')}<input value={path} onChange={e => setPath(e.target.value)} /></label><label>{t('workloads.templates.fileMode')}<select aria-label={t('workloads.templates.fileMode')} value={mode} onChange={e => setMode(Number(e.target.value))}><option value={256}>0400</option><option value={288}>0440</option></select></label></div> : <MountPath path={path} subPath={subPath} readOnly={readOnly} locked={kind === 'configMap'} onChange={p => {if(p.path !== undefined)setPath(p.path);if(p.subPath !== undefined)setSubPath(p.subPath);if(p.readOnly !== undefined)setReadOnly(p.readOnly);}} />}
    </fieldset>
    <div className="form-actions"><Button type="button" onClick={onClose}>{text('cancel')}</Button><Button type="button" tone="primary" disabled={!valid || disabled || kind === 'secret' && secretCatalogStatus !== 'ready' || kind === 'storage' && !storageReady} onClick={attach}>{text('attach')}</Button></div>
  </Dialog>;
}

export function UnassignedVolumeMounts({value,onChange,containers,disabled}:{value:Execution;onChange(v:Execution):void;containers:string[];disabled:boolean}) {
  const fields=['mounts','volumeMounts','secretFiles'] as const;
  if(!fields.some(field => value[field]?.some(m => !containers.includes(m.container)))) return null;
  return <fieldset disabled={disabled} className="content-panel editor-grid"><h3>{text('unassigned')}</h3>{fields.flatMap(field => (value[field] ?? []).map((m,index) => {
    if(containers.includes(m.container)) return null;
    const update=(container:string) => {
      if(field === 'mounts') onChange({...value,mounts:value.mounts!.map((v,i) => i === index ? {...v,container} : v)});
      else if(field === 'volumeMounts') onChange({...value,volumeMounts:value.volumeMounts!.map((v,i) => i === index ? {...v,container} : v)});
      else onChange({...value,secretFiles:value.secretFiles!.map((v,i) => i === index ? {...v,container} : v)});
    };
    const remove=() => {
      if(field === 'mounts') onChange({...value,mounts:value.mounts!.filter((_,i) => i !== index)});
      else if(field === 'volumeMounts') onChange({...value,volumeMounts:value.volumeMounts!.filter((_,i) => i !== index)});
      else onChange({...value,secretFiles:value.secretFiles!.filter((_,i) => i !== index)});
    };
    return <div className="field-grid" key={`${field}-${index}`}><span>{m.container} · {m.path}</span><label>{volumeText('container')}<select aria-label={volumeText('container')} value={m.container} onChange={e => update(e.target.value)}>{options(containers,m.container)}</select></label><Button type="button" tone="quiet" onClick={remove}>{text('remove')}</Button></div>;
  }))}</fieldset>;
}
