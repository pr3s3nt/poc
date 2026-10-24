import type { ResourceIntent } from '../workloads/api';
import { t } from '../../shared/i18n';

const ownershipLabels: Record<string, string> = { PLATFORM: t('preview.resourceIntents.platformSystemLink'), ORCHESTRATOR: t('preview.resourceIntents.orchestratorByResourceScope'), PREEXISTING: t('preview.resourceIntents.existingNeverCreatedOrDeleted') };
const intentLabels: Record<string, string> = {
  CREATE: t('preview.resourceIntents.create'), UPDATE: t('preview.resourceIntents.updateAnExistingResourceParameters'), RETAIN: t('preview.resourceIntents.unchangedVerifiedIdenticalToThe'),
  UNVERIFIED: t('preview.resourceIntents.existingChangesNotVerifiedMay'), BIND: t('preview.resourceIntents.systemLinkNotCreated'),
};
const label = (table: Record<string, string>, key: string) => (Object.hasOwn(table, key) ? table[key] : key);

// Where one provider of a module runs: the Definition's own Driver Account and
// region (AWS), or the cluster another previewed node provides (Kubernetes).
// Names only: never a credential, endpoint or account override.
function providerText(provider: NonNullable<ResourceIntent['providers']>[number]): string {
  if (provider.target === 'resource') return t('preview.resourceIntents.nameClusterProvidedByNode', { name: provider.name, value: provider.provider ?? 'EKS', value2: provider.connectionKey ? t('preview.resourceIntents.accountConnectionKey', { connectionKey: provider.connectionKey }) : '', value3: provider.region ? t('preview.resourceIntents.regionRegion', { region: provider.region }) : '' });
  if (provider.kind === 'aws') return t('preview.resourceIntents.nameAWSAccountOfThe', { name: provider.name, value: provider.connectionKey ? ` ${provider.connectionKey}` : '', value2: provider.region ? t('preview.resourceIntents.regionRegion', { region: provider.region }) : '' });
  return t('preview.resourceIntents.nameUnsupportedProviderTarget', { name: provider.name });
}

export const rendererText = (rendering: { definitionKey: string; class?: string; driverType: string; bundle: { version: string } } | undefined) =>
  rendering?.driverType ? t('preview.resourceIntents.driverTypeVersionTemplateDefinitionKeyValue', { driverType: rendering.driverType, version: rendering.bundle.version, definitionKey: rendering.definitionKey, value: rendering.class ? `, class ${rendering.class}` : '' }) : t('preview.resourceIntents.noScoreK8sRendererDetermined');

// The safe resource projection of a Preview (UC-05 BR-10). It states intent only
// and does not claim a final Terraform plan or manifest.
export function ResourceIntents({ intents, label: ariaLabel = t('preview.resourceIntents.infrastructureResourcesToBeProcessed') }: { intents: ResourceIntent[] | undefined; label?: string }) {
  // A missing projection is unavailable, never proof that nothing needs creating or binding.
  if (!intents) return <p className="form-warning" role="note">{t('preview.resourceIntents.thisPreviewResultHasNo')}</p>;
  if (intents.length === 0) return <p className="feature-note">{t('preview.resourceIntents.noInfrastructureResourcesNeedTo')}</p>;
  return <div className="resource-intents" aria-label={ariaLabel}>
    <table className="plain-table">
      <thead><tr><th>{t('preview.resourceIntents.resource')}</th><th>{t('preview.resourceIntents.typeClass')}</th><th>{t('preview.resourceIntents.definition')}</th><th>Module · revision</th><th>{t('preview.resourceIntents.providerTarget')}</th><th>{t('preview.resourceIntents.ownership')}</th><th>{t('preview.resourceIntents.intent')}</th></tr></thead>
      <tbody>{intents.map((intent) => <tr key={`${intent.descriptor}|${intent.scope}`}>
        <td>{intent.descriptor}<br /><small>{intent.scope}</small></td>
        <td>{intent.resourceType} / {intent.class || 'default'}</td>
        <td>{intent.definitionKey}<br /><small>{intent.driverType}{intent.driverAccount ? t('preview.resourceIntents.accountDriverAccountValue', { driverAccount: intent.driverAccount, value: intent.accountVersion ? ` v${intent.accountVersion}` : '' }) : ''}</small></td>
        <td>{intent.module ? <>{intent.module.module} · {intent.module.revision}<br /><small title={intent.module.contentSha256}>sha256:{intent.module.contentSha256.slice(0, 12)}</small></> : '—'}</td>
        <td>{intent.providers?.length ? <ul>{intent.providers.map((provider) => <li key={provider.name}>{providerText(provider)}</li>)}</ul> : '—'}</td>
        <td>{label(ownershipLabels, intent.ownership)}</td>
        <td>{label(intentLabels, intent.intent)}</td>
      </tr>)}</tbody>
    </table>
    <p><small>{t('preview.resourceIntents.onlyTheIntentPinnedAt')}</small></p>
  </div>;
}
