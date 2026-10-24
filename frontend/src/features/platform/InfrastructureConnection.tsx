import { Button } from '../../shared/ui/Button';
import type { Capability } from './capabilities';
import { bindingProviders, type BindingDraft, type MappingDraft, type MappingProblem } from './moduleMapping';
import { statusLabel, typeLabel, type Account } from './accounts/accountsApi';
import { t } from '../../shared/i18n';

// “Kết nối hạ tầng” of section 2 (UC-03 UI11-01). The cards come from the provider requirements and binding metadata of
// the verified module; the account and region live in the pair draft and the cluster source in the mapping draft that the
// advanced editor and the JSON mode serialize, so there is only one state per value.

type Props = {
  capability: Capability;
  accountRequired: boolean;
  accounts: { items: Account[]; loading: boolean; error: string; reload: () => void };
  account: string;
  region: string;
  regionRequired: boolean;
  onAccount: (account: string) => void;
  onRegion: (region: string) => void;
  mapping: MappingDraft;
  mappingEditable: boolean;
  clusterClasses: string[];
  onBinding: (provider: string, binding: BindingDraft) => void;
  problems: MappingProblem[];
  disabled?: boolean;
};

const accountLabel = (account: Account) => (account.name && account.name !== account.key ? `${account.name} (${account.key})` : account.key);

export function InfrastructureConnection({ capability, accountRequired, accounts, account, region, regionRequired, onAccount, onRegion, mapping, mappingEditable, clusterClasses, onBinding, problems, disabled = false }: Props) {
  const providers = bindingProviders(capability);
  const awsProviders = providers.filter((provider) => provider.binding === 'aws');
  const kubernetesProviders = providers.filter((provider) => provider.binding === 'kubernetes');
  const compatible = accounts.items.filter((item) => item.type === capability.account.type && item.status === 'READY');
  const selectedMissing = accountRequired && account !== '' && !compatible.some((item) => item.key === account);
  const accountName = (key: string) => { const found = accounts.items.find((item) => item.key === key); return found ? accountLabel(found) : key; };
  const typeName = typeLabel(capability.account.type ?? '');
  const providerNote = (list: typeof providers) => list.map((provider) => `${provider.source} ${provider.version}`.trim()).join(' · ');
  if (!accountRequired && kubernetesProviders.length === 0) return <p className="feature-note" role="note">{t('platform.infrastructureConnection.thisModuleNeedsNoInfrastructure')}</p>;

  return <div className="connection-section" aria-label={t('platform.infrastructureConnection.infrastructureConnection')}>
    {accountRequired ? <section className="connection-card" aria-label={t('platform.infrastructureConnection.accountConnection')} data-connection="account">
      <div className="connection-head"><span className="connection-icon" aria-hidden="true">A</span><div><h4>{t('platform.infrastructureConnection.typeNameConnection', { typeName })}</h4>{awsProviders.length > 0 ? <p className="muted">Provider {providerNote(awsProviders)}</p> : null}</div></div>
      <div className="field-grid">
        <label>{t('platform.infrastructureConnection.account')}{' '} {typeName}<select aria-label={t('platform.infrastructureConnection.infrastructureAccount')} aria-required="true" value={account} disabled={disabled || accounts.loading || accounts.error !== ''} onChange={(event) => onAccount(event.target.value)}>
          <option value="">{accounts.loading ? t('platform.infrastructureConnection.loadingAccounts') : accounts.error ? t('platform.infrastructureConnection.couldNotLoadAccounts') : compatible.length === 0 ? t('platform.infrastructureConnection.noReadyAccounts') : t('platform.infrastructureConnection.chooseAnAccount')}</option>
          {selectedMissing ? <option value={account}>{t('platform.infrastructureConnection.accountNameNoLongerCompatibleOr', { accountName: accountName(account) })}</option> : null}
          {compatible.map((item) => <option key={item.key} value={item.key}>{t('platform.infrastructureConnection.accountLabelStatusLabelVersionVersion', { accountLabel: accountLabel(item), statusLabel: statusLabel(item.status), version: item.version })}</option>)}</select></label>
        {regionRequired ? <label>{t('platform.infrastructureConnection.aWSRegion')}<input aria-label={t('platform.infrastructureConnection.aWSRegion')} aria-required="true" value={region} disabled={disabled} spellCheck={false} placeholder="ap-southeast-1" aria-describedby="definition-region-hint" onChange={(event) => onRegion(event.target.value)} /></label> : null}</div>
      {regionRequired ? <p id="definition-region-hint" className="feature-note">{t('platform.infrastructureConnection.theRegionIsALiteral')}</p> : null}
      <details className="definition-help"><summary>{t('platform.infrastructureConnection.aboutAccountsAndRegions')}</summary><p className="feature-note">{t('platform.infrastructureConnection.thisModuleRunsWithThe', { typeName })}</p></details>
      {accounts.error ? <div className="form-error" role="alert" aria-label={t('platform.infrastructureConnection.accountLoadError')}>{accounts.error} {' '}{t('platform.infrastructureConnection.noAccountSelectedYetThe')}{' '} <Button type="button" onClick={accounts.reload}>{t('platform.infrastructureConnection.retry')}</Button></div> : !accounts.loading && compatible.length === 0 ? <div className="form-warning" role="note" aria-label={t('platform.infrastructureConnection.noReadyAccounts')}>{t('platform.infrastructureConnection.thereIsNoReadyTypeName', { typeName })}</div> : null}
      {selectedMissing && !accounts.loading ? <div className="form-warning" role="alert">{t('platform.infrastructureConnection.theSelectedAccountAccountIs', { account })}</div> : null}
    </section> : null}
    {kubernetesProviders.map((provider) => {
      const binding = mapping.bindings[provider.name] ?? { target: '', clusterClass: '' };
      const problem = problems.find((item) => item.field === `binding:${provider.name}`)?.message;
      const supported = capability.bindings.find((item) => item.provider === provider.name)?.targets.some((item) => item.target === 'resource') ?? false;
      return <section key={provider.name} className="connection-card" aria-label={t('platform.infrastructureConnection.kubernetesConnectionName', { name: provider.name })} data-provider={provider.name}>
        <div className="connection-head"><span className="connection-icon connection-icon-k8s" aria-hidden="true">⎈</span><div><h4>{t('platform.infrastructureConnection.kubernetesConnection')}</h4><p className="muted">Provider <code>{provider.name}</code> · {provider.source} {provider.version}</p></div></div>
        {supported ? <>
          <div className="field-grid">
            <label>{t('platform.infrastructureConnection.clusterClass')}<select aria-label={t('platform.infrastructureConnection.clusterClassOfProviderName', { name: provider.name })} aria-invalid={problem ? true : undefined} value={binding.clusterClass} disabled={disabled || !mappingEditable} onChange={(event) => onBinding(provider.name, event.target.value === '' ? { target: '', clusterClass: '' } : { target: 'resource', clusterClass: event.target.value })}>
              <option value="">{t('platform.infrastructureConnection.notSelected')}</option>
              {binding.clusterClass && !clusterClasses.includes(binding.clusterClass) ? <option value={binding.clusterClass}>{t('platform.infrastructureConnection.clusterClassNoLongerInMetadata', { clusterClass: binding.clusterClass })}</option> : null}
              {clusterClasses.map((item) => <option key={item} value={item}>{item}</option>)}</select></label>
          </div>
          <div className="connection-note"><strong>{t('platform.infrastructureConnection.usesAccessInformationFromThe')}</strong><span>{t('platform.infrastructureConnection.doNotEnterAccountsTokens')}</span></div>
          {clusterClasses.length === 0 ? <p className="feature-note" role="note">{t('platform.infrastructureConnection.theCatalogHasNotPublished')}</p> : null}
          {!mappingEditable ? <p className="feature-note" role="note">{t('platform.infrastructureConnection.theModuleMappingIsBeing')}</p> : null}
          {problem ? <span className="form-error" role="note">{problem}</span> : null}
        </> : <p className="feature-note" role="note">{t('platform.infrastructureConnection.metadataHasNotPublishedA')}</p>}
      </section>;
    })}
  </div>;
}
