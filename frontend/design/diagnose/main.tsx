import { createRoot } from 'react-dom/client';
import { AppShell } from '../../src/app/AppShell';
import { AccountDetail } from '../../src/features/platform/accounts/AccountDetail';
import { Standalone, ReferenceApplications } from './Standalone';
import { AccountsMock } from './AccountsMock';

function ReferenceAccount() {
  return <AppShell username="platform.demo" role="PLATFORM_ENGINEER" activeRoute="driver-accounts" onSignOut={() => undefined}>
    <section className="page" inert><header className="page-header"><div><p className="eyebrow">Nền tảng</p><h1>Tài khoản hạ tầng</h1></div></header>
      <AccountDetail account={{ key: 'k8s-staging', name: 'K8S · staging-cluster', type: 'INTERNAL', authenticationType: 'KUBECONFIG', status: 'READY', version: 3, config: { cluster: 'staging-cluster', kubeContext: 'platform-staging', endpoint: 'https://kubernetes.staging.example:6443', tls: { mode: 'CUSTOM_CA' } }, verification: { verified: true, verifiedAt: '2026-10-24T09:10:00Z' } }} onChanged={() => undefined} onRefreshed={() => undefined} />
    </section>
  </AppShell>;
}
const query = new URLSearchParams(window.location.search);
createRoot(document.getElementById('root')!).render(query.get('reference') === 'accounts' ? <ReferenceAccount /> : query.has('reference') ? <ReferenceApplications /> : query.has('standalone') ? <Standalone /> : <AccountsMock />);
