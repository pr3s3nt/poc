import type { ResourceIntent } from '../workloads/api';

const ownershipLabels: Record<string, string> = { PLATFORM: 'Nền tảng (liên kết hệ thống)', ORCHESTRATOR: 'Orchestrator (theo phạm vi tài nguyên)', PREEXISTING: 'Có sẵn (không bao giờ bị tạo hoặc xóa)' };
const intentLabels: Record<string, string> = {
  CREATE: 'Tạo mới', UPDATE: 'Cập nhật tài nguyên đã có (tham số, cấu hình hoặc module thay đổi)', RETAIN: 'Giữ nguyên (đã xác minh không đổi so với lần triển khai trước)',
  UNVERIFIED: 'Đã có, chưa xác minh được thay đổi; có thể được cập nhật', BIND: 'Liên kết hệ thống, không tạo',
};
const label = (table: Record<string, string>, key: string) => (Object.hasOwn(table, key) ? table[key] : key);

// Where one provider of a module runs: the Definition's own Driver Account and
// region (AWS), the cluster another previewed node provides or the legacy
// Environment cluster (Kubernetes). Names only: never a credential, endpoint or
// account override.
function providerText(provider: NonNullable<ResourceIntent['providers']>[number]): string {
  if (provider.target === 'resource') return `${provider.name}: cluster do node ${provider.provider ?? 'EKS'} cung cấp${provider.connectionKey ? ` · tài khoản ${provider.connectionKey}` : ''}${provider.region ? ` · vùng ${provider.region}` : ''}`;
  if (provider.kind === 'aws') return `${provider.name}: tài khoản AWS của cấu hình${provider.connectionKey ? ` ${provider.connectionKey}` : ''}${provider.region ? ` · vùng ${provider.region}` : ''}`;
  return `${provider.name}: cluster của Môi trường (cách cũ, giữ tới V03)${provider.connectionKey ? ` ${provider.connectionKey}` : ''}`;
}

export const rendererText = (rendering: { definitionKey: string; class?: string; driverType: string; bundle: { version: string } } | undefined) =>
  rendering?.driverType ? `${rendering.driverType} ${rendering.bundle.version} (mẫu ${rendering.definitionKey}${rendering.class ? `, class ${rendering.class}` : ''})` : 'Chưa xác định trình dựng score-k8s (không có renderer mặc định)';

// The safe resource projection of a Preview (UC-05 BR-10). It states intent only
// and does not claim a final Terraform plan or manifest.
export function ResourceIntents({ intents, label: ariaLabel = 'Tài nguyên hạ tầng sẽ được xử lý' }: { intents: ResourceIntent[] | undefined; label?: string }) {
  // A missing projection is unavailable, never proof that nothing needs creating or binding.
  if (!intents) return <p className="form-warning" role="note">Chưa có thông tin tài nguyên hạ tầng trong kết quả xem trước này nên chưa thể kết luận có tài nguyên nào cần tạo, cập nhật hay liên kết. Hãy xem trước lại.</p>;
  if (intents.length === 0) return <p className="feature-note">Không có tài nguyên hạ tầng nào cần tạo, cập nhật hoặc liên kết.</p>;
  return <div className="resource-intents" aria-label={ariaLabel}>
    <table className="plain-table">
      <thead><tr><th>Tài nguyên</th><th>Loại / class</th><th>Cấu hình</th><th>Module · revision</th><th>Đích provider</th><th>Quyền sở hữu</th><th>Ý định</th></tr></thead>
      <tbody>{intents.map((intent) => <tr key={`${intent.descriptor}|${intent.scope}`}>
        <td>{intent.descriptor}<br /><small>{intent.scope}</small></td>
        <td>{intent.resourceType} / {intent.class || 'default'}</td>
        <td>{intent.definitionKey}<br /><small>{intent.driverType}{intent.driverAccount ? ` · tài khoản ${intent.driverAccount}${intent.accountVersion ? ` v${intent.accountVersion}` : ''}` : ''}</small></td>
        <td>{intent.module ? <>{intent.module.module} · {intent.module.revision}<br /><small title={intent.module.contentSha256}>sha256:{intent.module.contentSha256.slice(0, 12)}</small></> : '—'}</td>
        <td>{intent.providers?.length ? <ul>{intent.providers.map((provider) => <li key={provider.name}>{providerText(provider)}</li>)}</ul> : '—'}</td>
        <td>{label(ownershipLabels, intent.ownership)}</td>
        <td>{label(intentLabels, intent.intent)}</td>
      </tr>)}</tbody>
    </table>
    <p><small>Chỉ là ý định đã ghim tại thời điểm xem trước: không phải kế hoạch Terraform hay manifest cuối cùng, và không hiển thị tham số, bí mật, thông tin xác thực hay trạng thái. Thay đổi module, đích hoặc mẫu dựng sẽ làm kết quả này cũ.</small></p>
  </div>;
}
