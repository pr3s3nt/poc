import type { EnvironmentKey } from '../../shared/types/application';

// Vietnamese Console labels (glossary VI-01). Enum values and technical IDs stay
// as the API sends them; only the label shown next to them is translated.
const statusLabels: Record<string, string> = {
  READY: 'Sẵn sàng', PENDING: 'Đang chờ', FAILED: 'Thất bại', SUCCEEDED: 'Thành công',
  UNCONFIGURED: 'Chưa cấu hình', RUNNING: 'Đang chạy', INTERRUPTED: 'Bị gián đoạn', ACTIVE: 'Đang hoạt động',
};

export const statusLabel = (status: string): string => statusLabels[status] ? `${statusLabels[status]} (${status})` : status;
export const environmentTitle = (environment: EnvironmentKey): string => environment === 'staging' ? 'Staging' : 'Production';

const profileLabels: Record<string, string> = { 'internal-k8s': 'Cluster nội bộ', 'aws-eks': 'AWS' };
export const profileLabel = (profile: string): string => profileLabels[profile] ? `${profileLabels[profile]} (${profile})` : profile;
