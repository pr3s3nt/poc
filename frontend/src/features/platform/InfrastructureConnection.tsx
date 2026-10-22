import { Button } from '../../shared/ui/Button';
import type { Capability } from './capabilities';
import { bindingProviders, type BindingDraft, type MappingDraft, type MappingProblem } from './moduleMapping';
import { statusLabel, typeLabel, type Account } from './accounts/accountsApi';

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
const environmentScope = 'environments.@app.@env';

export function InfrastructureConnection({ capability, accountRequired, accounts, account, region, regionRequired, onAccount, onRegion, mapping, mappingEditable, clusterClasses, onBinding, problems, disabled = false }: Props) {
  const providers = bindingProviders(capability);
  const awsProviders = providers.filter((provider) => provider.binding === 'aws');
  const kubernetesProviders = providers.filter((provider) => provider.binding === 'kubernetes');
  const compatible = accounts.items.filter((item) => item.type === capability.account.type && item.status === 'READY');
  const selectedMissing = accountRequired && account !== '' && !compatible.some((item) => item.key === account);
  const accountName = (key: string) => { const found = accounts.items.find((item) => item.key === key); return found ? accountLabel(found) : key; };
  const typeName = typeLabel(capability.account.type ?? '');
  const providerNote = (list: typeof providers) => list.map((provider) => `${provider.source} ${provider.version}`.trim()).join(' · ');
  if (!accountRequired && kubernetesProviders.length === 0) return <p className="feature-note" role="note">Module này không cần kết nối hạ tầng: không chọn tài khoản và không có provider cần binding.</p>;

  return <div className="connection-section" aria-label="Kết nối hạ tầng">
    {accountRequired ? <section className="connection-card" aria-label="Kết nối tài khoản" data-connection="account">
      <div className="connection-head"><span className="connection-icon" aria-hidden="true">A</span><div><h4>Kết nối {typeName}</h4>{awsProviders.length > 0 ? <p className="muted">Provider {providerNote(awsProviders)}</p> : null}</div></div>
      <div className="field-grid">
        <label>Tài khoản {typeName}<select aria-label="Tài khoản hạ tầng" aria-required="true" value={account} disabled={disabled || accounts.loading || accounts.error !== ''} onChange={(event) => onAccount(event.target.value)}>
          <option value="">{accounts.loading ? 'Đang tải tài khoản…' : accounts.error ? 'Chưa tải được tài khoản' : compatible.length === 0 ? 'Chưa có tài khoản sẵn sàng' : 'Chọn tài khoản'}</option>
          {selectedMissing ? <option value={account}>{accountName(account)} (không còn tương thích hoặc chưa sẵn sàng)</option> : null}
          {compatible.map((item) => <option key={item.key} value={item.key}>{accountLabel(item)} · {statusLabel(item.status)} · phiên bản {item.version}</option>)}</select></label>
        {regionRequired ? <label>Vùng AWS<input aria-label="Vùng AWS" aria-required="true" value={region} disabled={disabled} spellCheck={false} placeholder="ap-southeast-1" aria-describedby="definition-region-hint" onChange={(event) => onRegion(event.target.value)} /></label> : null}</div>
      {regionRequired ? <p id="definition-region-hint" className="feature-note">Vùng là chuỗi literal (không dùng placeholder), nhập một lần và lưu trong Driver Inputs. Vùng xác minh của tài khoản chỉ là bằng chứng kiểm tra, không phải danh sách vùng được phép.</p> : null}
      <details className="definition-help"><summary>Về tài khoản và vùng</summary><p className="feature-note">Module này chạy bằng tài khoản {typeName} và vùng ghi ngay trong cấu hình. Cấu hình không lấy tài khoản hay vùng từ Môi trường; mỗi cấu hình có thể dùng tài khoản và vùng khác nhau.</p></details>
      {accounts.error ? <div className="form-error" role="alert" aria-label="Lỗi tải tài khoản">{accounts.error} Chưa chọn được tài khoản; dữ liệu đã nhập được giữ nguyên. <Button type="button" onClick={accounts.reload}>Thử lại</Button></div> : !accounts.loading && compatible.length === 0 ? <div className="form-warning" role="note" aria-label="Chưa có tài khoản sẵn sàng">Chưa có tài khoản {typeName} ở trạng thái Sẵn sàng trong Tổ chức. Tạo và xác minh tài khoản ở trang Tài khoản hạ tầng trước.</div> : null}
      {selectedMissing && !accounts.loading ? <div className="form-warning" role="alert">Tài khoản đã chọn ({account}) không còn tương thích hoặc chưa Sẵn sàng. Chọn tài khoản khác; máy chủ cũng kiểm tra lại khi lưu và khi lập kế hoạch.</div> : null}
    </section> : null}
    {kubernetesProviders.map((provider) => {
      const binding = mapping.bindings[provider.name] ?? { target: '', clusterClass: '' };
      const problem = problems.find((item) => item.field === `binding:${provider.name}`)?.message;
      const supported = capability.bindings.find((item) => item.provider === provider.name)?.targets.some((item) => item.target === 'resource') ?? false;
      return <section key={provider.name} className="connection-card" aria-label={`Kết nối Kubernetes ${provider.name}`} data-provider={provider.name}>
        <div className="connection-head"><span className="connection-icon connection-icon-k8s" aria-hidden="true">⎈</span><div><h4>Kết nối Kubernetes</h4><p className="muted">Provider <code>{provider.name}</code> · {provider.source} {provider.version}</p></div></div>
        {supported ? <>
          <div className="field-grid">
            <label>Nhóm cluster<select aria-label={`Nhóm cluster của provider ${provider.name}`} aria-invalid={problem ? true : undefined} value={binding.clusterClass} disabled={disabled || !mappingEditable} onChange={(event) => onBinding(provider.name, event.target.value === '' ? { target: '', clusterClass: '' } : { target: 'resource', clusterClass: event.target.value })}>
              <option value="">Chưa chọn</option>
              {binding.clusterClass && !clusterClasses.includes(binding.clusterClass) ? <option value={binding.clusterClass}>{binding.clusterClass} (không còn trong metadata)</option> : null}
              {clusterClasses.map((item) => <option key={item} value={item}>{item}</option>)}</select></label>
            <label>Phạm vi cluster<select aria-label={`Phạm vi cluster của provider ${provider.name}`} value={environmentScope} disabled={disabled || !mappingEditable} onChange={() => undefined}>
              <option value={environmentScope}>Cùng ứng dụng và môi trường</option></select></label></div>
          <div className="connection-note"><strong>✓ Dùng thông tin truy cập từ cluster</strong><span>Không nhập tài khoản, token hoặc kubeconfig ở đây. Cluster cụ thể được xác định khi Preview.</span></div>
          {clusterClasses.length === 0 ? <p className="feature-note" role="note">Danh mục chưa công bố class nào cho Loại k8s-cluster; chưa chọn được cluster.</p> : null}
          {!mappingEditable ? <p className="feature-note" role="note">Ánh xạ module đang chỉnh bằng JSON nâng cao; chuyển về Biểu mẫu để chọn nguồn cluster ở đây.</p> : null}
          {problem ? <span className="form-error" role="note">{problem}</span> : null}
        </> : <p className="feature-note" role="note">Metadata chưa công bố nguồn cluster được hỗ trợ cho provider này.</p>}
      </section>;
    })}
  </div>;
}
