import { Button } from '../../shared/ui/Button';
import type { ResourceType } from './api';

// Display names of the classes the catalog serves (UC-16 screens, "Class và metadata
// dependency"). Any other class is a custom/imported one and is shown as written.
const classLabels: Record<string, string> = { 'in-cluster': 'Trong cluster (in-cluster)', 'cloud-managed': 'Dịch vụ managed AWS (cloud-managed)' };
export const classLabel = (name: string) => (Object.hasOwn(classLabels, name) ? classLabels[name] ?? name : `Tùy chỉnh: ${name}`);

type Props = {
  index: number;
  alias: string;
  type: ResourceType | undefined;
  className: string;
  sharedId: string;
  metadata: 'ready' | 'loading' | 'error';
  disabled?: boolean;
  onClass(value: string): void;
  onSharedId(value: string): void;
  onReload(): void;
};

// "Cách cung cấp / Class" and the shared ID of one dependency. The choices are the
// classes GET /resource-types publishes for the Type; the Developer never picks a
// Terraform module, provider or credential. `default` (empty) keeps its Score
// meaning: it is never mapped to a class, and an imported/custom class is kept.
export function ClassControls({ index, alias, type, className, sharedId, metadata, disabled = false, onClass, onSharedId, onReload }: Props) {
  const name = alias || type?.key || `tài nguyên ${index + 1}`;
  const classes = type?.classes ?? [];
  const known = className === '' || classes.includes(className);
  const locked = disabled || metadata !== 'ready';
  return <div className="class-controls" data-dependency={name}>
    <div className="field-grid">
      <label>Cách cung cấp / Class
        <select aria-label={`Cách cung cấp (class) của ${name}`} value={className} disabled={locked || !type} onChange={(event) => onClass(event.target.value)}>
          <option value="">Mặc định (default)</option>
          {classes.map((item) => <option key={item} value={item}>{classLabel(item)}</option>)}
          {!known ? <option value={className}>{className} (đã nhập, giữ nguyên)</option> : null}
        </select></label>
      <label>ID dùng chung (tùy chọn)
        <input aria-label={`ID dùng chung của ${name}`} value={sharedId} disabled={disabled} spellCheck={false} placeholder="để trống nếu riêng của Workload" onChange={(event) => onSharedId(event.target.value)} /></label>
    </div>
    {index === 0 && metadata === 'loading' ? <p role="status" className="feature-note">Đang tải metadata class…</p> : null}
    {index === 0 && metadata === 'error' ? <div className="form-error" role="alert" aria-label="Lỗi tải metadata class">Không tải được metadata class từ máy chủ nên các lựa chọn class đang bị khóa. Giá trị bạn đã nhập được giữ nguyên và chưa lưu được cho đến khi tải lại thành công. <Button type="button" onClick={onReload}>Thử lại tải metadata class</Button></div> : null}
    {type && classes.length === 0 && metadata === 'ready' ? <p className="feature-note">Danh mục chưa công bố class nào cho loại {type.key}; class mặc định giữ nguyên ý nghĩa default. Hệ thống không tự đổi class.</p> : null}
    {!known ? <p className="feature-note" role="note">Class "{className}" không nằm trong metadata của loại {type?.key ?? 'này'}. Giá trị được giữ nguyên; việc khớp cấu hình được kiểm tra khi Xem trước.</p> : null}
    <p className="feature-note">Chỉ chọn cách cung cấp tài nguyên; Developer không chọn module Terraform, provider hay thông tin xác thực. Các Workload dùng cùng ID dùng chung phải thống nhất loại, class và tham số.</p>
  </div>;
}
