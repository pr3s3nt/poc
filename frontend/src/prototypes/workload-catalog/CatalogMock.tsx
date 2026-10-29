import { useState, type ReactNode } from 'react';
import { AppShell } from '../../app/AppShell';
import { Button } from '../../shared/ui/Button';
import { Dialog } from '../../shared/ui/Dialog';
import { ApplicationHomePage } from '../../features/applications/ApplicationHomePage';
import { application } from './fixtures';
import { t } from '../../shared/i18n';
import './mock.css';

const msg = (key: string) => t(`catalog.${key}`);
const templates = [
  { id: 'frontend', kind: 'Deployment', icon: '▤', color: 'blue', image: 'registry.example/frontend:v1', port: '8080' },
  { id: 'backend', kind: 'Deployment', icon: '⌘', color: 'teal', image: 'registry.example/backend:v1', port: '8080' },
  { id: 'worker', kind: 'Deployment', icon: '↻', color: 'violet', image: 'registry.example/worker:v1', port: '' },
  { id: 'stateful', kind: 'StatefulSet', icon: '▱', color: 'amber', image: 'registry.example/passbolt:staging', port: '80' },
  { id: 'cron', kind: 'CronJob', icon: '◷', color: 'rose', image: 'registry.example/job:v1', port: '' },
] as const;
type Template = typeof templates[number];
type Volume = { id: number; source: string; name: string; size: string; mount: string; readonly: boolean };
type Variable = { id: number; name: string; value: string };
function Field({ label, children }: { label: string; children: ReactNode }) { return <label>{msg(label)}{children}</label>; }

export function CatalogMock() {
  const [screen, setScreen] = useState<'home' | 'configure' | 'review' | 'example'>('home');
  const [environment, setEnvironment] = useState<'staging' | 'production'>('staging');
  const [pickerOpen, setPickerOpen] = useState(false);
  const [template, setTemplate] = useState<Template>(templates[0]);
  const [query, setQuery] = useState('');
  const [filter, setFilter] = useState('all');
  const [name, setName] = useState('');
  const [image, setImage] = useState('');
  const [port, setPort] = useState('');
  const [replicas, setReplicas] = useState('1');
  const [schedule, setSchedule] = useState('0 0 * * *');
  const [timezone, setTimezone] = useState('Asia/Ho_Chi_Minh');
  const [concurrency, setConcurrency] = useState('forbid');
  const [command, setCommand] = useState('');
  const [volumes, setVolumes] = useState<Volume[]>([]);
  const [resources, setResources] = useState<string[]>([]);
  const [reuseResources, setReuseResources] = useState(false);
  const [variables, setVariables] = useState<Variable[]>([]);
  const [error, setError] = useState(false);
  const [cpu, setCpu] = useState('100m');
  const [memory, setMemory] = useState('128Mi');
  const [probe, setProbe] = useState('/health');
  const cron = template.kind === 'CronJob';
  function choose(item: Template, backup = false) {
    setTemplate(item); setName(backup ? 'passbolt-mysql-auto-backup' : item.id === 'stateful' ? 'passbolt' : item.id);
    setImage(backup ? 'registry.example/mysql-backup:v1' : item.image); setPort(item.port); setReplicas('1'); setError(false);
    setSchedule('0 0 * * *'); setTimezone('Asia/Ho_Chi_Minh'); setConcurrency('forbid');
    setCpu('100m'); setMemory('128Mi'); setProbe('/health');
    setReuseResources(backup);
    setCommand(backup ? '/scripts/backup.sh' : ''); setResources(backup ? ['mysql'] : []);
    setVolumes(backup ? [{id: 1, source: 'existing', name: 'backup-storage', size: '20', mount: '/backups', readonly: false}] : []);
    setVariables(backup ? [{id: 1, name: 'DB_HOST', value: '${resources.mysql.host}'}, {id: 2, name: 'DB_PASSWORD', value: 'secret: MYSQL_BACKUP_PASSWORD'}] : []);
    setPickerOpen(false); setScreen('configure');
  }
  function patchVolume(id: number, patch: Partial<Volume>) { setVolumes(v => v.map(x => x.id === id ? {...x, ...patch} : x)); }
  const filtered = templates.filter(x => (filter === 'all' || (filter === 'scheduled') === (x.kind === 'CronJob')) && `${msg(x.id)} ${x.kind}`.toLowerCase().includes(query.toLowerCase()));
  return <div className="catalog-prototype" onClickCapture={event => { if ((event.target as HTMLElement).closest('.sidebar a, .sidebar-footer button')) { event.preventDefault(); event.stopPropagation(); } }}>
    <AppShell username="developer" role="ADMIN" activeRoute="workload" onSignOut={() => undefined}>
      {screen === 'home' && <div className="catalog-home">
        <div className="catalog-banner">{msg('mock')}</div>
        <div onClickCapture={event => {
          const button = (event.target as HTMLElement).closest('button');
          if (!button) return;
          if (button.textContent === t('applications.applicationHome.addWorkload')) {
            event.preventDefault(); event.stopPropagation(); setQuery(''); setFilter('all'); setPickerOpen(true);
          } else if (button.closest('.tabs')) { setEnvironment(button.textContent?.startsWith('production') ? 'production' : 'staging'); } else { event.preventDefault(); event.stopPropagation(); }
        }}><ApplicationHomePage application={application} initialEnvironment={environment} /></div>
      </div>}
      {screen !== 'home' && <section className="page catalog-page">
        <div className="catalog-banner">{msg('mock')}</div>
        <div className="catalog-breadcrumb">{t('app.appShell.applications')} / Passbolt / {environment} / {msg('title')}</div>
        <header className="page-header"><div><p className="eyebrow">Passbolt · {environment}</p><h1>{screen === 'example' ? msg('exampleTitle') : msg('title')}</h1></div><Button onClick={() => setScreen(screen === 'example' ? 'home' : 'example')}>{msg(screen === 'example' ? 'back' : 'example')}</Button></header>
        <div className="tabs">{(['staging', 'production'] as const).map(env => <button key={env} className={env === environment ? 'tab tab-active' : 'tab'} type="button" disabled={env !== environment}>{env}</button>)}</div>
        {screen !== 'example' && <ol className="catalog-steps">{['configure', 'review'].map((step, index) => <li key={step} aria-current={screen === step ? 'step' : undefined}><span>{index + 1}</span>{msg(step)}</li>)}</ol>}
        {screen === 'configure' && <form onSubmit={e => { e.preventDefault(); if (!/^[a-z][a-z0-9-]*$/.test(name) || !image.trim() || volumes.some(v => !v.mount.startsWith('/') || !v.name.trim())) {setError(true); return;} setError(false); setScreen('review'); }}>
          <div className="catalog-layout"><div>
            <section className="content-panel editor-grid"><div className="section-header"><h2>{msg('configure')}</h2><span className="catalog-kind">{template.kind}</span></div><div className="field-grid"><Field label="name"><input required value={name} onChange={e=>setName(e.target.value)} /></Field><Field label="image"><input required value={image} onChange={e=>setImage(e.target.value)} /></Field></div>
              {cron ? <><div className="field-grid"><Field label="schedule"><input required value={schedule} onChange={e=>setSchedule(e.target.value)} /></Field><Field label="timezone"><select value={timezone} onChange={e=>setTimezone(e.target.value)}><option>Asia/Ho_Chi_Minh</option><option>UTC</option></select></Field></div><Field label="concurrency"><select value={concurrency} onChange={e=>setConcurrency(e.target.value)}>{['forbid','allow','replace'].map(x=><option key={x} value={x}>{msg(x)}</option>)}</select></Field></> : <div className="field-grid"><Field label="replicas"><input type="number" min="1" max={template.id==='stateful'?1:20} value={replicas} onChange={e=>setReplicas(e.target.value)} /></Field>{template.id !== 'worker' && <Field label="port"><input type="number" min="1" max="65535" value={port} onChange={e=>setPort(e.target.value)} /></Field>}</div>}
              <Field label="command"><input value={command} onChange={e=>setCommand(e.target.value)} placeholder={cron?'/scripts/job.sh':''} /></Field>
            </section>
            <section className="content-panel editor-grid"><div className="section-header"><h2>{msg('resources')}</h2><select aria-label={msg('addResource')} value="" onChange={e=>{if(e.target.value)setResources([...resources,e.target.value]);}}><option value="">+ {msg('addResource')}</option>{['mysql','postgres','redis'].filter(x=>!resources.includes(x)).map(x=><option key={x} value={x}>{msg(x)}</option>)}</select></div>{!resources.length?<p>{msg('none')}</p>:resources.map(r=><div className="catalog-row" key={r}><strong>{msg(r)}</strong><span className="catalog-kind">{msg(reuseResources ? 'reuse' : 'create')}</span><Button type="button" tone="quiet" aria-label={`${msg('remove')} ${r}`} onClick={()=>setResources(resources.filter(x=>x!==r))}>{msg('remove')}</Button></div>)}</section>
            <section className="content-panel editor-grid"><div className="section-header"><h2>{msg('storage')}</h2><Button type="button" onClick={()=>setVolumes([...volumes,{id:Date.now(),source:'new',name:`${name}-data`,size:'2',mount:'/app/data',readonly:false}])}>+ {msg('addStorage')}</Button></div>
              {volumes.map(v=><div className="catalog-volume" key={v.id}><div className="field-grid"><Field label="source"><select value={v.source} onChange={e=>patchVolume(v.id,{source:e.target.value,name:e.target.value==='existing'?'backup-storage':`${name}-data`})}><option value="new">{msg('newStorage')}</option><option value="existing">{msg('existingStorage')}</option></select></Field><Field label="storageName">{v.source==='new'?<input required value={v.name} onChange={e=>patchVolume(v.id,{name:e.target.value})}/>:<select value={v.name} onChange={e=>patchVolume(v.id,{name:e.target.value})}><option>backup-storage</option><option>public-images</option></select>}</Field><Field label="mount"><input required value={v.mount} onChange={e=>patchVolume(v.id,{mount:e.target.value})}/></Field>{v.source==='new'&&<Field label="size"><input required type="number" min="1" value={v.size} onChange={e=>patchVolume(v.id,{size:e.target.value})}/></Field>}</div><div className="catalog-row"><label className="catalog-checkbox"><input type="checkbox" checked={v.readonly} onChange={e=>patchVolume(v.id,{readonly:e.target.checked})}/>{msg('readonly')}</label><Button type="button" tone="quiet" onClick={()=>setVolumes(volumes.filter(x=>x.id!==v.id))}>{msg('remove')}</Button></div><small className="muted">{msg('retain')}</small></div>)}
              {!volumes.length && <p>{msg('storageHint')}</p>}
            </section>
            <section className="content-panel editor-grid"><div className="section-header"><h2>{msg('bindings')}</h2><Button type="button" onClick={()=>setVariables([...variables,{id:Date.now(),name:'',value:''}])}>+ {msg('addVariable')}</Button></div>{variables.map(v=><div className="catalog-variable" key={v.id}><input aria-label={msg('variableName')} placeholder="DB_HOST" value={v.name} onChange={e=>setVariables(variables.map(x=>x.id===v.id?{...x,name:e.target.value}:x))}/><input aria-label={msg('value')} value={v.value} onChange={e=>setVariables(variables.map(x=>x.id===v.id?{...x,value:e.target.value}:x))}/><Button type="button" tone="quiet" onClick={()=>setVariables(variables.filter(x=>x.id!==v.id))}>{msg('remove')}</Button></div>)}<small className="muted">{msg('secretHint')}</small></section>
            <details className="content-panel editor-grid"><summary>{msg('advanced')}</summary><div className="field-grid"><Field label="cpu"><input value={cpu} onChange={e=>setCpu(e.target.value)}/></Field><Field label="memory"><input value={memory} onChange={e=>setMemory(e.target.value)}/></Field>{!cron&&template.id!=='worker'&&<Field label="probe"><input value={probe} onChange={e=>setProbe(e.target.value)}/></Field>}</div></details>
          </div><aside className="content-panel catalog-summary"><h2>{msg('summary')}</h2><dl><dt>{msg('template')}</dt><dd>{msg(template.id)}</dd><dt>{msg('version')}</dt><dd>1.0.0</dd><dt>{msg('runtime')}</dt><dd>{template.kind}</dd><dt>{msg('resources')}</dt><dd>{resources.length}</dd><dt>{msg('storage')}</dt><dd>{volumes.length}</dd></dl><p className="muted">{msg('fixture')}</p></aside></div>
          {error&&<p role="alert" className="form-error">{msg('error')}</p>}<div className="form-actions editor-actions"><Button type="button" onClick={()=>setScreen('home')}>{msg('back')}</Button><Button type="submit" tone="primary">{msg('save')}</Button></div>
        </form>}
        {screen === 'review' && <><p className="form-success" role="status">{msg('saved')}</p><section className="content-panel"><div className="section-header"><h2>{name}</h2><span className="pending-pill">{msg('draft')}</span></div><table className="plain-table"><tbody>{[[msg('template'),msg(template.id)],['Image',image],[msg('runtime'),template.kind],...(cron?[[msg('schedule'),`${schedule} · ${timezone}`],[msg('concurrency'),msg(concurrency)]]:[[msg('replicas'),replicas],[msg('port'),port||'—']]),[msg('command'),command||'—'],[msg('cpu'),cpu],[msg('memory'),memory],...resources.map(r=>[msg('resources'),`${r} · ${msg(reuseResources ? 'reuse' : 'create')}`]),...volumes.map(v=>[msg('storage'),`${v.name} → ${v.mount} · ${msg(v.source==='new'?'create':'reuse')} · ${msg('retain')}`]),...variables.map(v=>[v.name,v.value.startsWith('secret:')?'••••••':v.value])].map(([key,value],i)=><tr key={i}><th>{key}</th><td>{value}</td></tr>)}</tbody></table></section><div className="form-actions"><Button onClick={()=>setScreen('configure')}>{msg('edit')}</Button><Button tone="primary" onClick={()=>setScreen('home')}>{msg('startOver')}</Button></div><p className="muted">{msg('noApi')}</p></>}
        {screen === 'example' && <><p className="form-info">{msg('exampleNote')}</p><div className="catalog-example"><section className="content-panel"><h2>{msg('resourceList')}</h2>{[['mysql','MySQL · 50 Gi'],['public-images','PVC · 2 Gi'],['backup-storage','PVC / object storage']].map(([n,d])=><div className="catalog-row" key={n}><strong>{n}</strong><span>{d}</span></div>)}</section><section className="content-panel"><h2>{msg('workloads')}</h2>{[['passbolt','StatefulSet','mysql + public-images'],['backup','CronJob · 00:00','mysql + backup-storage'],['cleanup','CronJob · 01:00','backup-storage']].map(([n,k,d])=><div className="catalog-example-workload" key={n}><strong>{n}</strong><span className="catalog-kind">{k}</span><small>↳ {d}</small></div>)}</section></div><p className="muted">{msg('backupNote')}</p><Button tone="primary" onClick={()=>choose(templates[4],true)}>{msg('editBackup')}</Button></>}
      </section>}
              {pickerOpen && <Dialog label={msg('selectType')} onClose={() => setPickerOpen(false)}>
          <div className="section-header"><h2>{msg('selectType')}</h2><Button onClick={() => setPickerOpen(false)}>{msg('cancel')}</Button></div>
          <div className="catalog-toolbar"><input aria-label={msg('search')} placeholder={msg('search')} value={query} onChange={e => setQuery(e.target.value)} /><div className="source-choices">{['all', 'continuous', 'scheduled'].map(x => <Button key={x} tone={filter === x ? 'primary' : 'quiet'} aria-pressed={filter === x} onClick={() => setFilter(x)}>{msg(x)}</Button>)}</div></div>
          <div className="catalog-cards">{filtered.map(item => <article className="content-panel catalog-card" key={item.id}><span className="catalog-symbol" data-tone={item.color} aria-hidden="true">{item.icon}</span><h2>{msg(item.id)}</h2><p>{msg(`${item.id}Desc`)}</p><span className="catalog-kind">{item.kind}</span><Button onClick={() => choose(item)} aria-label={`${msg('choose')}: ${msg(item.id)}`}>{msg('choose')} →</Button></article>)}</div>
          {!filtered.length && <div className="empty-state">{msg('empty')}</div>}<p className="muted">{msg('resourceCatalog')}</p>
        </Dialog>}

    </AppShell>
  </div>;
}
