// The dependency graph of a plan (ADR-017) as the Console shows it: every ordering edge with its
// reason, the nodes that were only selected (no ordering edge), and the consumers a failed
// provider held back. Descriptors, reasons and safe codes only: never a param value, output
// value, secret reference or error text.
export type GraphNode = { descriptor: string; kind: string; resourceType?: string; origins?: string[]; paramKeys?: string[] };
export type GraphEdge = { consumer: string; provider: string; reason: string; path?: string };
export type BlockedConsumer = { consumer: string; provider: string; reason: string; code: string };

// Every edge of a plan is an ORDERING edge: the provider runs first. The label says why it exists.
const reasonLabels: Record<string, string> = {
  'resource-reference': 'tham chiếu output của tài nguyên khác',
  'workload-placeholder': 'workload đọc output của tài nguyên',
  'resource-input-placeholder': 'tham số tài nguyên đọc output của tài nguyên khác',
  'deployment-set-private': 'tài nguyên riêng khai báo trong Score',
  'provider-binding': 'provider Kubernetes của module chạy trên cluster này',
  'co-provision-is-dependent': 'tài nguyên đi kèm chạy sau tài nguyên chủ (is_dependent)',
  'co-provision-match-dependents': 'mọi bên dùng tài nguyên chủ chờ tài nguyên đi kèm (match_dependents)',
  'execution-profile': 'hồ sơ thực thi cũ (sẽ bị loại bỏ)',
};
export const reasonLabel = (reason: string) => (Object.hasOwn(reasonLabels, reason) ? reasonLabels[reason] : 'quan hệ phụ thuộc khác');

// Failure codes are fixed allowlist values; anything else renders generically.
const codeLabels: Record<string, string> = {
  REJECTED_BEFORE_RUN: 'bị từ chối trước khi module chạy (đầu ra thiếu/sai kiểu, đầu vào bí mật hoặc đích không dùng được)',
  OUTPUT_CONTRACT: 'đầu ra không đúng hợp đồng',
  NOT_READY: 'chưa sẵn sàng', READINESS_UNAVAILABLE: 'chưa kiểm tra được độ sẵn sàng', AUTH: 'lỗi xác thực', INPUT: 'đầu vào không hợp lệ',
  TIMEOUT: 'quá thời gian', FAILED: 'thất bại', OUTCOME_UNKNOWN: 'chưa rõ kết quả', SECRET_UNAVAILABLE: 'không đọc được bí mật cần thiết',
  DESTRUCTIVE_PLAN: 'kế hoạch sẽ hủy tài nguyên', STATE_BACKEND: 'lỗi kho trạng thái', STATE_NOT_ACKNOWLEDGED: 'trạng thái chưa được xác nhận lưu',
  TRANSIENT: 'lỗi tạm thời', CANCELLED: 'đã hủy', PHYSICAL_RESOURCE_REMAINS: 'vẫn còn tài nguyên vật lý',
};
export const codeLabel = (code: string) => (Object.hasOwn(codeLabels, code) ? codeLabels[code] : 'lỗi không xác định');

const originLabels: Record<string, string> = { 'co-provision': 'tài nguyên đi kèm', 'resource-reference': 'được tham chiếu', 'provider-binding': 'cluster của provider', 'shared-dependency': 'dùng chung', 'private-dependency': 'riêng của workload', 'implicit-workload': 'workload', 'execution-profile': 'hồ sơ thực thi cũ' };
const originLabel = (origin: string) => (Object.hasOwn(originLabels, origin) ? originLabels[origin] : 'nguồn khác');

export function DependencyGraph({ nodes, edges, blocked = [], definitionOf }: { nodes: GraphNode[]; edges: GraphEdge[]; blocked?: BlockedConsumer[]; definitionOf?: (descriptor: string) => string | undefined }) {
  const touched = new Set(edges.flatMap((edge) => [edge.consumer, edge.provider]));
  // A node that exists only because a rule selected it (no flag, no reference) has no ordering edge.
  const selectionOnly = nodes.filter((node) => !touched.has(node.descriptor));
  return <div className="dependency-graph" aria-label="Quan hệ phụ thuộc của kế hoạch">
    <p>{nodes.length} nút · {edges.length} cạnh thứ tự (bên cung cấp chạy trước bên sử dụng)</p>
    <h3>Nút</h3>
    <ul>{nodes.map((node) => <li key={node.descriptor}>{node.descriptor} · {node.kind}{definitionOf?.(node.descriptor) ? ` · ${definitionOf(node.descriptor)}` : ''}{node.paramKeys?.length ? ` · tham số: ${node.paramKeys.join(', ')}` : ''}</li>)}</ul>
    {edges.length ? <><h3>Phụ thuộc (bên sử dụng → bên cung cấp)</h3>
      <ul>{edges.map((edge, index) => <li key={`${edge.consumer}-${edge.provider}-${edge.reason}-${index}`} data-reason={edge.reason}>{edge.consumer} → {edge.provider} <small>({reasonLabel(edge.reason)})</small></li>)}</ul></> : null}
    {selectionOnly.length ? <><h3>Chỉ được chọn, không có thứ tự thực thi</h3>
      <ul>{selectionOnly.map((node) => <li key={node.descriptor}>{node.descriptor} <small>({(node.origins ?? []).map(originLabel).join(', ') || 'được chọn'}; không có cạnh phụ thuộc nào buộc thứ tự)</small></li>)}</ul></> : null}
    {blocked.length ? <section aria-label="Thành phần bị chặn"><h3>Bị chặn bởi phụ thuộc thất bại</h3>
      <ul>{blocked.map((item, index) => <li key={`${item.consumer}-${item.provider}-${index}`} data-code={item.code}>{item.consumer} <strong>không chạy</strong> vì {item.provider} thất bại: {codeLabel(item.code)} <small>({reasonLabel(item.reason)})</small></li>)}</ul></section> : null}
  </div>;
}
