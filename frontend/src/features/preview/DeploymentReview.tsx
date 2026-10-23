import { useEffect, useMemo, useState, useId, useRef } from 'react';
import { reasonLabel } from '../../shared/ui/DependencyGraph';
import { Button } from '../../shared/ui/Button';

export type ReviewSide = { availability: 'AVAILABLE' | 'PARTIAL' | 'ABSENT' | 'UNAVAILABLE'; text?: string; reason?: string; provenance?: Record<string, string>; redactedPaths?: string[]; unknownPaths?: string[] };
export type ReviewArtifact = { id: string; nodeId: string; kind: string; path: string; language: string; before: ReviewSide; after: ReviewSide };
export type ReviewNode = { id: string; kind: string; name: string; resourceType: string; class: string; definitionKey?: string; definitionVersion?: number; account?: string; region?: string; scope?: string; ownership?: string; intent?: string; module?: { module: string; sourceId: string; revision: string; subdirectory: string; contentSha256: string }; placement?: { cluster: string; namespace?: string; account: string; region?: string; source: string; provider?: string }; renderer?: string; rendererDigest?: string };
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
  useEffect(() => { const close = (event:KeyboardEvent) => { if (event.key === 'Escape') setExpanded(false); }; window.addEventListener('keydown',close); return () => window.removeEventListener('keydown',close); }, []);
  const copy = async () => { try { if (!navigator.clipboard) throw new Error(); await navigator.clipboard.writeText(after.text ?? ''); setCopyStatus('Đã sao chép'); } catch { setCopyStatus('Không thể sao chép tự động; hãy chọn code và nhấn Ctrl+C.'); } };
  const before = artifact.before; const after = artifact.after;
  const both = (before.availability === 'AVAILABLE' || before.availability === 'PARTIAL' || before.availability === 'ABSENT') && (after.availability === 'AVAILABLE' || after.availability === 'PARTIAL' || after.availability === 'ABSENT');
  const lines = useMemo(() => both ? diffLines(before.text ?? '', after.text ?? '') : undefined, [both, before.text, after.text]);
  return <div ref={codeRef} className={`review-code ${expanded ? 'review-code-expanded' : ''}`}>
    <div className="review-code-tools"><strong>{artifact.path}</strong><span>{artifact.kind} · {artifact.language}</span><Button tone="quiet" onClick={() => setAfterOnly((v) => !v)}>{afterOnly ? 'Xem diff' : 'Sau thay đổi'}</Button><Button tone="quiet" aria-expanded={expanded} onClick={() => { if (!expanded) { codeRef.current?.requestFullscreen?.().catch(() => undefined); } else if (document.fullscreenElement) { void document.exitFullscreen(); } setExpanded((v) => !v); }}>{expanded ? 'Thu gọn' : 'Mở rộng'}</Button><Button tone="quiet" disabled={!after.text} onClick={() => void copy()}>Sao chép bản sau</Button></div>
    {copyStatus ? <p role="status">{copyStatus}</p> : null}
    {before.availability === 'UNAVAILABLE' ? <p className="review-unavailable">Bản trước: {text(reason, before.reason ?? 'BASELINE_MISSING')}</p> : null}
    {after.availability === 'UNAVAILABLE' ? <p className="review-unavailable">Bản sau: {text(reason, after.reason ?? 'CONTENT_UNAVAILABLE')}</p> : null}
    {before.redactedPaths?.length ? <p className="review-unavailable">Bản trước có giá trị đã ẩn; không thể kết luận các giá trị ẩn không đổi.</p> : null}
    {after.redactedPaths?.length ? <p className="review-unavailable">Giá trị tại {after.redactedPaths.length} vị trí đã ẩn; thay đổi giá trị chưa xác định.</p> : null}
    {after.unknownPaths?.length ? <p className="review-unavailable">{after.unknownPaths.length} giá trị đang chờ đầu ra tài nguyên.</p> : null}
    {after.text || (both && before.text) ? afterOnly || !both || !lines ? <pre className="review-after"><code>{after.text}</code></pre> : <div className="review-diff" role="table" aria-label={`Diff ${artifact.path}`}>{lines.map((line, index) => <div className={`review-diff-line review-diff-${line.type}`} role="row" key={`${index}-${line.type}`}><span role="cell" className="review-gutter">{line.old ?? ''}</span><span role="cell" className="review-gutter">{line.next ?? ''}</span><span role="cell" className="review-sign">{line.type === 'old' ? '−' : line.type === 'new' ? '+' : ' '}</span><code role="cell">{line.value || ' '}</code></div>)}</div> : null}
    {!afterOnly && both && !lines ? <p className="review-unavailable">Diff quá lớn để hiển thị; bản sau vẫn có thể xem và sao chép.</p> : null}
    {after.provenance ? <p className="review-provenance">Nguồn: {Object.entries(after.provenance).map(([key, value]) => `${key}: ${value}`).join(' · ')}</p> : null}
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
  const labels: Record<string, string> = { 'k8s-cluster': 'Cluster', 'k8s-namespace': 'Namespace', postgres: 'PostgreSQL', redis: 'Redis', vpc: 'VPC' };
  return `${labels[node.resourceType] ?? node.resourceType} · ${node.class || 'default'}`;
}
type Position = { x: number; y: number };
function ReviewGraph({review, progress, selected, select, title}: { review: DeploymentReviewData; progress?: ReviewProgress[]; selected?: string; select(id:string):void; title:string }) {
  const arrowId = useId().replace(/:/g, '');
  const layers = useMemo(() => levelsOf(review), [review]);
  const groupOf = (node:ReviewNode) => node.placement?.cluster || (node.resourceType === 'k8s-cluster' ? node.id : 'outside');
  const groupIds = [...new Set(review.nodes.map(groupOf))].sort((a,b)=>a==='outside'?1:b==='outside'?-1:a.localeCompare(b));
  const groupWidth = (id:string) => Math.max(1,...layers.map(layer=>layer.filter(n=>groupOf(n)===id).length))*240+28;
  const starts=new Map<string,number>();let cursor=24;for(const id of groupIds){starts.set(id,cursor);cursor+=groupWidth(id)+20;}
  const positions=new Map<string,Position>();
  layers.forEach((layer,row)=>groupIds.forEach(id=>{const items=layer.filter(n=>groupOf(n)===id);items.forEach((n,col)=>positions.set(n.id,{x:(starts.get(id)??24)+(groupWidth(id)-items.length*240)/2+col*240,y:84+row*146}));}));
  const width=Math.max(620,cursor);const height=Math.max(350,layers.length*146+100);
  const nameOf=(id:string)=>{ const item=review.nodes.find(n=>n.id===id); return item ? nodeLabel(item) : id; };
  return <div className="review-graph"><h3>{title}</h3><p>Điều kiện sẵn sàng → thành phần phụ thuộc · {review.edges.length} liên kết</p>
    <div className="review-graph-scroll"><div className="review-graph-canvas" style={{width,height}}>
      {groupIds.map(id=>{const members=review.nodes.filter(n=>groupOf(n)===id);const ys=members.map(n=>positions.get(n.id)?.y??84);const top=Math.min(...ys)-34;const bottom=Math.max(...ys)+112;
        return <div key={id} className={`review-placement-group ${id==='outside'?'review-placement-outside':''}`} style={{left:starts.get(id),top,width:groupWidth(id),height:bottom-top}} data-cluster={id}><strong>{id==='outside'?'Hạ tầng ngoài Kubernetes':nameOf(id)}</strong></div>;})}
      <svg className="review-edges" width={width} height={height} aria-label="Các cạnh phụ thuộc"><defs><marker id={arrowId} markerWidth="7" markerHeight="7" refX="6" refY="3.5" orient="auto"><path d="M0 0L7 3.5L0 7" fill="#8c959f"/></marker></defs>{review.edges.map((edge,index)=>{const provider=positions.get(edge.provider),consumer=positions.get(edge.consumer);if(!provider||!consumer)return null;const x1=provider.x+110,y1=provider.y+100,x2=consumer.x+110,y2=consumer.y;return <path key={index} d={`M${x1},${y1} C${x1},${(y1+y2)/2} ${x2},${(y1+y2)/2} ${x2},${y2-3}`} markerEnd={`url(#${arrowId})`}><title>{nameOf(edge.provider)} → {nameOf(edge.consumer)}: {reasonLabel(edge.reason)}</title></path>;})}</svg>
      {review.nodes.map(n=>{const p=progress?.find(item=>item.nodeId===n.id);const position=positions.get(n.id);return <button type="button" key={n.id} style={{left:position?.x,top:position?.y}} title={n.id} className={`review-node ${selected===n.id?'review-node-selected':''}`} data-state={p?.state??'PENDING'} data-node-id={n.id} onClick={()=>select(n.id)} aria-pressed={selected===n.id}><strong>{nodeLabel(n)}</strong><small>{n.resourceType||n.kind} · {n.class||'default'}</small><span className="review-node-state">{p?text(states,p.state):'Chưa chạy'}</span><span className="review-node-intent">{intentLabel(n.intent)}</span></button>;})}
    </div></div>
  </div>;
}
const intents: Record<string,string> = { CREATE:'Tạo mới',UPDATE:'Cập nhật',RETAIN:'Giữ nguyên',REMOVE:'Gỡ bỏ',UNREFERENCED:'Gỡ tham chiếu',BIND:'Liên kết',UNVERIFIED:'Chưa xác minh' };
const intentLabel=(value?:string)=>text(intents,value??'UNVERIFIED');
function ProgressInfo({value}: {value:ReviewProgress}) {
  const [now,setNow]=useState(Date.now());
  const active=['PROVISIONING','CHECKING'].includes(value.state);
  useEffect(()=>{if(!active)return;const timer=window.setInterval(()=>setNow(Date.now()),1000);return()=>window.clearInterval(timer);},[active]);
  const start=value.startedAt?Date.parse(value.startedAt):NaN;
  const end=value.finishedAt?Date.parse(value.finishedAt):active?now:value.updatedAt?Date.parse(value.updatedAt):NaN;
  const seconds=Number.isFinite(start)&&Number.isFinite(end)?Math.max(0,Math.floor((end-start)/1000)):undefined;
  return <><dt>Bước / lần thử</dt><dd>{value.stage||value.status||'—'} {value.attempt?`#${value.attempt}`:''}</dd><dt>Thời gian</dt><dd>{seconds===undefined?'Chưa có timestamp bắt đầu':`${Math.floor(seconds/60)} phút ${seconds%60} giây`}</dd><dt>Cập nhật</dt><dd>{value.updatedAt?new Date(value.updatedAt).toLocaleString('vi-VN'):'Chưa có'}</dd></>;
}
function ArtifactChange({artifact}:{artifact:ReviewArtifact}) {
  const before=artifact.before,after=artifact.after;
  const safeBefore=before.availability==='ABSENT'||before.text!==undefined;
  const safeAfter=after.availability==='ABSENT'||after.text!==undefined;
  const lines=safeBefore&&safeAfter?diffLines(before.text??'',after.text??''):undefined;
  const additions=lines?.filter(l=>l.type==='new').length;
  const removals=lines?.filter(l=>l.type==='old').length;
  return <li className="review-change-card"><strong>{artifact.path}</strong>{lines?<span><em className="review-added">+{additions} dòng thêm</em> · <em className="review-removed">−{removals} dòng bỏ</em></span>:<span>{text(reason,before.reason??after.reason??'BASELINE_MISSING')}</span>}
    {before.text===after.text&&lines?<small>Phần nội dung hiển thị không đổi.</small>:null}
    {before.redactedPaths?.length||after.redactedPaths?.length||before.unknownPaths?.length||after.unknownPaths?.length?<small>Giá trị đã ẩn / chưa có chưa thể đối chiếu.</small>:null}
  </li>;
}
export function DeploymentReview({ review, progress, title = 'Bản đồ triển khai' }: { review?: DeploymentReviewData; progress?: ReviewProgress[]; title?: string }) {
  const [selected, setSelected] = useState(''); const [tab, setTab] = useState<'overview' | 'changes' | 'code'>('overview');
  const tabs=useRef<Array<HTMLButtonElement|null>>([]);
  const firstNodeId = review?.nodes[0]?.id;
  useEffect(() => { setSelected(firstNodeId ?? ''); setTab('overview'); }, [review?.planHash, firstNodeId]);
  const node = review?.nodes.find((n) => n.id === selected) ?? review?.nodes[0];
  const state = progress?.find((p) => p.nodeId === node?.id);
  const artifacts = review?.artifacts.filter((a) => a.nodeId === node?.id) ?? [];
  const dependencies = review?.edges.filter((e) => e.consumer === node?.id) ?? [];
  const ready = review?.nodes.filter(n=>progress?.some(p=>p.nodeId===n.id&&['READY','RETAINED'].includes(p.state))).length??0;
  if (!review || review.version !== 1) return <p className="review-unavailable">Bản review của kế hoạch này chưa có.</p>;
  return <section className="deployment-review" aria-label={title}>
    <div className="review-summary"><strong>{review.nodes.length} thành phần trong kế hoạch</strong><span>{progress ? `${ready}/${review.nodes.length} sẵn sàng hoặc giữ nguyên` : 'Chưa triển khai'}</span></div>
    <div className="review-layout"><ReviewGraph review={review} progress={progress} selected={node?.id} title={title} select={id=>{setSelected(id);setTab('overview');}}/>
      <div className="review-inspector">{node ? <><header><h3 title={node.id}>{nodeLabel(node)}</h3><p>{node.resourceType} · {node.class} · {node.definitionKey || node.renderer || '—'}</p></header><div className="review-tabs" role="tablist" aria-label="Chi tiết thành phần">{(['overview','changes','code'] as const).map((key,index)=><button key={key} ref={el=>{tabs.current[index]=el;}} role="tab" tabIndex={tab===key?0:-1} aria-selected={tab===key} onKeyDown={event=>{if(!['ArrowLeft','ArrowRight','Home','End'].includes(event.key))return;event.preventDefault();const next=event.key==='Home'?0:event.key==='End'?2:(index+(event.key==='ArrowRight'?1:-1)+3)%3;setTab((['overview','changes','code'] as const)[next]??'overview');tabs.current[next]?.focus();}} onClick={()=>setTab(key)}>{key==='overview'?'Tổng quan':key==='changes'?'Thay đổi':'Code'}</button>)}</div>
      {tab === 'overview' ? <div className="review-inspector-body"><dl><dt>Trạng thái</dt><dd>{state ? text(states, state.state) : 'Chưa chạy'}</dd><dt>Ý định</dt><dd>{intentLabel(node.intent)}</dd><dt>Definition</dt><dd>{node.definitionKey || '—'} {node.definitionVersion ? `v${node.definitionVersion}` : ''}</dd><dt>Tài khoản / vùng</dt><dd>{node.placement?.account || node.account || 'Chưa có'} / {node.placement?.region || node.region || '—'}</dd><dt>Cluster / namespace</dt><dd>{node.placement?.cluster || (node.resourceType==='k8s-cluster'?node.name:'Không thuộc Kubernetes')} / {node.placement?.namespace || (node.placement?'Chờ cấp phát / xem nguồn đích':'—')}</dd><dt>Module / renderer</dt><dd>{node.module ? `${node.module.module}@${node.module.revision}` : node.renderer || '—'}</dd>{state?<ProgressInfo value={state}/>:null}</dl><h4>Điều kiện sẵn sàng</h4>{dependencies.length ? dependencies.map((edge,index) => <div key={`${edge.provider}-${index}`}><button className="review-dependency" onClick={() => { setSelected(edge.provider); setTab('overview'); }}>{(() => { const provider = review.nodes.find((n) => n.id === edge.provider); return provider ? nodeLabel(provider) : edge.provider; })()} →</button><small>{reasonLabel(edge.reason)}{progress?.find(p=>p.nodeId===edge.provider)?` · ${text(states,progress.find(p=>p.nodeId===edge.provider)?.state??'UNKNOWN')}`:''}</small></div>) : <p>Không có phụ thuộc trong kế hoạch.</p>}{state?.blockingProviderIds.length ? <p>Đang chờ: {state.blockingProviderIds.join(', ')}</p> : null}{state?.safeErrorCode ? <p>Lỗi: {state.safeErrorCode}</p> : null}</div> : null}
      {tab === 'changes' ? <div className="review-inspector-body"><p>{intentLabel(node.intent)}; đây không phải Terraform plan hay kết quả provider.</p><ul className="review-change-list">{artifacts.map(artifact=><ArtifactChange key={artifact.id} artifact={artifact}/>)}</ul>{!artifacts.length ? <p>Chưa có artifact an toàn cho nút này.</p> : null}</div> : null}
      {tab === 'code' ? <div className="review-inspector-body">{artifacts.length ? artifacts.map((artifact) => <CodePane key={artifact.id} artifact={artifact} />) : <p className="review-unavailable">Chưa có code an toàn cho nút này.</p>}</div> : null}</> : null}</div></div>
  </section>;
}
