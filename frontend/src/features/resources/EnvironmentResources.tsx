import { useEffect, useRef, useState } from 'react';
import { ApiError } from '../../shared/api/client';
import { navigate } from '../../app/routes';
import type { EnvironmentKey } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { Status } from '../../shared/ui/Status';
import { listResources, type ResourceSummary } from './api';
import { resourceStatusLabel, scopeLabel } from './labels';
import { t } from '../../shared/i18n';

/**
 * Resources of the selected Environment, the navigation entry to each detail. A resource that lost
 * its last Workload stays listed as UNREFERENCED (nothing destroys it). A failed load is an error
 * with retry, never an empty list, and a reply for another Environment is ignored.
 */
export function EnvironmentResources({ applicationId, environment, refreshKey }: { applicationId: string; environment: EnvironmentKey; refreshKey?: string }) {
  const [items, setItems] = useState<ResourceSummary[]>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [unavailable, setUnavailable] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const latest = useRef(0);
  useEffect(() => {
    const current = ++latest.current;
    setLoading(true); setError(''); setUnavailable(false); setItems(undefined);
    listResources(applicationId, environment).then((list) => { if (current === latest.current) setItems(list); })
      .catch((failure: unknown) => { if (current !== latest.current) return; if (failure instanceof ApiError && failure.status === 503 && failure.code === 'SERVICE_UNAVAILABLE') setUnavailable(true); else setError(t('resources.environmentResources.couldNotLoadTheResource')); })
      .finally(() => { if (current === latest.current) setLoading(false); });
    return () => { latest.current += 1; };
  }, [applicationId, environment, refreshKey, attempt]);
  return <section className="content-panel" aria-label={t('resources.environmentResources.environmentResources')}><div className="section-header"><div><h2>{t('resources.environmentResources.resources')}</h2><p>{t('resources.environmentResources.infrastructureResourcesManagedByOrchestrator', { environment })}</p></div><Button onClick={() => setAttempt((n) => n + 1)}>{t('resources.environmentResources.reload')}</Button></div>
    {loading ? <p role="status" aria-busy="true">{t('resources.environmentResources.loadingResources')}</p> : unavailable ? <div role="status" className="form-warning"><strong>{t('resources.environmentResources.theResourceListIsCurrently')}</strong><p>{t('resources.environmentResources.theBackendCouldNotProvide')}</p><Button onClick={() => setAttempt(n => n + 1)}>{t('resources.environmentResources.checkAgain')}</Button></div> : error ? <div role="alert" className="form-error">{error} <Button onClick={() => setAttempt((n) => n + 1)}>{t('resources.environmentResources.retry')}</Button></div> : items && items.length === 0 ? <p>{t('resources.environmentResources.noResourcesInEnvironmentYet', { environment })}</p> : items ? <div className="deployment-list">{items.map((item) => <button className="deployment-entry" key={item.id} onClick={() => navigate({ name: 'resource', applicationId, environment, resourceId: item.id })}><span><strong>{item.descriptor}</strong><small>{t('resources.environmentResources.typeClassScopeLabelGenerationGeneration', { type: item.type, class: item.class, scopeLabel: scopeLabel(item.scope), generation: item.generation })}</small></span><Status tone={item.status === 'READY' ? 'good' : item.status === 'UNREFERENCED' ? 'draft' : 'neutral'}>{resourceStatusLabel(item.status)}</Status></button>)}</div> : null}
  </section>;
}
