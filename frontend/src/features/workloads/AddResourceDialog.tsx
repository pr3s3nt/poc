import { useState } from 'react';
import { Button } from '../../shared/ui/Button';
import { Dialog } from '../../shared/ui/Dialog';
import type { ResourceType } from './api';
import { DependencyFields, dependencyProblem, emptyDependency, reservedAlias, type ClassMetadata, type Dependency } from './DependencyFields';
import { t } from '../../shared/i18n';
import { sharedSelectionProblem, type SharedResource } from './sharedResources';

// Adds one dependency to the workload draft from an environment-variable row (UC-16 BR-22).
// It edits form state only: no HTTP call, no provisioning, no output chosen for the user.
export function AddResourceDialog({ existing, types, choices = [], classMetadata, onReload, onAdd, onClose }: {
  existing: Dependency[]; types: ResourceType[]; choices?: SharedResource[]; classMetadata: ClassMetadata;
  onReload(): void;
  onAdd(dependency: Dependency): void;
  onClose(): void;
}) {
  const [draft, setDraft] = useState<Dependency>(emptyDependency);
  const [error, setError] = useState('');

  function submit() {
    if (!draft.alias.trim() || !/^[A-Za-z0-9_-]+$/.test(draft.alias)) { setError(t('workloads.addResource.aliasInvalid')); return; }
    if (reservedAlias(draft.alias)) { setError(t('workloads.addResource.aliasReserved', { alias: draft.alias })); return; }
    if (existing.some((item) => item.alias === draft.alias)) { setError(t('workloads.addResource.aliasTaken', { alias: draft.alias })); return; }
    if (classMetadata !== 'ready') { setError(classMetadata === 'loading' ? t('workloads.workloadEditor.classMetadataIsLoadingWait') : t('workloads.workloadEditor.theResourceClassMetadataIs')); return; }
    const problem = sharedSelectionProblem(draft, choices) || dependencyProblem(draft, types);
    if (problem) { setError(problem); return; }
    onAdd(draft);
  }

  return <Dialog label={t('workloads.addResource.title')} onClose={onClose}>
    <h2>{t('workloads.addResource.title')}</h2>
    <DependencyFields index={0} dependency={draft} types={types} choices={choices} classMetadata={classMetadata} onChange={(patch) => { setDraft({ ...draft, ...patch }); setError(''); }} onReload={onReload} />
    {error ? <div className="form-error" role="alert">{error}</div> : null}
    <div className="form-actions"><Button type="button" onClick={onClose}>{t('workloads.addResource.cancel')}</Button><Button tone="primary" type="button" onClick={submit}>{t('workloads.addResource.submit')}</Button></div>
  </Dialog>;
}
