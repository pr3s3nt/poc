import { useEffect, useMemo, useState, useId, useRef, type ReactNode } from 'react';
import { routeReviewEdges, connectionPath, reviewRowGap } from './reviewEdges';
import { reviewMetrics } from './reviewMetrics';
import { nodeFacts } from './reviewProperties';
import { reasonLabel } from '../../shared/ui/DependencyGraph';

export type ReviewSide = { availability: 'AVAILABLE' | 'PARTIAL' | 'ABSENT' | 'UNAVAILABLE'; text?: string; reason?: string; provenance?: Record<string, string>; redactedPaths?: string[]; unknownPaths?: string[] };
export type ReviewArtifact = { id: string; nodeId: string; kind: string; path: string; language: string; before: ReviewSide; after: ReviewSide };
export type ReviewNode = { properties?: { key: string; availability: 'AVAILABLE' | 'UNKNOWN'; value?: string; source: string }[]; id: string; kind: string; name: string; resourceType: string; class: string; definitionKey?: string; definitionVersion?: number; account?: string; region?: string; scope?: string; ownership?: string; intent?: string; module?: { module: string; sourceId: string; revision: string; subdirectory: string; contentSha256: string }; placement?: { cluster: string; namespace?: string; account: string; region?: string; source: string; provider?: string }; renderer?: string; rendererDigest?: string };
export type DeploymentReviewData = { version: 1; planHash: string; nodes: ReviewNode[]; edges: { consumer: string; provider: string; reason: string; path?: string }[]; batches: string[][]; artifacts: ReviewArtifact[] };
export type ReviewProgress = { nodeId: string; state: string; status?: string; stage?: string; attempt?: number; startedAt?: string; updatedAt?: string; finishedAt?: string; blockingProviderIds: string[]; safeErrorCode?: string; observedAt: string };

const reason: Record<string, string> = { BASELINE_MISSING: 'Chưa có bản đã áp dụng phù hợp', CONTENT_UNAVAILABLE: 'Không có nội dung module đã xác minh', UNSUPPORTED_PREVIEW: 'Không thể xem trước cấu trúc này', LIMIT_EXCEEDED: 'Nội dung vượt giới hạn', REDACTION_REQUIRED: 'Không thể lọc an toàn', RENDER_UNAVAILABLE: 'Không thể dựng bản xem trước' };
const states: Record<string, string> = { PENDING: 'Chưa chạy', WAITING: 'Chờ phụ thuộc', PROVISIONING: 'Đang cấp phát', CHECKING: 'Kiểm tra sẵn sàng', READY: 'Sẵn sàng', RETAINED: 'Giữ nguyên', FAILED: 'Thất bại', BLOCKED: 'Bị chặn', UNKNOWN: 'Chưa xác định' };
const text = (map: Record<string, string>, key: string) => Object.hasOwn(map, key) ? map[key] : key;
type DiffLine = { type: 'same' | 'old' | 'new'; old?: number; next?: number; value: string };

// Bounds matrix work before allocation. A large safe side stays available in
// after-only mode even when a browser diff would be expensive.
export function diffLines(before: string, after: string): DiffLine[] | undefined {
  const split = (value: string) => value === '' ? [] : value.replace(/\n$/, '').split('\n');
  const a = split(before); const b = split(after);
  if (a.length * b.length > 360_000 || a.length + b.length > 1_200) return undefined;
  const width = b.length + 1;
  const table = new Uint16Array((a.length + 1) * width);
  for (let i = a.length - 1; i >= 0; i--) for (let j = b.length - 1; j >= 0; j--) table[i * width + j] = a[i] === b[j] ? (table[(i + 1) * width + j + 1] ?? 0) + 1 : Math.max(table[(i + 1) * width + j] ?? 0, table[i * width + j + 1] ?? 0);
  const rows: DiffLine[] = []; let i = 0; let j = 0;
  while (i < a.length || j < b.length) {
    if (i < a.length && j < b.length && a[i] === b[j]) rows.push({ type: 'same', old: ++i, next: ++j, value: a[i - 1] ?? '' });
    else if (i < a.length && (j === b.length || (table[(i + 1) * width + j] ?? 0) >= (table[i * width + j + 1] ?? 0))) rows.push({ type: 'old', old: ++i, value: a[i - 1] ?? '' });
    else rows.push({ type: 'new', next: ++j, value: b[j - 1] ?? '' });
  }
  return rows;
}

function CodePane({ artifact }: { artifact: ReviewArtifact }) {
  const [afterOnly, setAfterOnly] = useState(false);
  const [expanded, setExpanded] = useState(false);
  const [copyStatus, setCopyStatus] = useState('');
  const codeRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const close = (event: KeyboardEvent) => { if (event.key === 'Escape') setExpanded(false); };
    const fullscreen = () => { if (!document.fullscreenElement) setExpanded(false); };
    window.addEventListener('keydown', close); document.addEventListener('fullscreenchange', fullscreen);
    return () => { window.removeEventListener('keydown', close); document.removeEventListener('fullscreenchange', fullscreen); };
  }, []);
  const before = artifact.before; const after = artifact.after;
  const copy = async () => {
    try { if (!navigator.clipboard) throw new Error(); await navigator.clipboard.writeText(after.text ?? ''); setCopyStatus('Đã sao chép'); }
    catch { setCopyStatus('Không thể sao chép tự động; hãy chọn code và nhấn Ctrl+C.'); }
  };
  const both = [before, after].every(side => ['AVAILABLE', 'PARTIAL', 'ABSENT'].includes(side.availability));
  const lines = useMemo(() => both ? diffLines(before.text ?? '', after.text ?? '') : undefined, [both, before.text, after.text]);
  const additions = lines?.filter(line => line.type === 'new').length;
  const removals = lines?.filter(line => line.type === 'old').length;
  return <div ref={codeRef} className={`review-code ${expanded ? 'review-code-expanded' : ''}`}>
    <div className="review-code-toolbar"><div className="review-code-modes"><button aria-pressed={!afterOnly} onClick={() => setAfterOnly(false)}>Xem diff</button><button aria-pressed={afterOnly} onClick={() => setAfterOnly(true)}>Sau thay đổi</button></div><button className="review-text-button" aria-label={expanded ? 'Thu gọn' : 'Mở rộng'} aria-expanded={expanded} onClick={() => {
      if (!expanded) codeRef.current?.requestFullscreen?.().catch(() => undefined);
      else if (document.fullscreenElement) void document.exitFullscreen();
      setExpanded(value => !value);
    }}>{expanded ? 'Thu gọn' : 'Mở rộng'} ↗</button></div>
    {lines ? <div className="review-diff-summary"><span className="review-added">+{additions} dòng thêm</span><span className="review-removed">−{removals} dòng bỏ</span></div> : null}
    <div className="review-code-header"><strong>{artifact.path}</strong><button disabled={!after.text} onClick={() => void copy()}>Sao chép bản sau</button></div>
    {before.availability === 'UNAVAILABLE' ? <p className="review-unavailable">Bản trước: {text(reason, before.reason ?? 'BASELINE_MISSING')}</p> : null}
    {after.availability === 'UNAVAILABLE' ? <p className="review-unavailable">Bản sau: {text(reason, after.reason ?? 'CONTENT_UNAVAILABLE')}</p> : null}
    {after.text || (both && before.text) ? afterOnly || !both || !lines ? <pre className="review-after"><code>{after.text}</code></pre> : <div className="review-diff" role="table" aria-label={`Diff ${artifact.path}`}>{lines.map((line, index) => <div className={`review-diff-line review-diff-${line.type}`} role="row" key={`${index}-${line.type}`}><span role="cell" className="review-gutter">{line.old ?? ''}</span><span role="cell" className="review-gutter">{line.next ?? ''}</span><span role="cell" className="review-sign">{line.type === 'old' ? '−' : line.type === 'new' ? '+' : ' '}</span><code role="cell">{line.value || ' '}</code></div>)}</div> : null}
    {copyStatus ? <p role="status" className="review-code-note">{copyStatus}</p> : null}
    {!afterOnly && both && !lines ? <p className="review-unavailable">Diff quá lớn để hiển thị; bản sau vẫn có thể xem và sao chép.</p> : null}
    {before.redactedPaths?.length || after.redactedPaths?.length ? <p className="review-code-note">Giá trị đã ẩn không thể đối chiếu. Bản hiển thị đã được lọc, không dùng để thực thi.</p> : null}
    {after.unknownPaths?.length ? <p className="review-code-note">{after.unknownPaths.length} giá trị đang chờ đầu ra tài nguyên.</p> : null}
    {after.provenance ? <details className="review-technical"><summary>Nguồn và phiên bản</summary><dl className="review-properties">{Object.entries(after.provenance).map(([key, value]) => <div key={key}><dt>{key}</dt><dd>{value}</dd></div>)}</dl></details> : null}
  </div>;
}

function levelsOf(review: DeploymentReviewData): ReviewNode[][] {
  const remaining = new Map(review.nodes.map((n) => [n.id, n]));
  const result: ReviewNode[][] = []; const done = new Set<string>();
  while (remaining.size) {
    const layer = [...remaining.values()].filter((node) => review.edges.filter((edge) => edge.consumer === node.id).every((edge) => done.has(edge.provider) || !remaining.has(edge.provider)));
    if (!layer.length) { result.push([...remaining.values()]); break; }
    result.push(layer); for (const node of layer) { remaining.delete(node.id); done.add(node.id); }
  }
  return result;
}

function nodeLabel(node: ReviewNode): string {
  if (node.kind === 'workload' || !node.name.includes('#')) return node.name;
  const labels: Record<string, string> = { 'k8s-cluster': node.ownership === 'PREEXISTING' ? 'Cluster nội bộ' : 'Cluster', 'k8s-namespace': 'Namespace', postgres: 'PostgreSQL', redis: 'Redis', vpc: 'VPC' };
  return labels[node.resourceType] ?? node.resourceType;
}
function nodeDescription(node: ReviewNode): string {
  if (node.ownership === 'PREEXISTING') return 'Cluster có sẵn';
  const modules: Record<string, string> = { eks: 'Amazon EKS', aurora: 'Amazon Aurora', elasticache: 'Amazon ElastiCache' };
  if (node.module && modules[node.module.module]) return modules[node.module.module]!;
  const labels: Record<string, string> = { 'k8s-cluster': 'Kubernetes cluster', 'k8s-namespace': 'Kubernetes namespace', postgres: 'Database', redis: 'Cache', vpc: 'Mạng riêng trên cloud' };
  return node.kind === 'workload' ? 'Ứng dụng · ' + (node.class || 'default') : labels[node.resourceType] ?? node.class;
}
function NodeIcon({ node }: { node: ReviewNode }) {
  const paths: Record<string, ReactNode> = {
    'k8s-cluster': <><path d="m12 2 9 5v10l-9 5-9-5V7l9-5Z" /><circle cx="12" cy="12" r="3" /><path d="m12 5 0 4m0 6v4M6 8l3 2m6 4 3 2M6 16l3-2m6-4 3-2" /></>,
    'k8s-namespace': <><rect x="3" y="4" width="18" height="16" rx="3" /><path d="M3 9h18M8 4v5" /><rect x="7" y="12" width="4" height="4" rx="1" /><path d="M15 12h2m-2 4h2" /></>,
    workload: <><path d="m8 6-6 6 6 6m8-12 6 6-6 6m-3-15-2 18" /></>,
    postgres: <><ellipse cx="12" cy="5" rx="8" ry="3" /><path d="M4 5v14c0 4 16 4 16 0V5M4 12c0 4 16 4 16 0" /></>,
    redis: <><path d="m3 8 9-5 9 5-9 5-9-5Zm0 5 9 5 9-5M3 18l9 5 9-5" /></>,
    vpc: <><rect x="3" y="3" width="18" height="18" rx="3" /><path d="M8 8h8v8H8zM1 8h2m-2 8h2m18-8h2m-2 8h2" /></>,
  };
  return <svg viewBox="0 0 24 24" aria-hidden="true">{paths[node.kind === 'workload' ? 'workload' : node.resourceType] ?? <><rect x="3" y="4" width="18" height="16" rx="2" /><path d="M3 9h18M7 13h10M7 16h6" /></>}</svg>;
}
function StateBadge({ state = 'PENDING' }: { state?: string }) {
  return <span className="review-state-badge" data-state={state}><span aria-hidden="true">{['READY', 'RETAINED'].includes(state) ? '✓' : state === 'FAILED' ? '×' : '○'}</span>{text(states, state)}</span>;
}
type Position = { x: number; y: number };
function ReviewGraph({ review, progress, selected, select, title }: { review: DeploymentReviewData; progress?: ReviewProgress[]; selected?: string; select(id: string): void; title: string }) {
  const arrowId = useId().replace(/:/g, '');
  const viewport = useRef<HTMLDivElement>(null);
  const [availableWidth, setAvailableWidth] = useState(0);
  const [hovered, setHovered] = useState<string>();
  const [focusedOnly, setFocusedOnly] = useState(false);
  useEffect(() => {
    const element = viewport.current; if (!element) return;
    const resize = () => setAvailableWidth(element.clientWidth);
    resize(); if (typeof ResizeObserver === 'undefined') return;
    const observer = new ResizeObserver(resize); observer.observe(element); return () => observer.disconnect();
  }, []);
  const layers = useMemo(() => levelsOf(review), [review]);
  const groupOf = (node: ReviewNode) => node.placement?.cluster || (node.resourceType === 'k8s-cluster' ? node.id : node.region || node.resourceType === 'vpc' ? 'outside' : 'unresolved');
  const groupIds = [...new Set(review.nodes.map(groupOf))].sort((a, b) => a === 'outside' ? 1 : b === 'outside' ? -1 : a.localeCompare(b));
  const { nodeWidth, nodeHeight, columnGap, groupPadding } = reviewMetrics;
  const rowGap = reviewRowGap(review.edges, layers), rowPitch = nodeHeight + rowGap;
  const groupWidth = (id: string) => { const count = Math.max(1, ...layers.map(layer => layer.filter(n => groupOf(n) === id).length)); return count * nodeWidth + (count - 1) * columnGap + groupPadding * 2; };
  const starts = new Map<string, number>(); let cursor = 24;
  for (const id of groupIds) { starts.set(id, cursor); cursor += groupWidth(id) + 20; }
  const cloudScopes = [...new Set(review.nodes.flatMap(n => {
    const account = n.placement?.account || n.account; const region = n.placement?.region || n.region;
    return account && region ? [account + ' · ' + region] : [];
  }))];
  const cloudScope = cloudScopes.length === 1 ? cloudScopes[0] : undefined;
  const offset = cloudScope ? 36 : 0;
  const positions = new Map<string, Position>();
  layers.forEach((layer, row) => groupIds.forEach(id => {
    const items = layer.filter(n => groupOf(n) === id);
    items.forEach((n, col) => positions.set(n.id, { x: (starts.get(id) ?? 24) + (groupWidth(id) - (items.length * nodeWidth + Math.max(0, items.length - 1) * columnGap)) / 2 + col * (nodeWidth + columnGap), y: 58 + offset + row * rowPitch }));
  }));
  let connections = routeReviewEdges(review.edges, positions);
  const shift = Math.max(0, 24 - Math.min(...connections.flatMap(edge => edge.points.map(p => p.x))));
  if (shift > 0) { for (const [id, p] of positions) positions.set(id, { ...p, x: p.x + shift }); for (const [id, x] of starts) starts.set(id, x + shift); connections = routeReviewEdges(review.edges, positions); }
  const width = Math.max(620, cursor + shift, ...connections.flatMap(edge => edge.points.map(p => p.x + 24)));
  const height = Math.max(420, (layers.length - 1) * rowPitch + nodeHeight + 112 + offset);
  const facts = new Map(review.nodes.map(node => [node.id, nodeFacts(node, review.artifacts.filter(a => a.nodeId === node.id))]));
  const scale = availableWidth ? Math.max(.85, Math.min(1, (availableWidth - 16) / width)) : 1;
  const nameOf = (id: string) => { const item = review.nodes.find(n => n.id === id); return item ? nodeLabel(item) : id; };
  return <section className="review-graph" aria-label="Sơ đồ triển khai">
    <header className="review-panel-heading"><div><h3>{title}</h3><p>Chọn một thành phần để khám phá thay đổi.</p></div><span className="review-map-badge">{cloudScope ? 'Cloud' : review.nodes.some(n => n.resourceType === 'k8s-cluster') ? 'Kubernetes' : 'Kế hoạch'}</span></header>
    <div className="review-graph-controls"><label><input type="checkbox" checked={focusedOnly} onChange={event => setFocusedOnly(event.target.checked)} /> Chỉ đường nối của node đang chọn</label><span>Cuộn để xem toàn bộ sơ đồ</span></div>
    <div ref={viewport} className="review-graph-scroll"><div className="review-graph-canvas" style={{ width: width * scale, height: height * scale }}><div className="review-graph-stage" style={{ width, height, transform: `scale(${scale})` }}>
      {cloudScope ? <div className="review-cloud-boundary" style={{ width: width - 16, height: height - 24 }}><strong>{cloudScope}</strong></div> : null}
      {groupIds.map(id => {
        const members = review.nodes.filter(n => groupOf(n) === id); const ys = members.map(n => positions.get(n.id)?.y ?? 58);
        const top = Math.min(...ys) - 24; const bottom = Math.max(...ys) + nodeHeight + 20;
        return <div key={id} className={`review-placement-group ${id === 'outside' ? 'review-placement-outside' : ''}`} style={{ left: starts.get(id), top, width: groupWidth(id), height: bottom - top }} data-cluster={id}><strong>{id === 'outside' ? 'Hạ tầng ngoài Kubernetes' : id === 'unresolved' ? 'Vị trí chưa xác định' : nameOf(id)}</strong></div>;
      })}
      <svg className="review-edges" width={width} height={height} aria-label="Các cạnh phụ thuộc"><defs><marker id={arrowId} markerWidth="7" markerHeight="7" refX="6" refY="3.5" orient="auto"><path d="M0 0L7 3.5L0 7" fill="context-stroke" /></marker></defs>{connections.map(edge => {
        const focus = hovered ?? selected;
        return <path key={JSON.stringify([edge.provider, edge.consumer])} d={connectionPath(edge)} style={{ display: focusedOnly && selected && edge.provider !== selected && edge.consumer !== selected ? 'none' : undefined }} data-provider={edge.provider} data-consumer={edge.consumer} data-focused={focus === edge.provider || focus === edge.consumer} data-outer-lane={edge.indirectLane} markerEnd={`url(#${arrowId})`}><title>{nameOf(edge.provider)} → {nameOf(edge.consumer)}: {[...new Set(edge.reasons.map(reason => reasonLabel(reason.reason)))].join(' · ')}</title></path>;
      })}</svg>
      {review.nodes.map(n => { const p = progress?.find(item => item.nodeId === n.id); const position = positions.get(n.id);
        return <button type="button" key={n.id} style={{ left: position?.x, top: position?.y, width: nodeWidth, height: nodeHeight }} title={n.id} className={`review-node ${selected === n.id ? 'review-node-selected' : ''}`} data-resource-type={n.kind === 'workload' ? 'workload' : n.resourceType} data-state={p?.state ?? 'PENDING'} data-node-id={n.id} onClick={() => select(n.id)} onMouseEnter={() => setHovered(n.id)} onMouseLeave={() => setHovered(undefined)} aria-pressed={selected === n.id}>
          <span className="review-node-icon"><NodeIcon node={n} /></span><strong>{nodeLabel(n)}</strong><small>{nodeDescription(n)}</small><span className="review-node-facts">{(facts.get(n.id) ?? []).slice(0, 2).map(([label, value]) => <span key={label} title={`${label}: ${value}`}><span>{label}</span><strong>{value}</strong></span>)}</span><span className="review-node-bottom"><StateBadge state={p?.state} /><span>{intentLabel(n.intent)}</span></span>
        </button>;
      })}
    </div></div></div>
    <footer className="review-graph-legend"><span>→ Điều kiện sẵn sàng → thành phần phụ thuộc</span><span>Nét đứt: liên kết đi qua nhiều tầng</span><span>Chọn thành phần để làm rõ các đường nối.</span><span>Cuộn ngang/dọc để xem đầy đủ sơ đồ.</span></footer>
  </section>;
}
const intents: Record<string,string> = { CREATE:'Tạo mới',UPDATE:'Cập nhật',RETAIN:'Giữ nguyên',REMOVE:'Gỡ bỏ',UNREFERENCED:'Gỡ tham chiếu',BIND:'Liên kết',UNVERIFIED:'Chưa xác minh' };
const intentLabel=(value?:string)=>text(intents,value??'UNVERIFIED');
function ProgressInfo({ value }: { value?: ReviewProgress }) {
  const [now, setNow] = useState(Date.now());
  const active = value && ['PROVISIONING', 'CHECKING'].includes(value.state);
  useEffect(() => { if (!active) return; const timer = window.setInterval(() => setNow(Date.now()), 1000); return () => window.clearInterval(timer); }, [active]);
  const start = value?.startedAt ? Date.parse(value.startedAt) : NaN;
  const end = value?.finishedAt ? Date.parse(value.finishedAt) : active ? now : value?.updatedAt ? Date.parse(value.updatedAt) : NaN;
  const seconds = Number.isFinite(start) && Number.isFinite(end) ? Math.max(0, Math.floor((end - start) / 1000)) : undefined;
  const descriptions: Record<string, string> = { PENDING: 'Chưa có bản ghi bắt đầu thực thi.', WAITING: 'Đang chờ các thành phần phụ thuộc sẵn sàng.', PROVISIONING: 'Thành phần đang được cấp phát hoặc áp dụng.', CHECKING: 'Đang kiểm tra điều kiện sẵn sàng.', READY: 'Đã xác nhận sẵn sàng trong bản ghi thực thi.', RETAINED: 'Giữ lại tài nguyên đã được xác minh trong kế hoạch này.', FAILED: 'Thực thi chưa hoàn tất. Xem mã lỗi và các phụ thuộc bên dưới.', BLOCKED: 'Một phụ thuộc thất bại đang chặn thành phần này.', UNKNOWN: 'Chưa đủ dữ liệu để kết luận trạng thái.' };
  return <section className="review-runtime-card" data-state={value?.state ?? 'PENDING'}><div><strong>Trạng thái cấp phát</strong><StateBadge state={value?.state} /></div><p>{descriptions[value?.state ?? 'PENDING'] ?? descriptions.UNKNOWN}</p><dl><div><dt>Thời gian thực thi</dt><dd>{seconds === undefined ? 'Chưa có thời gian' : `${Math.floor(seconds / 60)} phút ${seconds % 60} giây`}</dd></div><div><dt>Bước hiện tại</dt><dd>{value?.stage || (value ? text(states, value.state) : 'Chưa chạy')}{value?.attempt ? ` · lần ${value.attempt}` : ''}</dd></div></dl></section>;
}

function ArtifactChange({artifact}:{artifact:ReviewArtifact}) {
  const before=artifact.before,after=artifact.after;
  const safeBefore=before.availability==='ABSENT'||before.text!==undefined;
  const safeAfter=after.availability==='ABSENT'||after.text!==undefined;
  const lines=safeBefore&&safeAfter?diffLines(before.text??'',after.text??''):undefined;
  const additions=lines?.filter(l=>l.type==='new').length;
  const removals=lines?.filter(l=>l.type==='old').length;
  return <li className="review-change-card"><strong>{artifact.path}</strong>{lines?<span><em className="review-added">+{additions} dòng thêm</em> · <em className="review-removed">−{removals} dòng bỏ</em></span>:<span>{text(reason,before.reason??after.reason??'BASELINE_MISSING')}</span>}
    {lines?.some(line => line.type === 'old') ? <div className="review-change-before"><small>Trước thay đổi</small><code>{lines.filter(line => line.type === 'old').slice(0, 3).map(line => line.value).join('\n')}</code></div> : null}
    {lines?.some(line => line.type === 'new') ? <div className="review-change-after"><small>Sau thay đổi</small><code>{lines.filter(line => line.type === 'new').slice(0, 3).map(line => line.value).join('\n')}</code></div> : null}
    {before.text===after.text&&lines?<small>Phần nội dung hiển thị không đổi.</small>:null}
    {before.redactedPaths?.length||after.redactedPaths?.length||before.unknownPaths?.length||after.unknownPaths?.length?<small>Giá trị đã ẩn / chưa có chưa thể đối chiếu.</small>:null}
  </li>;
}
export function DeploymentReview({ review, progress, title = 'Bản đồ triển khai' }: { review?: DeploymentReviewData; progress?: ReviewProgress[]; title?: string }) {
  // No-change/legacy responses may carry the Go zero value (version 0 and
  // null collections). Check the wire shape before reading any collection.
  if (!review || review.version !== 1 || !Array.isArray(review.nodes) || !Array.isArray(review.edges) || !Array.isArray(review.artifacts) || !Array.isArray(review.batches)) {
    return <p className="review-unavailable">Bản review của kế hoạch này chưa có.</p>;
  }
  return <DeploymentReviewContent review={review} progress={progress} title={title} />;
}

function DeploymentReviewContent({ review, progress, title }: { review: DeploymentReviewData; progress?: ReviewProgress[]; title: string }) {
  const [selected, setSelected] = useState('');
  const [tab, setTab] = useState<'overview' | 'changes' | 'code'>('overview');
  const [fileId, setFileId] = useState('');
  const tabs = useRef<Array<HTMLButtonElement | null>>([]);
  const firstNodeId = review?.nodes.find(n => n.kind === 'workload' && n.intent !== 'RETAIN')?.id ?? review?.nodes[0]?.id;
  useEffect(() => { setSelected(firstNodeId ?? ''); setTab('overview'); setFileId(''); }, [review?.planHash, firstNodeId]);
  const node = review?.nodes.find(n => n.id === selected) ?? review?.nodes[0];
  const state = progress?.find(p => p.nodeId === node?.id);
  const artifacts = review?.artifacts.filter(a => a.nodeId === node?.id) ?? [];
  const artifact = artifacts.find(a => a.id === fileId) ?? artifacts[0];
  const dependencies = review?.edges.filter(e => e.consumer === node?.id) ?? [];
  const dependenciesByProvider = new Map<string, typeof dependencies>();
  for (const edge of dependencies) dependenciesByProvider.set(edge.provider, [...(dependenciesByProvider.get(edge.provider) ?? []), edge]);
  const ready = review?.nodes.filter(n => progress?.some(p => p.nodeId === n.id && ['READY', 'RETAINED'].includes(p.state))).length ?? 0;
  const running = progress?.filter(p => ['PROVISIONING', 'CHECKING'].includes(p.state)).length ?? 0;
  const properties = node ? nodeFacts(node, artifacts).filter(([label]) => !['Tài khoản', 'Vùng', 'Module'].includes(label)) : [];
  const selectNode = (id: string) => { setSelected(id); setTab('overview'); setFileId(''); };
  const scopeNames = [...new Set(review.nodes.flatMap(n => { const account = n.placement?.account || n.account; const region = n.placement?.region || n.region; return account && region ? [account + ' · ' + region] : []; }))];
  const totals = [{ intents: ['CREATE'], label: 'tạo mới', tone: 'create' }, { intents: ['UPDATE'], label: 'cập nhật', tone: 'update' }, { intents: ['RETAIN', 'BIND'], label: 'giữ nguyên / liên kết', tone: 'retain' }, { intents: ['REMOVE', 'UNREFERENCED'], label: 'gỡ bỏ / tham chiếu', tone: 'remove' }];
  const stages = levelsOf(review);
  return <section className="deployment-review" aria-label={title}>
    <div className="review-summary"><div className="review-summary-title"><span className="review-summary-icon" aria-hidden="true">✓</span><div><strong>{review.nodes.length} thành phần trong kế hoạch</strong><small>{scopeNames.join(' · ') || 'Đích triển khai theo Resource Definition'}</small></div></div><div className="review-intent-counts">{totals.map(total => { const count = review.nodes.filter(n => total.intents.includes(n.intent ?? '')).length; return count ? <span key={total.tone} data-intent={total.tone}><i />{count} {total.label}</span> : null; })}</div></div>
    <div className="review-layout"><ReviewGraph review={review} progress={progress} selected={node?.id} title={title} select={selectNode} />
      <aside className="review-inspector" aria-label="Chi tiết thành phần">{node ? <><header className="review-detail-heading" data-resource-type={node.kind === 'workload' ? 'workload' : node.resourceType}><span className="review-detail-icon"><NodeIcon node={node} /></span><div><h3 title={node.id}>{nodeLabel(node)}</h3><p>{nodeDescription(node)}</p></div><StateBadge state={state?.state} /></header>
        <div className="review-tabs" role="tablist" aria-label="Chi tiết thành phần">{(['overview', 'changes', 'code'] as const).map((key, index) => <button key={key} ref={el => { tabs.current[index] = el; }} role="tab" tabIndex={tab === key ? 0 : -1} aria-selected={tab === key} onKeyDown={event => {
          if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
          event.preventDefault(); const next = event.key === 'Home' ? 0 : event.key === 'End' ? 2 : (index + (event.key === 'ArrowRight' ? 1 : -1) + 3) % 3;
          setTab((['overview', 'changes', 'code'] as const)[next] ?? 'overview'); tabs.current[next]?.focus();
        }} onClick={() => setTab(key)}>{key === 'overview' ? 'Tổng quan' : key === 'changes' ? <>Thay đổi <span className="review-tab-count">{artifacts.length}</span></> : 'Code'}</button>)}</div>
        {tab === 'overview' ? <div className="review-inspector-body"><ProgressInfo value={state} /><h4>Cấu hình dự kiến</h4><dl className="review-properties">
          {properties.map(([label, value]) => <div key={label}><dt>{label}</dt><dd title={value}>{value}</dd></div>)}
          <div><dt>Vị trí triển khai</dt><dd>{node.placement?.cluster ? nodeLabel(review.nodes.find(n => n.id === node.placement?.cluster) ?? node) : node.resourceType === 'k8s-cluster' ? nodeLabel(node) : 'Ngoài Kubernetes'}</dd></div>
          {node.account || node.placement?.account ? <div><dt>Tài khoản / vùng</dt><dd>{node.placement?.account || node.account}{node.placement?.region || node.region ? ` / ${node.placement?.region || node.region}` : ''}</dd></div> : null}
          <div><dt>Module / renderer</dt><dd title={node.module?.revision || node.renderer}>{node.module?.module || node.renderer?.split('@')[0] || 'Liên kết có sẵn'}</dd></div>
          <div><dt>Thay đổi dự kiến</dt><dd><span className="review-intent-badge" data-intent={node.intent}>{intentLabel(node.intent)}</span></dd></div>
        </dl><h4>Điều kiện sẵn sàng</h4>{dependencies.length ? <div className="review-dependencies">{[...dependenciesByProvider].map(([id, edges]) => {
          const provider = review.nodes.find(n => n.id === id); const observation = progress?.find(p => p.nodeId === id);
          const reasons = [...new Set(edges.map(edge => reasonLabel(edge.reason)))].join(' · ');
          return <button key={id} className="review-dependency" title={reasons} onClick={() => selectNode(id)}><span>{provider ? nodeLabel(provider) : id}<small>{reasons}</small></span><span>{observation ? text(states, observation.state) : 'Chưa chạy'} →</span></button>;
        })}</div> : <p className="review-description">Không có phụ thuộc trong kế hoạch.</p>}
        {state?.blockingProviderIds.length ? <p className="review-unavailable">Đang chờ: {state.blockingProviderIds.map(id => { const provider = review.nodes.find(n => n.id === id); return provider ? nodeLabel(provider) : id; }).join(', ')}</p> : null}
        {state?.safeErrorCode ? <p className="review-unavailable">Lỗi: {state.safeErrorCode}</p> : null}
        {artifacts.length ? <button className="review-code-link" onClick={() => setTab('code')}>Xem {node.kind === 'workload' ? 'Kubernetes manifest' : 'Terraform source'} ↗</button> : null}
        <details className="review-technical"><summary>Thông tin kỹ thuật</summary><dl className="review-properties"><div><dt>ID</dt><dd>{node.id}</dd></div><div><dt>Definition</dt><dd>{node.definitionKey || '—'} {node.definitionVersion ? `v${node.definitionVersion}` : ''}</dd></div><div><dt>Cluster / namespace</dt><dd>{node.placement?.cluster || '—'} / {node.placement?.namespace || 'Chờ cấp phát'}</dd></div><div><dt>Phiên bản</dt><dd>{node.module?.revision || node.renderer || '—'}</dd></div></dl></details></div> : null}
        {tab === 'changes' ? <div className="review-inspector-body"><p className="review-description">{intentLabel(node.intent)} · So sánh nội dung được phép hiển thị trước và sau thay đổi.</p><ul className="review-change-list">{artifacts.map(a => <ArtifactChange key={a.id} artifact={a} />)}</ul>{!artifacts.length ? <p>Chưa có artifact an toàn cho nút này.</p> : null}<p className="review-code-note">Đây là thay đổi cấu hình dự kiến, không phải Terraform plan hay kết quả provider.</p></div> : null}
        {tab === 'code' ? <div className="review-inspector-body">{artifacts.length ? <><div className="review-file-tabs" aria-label="Chọn file">{artifacts.map(a => <button key={a.id} aria-pressed={artifact?.id === a.id} onClick={() => setFileId(a.id)}>{a.path}</button>)}</div>{artifact ? <CodePane key={artifact.id} artifact={artifact} /> : null}</> : <p className="review-unavailable">Chưa có code an toàn cho nút này.</p>}</div> : null}
      </> : null}</aside></div>
    <section className="review-execution"><div className="review-execution-heading"><div><h3>{progress ? 'Tiến độ triển khai' : 'Điều gì sẽ xảy ra?'}</h3><p>{progress ? 'Trạng thái được đọc từ bản ghi thực thi của từng thành phần.' : 'Các phụ thuộc sẵn sàng trước, ứng dụng triển khai sau.'}</p></div><span>{progress ? `${running} đang xử lý` : 'Chưa triển khai'}</span></div>
      {progress ? <><div className="review-progress-track" aria-hidden="true"><span style={{ width: `${review.nodes.length ? ready / review.nodes.length * 100 : 0}%` }} /></div><p className="review-progress-caption">{ready}/{review.nodes.length} thành phần sẵn sàng hoặc giữ nguyên</p></> : null}
      {stages.length ? <ol className="review-execution-steps" aria-label="Thứ tự phụ thuộc">{stages.map((stage, index) => <li key={index}><span className="review-step-number">{index + 1}</span><div><strong>{stage.map(nodeLabel).join(' · ')}</strong><small>{stage.every(n => n.kind === 'workload') ? 'Theo điều kiện của ứng dụng' : 'Theo điều kiện của tài nguyên'}</small></div></li>)}</ol> : <p className="review-description">Theo dõi điều kiện sẵn sàng trực tiếp trên sơ đồ.</p>}
    </section>
  </section>;
}
