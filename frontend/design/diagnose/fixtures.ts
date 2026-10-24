export type Target = 'kubernetes' | 'aws';
export type Mode = 'conditions' | 'execution';
export type Scenario = 'issues' | 'ready' | 'empty' | 'error';
export type Verdict = 'PASS' | 'FAIL' | 'BLOCKED' | 'UNKNOWN' | 'NA';
export type UseCase = 'preview' | 'deploy' | 'database' | 'cache' | 'secrets' | 'https' | 'lifecycle';
export type Scope = {
  target: Target;
  region: string;
  application: string;
  environment: 'staging' | 'production';
  mode: Mode;
  useCases: UseCase[];
};
export type Check = {
  id: string;
  name: string;
  group: string;
  status: Verdict;
  useCases: UseCase[];
  observed: string;
  expected: string;
  steps: string[];
  technical: string;
  dependencies?: string[];
  milliseconds: number;
};
export type Report = {
  id: string;
  scope: Scope;
  scenario: 'issues' | 'ready';
  checkedAt: string;
  checks: Check[];
};

export const useCases: UseCase[] = ['preview', 'deploy', 'database', 'cache', 'secrets', 'https', 'lifecycle'];
export const initialScope: Scope = {
  target: 'kubernetes', region: 'ap-southeast-1', application: 'storefront',
  environment: 'staging', mode: 'conditions',
  useCases: ['preview', 'deploy', 'database', 'secrets', 'https', 'lifecycle'],
};
export const targetName = (target: Target) => target === 'kubernetes' ? 'K8S · staging-cluster' : 'AWS · Sandbox';
export const executorIdentity = (target: Target) => target === 'kubernetes'
  ? 'system:serviceaccount:platform:orchestrator'
  : 'arn:aws:iam::123456789012:role/platform-executor';

const check = (id: string, name: string, group: string, affected: UseCase[], observed: string, extra: Partial<Check> = {}): Check => ({
  id, name, group, useCases: affected, observed,
  expected: observed, steps: [], technical: `probe=${id}\nsource=executor-runtime\nfixture=true`,
  status: 'PASS', milliseconds: 80 + id.length * 31, ...extra,
});

function common(): Check[] {
  return [
    check('git-source', 'Truy cập nguồn Git', 'Nguồn triển khai', ['preview', 'deploy'], 'Runtime truy cập được render bundle và module Git đã chọn.'),
    check('definitions', 'Resource Definition & matching', 'Cấu hình platform', ['preview', 'deploy', 'database', 'cache'], 'Các tài nguyên có một Definition phù hợp; inputs bắt buộc đã được cung cấp.'),
    check('render', 'Validate & render workload', 'Cấu hình platform', ['preview', 'deploy'], 'Score hợp lệ; render bundle đã chọn tạo được manifest mẫu.'),
    check('registry', 'Truy cập image registry', 'Nguồn triển khai', ['deploy'], 'Registry xác thực thành công; image mẫu có thể truy cập từ runtime.'),
    check('vault-tls', 'Chứng chỉ TLS của Vault', 'Secrets & chứng chỉ', ['secrets', 'deploy'], 'Runtime không tin cậy CA ký chứng chỉ của vault.platform.example.', {
      status: 'FAIL', expected: 'Runtime xác minh được chuỗi chứng chỉ và hostname của endpoint Vault.',
      steps: ['Cấu hình CA của Vault vào trust store mà runtime của platform sử dụng.', 'Kiểm tra endpoint có trả đủ chuỗi chứng chỉ và hostname khớp cấu hình.', 'Kiểm tra lại TLS, đồng bộ secrets và điều kiện deploy liên quan.'],
      technical: 'endpoint=https://vault.platform.example:8200\ncode=TLS_CA_UNTRUSTED\nsource=executor-runtime\nfixture=true',
    }),
    check('vault-auth', 'Xác thực & đồng bộ secrets', 'Secrets & chứng chỉ', ['secrets', 'deploy'], 'Chưa thể kiểm tra xác thực Vault do kết nối TLS thất bại.', {
      status: 'BLOCKED', dependencies: ['vault-tls'],
      expected: 'Platform đọc được secret tham chiếu bằng danh tính đã cấu hình.',
      steps: ['Khắc phục chứng chỉ TLS của Vault trước.', 'Chạy lại kiểm tra để xác minh quyền đọc và đồng bộ secrets.'],
    }),
    check('https-cert', 'Chứng chỉ HTTPS của ứng dụng', 'Secrets & chứng chỉ', ['https'], 'Chứng chỉ cho staging.storefront.example đã hết hạn trong tình huống mẫu.', {
      status: 'FAIL', expected: 'Chứng chỉ còn hạn và bao phủ hostname của môi trường.',
      steps: ['Gia hạn hoặc thay chứng chỉ trong TLS Secret được ingress tham chiếu.', 'Kiểm tra hostname, chuỗi chứng chỉ và thời điểm hết hạn.', 'Kiểm tra lại truy cập HTTPS từ runtime.'],
      technical: 'host=staging.storefront.example\nsecret=storefront-tls\ncode=CERTIFICATE_EXPIRED\nfixture=true',
    }),
    check('cert-manager', 'Gia hạn qua cert-manager', 'Secrets & chứng chỉ', ['https'], 'Mẫu này dùng TLS Secret quản lý thủ công; không yêu cầu cert-manager.', { status: 'NA' }),
    check('lifecycle', 'Thực thi update & cleanup', 'Vòng đời tài nguyên', ['lifecycle'], 'Mới kiểm tra cấu hình và quyền; chưa thực hiện thao tác update hoặc xóa tài nguyên thử.', {
      status: 'UNKNOWN', expected: 'Thao tác thử thành công và chỉ tài nguyên thuộc lần kiểm tra được cleanup.',
      steps: ['Chọn mức “Kiểm chứng thực thi” để thử thao tác và xác nhận cleanup.', 'Xem phạm vi tài nguyên tạm trước khi chạy trên hạ tầng thật.'],
    }),
  ];
}

function kubernetes(): Check[] {
  return [
    check('k8s-api', 'Kết nối Kubernetes API', 'Kết nối & danh tính', ['deploy', 'database', 'cache'], 'Kubernetes API phản hồi; chứng chỉ được runtime tin cậy.'),
    check('k8s-rbac', 'Quyền triển khai trong namespace', 'Kết nối & danh tính', ['deploy', 'database', 'cache', 'lifecycle'], 'Danh tính platform có quyền thao tác với các loại tài nguyên được chọn.'),
    check('nodes', 'Node & tài nguyên khả dụng', 'Kubernetes', ['deploy', 'database', 'cache'], '3/3 node Ready; tài nguyên yêu cầu của workload mẫu nằm trong dung lượng khả dụng.'),
    check('dns', 'DNS & kết nối dịch vụ', 'Mạng', ['deploy', 'database', 'cache', 'https'], 'Runtime phân giải được các endpoint cấu hình và kết nối được Kubernetes API.'),
    check('storage', 'StorageClass cho PostgreSQL', 'Kubernetes', ['database'], 'Definition yêu cầu StorageClass “gp3”; cụm hiện chỉ có “standard”.', {
      status: 'FAIL', expected: 'StorageClass được Definition tham chiếu tồn tại và có provisioner phù hợp.',
      steps: ['Đối chiếu StorageClass hiện có trên cụm với Definition PostgreSQL.', 'Chọn StorageClass phù hợp hoặc cài provisioner mà cụm này cần.', 'Kiểm tra lại storage và khả năng provision PostgreSQL.'],
      technical: 'requestedStorageClass=gp3\nobservedStorageClasses=[standard]\ncode=STORAGE_CLASS_NOT_FOUND\nfixture=true',
    }),
    check('postgres', 'Điều kiện provision PostgreSQL', 'Provision tài nguyên', ['database'], 'Chưa thể xác nhận provision PostgreSQL vì StorageClass yêu cầu không tồn tại.', {
      status: 'BLOCKED', dependencies: ['storage'], expected: 'Các điều kiện storage và Definition PostgreSQL đều đạt.',
      steps: ['Khắc phục StorageClass của PostgreSQL.', 'Kiểm tra lại các điều kiện provision; dùng kiểm chứng thực thi để xác minh tạo PVC.'],
    }),
    check('redis', 'Điều kiện provision Redis', 'Provision tài nguyên', ['cache'], 'Definition Redis và các điều kiện triển khai trong cluster đã đạt.'),
    check('vso', 'Vault Secrets Operator', 'Secrets & chứng chỉ', ['secrets', 'deploy'], 'Các CRD cần dùng có sẵn; controller đang Ready.'),
  ];
}

function aws(): Check[] {
  return [
    check('aws-identity', 'Danh tính AWS & region', 'Kết nối & danh tính', ['preview', 'deploy', 'database', 'cache', 'lifecycle'], 'Danh tính mẫu thuộc account 123456789012; region đã chọn khả dụng.'),
    check('aws-network', 'Kết nối AWS API', 'Mạng', ['deploy', 'database', 'cache'], 'Runtime kết nối được các API AWS cần cho cấu hình mẫu.'),
    check('aws-pass-role', 'Quyền chuyển IAM role cho EKS', 'Quyền cloud', ['deploy'], 'Kiểm tra quyền mẫu trả về denied cho iam:PassRole trên role EKS đã chọn.', {
      status: 'FAIL', expected: 'Danh tính thực thi được phép chuyển đúng role cho dịch vụ mà module EKS sử dụng.',
      steps: ['Đối chiếu role EKS mà Definition/module thực sự sử dụng.', 'Cập nhật quyền iam:PassRole cho role và dịch vụ đích theo chính sách của tài khoản.', 'Kiểm tra lại quyền và các điều kiện provision EKS.'],
      technical: 'action=iam:PassRole\nresource=arn:aws:iam::123456789012:role/platform-eks\ncode=PERMISSION_DENIED\nfixture=true',
    }),
    check('eks', 'Điều kiện provision EKS', 'Provision tài nguyên', ['deploy'], 'Chưa đủ điều kiện provision EKS do quyền chuyển role chưa đạt.', {
      status: 'BLOCKED', dependencies: ['aws-pass-role'], expected: 'Quyền và cấu hình module EKS đã đạt.',
      steps: ['Khắc phục quyền iam:PassRole rồi kiểm tra lại EKS.'],
    }),
    check('aws-quota', 'Quota tài nguyên đã chọn', 'Cloud', ['deploy', 'database', 'cache'], 'Quota mẫu còn đủ cho VPC, EKS và database trong cấu hình này.'),
    check('state-backend', 'Terraform state & locking', 'Cấu hình platform', ['deploy', 'database', 'cache', 'lifecycle'], 'Platform truy cập được state backend và dịch vụ khóa đã cấu hình.'),
    check('aurora', 'Điều kiện provision PostgreSQL', 'Provision tài nguyên', ['database'], 'Các inputs Aurora, subnet và quyền cần thiết trong cấu hình mẫu đã đạt.'),
    check('elasticache', 'Điều kiện provision Redis', 'Provision tài nguyên', ['cache'], 'Các inputs ElastiCache, subnet và quyền cần thiết trong cấu hình mẫu đã đạt.'),
  ];
}

export function makeReport(scope: Scope, scenario: Scenario, sequence: number): Report {
  const checks = [...(scope.target === 'kubernetes' ? kubernetes() : aws()), ...common()]
    .filter((item) => !['vault-tls', 'vault-auth', 'vso'].includes(item.id) || scope.useCases.includes('secrets'))
    .filter((item) => item.useCases.some((uc) => scope.useCases.includes(uc)))
    .map((item): Check => {
      const copy = { ...item, steps: [...item.steps] };
      if (item.id === 'https-cert') {
        const hostname = `${scope.environment === 'staging' ? 'staging.' : ''}${scope.application}.example`;
        copy.observed = `Chứng chỉ cho ${hostname} đã hết hạn trong tình huống mẫu.`;
        copy.technical = `host=${hostname}\nsecret=${scope.application}-tls\ncode=CERTIFICATE_EXPIRED\nfixture=true`;
      }
      if (scenario === 'ready' && (item.status === 'FAIL' || item.status === 'BLOCKED')) {
        copy.status = 'PASS';
        copy.observed = 'Điều kiện mẫu đã đạt sau khi sửa cấu hình.';
        copy.technical = `probe=${item.id}\nresult=PASS\nsource=executor-runtime\nfixture=true`;
        copy.steps = [];
      }
      if (scope.mode === 'execution' && item.id === 'lifecycle') {
        copy.status = 'PASS';
        copy.observed = 'Thao tác update và cleanup tài nguyên thử đã thành công trong mô phỏng.';
        copy.steps = [];
      }
      return copy;
    });
  return {
    id: `DEMO-${String(sequence).padStart(3, '0')}`,
    scope: { ...scope, useCases: [...scope.useCases] },
    scenario: scenario === 'ready' ? 'ready' : 'issues',
    checkedAt: new Date().toISOString(), checks,
  };
}

export function countVerdicts(checks: Check[]): Record<Verdict, number> {
  return checks.reduce((counts, item) => ({ ...counts, [item.status]: counts[item.status] + 1 }), { PASS: 0, FAIL: 0, BLOCKED: 0, UNKNOWN: 0, NA: 0 });
}
