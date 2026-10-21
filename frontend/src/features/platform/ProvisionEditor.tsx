import { Button } from '../../shared/ui/Button';
import { newProvisionRow, type ProvisionRow } from './typedProvision';

type Props = { rows: ProvisionRow[]; onChange: (rows: ProvisionRow[]) => void; disabled?: boolean };

// Provision editor (UC-03 BR-26): descriptor, params and the two flags of
// UC-06 BR-08. Editing never previews or provisions; reference edges stay
// consumer -> provider as the planner builds them.
export function ProvisionEditor({ rows, onChange, disabled = false }: Props) {
  const patch = (id: number, change: Partial<ProvisionRow>) => onChange(rows.map((row) => (row.id === id ? { ...row, ...change } : row)));
  return <section className="catalog-fields" aria-label="Tài nguyên đi kèm">
    <div className="section-header"><div><h4>Tài nguyên đi kèm</h4>
      <p className="feature-note">Mỗi quy tắc mô tả một tài nguyên được tạo cùng tài nguyên chứa Cấu hình này (consumer là bên dùng, provider là bên được dùng). Reference luôn đi theo chiều consumer → provider; form không tự Xem trước hay triển khai.</p></div>
      <Button type="button" disabled={disabled} onClick={() => onChange([...rows, newProvisionRow()])}>+ Thêm tài nguyên đi kèm</Button></div>
    {rows.length === 0 ? <p className="muted">Chưa có tài nguyên đi kèm.</p> : null}
    {rows.map((row, index) => <div key={row.id} className="provision-row">
      <label>Descriptor {index + 1}<input aria-label={`Descriptor tài nguyên đi kèm ${index + 1}`} value={row.descriptor} spellCheck={false} disabled={disabled} placeholder="loại[.class][#id], ví dụ vpc.default#@infra" onChange={(event) => patch(row.id, { descriptor: event.target.value })} /></label>
      <label>Params {index + 1} (JSON object)<textarea aria-label={`Params tài nguyên đi kèm ${index + 1} (JSON object)`} rows={3} value={row.params} spellCheck={false} disabled={disabled} onChange={(event) => patch(row.id, { params: event.target.value })} /></label>
      <label className="catalog-check"><input type="checkbox" aria-label={`Tài nguyên đi kèm ${index + 1} phụ thuộc vào tài nguyên cha (is_dependent)`} checked={row.isDependent} disabled={disabled} onChange={(event) => patch(row.id, { isDependent: event.target.checked })} /> Phụ thuộc vào tài nguyên cha</label>
      <details><summary>Chi tiết is_dependent</summary><p className="feature-note">Đảo vai trò: tài nguyên đi kèm (consumer) phụ thuộc vào tài nguyên cha (provider), nên tài nguyên cha được lập kế hoạch trước. Không có cờ này thì hai tài nguyên không có cạnh phụ thuộc riêng.</p></details>
      <label className="catalog-check"><input type="checkbox" aria-label={`Nối consumer của tài nguyên cha tới tài nguyên đi kèm ${index + 1} (match_dependents)`} checked={row.matchDependents} disabled={disabled} onChange={(event) => patch(row.id, { matchDependents: event.target.checked })} /> Nối các consumer của tài nguyên cha</label>
      <details><summary>Chi tiết match_dependents</summary><p className="feature-note">Nối mọi consumer đang dùng tài nguyên cha tới tài nguyên đi kèm (provider): mỗi consumer cũng phụ thuộc vào tài nguyên đi kèm.</p></details>
      <Button type="button" tone="quiet" aria-label={`Xóa tài nguyên đi kèm ${index + 1}`} disabled={disabled} onClick={() => onChange(rows.filter((item) => item.id !== row.id))}>Xóa</Button>
    </div>)}
  </section>;
}
