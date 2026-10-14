import { cleanupReasonLabels, type CleanupReason, cleanupReasons } from '../../shared/api/errors';

// Own-key lookup with a generic fallback: a persisted value outside the table is never shown as text.
const pick = (map: Record<string, string>, key: string, generic: string) => (Object.hasOwn(map, key) ? map[key] ?? generic : generic);

export const resourceStatusLabel = (key: string) => pick({
  PENDING: 'Đang chờ', PROVISIONING: 'Đang tạo', READY: 'Sẵn sàng', FAILED: 'Thất bại', UNREFERENCED: 'Không còn được tham chiếu', INTERRUPTED: 'Bị gián đoạn',
  NEEDS_ATTENTION: 'Cần xử lý', DESTROYING: 'Đang xóa', DESTROYED: 'Đã xóa',
}, key, 'Trạng thái không rõ');

export const scopeLabel = (key: string) => pick({ APPLICATION: 'Ứng dụng', ENVIRONMENT: 'Môi trường', WORKLOAD: 'Workload', SHARED: 'Dùng chung' }, key, 'Phạm vi không rõ');

export const stageLabel = (key: string) => pick({
  REQUESTED: 'Đã nhận yêu cầu', CHECKING: 'Đang kiểm tra', DESTROYING: 'Đang xóa', VERIFYING: 'Đang xác minh đã hết', INTENT: 'Ghi nhận ý định', INIT: 'Khởi tạo module', PLANNING: 'Đang lập kế hoạch',
  PLANNED: 'Đã lập kế hoạch', APPLYING: 'Đang áp dụng', APPLIED: 'Đã áp dụng', OUTPUTS: 'Đọc đầu ra', READINESS: 'Kiểm tra sẵn sàng', READY: 'Sẵn sàng', NO_CHANGE: 'Không có thay đổi', DESTROYED: 'Đã xóa', RECOVERY: 'Đang khôi phục',
}, key, 'Giai đoạn không rõ');

export const operationStatusLabel = (key: string) => pick({
  ACTIVE: 'Đang chạy', RECOVERING: 'Đang khôi phục', INTERRUPTED: 'Bị gián đoạn — cần xử lý', SUCCEEDED: 'Hoàn tất', FAILED: 'Thất bại', RECOVERED: 'Đã khôi phục',
}, key, 'Trạng thái không rõ');

export const outcomeLabel = (key: string) => pick({
  DESTROYED: 'Đã xóa và xác minh đã hết', DESTROYING: 'Đang xóa', PENDING: 'Đang chờ', REMAINING: 'Chưa xóa hết — còn lại', RETAINED: 'Được giữ lại (không xóa)', NEEDS_ATTENTION: 'Cần xử lý', UNKNOWN: 'Chưa rõ',
}, key, 'Chưa rõ');

export const reasonCodeLabel = (key: string) => pick({
  NOT_ATTEMPTED: 'Chưa thử xóa vì một bước trước cần xử lý', RETAINED_CONSUMER: 'Giữ lại vì một tài nguyên dùng nó chưa được xóa', NEEDS_ATTENTION: 'Chưa rõ kết quả; cần kiểm tra thủ công',
  INSTANCE_HELD: 'Lần thử trước chưa được giải quyết', ORIGINAL_TARGET_UNAVAILABLE: 'Không dùng được đích gốc', DESTRUCTIVE_PLAN: 'Kế hoạch xóa không hợp lệ nên bị chặn',
  NAMESPACE_NOT_OWNED: 'Namespace còn đối tượng không thuộc tập đã chọn', DESTROY_FAILED: 'Xóa không hoàn tất', UNKNOWN_ERROR: 'Lỗi không xác định',
}, key, 'Lỗi không xác định');

export const failureLabel = (key: string) => pick({
  CLEANUP_INCOMPLETE: 'Chưa xóa hết tài nguyên đã chọn.', CLEANUP_REJECTED: 'Yêu cầu bị từ chối trước khi xóa; chưa có gì bị xóa.', CLEANUP_ERROR: 'Thao tác xóa gặp lỗi.', CLEANUP_NEEDS_ATTENTION: 'Chưa rõ kết quả; Môi trường được giữ cho tới khi xử lý.',
  CLEANUP_FAILED: 'Thao tác xóa không hoàn tất.',
}, key, 'Thao tác xóa không hoàn tất.');

export const consumerKindLabel = (key: string) => pick({
  RESOURCE: 'Tài nguyên', WORKLOAD: 'Workload hiện tại', PENDING_WORKLOAD: 'Workload đang chờ triển khai', DEPLOYMENT_SET: 'Tập triển khai hiện tại',
}, key, 'Bên dùng không rõ');

export const refusalLabel = (key: string) => (cleanupReasons as readonly string[]).includes(key) ? cleanupReasonLabels[key as CleanupReason] : 'Không đủ điều kiện xóa.';
