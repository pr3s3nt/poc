import type { Placement } from '../workloads/api';
import { t } from '../../shared/i18n';

const sourceLabels: Record<string, string> = { 'workload-reference': t('preview.placements.workloadReference'), 'provider-binding': t('preview.placements.moduleProviderBinding') };
const providerLabels: Record<string, string> = { 'existing-cluster': t('preview.placements.existingCluster'), eks: t('preview.placements.eKSCreatedInThisDeployment') };
const providerLabel = (key?: string) => (key && Object.hasOwn(providerLabels, key) ? providerLabels[key] : '—');
const label = (key: string) => (Object.hasOwn(sourceLabels, key) ? sourceLabels[key] : key);

// Per workload/resource target provenance (ADR-016): which cluster Definition and
// Internal account a node runs on and, for a workload, which namespace. Names and
// versions only; never a credential, kubeconfig, endpoint or secret reference.
export function Placements({ placements, label: ariaLabel = t('preview.placements.deploymentTargetOfEachComponent') }: { placements: Placement[] | undefined; label?: string }) {
  if (!placements || placements.length === 0) return null;
  return <div className="placements" aria-label={ariaLabel}>
    <table className="plain-table">
      <thead><tr><th>{t('preview.placements.component')}</th><th>Cluster</th><th>{t('preview.placements.clusterDefinition')}</th><th>{t('preview.placements.clusterType')}</th><th>{t('preview.placements.account')}</th><th>{t('preview.placements.region')}</th><th>Namespace</th><th>{t('preview.placements.targetSource')}</th></tr></thead>
      <tbody>{placements.map((placement) => <tr key={placement.node}>
        <td>{placement.node}</td>
        <td>{placement.cluster}</td>
        <td>{placement.clusterDefinition} (v{placement.clusterDefinitionVersion})</td>
        <td>{providerLabel(placement.provider)}</td>
        <td>{placement.account} (v{placement.accountVersion})</td>
        <td>{placement.region || '—'}</td>
        <td>{placement.namespace || '—'}</td>
        <td>{label(placement.source)}</td>
      </tr>)}</tbody>
    </table>
    <p><small>{t('preview.placements.theTargetIsDecidedBy')}</small></p>
  </div>;
}
