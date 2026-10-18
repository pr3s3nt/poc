import type { Placement } from '../workloads/api';

const sourceLabels: Record<string, string> = { 'workload-reference': 'tham chiếu của workload', 'provider-binding': 'provider binding của module' };
const label = (key: string) => (Object.hasOwn(sourceLabels, key) ? sourceLabels[key] : key);

// Per workload/resource target provenance (ADR-016): which cluster Definition and
// Internal account a node runs on and, for a workload, which namespace. Names and
// versions only; never a credential, kubeconfig, endpoint or secret reference.
export function Placements({ placements, label: ariaLabel = 'Đích triển khai của từng thành phần' }: { placements: Placement[] | undefined; label?: string }) {
  if (!placements || placements.length === 0) return null;
  return <div className="placements" aria-label={ariaLabel}>
    <table className="plain-table">
      <thead><tr><th>Thành phần</th><th>Cluster</th><th>Cấu hình cluster</th><th>Tài khoản Internal</th><th>Namespace</th><th>Nguồn đích</th></tr></thead>
      <tbody>{placements.map((placement) => <tr key={placement.node}>
        <td>{placement.node}</td>
        <td>{placement.cluster}</td>
        <td>{placement.clusterDefinition} (v{placement.clusterDefinitionVersion})</td>
        <td>{placement.account} (v{placement.accountVersion})</td>
        <td>{placement.namespace || '—'}</td>
        <td>{label(placement.source)}</td>
      </tr>)}</tbody>
    </table>
    <p><small>Đích do Resource Definition và tham chiếu quyết định, đã ghim tại thời điểm xem trước. Môi trường không chọn tài khoản; nhiều hơn một cluster trong một Môi trường chưa được hỗ trợ.</small></p>
  </div>;
}
