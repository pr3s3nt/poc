import { useRef, useState, useEffect } from 'react';
import { api, describeError, isStaleConflict } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { Dialog } from '../../shared/ui/Dialog';
import { t } from '../../shared/i18n';
import type { EnvironmentKey } from '../../shared/types/application';
import type { DeployReport } from '../workloads/api';

type Preview = { token: string; targetOperationId?: string; empty: boolean; restore: string[]; remove: string[]; resources?: string[] };

export function EnvironmentRollback({ applicationId, environment, disabled, onCompleted }: {applicationId:string;environment:EnvironmentKey;disabled:boolean;onCompleted():void}) {
 const [preview,setPreview]=useState<Preview>();
 const [busy,setBusy]=useState(false);
 const [confirmation,setConfirmation]=useState('');
 const [error,setError]=useState('');
 const [completed,setCompleted]=useState(false);
 const mounted=useRef(true);
 useEffect(()=>{mounted.current=true;return ()=>{mounted.current=false;};},[]);
 const base=`/applications/${encodeURIComponent(applicationId)}/environments/${environment}`;
 async function inspect(){
  if(busy||disabled)return;
  setBusy(true);setError('');setCompleted(false);setConfirmation('');
  try{const value=await api<Preview>(`${base}/rollback-preview`,{method:'POST',body:'{}'});if(mounted.current)setPreview(value);}
  catch(cause){if(mounted.current)setError(describeError(cause,t('deployments.rollback.previewError')));}
  finally{if(mounted.current)setBusy(false);}
 }
 async function restore(){
  if(!preview||busy||disabled||confirmation!==applicationId)return;
  setBusy(true);setError('');
  try{
   const report=await api<DeployReport>(`${base}/rollback`,{method:'POST',body:JSON.stringify({token:preview.token,confirmation})});
   if(!mounted.current)return;
   if(report.status!=='SUCCEEDED'){setError(t('deployments.rollback.failed'));return;}
   setPreview(undefined);setCompleted(true);onCompleted();
  }catch(cause){if(mounted.current){setError(describeError(cause,t('deployments.rollback.failed')));if(isStaleConflict(cause))setPreview(undefined);}}
  finally{if(mounted.current)setBusy(false);}
 }
 return <section className="content-panel">
  <div className="section-header"><h2>{t('deployments.rollback.title')}</h2><Button disabled={disabled||busy} onClick={inspect}>{t('deployments.rollback.preview')}</Button></div>
  {error&&!preview?<div role="alert" className="form-error">{error}</div>:null}
  {completed?<p role="status" className="form-success">{t('deployments.rollback.succeeded')}</p>:null}
  {busy&&!preview?<p role="status">{t('deployments.rollback.loading')}</p>:null}
  {preview?<Dialog label={t('deployments.rollback.confirmTitle')} busy={busy} onClose={()=>setPreview(undefined)}>
   <h2>{t('deployments.rollback.confirmTitle')}</h2>
   <p>{t(preview.empty?'deployments.rollback.emptyWarning':'deployments.rollback.scopeWarning',{environment})}</p>
   {preview.restore.length?<p>{t('deployments.rollback.restore')}: {preview.restore.join(', ')}</p>:null}
   {preview.remove.length?<p>{t('deployments.rollback.remove')}: {preview.remove.join(', ')}</p>:null}
   {preview.resources?.length?<p>{t('deployments.rollback.resources')}: {preview.resources.join(', ')}</p>:null}
   <label>{t('deployments.rollback.confirmName',{name:applicationId})}<input autoComplete="off" value={confirmation} disabled={busy} onChange={event=>setConfirmation(event.target.value)}/></label>
   {error?<div role="alert" className="form-error">{error}</div>:null}
   {busy?<p role="status">{t('deployments.rollback.running')}</p>:null}
   <div className="form-actions"><Button disabled={busy} onClick={()=>setPreview(undefined)}>{t('deployments.rollback.cancel')}</Button><Button tone="danger" disabled={busy||disabled||confirmation!==applicationId} onClick={restore}>{t('deployments.rollback.confirm')}</Button></div>
  </Dialog>:null}
 </section>;
}
