// Stable public error codes of the backend contract (operation-contracts.md,
// "Public HTTP error contract — T20"). The Console never reads or translates the
// backend `error` sentence: it chooses Vietnamese text from the code.

const messageTable = {
  INVALID_REQUEST: 'Dữ liệu gửi lên không hợp lệ. Kiểm tra các trường rồi thử lại; nội dung đang nhập được giữ nguyên.',
  INVALID_ID: 'ID không đúng quy tắc đặt tên. Sửa ID rồi thử lại; Console không tự chuẩn hóa ID.',
  INVALID_VARIABLE: 'Tên, giá trị hoặc tham chiếu biến không hợp lệ. Kiểm tra lại biến; giá trị đã nhập không được hiển thị lại.',
  INVALID_SCORE: 'Score hoặc tham số tài nguyên không hợp lệ. Sửa Score rồi thử lại.',
  PLANNING_REJECTED: 'Không thể lập kế hoạch cho thay đổi này. Chưa có gì được triển khai và không có phương án thay thế tự động.',
  DUPLICATE: 'Định danh này đã tồn tại trong phạm vi hiện tại. Dùng giá trị khác; dữ liệu cũ không bị ghi đè.',
  UNAUTHORIZED: 'Phiên đăng nhập đã hết hạn hoặc chưa đăng nhập. Hãy đăng nhập lại.',
  INVALID_CREDENTIALS: 'Tên đăng nhập hoặc mật khẩu không đúng.',
  FORBIDDEN: 'Bạn không có quyền thực hiện thao tác này.',
  NOT_FOUND: 'Không tìm thấy hoặc không truy cập được mục này.',
  REQUEST_TOO_LARGE: 'Nội dung gửi lên quá lớn. Hãy rút gọn rồi thử lại.',
  STALE_VERSION: 'Dữ liệu đã thay đổi kể từ lần tải trước. Tải lại và xem lại trước khi lưu.',
  STALE_PREVIEW: 'Bản xem trước đã cũ. Hãy xem trước lại; chưa có thay đổi nào được áp dụng.',
  ENVIRONMENT_BUSY: 'Môi trường đang bận với thao tác khác. Hãy chờ hoặc xem trạng thái thao tác; thay đổi đang nhập được giữ nguyên.',
  ENVIRONMENT_UNCONFIGURED: 'Môi trường chưa có kết nối triển khai. Hãy chọn kết nối trong Cài đặt môi trường; chưa có gì được lập kế hoạch hoặc triển khai.',
  CONNECTION_UNAVAILABLE: 'Kết nối không hợp lệ hoặc chưa sẵn sàng (READY) trong phạm vi này. Hãy nhờ kỹ sư nền tảng kiểm tra kết nối.',
  VERIFICATION_FAILED: 'Xác minh không thành công. Kiểm tra thông tin rồi thử lại; Console không hiển thị chi tiết từ nhà cung cấp.',
  VERIFICATION_CONTEXT_MISSING: 'Xác minh không thành công: context chưa được cấu hình trên máy chủ backend.',
  VERIFICATION_UNREACHABLE: 'Xác minh không thành công: không kết nối được tới địa chỉ đã chọn. Kiểm tra địa chỉ, mạng và TLS.',
  VERIFICATION_PERMISSION_DENIED: 'Xác minh không thành công: thiếu quyền cần thiết cho kết nối này.',
  VERIFICATION_CREDENTIAL_REJECTED: 'Xác minh không thành công: thông tin xác thực bị từ chối. Kiểm tra token hoặc chứng chỉ còn hiệu lực.',
  VERIFICATION_REGION_UNAVAILABLE: 'Xác minh không thành công: một vùng được phép chưa dùng được với thông tin xác thực này (vùng bị tắt hoặc dịch vụ cần thiết không truy cập được).',
  VERIFICATION_CAPABILITY_UNVERIFIABLE: 'Xác minh không thành công: chưa chứng minh được quyền cấp phát bằng cách chỉ đọc. Cho phép người dùng IAM này đọc chính sách của mình (iam:SimulatePrincipalPolicy).',
  VERIFICATION_PRINCIPAL_UNSUPPORTED: 'Xác minh không thành công: loại thông tin xác thực này chưa được hỗ trợ. Dùng access key dài hạn của một người dùng IAM riêng, không dùng tài khoản gốc hoặc thông tin xác thực tạm thời.',
  KUBECONFIG_INVALID: 'Tệp kubeconfig không hợp lệ.',
  KUBECONFIG_NO_VALID_CONTEXT: 'Kubeconfig không có context dùng được. Cần context có cluster và người dùng với token hoặc chứng chỉ nhúng sẵn.',
  KUBECONFIG_CONTEXT_REQUIRED: 'Hãy chọn một context trong kubeconfig trước khi kiểm tra và lưu.',
  KUBECONFIG_CONTEXT_NOT_FOUND: 'Context đã chọn không có trong kubeconfig. Kiểm tra lại kubeconfig rồi chọn lại.',
  KUBECONFIG_UNSUPPORTED_AUTH: 'Context dùng phương thức xác thực chưa được hỗ trợ. Hãy dùng kubeconfig có token hoặc chứng chỉ client nhúng sẵn.',
  KUBECONFIG_UNSUPPORTED_CLUSTER: 'Cluster dùng thiết lập chưa được hỗ trợ (tệp CA ngoài, proxy hoặc trường lạ). Hãy nhúng sẵn dữ liệu CA.',
  KUBECONFIG_INVALID_SERVER: 'Địa chỉ máy chủ cluster phải là URL http hoặc https, không chứa thông tin người dùng, query hay fragment.',
  KUBECONFIG_INVALID_TLS: 'Dữ liệu TLS nhúng trong context không hợp lệ (CA, chứng chỉ client hoặc khóa).',
  SECRET_STORE_REQUIRED: 'Hãy chọn kho bí mật trong Cài đặt môi trường trước khi thêm bí mật.',
  SECRET_STORE_UNAVAILABLE: 'Kho bí mật đã chọn không sẵn sàng. Kiểm tra kho hoặc chọn kho khác.',
  REFERENCE_REWRITE_UNSAFE: 'Không thể tự cập nhật tham chiếu mà vẫn giữ nguyên toàn bộ cấu hình workload. Chưa có gì được lưu; hãy bỏ chọn cập nhật tham chiếu hoặc sửa các workload liên quan tường minh trước khi đổi tên.',
  SECRET_COPY_FAILED: 'Không sao chép được bí mật sang kho mới. Lựa chọn kho và phiên bản cũ được giữ nguyên.',
  TRANSITION_REQUIRED: 'Môi trường đã có tài nguyên runtime. Hãy dùng luồng chuyển kết nối để đổi đích.',
  TRANSITION_UNSUPPORTED: 'Chế độ chuyển kết nối này chưa được hỗ trợ.',
  TRANSITION_SAME_DESTINATION: 'Hãy chọn kết nối đích khác kết nối hiện tại.',
  DESTINATION_UNAVAILABLE: 'Kết nối đích không sẵn sàng. Chọn kết nối khác hoặc nhờ kỹ sư nền tảng kiểm tra.',
  NO_RUNTIME: 'Môi trường chưa có tài nguyên runtime nên không cần chuyển; hãy đổi kết nối trong Cài đặt.',
  DOWNTIME_ACK_REQUIRED: 'Cần xác nhận chấp nhận thời gian ngừng ứng dụng trước khi chuyển dữ liệu.',
  RECOVERY_CONFIRMATION_REQUIRED: 'Hãy xác nhận thao tác bị gián đoạn đã dừng hẳn trước khi khôi phục.',
  CLEANUP_REFUSED: 'Chưa thể dọn nguồn cũ. Xem trạng thái chuyển kết nối để biết bước cần xử lý.',
  RESOURCE_CONFIRMATION_MISMATCH: 'Tên nhập lại không khớp tên tài nguyên. Nhập chính xác từng ký tự như hiển thị; chưa có gì bị xóa.',
  NOT_RECOVERABLE: 'Thao tác này hiện không thể khôi phục.',
  RECOVERY_INCOMPLETE: 'Khôi phục chưa hoàn tất. Môi trường vẫn bị giữ cho tới khi khôi phục thành công; cần can thiệp theo hướng dẫn.',
  OPERATION_LOST: 'Thao tác đã bị gián đoạn. Xem trạng thái thao tác; Console không tự tiếp quản.',
  ROUTE_RECONCILE_FAILED: 'Workload đã được áp dụng nhưng chưa cập nhật được route công khai. Hãy xem trước lại để thử lại route.',
  SERVICE_UNAVAILABLE: 'Dịch vụ cần thiết chưa sẵn sàng. Hãy thử lại sau.',
  INTERNAL_ERROR: 'Máy chủ gặp lỗi không xác định. Hãy thử lại; nếu lỗi lặp lại, liên hệ quản trị viên.',
} as const;
export const errorMessages: Readonly<Record<string, string>> & typeof messageTable = Object.freeze(messageTable);

const fallbackByStatus = (status: number): string => {
  if (status === 401) return messageTable.UNAUTHORIZED;
  if (status === 403) return messageTable.FORBIDDEN;
  if (status === 404) return messageTable.NOT_FOUND;
  if (status === 409) return 'Dữ liệu đang xung đột với trạng thái hiện tại. Tải lại và xem lại trước khi thử lại.';
  if (status === 413) return messageTable.REQUEST_TOO_LARGE;
  if (status >= 500) return messageTable.INTERNAL_ERROR;
  if (status >= 400) return 'Máy chủ không chấp nhận yêu cầu này. Kiểm tra dữ liệu rồi thử lại.';
  return 'Yêu cầu không thành công. Hãy thử lại.';
};

export const plannerStages = ['score', 'before-state', 'candidate', 'public-routes', 'service-references', 'catalog', 'graph', 'reference', 'contract', 'schedule'] as const;
export const plannerReasons = ['workload-exists', 'workload-not-current', 'before-mismatch', 'legacy-infrastructure-reference', 'environment-unconfigured', 'renderer-no-match', 'renderer-ambiguous', 'renderer-unavailable'] as const;
export type PlannerStage = (typeof plannerStages)[number];
export type PlannerReason = (typeof plannerReasons)[number];
// Allowlisted refusal reasons of a resource deletion (OC-19); the server sends them under `reason`.
export const cleanupReasons = ['NOT_UNREFERENCED', 'HAS_CONSUMERS', 'PLATFORM_RESOURCE', 'OWNERSHIP_UNVERIFIED', 'ORIGINAL_TARGET_UNAVAILABLE', 'ENVIRONMENT_BUSY'] as const;
export type CleanupReason = (typeof cleanupReasons)[number];
export const cleanupReasonLabels: Readonly<Record<CleanupReason, string>> = Object.freeze({
  NOT_UNREFERENCED: 'Tài nguyên chưa ở trạng thái không còn được tham chiếu (UNREFERENCED).',
  HAS_CONSUMERS: 'Vẫn còn Workload hoặc tài nguyên khác đang dùng tài nguyên này, ngoài tập đã chọn.',
  PLATFORM_RESOURCE: 'Đây là tài nguyên nền tảng, không thuộc quyền xóa của môi trường.',
  OWNERSHIP_UNVERIFIED: 'Chưa có bằng chứng bền vững rằng Orchestrator quản lý tài nguyên này.',
  ORIGINAL_TARGET_UNAVAILABLE: 'Chưa dùng được đích, module hoặc trạng thái gốc của tài nguyên này.',
  ENVIRONMENT_BUSY: 'Môi trường đang bận với thao tác khác.',
});
export interface ErrorParams { stage?: PlannerStage; reason?: PlannerReason; cleanupReason?: CleanupReason; resourceType?: string }

// A registered resource Type key: lowercase letters, digits and hyphens, at most 63 characters.
const technicalID = /^[a-z0-9][a-z0-9-]{0,62}$/;

export const stageLabels: Readonly<Record<PlannerStage, string>> = Object.freeze({
  score: 'Score không khớp hợp đồng của loại tài nguyên đã đăng ký',
  'before-state': 'Score trước không khớp trạng thái hiện tại của workload',
  candidate: 'Không dựng được tập triển khai ứng viên từ Score này (ví dụ tài nguyên dùng chung bị xung đột)',
  'public-routes': 'Route công khai không hợp lệ: trùng đường dẫn hoặc cổng Service chưa khai báo',
  'service-references': 'Một workload tham chiếu Service hoặc cổng không còn tồn tại; hãy cập nhật hoặc xóa workload dùng nó trong cùng thay đổi',
  catalog: 'Cấu hình tài nguyên đã đăng ký chưa dùng được để lập kế hoạch; hãy nhờ kỹ sư nền tảng kiểm tra',
  graph: 'Không dựng hoặc khớp được đồ thị tài nguyên; kiểm tra loại, class và ID so với cấu hình tài nguyên đã đăng ký',
  reference: 'Không giải quyết được một tham chiếu tài nguyên với cấu hình đã khớp',
  contract: 'Một cấu hình tài nguyên khớp có hợp đồng Terraform không hợp lệ; hãy nhờ kỹ sư nền tảng kiểm tra',
  schedule: 'Đồ thị tài nguyên có vòng phụ thuộc',
});

export const reasonLabels: Readonly<Record<PlannerReason, string>> = Object.freeze({
  'workload-exists': 'Workload đã có trong tập triển khai hiện tại; hãy dùng cập nhật kèm Score trước',
  'workload-not-current': 'Workload chưa thuộc tập triển khai hiện tại; hãy dùng triển khai mới',
  'before-mismatch': 'Score trước không khớp bản đang triển khai; hãy lấy lại từ Score đang chạy',
  'legacy-infrastructure-reference': 'Một cấu hình tài nguyên còn tham chiếu VPC/EKS theo ứng dụng; hãy nhờ kỹ sư nền tảng dùng token @infra',
  'environment-unconfigured': 'Môi trường chưa có kết nối triển khai; hãy chọn trong Cài đặt môi trường',
  'renderer-no-match': 'Chưa có Mẫu dựng ứng dụng (score-k8s) khớp Workload này ở phạm vi triển khai của Môi trường; hãy nhờ kỹ sư nền tảng đăng ký đúng một mẫu. Hệ thống không dùng renderer mặc định',
  'renderer-ambiguous': 'Có nhiều Mẫu dựng ứng dụng khớp Workload này với mức ưu tiên bằng nhau; hãy nhờ kỹ sư nền tảng để lại đúng một mẫu',
  'renderer-unavailable': 'Mẫu dựng ứng dụng cần bundle score-k8s chưa được cài trên máy chủ; hãy nhờ kỹ sư nền tảng kiểm tra bản phát hành nền tảng',
});

const has = (table: object, key: unknown): key is string => typeof key === 'string' && Object.prototype.hasOwnProperty.call(table, key);

// Field names the Console recognises; everything else is dropped.
const knownFields: ReadonlySet<string> = new Set([
  'name', 'subdomain', 'connectionKey', 'expectedVersion', 'expectedConfigVersion', 'secretStoreKey', 'destinationKey',
  'acknowledgeDowntime', 'priorExecutionStopped', 'resources', 'status', 'scoreBefore', 'scoreAfter', 'version', 'token', 'kubeconfig',
  'type.key', 'type.schema', 'definition.schema', 'definition.driver', 'definition.references', 'definition.key',
  'definition.resourceType', 'definition.executionProfile', 'definition.driverType', 'definition.criteria',
  'definition.connectionKey', 'definition.driverInputs.values', 'definition.driverInputs.values.source', 'definition.driverInputs.values.provider_bindings', 'driverInputs.values.variables', 'context.applicationId', 'context.environmentId', 'context.resourceId', 'context.class', 'service.publicPort',
  'accessKeyId', 'secretAccessKey', 'allowedRegions', 'confirmationName', 'selectedResourceIds',
]);

export function knownCode(code: unknown): code is string { return has(errorMessages, code); }
export function knownField(field: unknown): field is string { return typeof field === 'string' && knownFields.has(field); }

export function safeParams(params: unknown): ErrorParams | undefined {
  if (typeof params !== 'object' || params === null || Array.isArray(params)) return undefined;
  const source = params as Record<string, unknown>;
  const out: ErrorParams = {};
  if (typeof source.stage === 'string' && (plannerStages as readonly string[]).includes(source.stage)) out.stage = source.stage as PlannerStage;
  if (typeof source.reason === 'string' && (plannerReasons as readonly string[]).includes(source.reason)) out.reason = source.reason as PlannerReason;
  else if (typeof source.reason === 'string' && (cleanupReasons as readonly string[]).includes(source.reason)) out.cleanupReason = source.reason as CleanupReason;
  if (typeof source.resourceType === 'string' && technicalID.test(source.resourceType)) out.resourceType = source.resourceType;
  return out.stage || out.reason || out.cleanupReason || out.resourceType ? out : undefined;
}

/** Vietnamese text for a code/status; never uses backend sentences. */
export function messageFor(status: number, code?: string, params?: ErrorParams, field?: string): string {
  const base = (knownCode(code) ? errorMessages[code] : undefined) ?? fallbackByStatus(status);
  if (code === 'INVALID_SCORE' && params?.resourceType) {
    return `${base} Tham số của tài nguyên thuộc loại ${params.resourceType} chưa đúng: kiểm tra tham số đã khai báo, kiểu giá trị và trường bắt buộc của loại này.`;
  }
  if (code === 'INVALID_SCORE' && field === 'service.publicPort') {
    return `${base} Trường publicPort không còn được hỗ trợ; khai báo đường dẫn công khai bằng publicRoutes. Hệ thống không tự chuyển đổi hay lưu thay đổi.`;
  }
  if (code === 'INVALID_SCORE' && field === 'resources') {
    return `${base} Một tài nguyên trong Score dùng loại chưa được đăng ký; hãy chọn loại tài nguyên đã đăng ký.`;
  }
  if (code === 'CLEANUP_REFUSED' && params?.cleanupReason) {
    return `Không thể xóa: ${cleanupReasonLabels[params.cleanupReason]} Chưa có gì bị xóa.`;
  }
  if (code === 'PLANNING_REJECTED' || code === 'INVALID_SCORE') {
    const detail = (params?.reason && reasonLabels[params.reason]) || (params?.stage && stageLabels[params.stage]);
    if (detail) return `${base} ${detail}.`;
  }
  return base;
}

const fieldLabels: Readonly<Record<string, string>> = Object.freeze({
  name: 'Tên', subdomain: 'Subdomain', connectionKey: 'Kết nối', expectedVersion: 'Phiên bản', expectedConfigVersion: 'Phiên bản cấu hình',
  secretStoreKey: 'Kho bí mật', destinationKey: 'Kết nối đích', acknowledgeDowntime: 'Xác nhận thời gian ngừng', priorExecutionStopped: 'Xác nhận thao tác đã dừng',
  status: 'Trạng thái', resources: 'Tài nguyên trong Score', scoreBefore: 'Score trước', scoreAfter: 'Score sau', version: 'Phiên bản', token: 'Token', kubeconfig: 'Kubeconfig',
  'type.key': 'ID loại tài nguyên', 'type.schema': 'Cấu trúc đầu vào/đầu ra của loại tài nguyên', 'definition.schema': 'Cấu hình tài nguyên',
  'definition.driver': 'Trình điều khiển và tham số của cấu hình', 'definition.references': 'Tham chiếu trong tham số/quy tắc tạo tài nguyên liên quan',
  'definition.key': 'ID cấu hình', 'definition.resourceType': 'Loại tài nguyên', 'definition.executionProfile': 'Phạm vi triển khai',
  'definition.driverType': 'Loại trình điều khiển', 'definition.criteria': 'Điều kiện áp dụng', 'definition.connectionKey': 'Kết nối của cấu hình',
  'definition.driverInputs.values': 'Ánh xạ đầu vào/đầu ra của module', 'definition.driverInputs.values.source': 'Nguồn module đã kiểm chứng (source/revision/digest)',
  'definition.driverInputs.values.provider_bindings': 'Provider binding của module', 'driverInputs.values.variables': 'Tham số cấu hình',
  'context.applicationId': 'Ứng dụng mẫu', 'context.environmentId': 'Môi trường mẫu', 'context.resourceId': 'ID tài nguyên mẫu', 'context.class': 'Class mẫu',
  accessKeyId: 'Access key ID', secretAccessKey: 'Secret access key', allowedRegions: 'Vùng được phép',
  confirmationName: 'Tên nhập lại', selectedResourceIds: 'Tài nguyên đã chọn',
});

/** " Trường liên quan: <nhãn>." for a trusted field, otherwise nothing. */
export function fieldNote(field?: string): string {
  return field && Object.prototype.hasOwnProperty.call(fieldLabels, field) ? ` Trường liên quan: ${fieldLabels[field]}.` : '';
}

// Codes whose page-level text stays status-specific (session, permission, size, duplicate, server).
const statusOwned: ReadonlySet<string> = new Set(['UNAUTHORIZED', 'FORBIDDEN', 'REQUEST_TOO_LARGE', 'DUPLICATE', 'INTERNAL_ERROR', 'SERVICE_UNAVAILABLE', 'NOT_FOUND']);

/** Code-specific guidance for a known code, or undefined so the page falls back to its status text. */
export function specificGuidance(code: string | undefined, message: string, field?: string): string | undefined {
  return code && !statusOwned.has(code) ? `${message}${fieldNote(field)}` : undefined;
}
