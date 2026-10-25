import { initialScope, makeReport, type Check, type Report, type Scenario, type Scope, type Target } from './fixtures';

export type MockAccount = {
  key: string;
  name: string;
  target: Target;
  authentication: string;
  destination: string;
  version: number;
};
export const accounts: MockAccount[] = [
  { key: 'k8s-staging', name: 'K8S · staging-cluster', target: 'kubernetes', authentication: 'Kubeconfig', destination: 'staging-cluster', version: 3 },
  { key: 'aws-sandbox', name: 'AWS · Sandbox', target: 'aws', authentication: 'IAM Role của EC2', destination: '123456789012', version: 2 },
  { key: 'k8s-new', name: 'K8S · new-cluster', target: 'kubernetes', authentication: 'Kubeconfig', destination: 'new-cluster', version: 1 },
];
export type Purpose = 'general' | 'usecase';
export function defaultScope(account: MockAccount, purpose: Purpose): Scope {
  return { ...initialScope, accountKey: account.key, target: account.target, purpose,
    application: purpose === 'general' ? '' : 'storefront',
    useCases: purpose === 'general' ? [] : [...initialScope.useCases] };
}
const item = (id: string, name: string, group: string, observed: string, extra: Partial<Check> = {}): Check => ({
  id, name, group, observed, expected: observed, useCases: [], status: 'PASS', steps: [],
  milliseconds: 60 + id.length * 25,
  technical: `probe=${id}\nsource=executor-runtime\nfixture=true`, ...extra,
});
function generalChecks(scope: Scope): Check[] {
  if (scope.target === 'aws') return [
    item('account-identity', 'Danh tính tài khoản AWS', 'Kết nối & danh tính', 'Danh tính mẫu thuộc account 123456789012 và role platform-executor.'),
    item('account-region', 'Truy cập region đã chọn', 'Kết nối & danh tính', `Các API mẫu phản hồi trong region ${scope.region}.`),
    item('account-tls', 'TLS tới AWS API', 'Mạng & chứng chỉ', 'Runtime tin cậy chuỗi chứng chỉ của các endpoint AWS đã kiểm tra.'),
    item('account-state', 'Kết nối Terraform state backend', 'Cấu hình platform', 'Platform truy cập được dịch vụ state và khóa đã cấu hình.'),
    item('iam-inspection', 'Quyền kiểm tra chính sách IAM', 'Quyền cloud', 'Danh tính thực thi thiếu quyền iam:SimulatePrincipalPolicy trong tình huống mẫu.', {
      status: 'FAIL', expected: 'Platform được phép kiểm tra các quyền của danh tính thực thi đã chọn.',
      steps: ['Đối chiếu chính sách IAM của role thực thi trong tài khoản này.', 'Cấp quyền kiểm tra chính sách theo quy định của tài khoản, hoặc chọn phương thức xác minh quyền phù hợp.', 'Chạy lại để xác minh quyền trước khi kiểm tra các use case.'],
      technical: 'action=iam:SimulatePrincipalPolicy\ncode=PERMISSION_INSPECTION_DENIED\nfixture=true',
    }),
    item('permission-inventory', 'Khả năng xác minh quyền cloud', 'Quyền cloud', 'Chưa xác minh được quyền theo use case vì bước kiểm tra chính sách IAM bị từ chối.', {
      status: 'BLOCKED', dependencies: ['iam-inspection'], expected: 'Platform có bằng chứng về những quyền đã kiểm tra.',
      steps: ['Xử lý quyền kiểm tra chính sách IAM.', 'Chọn use case để kiểm tra các quyền thực sự cần sử dụng.'],
    }),
    item('quota-inventory', 'Đọc thông tin quota', 'Cloud', 'Đọc được thông tin quota trong region mẫu; chưa đối chiếu với nhu cầu của một deployment.'),
    item('organization-policy', 'Ảnh hưởng của chính sách tổ chức', 'Quyền cloud', 'Danh tính hiện tại không đọc được đầy đủ các chính sách tổ chức áp dụng cho tài khoản.', {
      status: 'UNKNOWN', expected: 'Có đủ thông tin để xác định các giới hạn tổ chức ảnh hưởng đến thao tác được chọn.',
      steps: ['Đối chiếu các giới hạn tổ chức với quản trị viên tài khoản.', 'Dùng kiểm chứng thực thi cho use case cụ thể khi cần xác nhận thêm.'],
    }),
  ];
  return [
    item('account-api', 'Kết nối Kubernetes API', 'Kết nối & danh tính', 'Kubernetes API phản hồi từ runtime của platform.'),
    item('account-tls', 'Chứng chỉ API server', 'Mạng & chứng chỉ', 'Runtime xác minh được chuỗi chứng chỉ và hostname của API server.'),
    item('account-rbac', 'Quyền đọc thông tin cụm', 'Kết nối & danh tính', 'Danh tính platform đọc được node, namespace và thông tin storage cần kiểm tra.'),
    item('account-nodes', 'Trạng thái node', 'Kubernetes', '3/3 node của cụm mẫu đang Ready.'),
    item('account-dns', 'Trạng thái CoreDNS', 'Mạng', 'Các pod CoreDNS mẫu đang Ready.'),
    item('storage-controller', 'Controller cấp phát storage', 'Kubernetes', 'StorageClass “standard” dùng CSI controller đang có 0/1 pod Ready trong tình huống mẫu.', {
      status: 'FAIL', expected: 'Controller của provisioner đang được StorageClass sử dụng phải hoạt động.',
      steps: ['Kiểm tra trạng thái và sự kiện của CSI controller đang được StorageClass sử dụng.', 'Khắc phục cấu hình hoặc kết nối khiến controller chưa Ready.', 'Kiểm tra lại controller; chọn use case PostgreSQL để kiểm chứng yêu cầu storage cụ thể.'],
      technical: 'storageClass=standard\ncontroller=example-csi-controller\nready=0/1\ncode=STORAGE_CONTROLLER_NOT_READY\nfixture=true',
    }),
    item('storage-execution', 'Khả năng cấp phát volume thực tế', 'Kubernetes', 'Chưa tạo PVC thử; trạng thái controller và cấu hình chưa chứng minh cấp phát volume thành công.', {
      status: 'UNKNOWN', expected: 'PVC thử được cấp phát và cleanup thành công cho cấu hình đã chọn.',
      steps: ['Xử lý controller storage trước.', 'Chọn “Theo use case”, provision PostgreSQL và mức “Kiểm chứng thực thi” để thử cấp phát.'],
    }),
  ];
}

export function makeAccountReport(scope: Scope, scenario: Scenario, sequence: number): Report {
  if (scope.purpose !== 'general') {
    const report = makeReport(scope, scenario, sequence);
    if (!scope.application) {
      const requiresContext = ['git-source', 'definitions', 'render', 'registry', 'vault-tls', 'vault-auth', 'https-cert', 'storage', 'postgres', 'aurora', 'elasticache', 'aws-pass-role', 'eks'];
      report.checks = report.checks.map((check) => requiresContext.includes(check.id) ? {
        ...check, status: 'UNKNOWN', observed: 'Chưa chọn ứng dụng để đối chiếu Definition, nguồn và cấu hình triển khai cụ thể.',
        steps: ['Chọn ứng dụng trong thiết lập kiểm tra để xác minh điều kiện này.'],
        technical: `probe=${check.id}\ncode=APPLICATION_CONTEXT_REQUIRED\nfixture=true`, dependencies: undefined,
      } : check);
    }
    return report;
  }
  const checks = generalChecks(scope).map((check): Check => scenario === 'ready' && (check.status === 'FAIL' || check.status === 'BLOCKED') ? {
    ...check, status: 'PASS', observed: 'Điều kiện chung mẫu đã đạt sau khi sửa cấu hình.', steps: [],
    technical: `probe=${check.id}\nresult=PASS\nfixture=true`,
  } : check);
  return { id: `DEMO-${String(sequence).padStart(3, '0')}`, scope: { ...scope, application: '', mode: 'conditions', useCases: [] }, scenario: scenario === 'ready' ? 'ready' : 'issues', checkedAt: new Date().toISOString(), checks };
}

export function initialAccountReports(): Report[] {
  return [
    makeAccountReport(defaultScope(accounts[1]!, 'usecase'), 'issues', 4),
    makeAccountReport(defaultScope(accounts[0]!, 'general'), 'issues', 1),
    makeAccountReport(defaultScope(accounts[0]!, 'usecase'), 'issues', 2),
    makeAccountReport(defaultScope(accounts[1]!, 'general'), 'issues', 3),
  ];
}
