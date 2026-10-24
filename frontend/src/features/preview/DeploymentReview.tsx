import { useEffect, useMemo, useState, useId, useRef, type ReactNode } from 'react';
import { routeReviewEdges, connectionPath, reviewRowGap } from './reviewEdges';
import { reviewMetrics } from './reviewMetrics';
import { nodeFacts } from './reviewProperties';
import { reasonLabel } from '../../shared/ui/DependencyGraph';
import { t } from '../../shared/i18n';

export type ReviewSide = { availability: 'AVAILABLE' | 'PARTIAL' | 'ABSENT' | 'UNAVAILABLE'; text?: string; reason?: string; provenance?: Record<string, string>; redactedPaths?: string[]; unknownPaths?: string[] };
export type ReviewArtifact = { id: string; nodeId: string; kind: string; path: string; language: string; before: ReviewSide; after: ReviewSide };
export type ReviewNode = { properties?: { key: string; availability: 'AVAILABLE' | 'UNKNOWN'; value?: string; source: string }[]; id: string; kind: string; name: string; resourceType: string; class: string; definitionKey?: string; definitionVersion?: number; account?: string; region?: string; scope?: string; ownership?: string; intent?: string; module?: { module: string; sourceId: string; revision: string; subdirectory: string; contentSha256: string }; placement?: { cluster: string; namespace?: string; account: string; region?: string; source: string; provider?: string }; renderer?: string; rendererDigest?: string };
export type DeploymentReviewData = { version: 1; planHash: string; nodes: ReviewNode[]; edges: { consumer: string; provider: string; reason: string; path?: string }[]; batches: string[][]; artifacts: ReviewArtifact[] };
export type ReviewProgress = { nodeId: string; state: string; status?: string; stage?: string; attempt?: number; startedAt?: string; updatedAt?: string; finishedAt?: string; blockingProviderIds: string[]; safeErrorCode?: string; observedAt: string };

const reason: Record<string, string> = { BASELINE_MISSING: t('preview.deploymentReview.noMatchingAppliedVersionYet'), CONTENT_UNAVAILABLE: t('preview.deploymentReview.noVerifiedModuleContent'), UNSUPPORTED_PREVIEW: t('preview.deploymentReview.thisStructureCannotBePreviewed'), LIMIT_EXCEEDED: t('preview.deploymentReview.contentExceedsTheLimit'), REDACTION_REQUIRED: t('preview.deploymentReview.cannotBeFilteredSafely'), RENDER_UNAVAILABLE: t('preview.deploymentReview.couldNotBuildThePreview') };
const states: Record<string, string> = { PENDING: t('preview.deploymentReview.notStarted'), WAITING: t('preview.deploymentReview.waitingForDependencies'), PROVISIONING: t('preview.deploymentReview.provisioning'), CHECKING: t('preview.deploymentReview.readinessCheck'), READY: t('preview.deploymentReview.ready'), RETAINED: t('preview.deploymentReview.unchanged'), FAILED: t('preview.deploymentReview.failed'), BLOCKED: t('preview.deploymentReview.blocked'), UNKNOWN: t('preview.deploymentReview.undetermined') };
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
    try { if (!navigator.clipboard) throw new Error(); await navigator.clipboard.writeText(after.text ?? ''); setCopyStatus(t('preview.deploymentReview.copied')); }
    catch { setCopyStatus(t('preview.deploymentReview.couldNotCopyAutomaticallySelect')); }
  };
  const both = [before, after].every(side => ['AVAILABLE', 'PARTIAL', 'ABSENT'].includes(side.availability));
  const lines = useMemo(() => both ? diffLines(before.text ?? '', after.text ?? '') : undefined, [both, before.text, after.text]);
  const additions = lines?.filter(line => line.type === 'new').length;
  const removals = lines?.filter(line => line.type === 'old').length;
  return <div ref={codeRef} className={`review-code ${expanded ? 'review-code-expanded' : ''}`}>
    <div className="review-code-toolbar"><div className="review-code-modes"><button aria-pressed={!afterOnly} onClick={() => setAfterOnly(false)}>{t('preview.deploymentReview.viewDiff')}</button><button aria-pressed={afterOnly} onClick={() => setAfterOnly(true)}>{t('preview.deploymentReview.afterChange')}</button></div><button className="review-text-button" aria-label={expanded ? t('preview.deploymentReview.collapse') : t('preview.deploymentReview.expand')} aria-expanded={expanded} onClick={() => {
      if (!expanded) codeRef.current?.requestFullscreen?.().catch(() => undefined);
      else if (document.fullscreenElement) void document.exitFullscreen();
      setExpanded(value => !value);
    }}>{expanded ? t('preview.deploymentReview.collapse') : t('preview.deploymentReview.expand')} ↗</button></div>
    {lines ? <div className="review-diff-summary"><span className="review-added">{t('preview.deploymentReview.additionsLinesAdded', { additions })}</span><span className="review-removed">{t('preview.deploymentReview.removalsLinesRemoved', { removals })}</span></div> : null}
    <div className="review-code-header"><strong>{artifact.path}</strong><button disabled={!after.text} onClick={() => void copy()}>{t('preview.deploymentReview.copyAfterVersion')}</button></div>
    {before.availability === 'UNAVAILABLE' ? <p className="review-unavailable">{t('preview.deploymentReview.beforeText', { text: text(reason, before.reason ?? 'BASELINE_MISSING') })}</p> : null}
    {after.availability === 'UNAVAILABLE' ? <p className="review-unavailable">{t('preview.deploymentReview.afterText', { text: text(reason, after.reason ?? 'CONTENT_UNAVAILABLE') })}</p> : null}
    {after.text || (both && before.text) ? afterOnly || !both || !lines ? <pre className="review-after"><code>{after.text}</code></pre> : <div className="review-diff" role="table" aria-label={`Diff ${artifact.path}`}>{lines.map((line, index) => <div className={`review-diff-line review-diff-${line.type}`} role="row" key={`${index}-${line.type}`}><span role="cell" className="review-gutter">{line.old ?? ''}</span><span role="cell" className="review-gutter">{line.next ?? ''}</span><span role="cell" className="review-sign">{line.type === 'old' ? '−' : line.type === 'new' ? '+' : ' '}</span><code role="cell">{line.value || ' '}</code></div>)}</div> : null}
    {copyStatus ? <p role="status" className="review-code-note">{copyStatus}</p> : null}
    {!afterOnly && both && !lines ? <p className="review-unavailable">{t('preview.deploymentReview.theDiffIsTooLarge')}</p> : null}
    {before.redactedPaths?.length || after.redactedPaths?.length ? <p className="review-code-note">{t('preview.deploymentReview.hiddenValuesCannotBeCompared')}</p> : null}
    {after.unknownPaths?.length ? <p className="review-code-note">{t('preview.deploymentReview.countValuesAreWaitingFor', { count: after.unknownPaths.length })}</p> : null}
    {after.provenance ? <details className="review-technical"><summary>{t('preview.deploymentReview.sourceAndVersion')}</summary><dl className="review-properties">{Object.entries(after.provenance).map(([key, value]) => <div key={key}><dt>{key}</dt><dd>{value}</dd></div>)}</dl></details> : null}
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
  const labels: Record<string, string> = { 'k8s-cluster': node.ownership === 'PREEXISTING' ? t('preview.deploymentReview.internalCluster') : 'Cluster', 'k8s-namespace': 'Namespace', postgres: 'PostgreSQL', redis: 'Redis', vpc: 'VPC' };
  return labels[node.resourceType] ?? node.resourceType;
}
function nodeDescription(node: ReviewNode): string {
  if (node.ownership === 'PREEXISTING') return t('preview.deploymentReview.existingCluster');
  const modules: Record<string, string> = { eks: 'Amazon EKS', aurora: 'Amazon Aurora', elasticache: 'Amazon ElastiCache' };
  if (node.module && modules[node.module.module]) return modules[node.module.module]!;
  const labels: Record<string, string> = { 'k8s-cluster': 'Kubernetes cluster', 'k8s-namespace': 'Kubernetes namespace', postgres: 'Database', redis: 'Cache', vpc: t('preview.deploymentReview.cloudPrivateNetwork') };
  return node.kind === 'workload' ? t('preview.deploymentReview.application') + (node.class || 'default') : labels[node.resourceType] ?? node.class;
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
  return <section className="review-graph" aria-label={t('preview.deploymentReview.deploymentDiagram')}>
    <header className="review-panel-heading"><div><h3>{title}</h3><p>{t('preview.deploymentReview.chooseAComponentToExplore')}</p></div><span className="review-map-badge">{cloudScope ? 'Cloud' : review.nodes.some(n => n.resourceType === 'k8s-cluster') ? 'Kubernetes' : t('preview.deploymentReview.plan')}</span></header>
    <div className="review-graph-controls"><label><input type="checkbox" checked={focusedOnly} onChange={event => setFocusedOnly(event.target.checked)} /> {' '}{t('preview.deploymentReview.onlyTheEdgesOfThe')}</label><span>{t('preview.deploymentReview.scrollToSeeTheWhole')}</span></div>
    <div ref={viewport} className="review-graph-scroll"><div className="review-graph-canvas" style={{ width: width * scale, height: height * scale }}><div className="review-graph-stage" style={{ width, height, transform: `scale(${scale})` }}>
      {cloudScope ? <div className="review-cloud-boundary" style={{ width: width - 16, height: height - 24 }}><strong>{cloudScope}</strong></div> : null}
      {groupIds.map(id => {
        const members = review.nodes.filter(n => groupOf(n) === id); const ys = members.map(n => positions.get(n.id)?.y ?? 58);
        const top = Math.min(...ys) - 24; const bottom = Math.max(...ys) + nodeHeight + 20;
        return <div key={id} className={`review-placement-group ${id === 'outside' ? 'review-placement-outside' : ''}`} style={{ left: starts.get(id), top, width: groupWidth(id), height: bottom - top }} data-cluster={id}><strong>{id === 'outside' ? t('preview.deploymentReview.infrastructureOutsideKubernetes') : id === 'unresolved' ? t('preview.deploymentReview.undeterminedLocation') : nameOf(id)}</strong></div>;
      })}
      <svg className="review-edges" width={width} height={height} aria-label={t('preview.deploymentReview.dependencyEdges')}><defs><marker id={arrowId} markerWidth="7" markerHeight="7" refX="6" refY="3.5" orient="auto"><path d="M0 0L7 3.5L0 7" fill="context-stroke" /></marker></defs>{connections.map(edge => {
        const focus = hovered ?? selected;
        return <path key={JSON.stringify([edge.provider, edge.consumer])} d={connectionPath(edge)} style={{ display: focusedOnly && selected && edge.provider !== selected && edge.consumer !== selected ? 'none' : undefined }} data-provider={edge.provider} data-consumer={edge.consumer} data-focused={focus === edge.provider || focus === edge.consumer} data-outer-lane={edge.indirectLane} markerEnd={`url(#${arrowId})`}><title>{nameOf(edge.provider)} → {nameOf(edge.consumer)}: {[...new Set(edge.reasons.map(reason => reasonLabel(reason.reason)))].join(' · ')}</title></path>;
      })}</svg>
      {review.nodes.map(n => { const p = progress?.find(item => item.nodeId === n.id); const position = positions.get(n.id);
        return <button type="button" key={n.id} style={{ left: position?.x, top: position?.y, width: nodeWidth, height: nodeHeight }} title={n.id} className={`review-node ${selected === n.id ? 'review-node-selected' : ''}`} data-resource-type={n.kind === 'workload' ? 'workload' : n.resourceType} data-state={p?.state ?? 'PENDING'} data-node-id={n.id} onClick={() => select(n.id)} onMouseEnter={() => setHovered(n.id)} onMouseLeave={() => setHovered(undefined)} aria-pressed={selected === n.id}>
          <span className="review-node-icon"><NodeIcon node={n} /></span><strong>{nodeLabel(n)}</strong><small>{nodeDescription(n)}</small><span className="review-node-facts">{(facts.get(n.id) ?? []).slice(0, 2).map(([label, value]) => <span key={label} title={`${label}: ${value}`}><span>{label}</span><strong>{value}</strong></span>)}</span><span className="review-node-bottom"><StateBadge state={p?.state} /><span>{intentLabel(n.intent)}</span></span>
        </button>;
      })}
    </div></div></div>
    <footer className="review-graph-legend"><span>{t('preview.deploymentReview.readinessConditionDependentComponent')}</span><span>{t('preview.deploymentReview.dashedLinkAcrossSeveralLayers')}</span><span>{t('preview.deploymentReview.chooseAComponentToHighlight')}</span><span>{t('preview.deploymentReview.scrollHorizontallyVerticallyToSee')}</span></footer>
  </section>;
}
const intents: Record<string,string> = { CREATE:t('preview.deploymentReview.create'),UPDATE:t('preview.deploymentReview.update'),RETAIN:t('preview.deploymentReview.unchanged'),REMOVE:t('preview.deploymentReview.remove'),UNREFERENCED:t('preview.deploymentReview.removeReference'),BIND:t('preview.deploymentReview.link'),UNVERIFIED:t('preview.deploymentReview.notVerified') };
const intentLabel=(value?:string)=>text(intents,value??'UNVERIFIED');
function ProgressInfo({ value }: { value?: ReviewProgress }) {
  const [now, setNow] = useState(Date.now());
  const active = value && ['PROVISIONING', 'CHECKING'].includes(value.state);
  useEffect(() => { if (!active) return; const timer = window.setInterval(() => setNow(Date.now()), 1000); return () => window.clearInterval(timer); }, [active]);
  const start = value?.startedAt ? Date.parse(value.startedAt) : NaN;
  const end = value?.finishedAt ? Date.parse(value.finishedAt) : active ? now : value?.updatedAt ? Date.parse(value.updatedAt) : NaN;
  const seconds = Number.isFinite(start) && Number.isFinite(end) ? Math.max(0, Math.floor((end - start) / 1000)) : undefined;
  const descriptions: Record<string, string> = { PENDING: t('preview.deploymentReview.noExecutionStartRecordYet'), WAITING: t('preview.deploymentReview.waitingForDependentComponentsTo'), PROVISIONING: t('preview.deploymentReview.theComponentIsBeingProvisioned'), CHECKING: t('preview.deploymentReview.checkingReadinessConditions'), READY: t('preview.deploymentReview.readinessConfirmedInTheExecution'), RETAINED: t('preview.deploymentReview.keepsTheResourcesVerifiedIn'), FAILED: t('preview.deploymentReview.executionNotCompletedSeeThe'), BLOCKED: t('preview.deploymentReview.aFailedDependencyIsBlocking'), UNKNOWN: t('preview.deploymentReview.notEnoughDataToDetermine') };
  return <section className="review-runtime-card" data-state={value?.state ?? 'PENDING'}><div><strong>{t('preview.deploymentReview.provisioningStatus')}</strong><StateBadge state={value?.state} /></div><p>{descriptions[value?.state ?? 'PENDING'] ?? descriptions.UNKNOWN}</p><dl><div><dt>{t('preview.deploymentReview.executionTime')}</dt><dd>{seconds === undefined ? t('preview.deploymentReview.noTimeYet') : t('preview.deploymentReview.floorMinValueS', { floor: Math.floor(seconds / 60), value: seconds % 60 })}</dd></div><div><dt>{t('preview.deploymentReview.currentStep')}</dt><dd>{value?.stage || (value ? text(states, value.state) : t('preview.deploymentReview.notStarted'))}{value?.attempt ? t('preview.deploymentReview.attemptAttempt', { attempt: value.attempt }) : ''}</dd></div></dl></section>;
}

function ArtifactChange({artifact}:{artifact:ReviewArtifact}) {
  const before=artifact.before,after=artifact.after;
  const safeBefore=before.availability==='ABSENT'||before.text!==undefined;
  const safeAfter=after.availability==='ABSENT'||after.text!==undefined;
  const lines=safeBefore&&safeAfter?diffLines(before.text??'',after.text??''):undefined;
  const additions=lines?.filter(l=>l.type==='new').length;
  const removals=lines?.filter(l=>l.type==='old').length;
  return <li className="review-change-card"><strong>{artifact.path}</strong>{lines?<span><em className="review-added">{t('preview.deploymentReview.additionsLinesAdded', { additions })}</em> · <em className="review-removed">{t('preview.deploymentReview.removalsLinesRemoved', { removals })}</em></span>:<span>{text(reason,before.reason??after.reason??'BASELINE_MISSING')}</span>}
    {lines?.some(line => line.type === 'old') ? <div className="review-change-before"><small>{t('preview.deploymentReview.beforeChange')}</small><code>{lines.filter(line => line.type === 'old').slice(0, 3).map(line => line.value).join('\n')}</code></div> : null}
    {lines?.some(line => line.type === 'new') ? <div className="review-change-after"><small>{t('preview.deploymentReview.afterChange')}</small><code>{lines.filter(line => line.type === 'new').slice(0, 3).map(line => line.value).join('\n')}</code></div> : null}
    {before.text===after.text&&lines?<small>{t('preview.deploymentReview.theDisplayedContentIsUnchanged')}</small>:null}
    {before.redactedPaths?.length||after.redactedPaths?.length||before.unknownPaths?.length||after.unknownPaths?.length?<small>{t('preview.deploymentReview.hiddenMissingValuesCannotBe')}</small>:null}
  </li>;
}
export function DeploymentReview({ review, progress, title = t('preview.deploymentReview.deploymentMap') }: { review?: DeploymentReviewData; progress?: ReviewProgress[]; title?: string }) {
  // No-change/legacy responses may carry the Go zero value (version 0 and
  // null collections). Check the wire shape before reading any collection.
  if (!review || review.version !== 1 || !Array.isArray(review.nodes) || !Array.isArray(review.edges) || !Array.isArray(review.artifacts) || !Array.isArray(review.batches)) {
    return <p className="review-unavailable">{t('preview.deploymentReview.theReviewOfThisPlan')}</p>;
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
  const properties = node ? nodeFacts(node, artifacts).filter(([label]) => ![t('preview.deploymentReview.account'), t('preview.deploymentReview.region'), 'Module'].includes(label)) : [];
  const selectNode = (id: string) => { setSelected(id); setTab('overview'); setFileId(''); };
  const scopeNames = [...new Set(review.nodes.flatMap(n => { const account = n.placement?.account || n.account; const region = n.placement?.region || n.region; return account && region ? [account + ' · ' + region] : []; }))];
  const totals = [{ intents: ['CREATE'], label: t('preview.deploymentReview.create2'), tone: 'create' }, { intents: ['UPDATE'], label: t('preview.deploymentReview.update2'), tone: 'update' }, { intents: ['RETAIN', 'BIND'], label: t('preview.deploymentReview.unchangedLink'), tone: 'retain' }, { intents: ['REMOVE', 'UNREFERENCED'], label: t('preview.deploymentReview.removeReference2'), tone: 'remove' }];
  const stages = levelsOf(review);
  return <section className="deployment-review" aria-label={title}>
    <div className="review-summary"><div className="review-summary-title"><span className="review-summary-icon" aria-hidden="true">✓</span><div><strong>{t('preview.deploymentReview.countComponentsInThePlan', { count: review.nodes.length })}</strong><small>{scopeNames.join(' · ') || t('preview.deploymentReview.deploymentTargetPerResourceDefinition')}</small></div></div><div className="review-intent-counts">{totals.map(total => { const count = review.nodes.filter(n => total.intents.includes(n.intent ?? '')).length; return count ? <span key={total.tone} data-intent={total.tone}><i />{count} {total.label}</span> : null; })}</div></div>
    <div className="review-layout"><ReviewGraph review={review} progress={progress} selected={node?.id} title={title} select={selectNode} />
      <aside className="review-inspector" aria-label={t('preview.deploymentReview.componentDetails')}>{node ? <><header className="review-detail-heading" data-resource-type={node.kind === 'workload' ? 'workload' : node.resourceType}><span className="review-detail-icon"><NodeIcon node={node} /></span><div><h3 title={node.id}>{nodeLabel(node)}</h3><p>{nodeDescription(node)}</p></div><StateBadge state={state?.state} /></header>
        <div className="review-tabs" role="tablist" aria-label={t('preview.deploymentReview.componentDetails')}>{(['overview', 'changes', 'code'] as const).map((key, index) => <button key={key} ref={el => { tabs.current[index] = el; }} role="tab" tabIndex={tab === key ? 0 : -1} aria-selected={tab === key} onKeyDown={event => {
          if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
          event.preventDefault(); const next = event.key === 'Home' ? 0 : event.key === 'End' ? 2 : (index + (event.key === 'ArrowRight' ? 1 : -1) + 3) % 3;
          setTab((['overview', 'changes', 'code'] as const)[next] ?? 'overview'); tabs.current[next]?.focus();
        }} onClick={() => setTab(key)}>{key === 'overview' ? t('preview.deploymentReview.overview') : key === 'changes' ? <>{t('preview.deploymentReview.changes')}{' '} <span className="review-tab-count">{artifacts.length}</span></> : 'Code'}</button>)}</div>
        {tab === 'overview' ? <div className="review-inspector-body"><ProgressInfo value={state} /><h4>{t('preview.deploymentReview.expectedConfiguration')}</h4><dl className="review-properties">
          {properties.map(([label, value]) => <div key={label}><dt>{label}</dt><dd title={value}>{value}</dd></div>)}
          <div><dt>{t('preview.deploymentReview.deploymentLocation')}</dt><dd>{node.placement?.cluster ? nodeLabel(review.nodes.find(n => n.id === node.placement?.cluster) ?? node) : node.resourceType === 'k8s-cluster' ? nodeLabel(node) : t('preview.deploymentReview.outsideKubernetes')}</dd></div>
          {node.account || node.placement?.account ? <div><dt>{t('preview.deploymentReview.accountRegion')}</dt><dd>{node.placement?.account || node.account}{node.placement?.region || node.region ? ` / ${node.placement?.region || node.region}` : ''}</dd></div> : null}
          <div><dt>Module / renderer</dt><dd title={node.module?.revision || node.renderer}>{node.module?.module || node.renderer?.split('@')[0] || t('preview.deploymentReview.existingLink')}</dd></div>
          <div><dt>{t('preview.deploymentReview.expectedChanges')}</dt><dd><span className="review-intent-badge" data-intent={node.intent}>{intentLabel(node.intent)}</span></dd></div>
        </dl><h4>{t('preview.deploymentReview.readinessCondition')}</h4>{dependencies.length ? <div className="review-dependencies">{[...dependenciesByProvider].map(([id, edges]) => {
          const provider = review.nodes.find(n => n.id === id); const observation = progress?.find(p => p.nodeId === id);
          const reasons = [...new Set(edges.map(edge => reasonLabel(edge.reason)))].join(' · ');
          return <button key={id} className="review-dependency" title={reasons} onClick={() => selectNode(id)}><span>{provider ? nodeLabel(provider) : id}<small>{reasons}</small></span><span>{observation ? text(states, observation.state) : t('preview.deploymentReview.notStarted')} →</span></button>;
        })}</div> : <p className="review-description">{t('preview.deploymentReview.noDependenciesInThePlan')}</p>}
        {state?.blockingProviderIds.length ? <p className="review-unavailable">{t('preview.deploymentReview.waitingForValue', { value: state.blockingProviderIds.map(id => { const provider = review.nodes.find(n => n.id === id); return provider ? nodeLabel(provider) : id; }).join(', ') })}</p> : null}
        {state?.safeErrorCode ? <p className="review-unavailable">{t('preview.deploymentReview.errorSafeErrorCode', { safeErrorCode: state.safeErrorCode })}</p> : null}
        {artifacts.length ? <button className="review-code-link" onClick={() => setTab('code')}>{t('preview.deploymentReview.viewValue', { value: node.kind === 'workload' ? 'Kubernetes manifest' : 'Terraform source' })}</button> : null}
        <details className="review-technical"><summary>{t('preview.deploymentReview.technicalInformation')}</summary><dl className="review-properties"><div><dt>ID</dt><dd>{node.id}</dd></div><div><dt>Definition</dt><dd>{node.definitionKey || '—'} {node.definitionVersion ? `v${node.definitionVersion}` : ''}</dd></div><div><dt>Cluster / namespace</dt><dd>{node.placement?.cluster || '—'} / {node.placement?.namespace || t('preview.deploymentReview.waitingForProvisioning')}</dd></div><div><dt>{t('preview.deploymentReview.version')}</dt><dd>{node.module?.revision || node.renderer || '—'}</dd></div></dl></details></div> : null}
        {tab === 'changes' ? <div className="review-inspector-body"><p className="review-description">{t('preview.deploymentReview.intentLabelComparesTheDisplayableContent', { intentLabel: intentLabel(node.intent) })}</p><ul className="review-change-list">{artifacts.map(a => <ArtifactChange key={a.id} artifact={a} />)}</ul>{!artifacts.length ? <p>{t('preview.deploymentReview.noSafeArtifactForThis')}</p> : null}<p className="review-code-note">{t('preview.deploymentReview.thisIsTheExpectedConfiguration')}</p></div> : null}
        {tab === 'code' ? <div className="review-inspector-body">{artifacts.length ? <><div className="review-file-tabs" aria-label={t('preview.deploymentReview.chooseFile')}>{artifacts.map(a => <button key={a.id} aria-pressed={artifact?.id === a.id} onClick={() => setFileId(a.id)}>{a.path}</button>)}</div>{artifact ? <CodePane key={artifact.id} artifact={artifact} /> : null}</> : <p className="review-unavailable">{t('preview.deploymentReview.noSafeCodeForThis')}</p>}</div> : null}
      </> : null}</aside></div>
    <section className="review-execution"><div className="review-execution-heading"><div><h3>{progress ? t('preview.deploymentReview.deploymentProgress') : t('preview.deploymentReview.whatWillHappen')}</h3><p>{progress ? t('preview.deploymentReview.statusIsReadFromThe') : t('preview.deploymentReview.dependenciesBecomeReadyFirstThen')}</p></div><span>{progress ? t('preview.deploymentReview.runningInProgress', { running }) : t('preview.deploymentReview.notDeployed')}</span></div>
      {progress ? <><div className="review-progress-track" aria-hidden="true"><span style={{ width: `${review.nodes.length ? ready / review.nodes.length * 100 : 0}%` }} /></div><p className="review-progress-caption">{t('preview.deploymentReview.readyCountComponentsReadyOr', { ready, count: review.nodes.length })}</p></> : null}
      {stages.length ? <ol className="review-execution-steps" aria-label={t('preview.deploymentReview.dependencyOrder')}>{stages.map((stage, index) => <li key={index}><span className="review-step-number">{index + 1}</span><div><strong>{stage.map(nodeLabel).join(' · ')}</strong><small>{stage.every(n => n.kind === 'workload') ? t('preview.deploymentReview.byApplicationCondition') : t('preview.deploymentReview.byResourceCondition')}</small></div></li>)}</ol> : <p className="review-description">{t('preview.deploymentReview.trackReadinessConditionsDirectlyOn')}</p>}
    </section>
  </section>;
}
