import { t } from '../../shared/i18n';
import type { CapabilitySource } from './capabilities';
import type { RenderSource } from './metadata';
import type { CatalogModuleSelection } from './CatalogModuleSource';

type Source = { kind: string; url?: string; path?: string; revision?: string };
export function moduleOrigin(source: CapabilitySource): Source {
 return {kind: source.sourceId.startsWith('zip-') ? 'zip' : source.repositoryUrl || source.sourceId.startsWith('git-') ? 'git' : 'platform',url:source.repositoryUrl,path:source.subdirectory,revision:source.revision};
}
function links(source: Source) {
 try {
  const url=new URL(source.url ?? '');
  if(url.protocol!=='https:'||url.username||url.password||url.search||url.hash)return {};
  const repository=url.href;
  if(!/^[a-f0-9]{40,64}$/.test(source.revision ?? ''))return {repository};
  const path=(source.path ?? '').split('/').filter(p=>p!=='.').map(encodeURIComponent).join('/');
  const base=repository.replace(/\.git\/?$/,'').replace(/\/$/,'');
  const code=url.hostname==='github.com'?`${base}/tree/${source.revision}/${path}`:url.hostname==='gitlab.com'?`${base}/-/tree/${source.revision}/${path}`:undefined;
  return {repository,code};
 }catch{return {};}
}
export function SavedSourceSummary({source,catalog}:{source?:Source|RenderSource;catalog?:CatalogModuleSelection}) {
 const kind=catalog?'Catalog':source?.kind==='git'?'Git':source?.kind==='zip'?'ZIP':source?.kind==='inline'?'Inline':source?.kind==='registered'?t('sourceSummary.registered'):t('sourceSummary.platform');
 const urls=source?links(source):{};
 return <dl className="pin-panel saved-source-summary" aria-label={t('sourceSummary.saved')}>
  <div><dt>{t('platform.resourceDefinitions.source')}</dt><dd>{kind}</dd></div>
  {catalog?<><div><dt>{t('sourceSummary.catalogSource')}</dt><dd>{catalog.sourceId}</dd></div><div><dt>{t('sourceSummary.version')}</dt><dd>{catalog.tag}</dd></div><div><dt>Module</dt><dd>{catalog.componentId}</dd></div></>:null}
  {source?.kind==='git'?<div><dt>{t('sourceSummary.repository')}</dt><dd>{urls.repository?<a href={urls.repository} target="_blank" rel="noreferrer">{source.url}</a>:<code>{source.url ?? t('sourceSummary.unavailable')}</code>}</dd></div>:null}
  {source?.path?<div><dt>{t('platform.resourceDefinitions.subdirectory')}</dt><dd><code>{source.path}</code></dd></div>:null}
  {source?.revision?<div><dt>{source.kind==='git'?'Commit':t('sourceSummary.version')}</dt><dd>{urls.code?<a href={urls.code} target="_blank" rel="noreferrer"><code>{source.revision}</code></a>:<code>{source.revision}</code>}</dd></div>:null}
 </dl>;
}
