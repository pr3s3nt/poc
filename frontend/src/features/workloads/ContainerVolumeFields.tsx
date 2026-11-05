import { useState } from 'react';
import { t } from '../../shared/i18n';
import { Button } from '../../shared/ui/Button';
import { Dialog } from '../../shared/ui/Dialog';
import { VolumeSourceFields } from './VolumeSourceFields';
import { validRuntimeIntent } from './runtimeIntent';
import type { SharedResource } from './sharedResources';
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

type Props = { value: Execution; onChange(value: Execution): void; container: string; storage: string[]; secrets: string[]; disabled: boolean; secretCatalogStatus: 'loading' | 'ready' | 'error'; storageReady: boolean; storageStatus: 'ready' | 'loading' | 'error'; sharedStorage?: SharedResource[]; onUseStorage?: (id: string, mount: {path: string; subPath: string; readOnly: boolean}) => void; onAddStorage?: (mount: {path: string; subPath: string; readOnly: boolean}) => void; onCreateSecret?: (mount: {path: string; mode: number}) => void };
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

function AddVolumeDialog({ value, onChange, container, storage, secrets, disabled, storageReady, storageStatus, secretCatalogStatus, sharedStorage = [], onUseStorage, onAddStorage, onCreateSecret, onClose }: Props & {onClose(): void}) {
  type Choice = {id: string; name: string; kind: Kind; detail?: string; users: string[]; sharedId?: string};
  const [creating,setCreating]=useState(false);
  const [query,setQuery]=useState('');
  const [filter,setFilter]=useState<Kind | 'all'>('all');
  const [choiceId,setChoiceId]=useState('');
  const [kind,setKind]=useState<Kind>('emptyDir');
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
  const users = (mounts: {container: string}[]) => [...new Set(mounts.map(m => m.container))];
  const choices:Choice[] = [
    ...storage.map(name => ({id:`storage:${name}`,name,kind:'storage' as const,users:users((value.mounts ?? []).filter(m => m.resource === name))})),
    ...sharedStorage.filter(s => s.dependency).map(s => ({id:`shared:${s.id}`,name:s.dependency!.alias,kind:'storage' as const,detail:s.label,users:[],sharedId:s.id})),
    ...(value.volumes ?? []).map(v => ({id:`volume:${v.name}`,name:v.name,kind:sourceKind(v),users:users((value.volumeMounts ?? []).filter(m => m.volume === v.name))})),
    ...secrets.map(name => ({id:`secret:${name}`,name,kind:'secret' as const,users:users((value.secretFiles ?? []).filter(m => secretKey(m.ref) === name))})),
  ];
  const selectedChoice=choices.find(c => c.id === choiceId);
  const selected=creating ? '' : selectedChoice?.name ?? '';
  const local=kind !== 'storage' && kind !== 'secret';
  const source=local ? creating ? draft : value.volumes?.find(v => v.name === selected) : undefined;
  const candidate:Execution={version:1,kind:'Deployment',...(kind === 'storage' ? {mounts:[{resource:selected,container,path,subPath,readOnly}]} : kind === 'secret' ? {secretFiles:[{ref:`\${resources.env.${selected}}`,container,path,mode}]} : source ? {volumes:[source],volumeMounts:[{volume:source.name,container,path,subPath,readOnly}]} : {})};
  const ready=kind === 'storage' ? storageReady : kind === 'secret' ? secretCatalogStatus === 'ready' : true;
  const valid=ready && !!container && !!path && !existingPaths.some(p => overlaps(p,path)) && (creating ? local && !!source && !value.volumes?.some(v => v.name === draft.name) : !!selectedChoice) && validRuntimeIntent([{name:container,image:'validation'}],candidate);
  const visible=choices.filter(c => (filter === 'all' || c.kind === filter) && `${c.name} ${text(c.kind)} ${c.detail ?? ''} ${c.users.join(' ')}`.toLocaleLowerCase().includes(query.trim().toLocaleLowerCase()));
  function changeKind(k:Kind) {setKind(k);setReadOnly(k === 'hostPath' || k === 'configMap' || k === 'secret');setSubPath('');setPath(defaultPath(k));}
  function attach() {
    if (!valid || disabled) return;
    if (selectedChoice?.sharedId && !creating) {onUseStorage?.(selectedChoice.sharedId,{path,subPath,readOnly});onClose();return;}
    if (kind === 'storage') onChange({...value,mounts:[...(value.mounts ?? []),candidate.mounts![0]!]});
    else if (kind === 'secret') onChange({...value,secretFiles:[...(value.secretFiles ?? []),candidate.secretFiles![0]!]});
    else onChange({...value,...(creating ? {volumes:[...(value.volumes ?? []),draft]} : {}),volumeMounts:[...(value.volumeMounts ?? []),candidate.volumeMounts![0]!]});
    onClose();
  }
  return <Dialog label={text('add')} busy={disabled} onClose={onClose}>
    <h2>{text('add')}</h2>
    <fieldset disabled={disabled} className="editor-grid volume-fields">
      {!creating ? <>
        <div className="field-grid"><label>{text('search')}<input type="search" value={query} onChange={e => {setQuery(e.target.value);setChoiceId('');}} /></label>
          <label>{text('filter')}<select aria-label={text('filter')} value={filter} onChange={e => {setFilter(e.target.value as Kind | 'all');setChoiceId('');}}><option value="all">{text('allTypes')}</option>{(['storage','emptyDir','configMap','hostPath','secret'] as const).map(k => <option key={k} value={k}>{text(k)}</option>)}</select></label></div>
        {!storageReady ? <small role="status">{text(storageStatus === 'loading' ? 'loadingStorage' : 'unavailableStorage')}</small> : null}
        {secretCatalogStatus !== 'ready' ? <small role="status">{text(secretCatalogStatus === 'loading' ? 'loadingSecrets' : 'unavailableSecrets')}</small> : null}
        <div className="volume-source-list" aria-label={text('existing')}>
          {visible.map(c => <button type="button" key={c.id} aria-label={text('useSource',{name:c.detail ?? c.name,kind:text(c.kind)})} aria-pressed={choiceId === c.id} disabled={disabled || c.kind === 'storage' && (!storageReady || !!c.sharedId && !onUseStorage) || c.kind === 'secret' && secretCatalogStatus !== 'ready'} onClick={() => {setChoiceId(c.id);changeKind(c.kind);}}>
            <strong>{c.name}</strong><span>{text(c.kind)}</span>{c.detail ? <small>{c.detail}</small> : null}{c.users.length ? <small>{text('mountedIn',{names:c.users.join(', ')})}</small> : null}
          </button>)}
        </div>
        {!visible.length && (choices.length || storageReady && secretCatalogStatus === 'ready') ? <p className="muted">{text(query.trim() || filter !== 'all' ? 'noMatches' : 'noSources')}</p> : null}
        <Button type="button" onClick={() => {setCreating(true);changeKind('emptyDir');setDraft(makeVolume('emptyDir'));}}>{text('createNew')}</Button>
        {choiceId && !selectedChoice ? <p role="alert">{text('missingSource')}</p> : null}
      </> : <>
        <Button type="button" tone="quiet" onClick={() => {setCreating(false);setChoiceId('');}}>{text('backToSources')}</Button>
        <label>{text('kind')}<select aria-label={text('kind')} value={kind} onChange={e => {const k=e.target.value as Kind;changeKind(k);setDraft(makeVolume(k));}}>{(['storage','emptyDir','configMap','hostPath','secret'] as const).map(k => <option key={k} value={k}>{text(k)}</option>)}</select></label>
        {local ? <VolumeSourceFields volume={draft} creating disabled={disabled} onChange={setDraft} /> : null}
        {kind === 'storage' ? onAddStorage ? <Button type="button" onClick={() => {onClose();onAddStorage({path,subPath,readOnly});}}>{text('createStorage')}</Button> : <small className="muted">{t('workloads.templates.storageUnavailable')}</small> : null}
        {kind === 'secret' ? secretCatalogStatus === 'ready' && onCreateSecret ? <Button type="button" onClick={() => {onClose();onCreateSecret({path,mode});}}>{volumeText('createSecret')}</Button> : <p role="status">{text(secretCatalogStatus === 'loading' ? 'loadingSecrets' : 'unavailableSecrets')}</p> : null}
      </>}
      {kind === 'hostPath' && !creating && selectedChoice ? <small className="muted">{volumeText('hostPathHelp')}</small> : null}
      {creating || selectedChoice ? kind === 'secret' ? <div className="field-grid"><label>{t('workloads.templates.filePath')}<input value={path} onChange={e => setPath(e.target.value)} /></label><label>{t('workloads.templates.fileMode')}<select aria-label={t('workloads.templates.fileMode')} value={mode} onChange={e => setMode(Number(e.target.value))}><option value={256}>0400</option><option value={288}>0440</option></select></label></div> : <MountPath path={path} subPath={subPath} readOnly={readOnly} locked={kind === 'configMap'} onChange={p => {if(p.path !== undefined)setPath(p.path);if(p.subPath !== undefined)setSubPath(p.subPath);if(p.readOnly !== undefined)setReadOnly(p.readOnly);}} /> : null}
    </fieldset>
    <div className="form-actions"><Button type="button" onClick={onClose}>{text('cancel')}</Button><Button type="button" tone="primary" disabled={!valid || disabled} onClick={attach}>{text('attach')}</Button></div>
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
