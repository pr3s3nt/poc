import { useEffect, useRef, useState } from 'react';
import { replaceRoute } from '../../../app/routes';
import { Button } from '../../../shared/ui/Button';
import { useCatalogList } from '../useCatalogList';
import { AccountDetail } from './AccountDetail';
import { CreateAccountForm } from './CreateAccountForm';
import { accountGuidance, authLabel, getAccount, listAccounts, statusLabel, typeLabel, type Account } from './accountsApi';
import { t } from '../../../shared/i18n';

const listFailure = () => t('platform.accounts.driverAccounts.couldNotLoadTheInfrastructure');

function destination(account: Account): string {
  const config = account.config ?? {};
  return account.type === 'AWS'
    ? [config.accountId, (config.verificationRegions ?? []).length ? t('platform.accounts.driverAccounts.checkRegionsValue', { value: config.verificationRegions?.join(', ') }) : ''].filter(Boolean).join(' · ')
    : [config.cluster, config.endpoint].filter(Boolean).join(' · ');
}

/**
 * V01 infrastructure accounts: AWS and Internal, version-guarded management
 * over the same backing records as the legacy Connections page. No Environment
 * assignment and no applicability controls exist here.
 */
export function DriverAccountsPage({ initialAccount }: { initialAccount?: string }) {
  const { items, loading, loadError, reload } = useCatalogList(listAccounts, listFailure);
  const [selectedKey, setSelectedKey] = useState(initialAccount ?? '');
  const [detail, setDetail] = useState<Account | null>(null);
  const [detailState, setDetailState] = useState<'idle' | 'loading' | 'error'>('idle');
  const [detailError, setDetailError] = useState('');
  const [listNotice, setListNotice] = useState('');
  const request = useRef(0);

  async function loadDetail(key: string) {
    const id = ++request.current;
    setDetail(null); setDetailState('loading'); setDetailError('');
    try {
      const account = await getAccount(key);
      if (id !== request.current) return;
      setDetail(account); setDetailState('idle');
    } catch (reason) {
      if (id !== request.current) return;
      setDetailState('error'); setDetailError(accountGuidance('rename', reason));
    }
  }

  useEffect(() => {
    if (!selectedKey) { request.current += 1; setDetail(null); setDetailState('idle'); return; }
    void loadDetail(selectedKey);
  }, [selectedKey]);
  useEffect(() => () => { request.current += 1; }, []);

  function select(key: string) {
    setSelectedKey(key);
    replaceRoute({ name: 'driver-accounts', ...(key ? { account: key } : {}) });
  }

  // A committed change updates the detail at once; the list refresh is a
  // separate request whose failure never contradicts the committed result.
  function changed(account: Account) {
    setDetail(account);
    setListNotice('');
    void reload();
  }

  async function created(account: Account) {
    setListNotice('');
    await reload();
    select(account.key);
  }

  const visible = items;
  return <section className="page"><header className="page-header"><div><p className="eyebrow">{t('platform.accounts.driverAccounts.platform')}</p><h1>{t('platform.accounts.driverAccounts.infrastructureAccounts')}</h1><p>{t('platform.accounts.driverAccounts.catalogOfTheOrganizationsAWS')}</p></div></header>
    <section className="content-panel"><div className="section-header"><h2>{t('platform.accounts.driverAccounts.registeredAccounts')}</h2><Button type="button" onClick={() => { setListNotice(''); void reload(); }} disabled={loading}>{t('platform.accounts.driverAccounts.reloadList')}</Button></div>
      {loadError ? <div className="form-error" role="alert" aria-label={t('platform.accounts.driverAccounts.listLoadError')}>{loadError} <Button type="button" onClick={() => void reload()}>{t('platform.accounts.driverAccounts.retry')}</Button></div> : null}
      {listNotice ? <div className="form-success" role="status">{listNotice}</div> : null}
      {loading && visible.length === 0 ? <p>{t('platform.accounts.driverAccounts.loadingAccounts')}</p> : !loadError && visible.length === 0 ? <p>{t('platform.accounts.driverAccounts.noInfrastructureAccountsRegisteredYet')}</p> : null}
      {visible.length > 0 ? <table className="connection-table" aria-label={t('platform.accounts.driverAccounts.infrastructureAccounts')}>
        <thead><tr><th scope="col">{t('platform.accounts.driverAccounts.name')}</th><th scope="col">{t('platform.accounts.driverAccounts.infrastructureType')}</th><th scope="col">{t('platform.accounts.driverAccounts.authentication')}</th><th scope="col">{t('platform.accounts.driverAccounts.target')}</th><th scope="col">{t('platform.accounts.driverAccounts.status')}</th><th scope="col">{t('platform.accounts.driverAccounts.version')}</th></tr></thead>
        <tbody>{visible.map((account) => <tr key={account.key} className={account.key === selectedKey ? 'row-selected' : undefined}>
          <td data-label={t('platform.accounts.driverAccounts.name')}><button type="button" className="link-button" aria-label={t('platform.accounts.driverAccounts.viewName', { name: account.name })} aria-pressed={account.key === selectedKey} onClick={() => select(account.key)}><strong>{account.name}</strong></button>{account.name !== account.key ? <small className="connection-key">{account.key}</small> : null}</td>
          <td data-label={t('platform.accounts.driverAccounts.infrastructureType')}>{typeLabel(account.type)}</td>
          <td data-label={t('platform.accounts.driverAccounts.authentication')}>{authLabel(account.authenticationType)}</td>
          <td data-label={t('platform.accounts.driverAccounts.target')}>{destination(account) || <span className="muted">—</span>}</td>
          <td data-label={t('platform.accounts.driverAccounts.status')}><span className={`status-badge${account.status === 'READY' ? ' status-badge-ready' : ''}`}>{statusLabel(account.status)}</span></td>
          <td data-label={t('platform.accounts.driverAccounts.version')}>{account.version}</td>
        </tr>)}</tbody>
      </table> : null}
    </section>
    {selectedKey ? detailState === 'loading' ? <section className="content-panel" role="status" aria-busy="true">{t('platform.accounts.driverAccounts.loadingAccountDetails')}</section>
      : detailState === 'error' ? <section className="content-panel"><div className="form-error" role="alert">{detailError} <Button type="button" onClick={() => void loadDetail(selectedKey)}>{t('platform.accounts.driverAccounts.retry')}</Button> <Button type="button" onClick={() => select('')}>{t('platform.accounts.driverAccounts.close')}</Button></div></section>
      : detail ? <AccountDetail key={detail.key} account={detail} onChanged={changed} onRefreshed={(account) => { setDetail(account); void reload(); }} /> : null : null}
    <section className="content-panel"><div className="connection-register">
      <div className="section-header"><div><h2>{t('platform.accounts.driverAccounts.registerNewAccount')}</h2><p>{t('platform.accounts.driverAccounts.chooseTheInfrastructureTypeAnd')}</p></div></div>
      <CreateAccountForm onCreated={created} onUnknownOutcome={() => { setListNotice(t('platform.accounts.driverAccounts.itIsUnknownWhetherThe')); return reload(); }} />
    </div></section>
  </section>;
}
