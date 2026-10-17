import { useId, useState } from 'react';
import { Button } from '../../shared/ui/Button';
import { MatchingPreviewPanel, type MatchingCandidateInput } from './MatchingPreviewPanel';

// Shared "Điều kiện áp dụng" editor (UC-03 T02B, V02-02). It edits the five-field
// Matching Criteria contract and serializes it unchanged; modes are only a
// friendlier projection. There is no profile, account or region predicate and
// the browser never ranks Definitions: only the server matching preview names a
// winner. The draft is controlled by the owner so the editor can serve Resource
// Definition and renderer forms alike.

export type CriterionField = 'env_type' | 'app_id' | 'env_id' | 'res_id' | 'class';
export type Criterion = Record<CriterionField, string>;
export type CriteriaMode = 'all' | 'env_type' | 'app_env' | 'advanced';
export type CriteriaDraft = { mode: CriteriaMode; rows: Criterion[] };
export type EnvironmentOption = { key: string; name: string; environmentType: string; /** Opaque non-secret target/version fingerprint of the Environment DTO; a change invalidates exact previews. */ snapshotKey?: string };
export type ApplicationOption = { key: string; name: string; environments: EnvironmentOption[] };
// Public Definition metadata that decides a matching result (never secret material); the whole record is part of the preview signature.
export type CriteriaSource = { key: string; name?: string; version?: number; resourceType: string; driverType?: string; driverAccount?: string; driverInputs?: unknown; criteria?: Partial<Criterion>[] };
export type ListState<T> = { items: readonly T[]; loading: boolean; error: string; onRetry(): void };

const fields: { key: CriterionField; label: string }[] = [
  { key: 'env_type', label: 'Loại môi trường' }, { key: 'app_id', label: 'ID ứng dụng' },
  { key: 'env_id', label: 'ID môi trường' }, { key: 'res_id', label: 'ID tài nguyên' }, { key: 'class', label: 'Class' },
];
const modes: { key: CriteriaMode; label: string; hint: string }[] = [
  { key: 'all', label: 'Mọi nơi', hint: 'Áp dụng cho mọi ứng dụng và môi trường (điều kiện rỗng).' },
  { key: 'env_type', label: 'Theo loại môi trường', hint: 'Chỉ áp dụng cho môi trường thuộc một loại (env_type).' },
  { key: 'app_env', label: 'Theo ứng dụng (+ môi trường)', hint: 'Áp dụng cho một ứng dụng, có thể giới hạn một môi trường của ứng dụng đó.' },
  { key: 'advanced', label: 'Tùy chỉnh nâng cao', hint: 'Năm field chuẩn, nhiều dòng; các dòng là lựa chọn thay thế nhau.' },
];
const modeLabel = (mode: CriteriaMode) => modes.find((item) => item.key === mode)?.label ?? mode;

export const emptyCriterion = (): Criterion => ({ env_type: '', app_id: '', env_id: '', res_id: '', class: '' });
export const emptyDraft = (): CriteriaDraft => ({ mode: 'all', rows: [emptyCriterion()] });
const isBlank = (row: Criterion) => fields.every(({ key }) => row[key].trim() === '');
const hasData = (rows: Criterion[]) => rows.some((row) => !isBlank(row));
const only = (row: Criterion, allowed: CriterionField[]) => fields.every(({ key }) => allowed.includes(key) || row[key].trim() === '');
const cloneRows = (rows: Criterion[]) => rows.map((row) => ({ ...row }));

// Missing fields of an API criterion are empty; the result never aliases the source.
// Only the five known own fields are read: no prototype keys, no unknown strings.
function normalize(rows: Partial<Criterion>[]): Criterion[] {
  return rows.map((row) => {
    const next = emptyCriterion();
    for (const { key } of fields) { const value = Object.hasOwn(row, key) ? row[key] : undefined; if (typeof value === 'string') next[key] = value; }
    return next;
  });
}

// The simplest mode that shows these rows without losing a field or a row:
// rows are alternatives (OR), so only exactly one row can be simple.
function classify(rows: Criterion[]): CriteriaMode {
  const [row] = rows;
  if (rows.length !== 1 || !row) return 'advanced';
  if (isBlank(row)) return 'all';
  if (row.env_type.trim() !== '' && only(row, ['env_type'])) return 'env_type';
  if (row.app_id.trim() !== '' && only(row, ['app_id', 'env_id'])) return 'app_env';
  return 'advanced';
}

// A missing list is a wildcard draft; an empty list stays zero advanced rows, never rewritten to a wildcard.
export function draftFromCriteria(criteria: Partial<Criterion>[] | undefined): CriteriaDraft {
  if (criteria === undefined) return emptyDraft();
  const rows = normalize(criteria);
  const mode = classify(rows);
  return { mode, rows: mode === 'all' ? [emptyCriterion()] : rows };
}

// Rows for a target mode. Simple modes keep only the fields they can show.
function convert(rows: Criterion[], mode: CriteriaMode): Criterion[] {
  const first = rows[0] ?? emptyCriterion();
  if (mode === 'advanced') return cloneRows(rows);
  if (mode === 'env_type') return [{ ...emptyCriterion(), env_type: first.env_type }];
  if (mode === 'app_env') return [{ ...emptyCriterion(), app_id: first.app_id, env_id: first.env_id }];
  return [emptyCriterion()];
}

// Leaving advanced is free only when exactly one row maps onto the target without dropping a field.
function lossless(rows: Criterion[], mode: CriteriaMode): boolean {
  const [row] = rows;
  if (rows.length !== 1 || !row) return false;
  return isBlank(row) || classify(rows) === mode;
}

// Wire payload: wildcard is [{}], empty fields are dropped; an incomplete simple mode is a problem, never a silent wildcard.
export function draftToCriteria(draft: CriteriaDraft): { criteria: Record<string, string>[]; problem: string } {
  const clean = (rows: Criterion[]) => rows.map((row) => Object.fromEntries(Object.entries(row).filter(([, value]) => value.trim() !== '')) as Record<string, string>);
  const [row = emptyCriterion()] = draft.rows;
  if (draft.mode === 'all') return { criteria: [{}], problem: '' };
  if (draft.mode === 'env_type') return row.env_type.trim() === '' ? { criteria: [], problem: 'Nhập loại môi trường cho điều kiện áp dụng, hoặc chọn “Mọi nơi”.' } : { criteria: clean([{ ...emptyCriterion(), env_type: row.env_type }]), problem: '' };
  if (draft.mode === 'app_env') return row.app_id.trim() === '' ? { criteria: [], problem: 'Chọn ứng dụng cho điều kiện áp dụng, hoặc chọn “Mọi nơi”.' } : { criteria: clean([{ ...emptyCriterion(), app_id: row.app_id, env_id: row.env_id }]), problem: '' };
  return draft.rows.length === 0 ? { criteria: [], problem: 'Thêm ít nhất một điều kiện, hoặc chọn “Mọi nơi”.' } : { criteria: clean(draft.rows), problem: '' };
}

const canOverlap = (a: Record<string, string>, b: Record<string, string>) => fields.every(({ key }) => !a[key] || !b[key] || a[key] === b[key]);

function describeRow(row: Record<string, string>, applications: readonly ApplicationOption[]): string {
  const parts: string[] = [];
  if (row.env_type) parts.push(`Loại môi trường (env_type): ${row.env_type}`);
  if (row.app_id) { const app = applications.find((item) => item.key === row.app_id); parts.push(`Ứng dụng (app_id): ${app && app.name !== app.key ? `${app.name} (${app.key})` : row.app_id}`); }
  if (row.env_id) parts.push(`Môi trường (env_id): ${row.env_id}`);
  if (row.res_id) parts.push(`Tài nguyên (res_id): ${row.res_id}`);
  if (row.class) parts.push(`Class: ${row.class}`);
  return parts.length === 0 ? 'Mọi nơi (không giới hạn)' : parts.join(' · ');
}

type Pending = { kind: 'mode'; target: CriteriaMode } | { kind: 'copy'; source: string; draft: CriteriaDraft };

export function CriteriaEditor({ draft, onChange, applications, sources, resourceType = '', matching, scopeKey = '', excludeKey = '' }: {
  draft: CriteriaDraft;
  onChange(next: CriteriaDraft): void;
  applications: ListState<ApplicationOption>;
  /** Other definitions: copy source and overlap candidates. */
  sources: ListState<CriteriaSource>;
  /** Resource Type of the form being edited. */
  resourceType?: string;
  /** Full unsaved candidate for the exact server-side matching preview (T19); omitted = panel hidden. */
  matching?: MatchingCandidateInput;
  /** Session/scope identity: a change invalidates the exact preview result. */
  scopeKey?: string;
  /** The Definition being edited: never its own copy or overlap source. */
  excludeKey?: string;
}) {
  const id = useId();
  const [pending, setPending] = useState<Pending | null>(null);
  const [copyFrom, setCopyFrom] = useState('');
  const [sample, setSample] = useState({ app: '', env: '', resId: '', cls: '' });
  const rows = draft.rows;
  const [row = emptyCriterion()] = rows;
  const { criteria, problem } = draftToCriteria(draft);
  const selectedApp = applications.items.find((app) => app.key === row.app_id);
  const knownTypes = [...new Set(applications.items.flatMap((app) => app.environments.map((env) => env.environmentType)).filter(Boolean))].sort();

  const setRows = (next: Criterion[]) => onChange({ mode: draft.mode, rows: next });
  const setRow = (index: number, field: CriterionField, value: string) => setRows(rows.map((item, position) => (position === index ? { ...item, [field]: value } : item)));

  function chooseMode(target: CriteriaMode) {
    if (target === draft.mode) { setPending(null); return; }
    if (draft.mode === 'advanced' && !lossless(rows, target)) { setPending({ kind: 'mode', target }); return; }
    setPending(null);
    onChange({ mode: target, rows: convert(rows, target) });
  }
  function copy() {
    const source = sources.items.find((item) => item.key === copyFrom);
    if (!source) return;
    const next = draftFromCriteria(source.criteria); // normalize() builds new objects: no live link
    if (hasData(rows) || rows.length > 1) { setPending({ kind: 'copy', source: source.key, draft: next }); return; }
    setPending(null);
    onChange(next);
  }
  function confirm() {
    if (pending?.kind === 'mode') onChange({ mode: pending.target, rows: convert(rows, pending.target) });
    if (pending?.kind === 'copy') onChange({ mode: pending.draft.mode, rows: cloneRows(pending.draft.rows) });
    setPending(null);
  }

  const overlaps = resourceType === '' || problem !== '' ? [] : sources.items.filter((source) => source.key !== excludeKey && source.resourceType === resourceType
    && (source.criteria ?? []).some((other) => criteria.some((mine) => canOverlap(mine, other as Record<string, string>))));

  const sampleApp = applications.items.find((app) => app.key === sample.app);
  const sampleEnv = sampleApp?.environments.find((env) => env.key === sample.env);
  const missing = [sampleApp && sampleEnv ? '' : 'ứng dụng và môi trường', sample.resId.trim() ? '' : 'ID tài nguyên (res_id)', sample.cls.trim() ? '' : 'Class'].filter(Boolean);

  return <section className="catalog-fields criteria-editor" aria-labelledby={`${id}-title`}>
    <div className="section-header"><div><h3 id={`${id}-title`}>Điều kiện áp dụng</h3><p>Chọn nơi cấu hình này được xét. Hệ thống chỉ dùng năm field chuẩn; chế độ chỉ giúp nhập nhanh hơn. Không có bộ lọc theo profile, tài khoản hay vùng.</p></div></div>
    <div className="feature-note criteria-rules" role="note" aria-label="Quy tắc so khớp"><p><strong>AND trong một dòng, OR giữa các dòng.</strong> Mọi field đã nhập trong cùng một dòng phải khớp cùng lúc; cấu hình khớp khi có ít nhất một dòng khớp, và lấy điểm của dòng tốt nhất.</p>
      <p>Trọng số cố định: env_type 1 · app_id 2 · env_id 4 · res_id 8 · class 16. Cấu hình có điểm cao nhất duy nhất sẽ thắng; hòa điểm cao nhất và không có cấu hình nào khớp là hai kết quả khác nhau.</p>
      <p><strong>Không có điều kiện</strong> (danh sách trống) bị từ chối. <strong>Dòng trống <code>{'{}'}</code></strong> là wildcard tường minh, khớp mọi nơi với điểm 0 (chế độ “Mọi nơi”).</p>
      <p>Lưu ý: ID cấu hình (Definition ID) khác ID tài nguyên <code>res_id</code>; chỉ <code>res_id</code> là điều kiện so khớp.</p></div>
    <div role="radiogroup" aria-label="Chế độ điều kiện áp dụng" className="criteria-modes">{modes.map((mode) => <label key={mode.key} className="criteria-mode"><input type="radio" name={`${id}-mode`} checked={draft.mode === mode.key} onChange={() => chooseMode(mode.key)} /><span><strong>{mode.label}</strong><small>{mode.hint}</small></span></label>)}</div>

    {pending ? <div role="group" aria-label="Xác nhận thay đổi điều kiện" className="form-warning criteria-confirm">
      <p>{pending.kind === 'mode' ? `Chế độ “${modeLabel(pending.target)}” không biểu diễn đầy đủ các điều kiện nâng cao hiện tại. Nếu tiếp tục, các điều kiện này sẽ bị bỏ.` : `Sao chép từ ${pending.source} sẽ thay thế các điều kiện đang nhập.`}</p>
      <div className="form-actions"><Button type="button" onClick={() => setPending(null)}>{pending.kind === 'mode' ? 'Giữ điều kiện nâng cao' : 'Giữ điều kiện hiện tại'}</Button><Button type="button" tone="danger" onClick={confirm}>{pending.kind === 'mode' ? 'Bỏ điều kiện và chuyển chế độ' : 'Thay bằng bản sao chép'}</Button></div>
    </div> : null}

    {draft.mode === 'env_type' ? <label>Loại môi trường<input list={`${id}-types`} value={row.env_type} aria-invalid={problem ? true : undefined} onChange={(event) => setRows([{ ...emptyCriterion(), env_type: event.target.value }])} /><datalist id={`${id}-types`}>{knownTypes.map((type) => <option key={type} value={type} />)}</datalist></label> : null}
    {draft.mode === 'app_env' ? <div className="field-grid">
      <label>Ứng dụng<select value={row.app_id} aria-invalid={problem ? true : undefined} onChange={(event) => setRows([{ ...emptyCriterion(), app_id: event.target.value }])}>
        <option value="">{applications.loading && applications.items.length === 0 ? 'Đang tải ứng dụng…' : 'Chọn ứng dụng'}</option>
        {row.app_id !== '' && !selectedApp ? <option value={row.app_id}>{row.app_id} (không có trong danh sách hiện tại)</option> : null}
        {applications.items.map((app) => <option key={app.key} value={app.key}>{app.name !== app.key ? `${app.name} (${app.key})` : app.key}</option>)}</select></label>
      <label>Môi trường (tùy chọn)<select value={row.env_id} disabled={row.app_id === ''} onChange={(event) => setRows([{ ...row, env_id: event.target.value }])}>
        <option value="">Mọi môi trường của ứng dụng</option>
        {row.env_id !== '' && !selectedApp?.environments.some((env) => env.key === row.env_id) ? <option value={row.env_id}>{row.env_id} (không có trong danh sách hiện tại)</option> : null}
        {(selectedApp?.environments ?? []).map((env) => <option key={env.key} value={env.key}>{env.name !== env.key ? `${env.name} (${env.key})` : env.key} · loại {env.environmentType}</option>)}</select></label>
    </div> : null}
    {draft.mode === 'advanced' ? <>{rows.map((item, index) => <div className="catalog-criterion" key={index}>{fields.map(({ key, label }) => <label key={key}>{label}<input aria-label={`Điều kiện ${index + 1} ${label}`} value={item[key]} onChange={(event) => setRow(index, key, event.target.value)} /></label>)}<Button type="button" tone="quiet" aria-label={`Xóa điều kiện ${index + 1}`} disabled={rows.length <= 1} onClick={() => setRows(rows.filter((_, position) => position !== index))}>Xóa</Button></div>)}
      <div><Button type="button" onClick={() => setRows([...rows, emptyCriterion()])}>+ Thêm điều kiện</Button></div>
      <p className="feature-note">Dòng để trống là wildcard tường minh. Các dòng là lựa chọn thay thế nhau.</p></> : null}

    {applications.error ? <div className="form-error" role="alert" aria-label="Lỗi tải ứng dụng">Không tải được danh sách ứng dụng nên chưa có lựa chọn ứng dụng/môi trường. Điều kiện đang nhập được giữ nguyên. <Button type="button" onClick={applications.onRetry}>Thử lại</Button></div> : null}
    {sources.error ? <div className="form-error" role="alert" aria-label="Lỗi tải cấu hình để sao chép">Không tải được danh sách cấu hình để sao chép hoặc kiểm tra chồng lấn. Điều kiện đang nhập được giữ nguyên. <Button type="button" onClick={sources.onRetry}>Thử lại</Button></div> : null}
    {problem ? <p className="feature-note" id={`${id}-problem`}>{problem}</p> : null}

    <div className="criteria-copy field-grid"><label>Sao chép điều kiện từ cấu hình khác<select value={copyFrom} onChange={(event) => setCopyFrom(event.target.value)}><option value="">{sources.loading && sources.items.length === 0 ? 'Đang tải cấu hình…' : sources.items.length === 0 ? 'Chưa có cấu hình để sao chép' : 'Chọn cấu hình'}</option>{sources.items.filter((source) => source.key !== excludeKey).map((source) => <option key={source.key} value={source.key}>{source.name && source.name !== source.key ? `${source.name} (${source.key})` : source.key} ({source.resourceType})</option>)}</select></label>
      <div className="form-actions"><Button type="button" disabled={copyFrom === ''} onClick={copy}>Sao chép điều kiện</Button></div></div>
    <p className="feature-note">Sao chép chỉ lấy một bản độc lập tại thời điểm bấm; sửa cấu hình nguồn sau đó không làm đổi điều kiện này.</p>

    <div className="criteria-summary" role="group" aria-label="Tóm tắt phạm vi áp dụng">
      <h4>Phạm vi áp dụng</h4>
      {problem ? <p>Chưa đủ thông tin để tóm tắt phạm vi.</p> : <ul>{criteria.map((item, index) => <li key={index}>{describeRow(item, applications.items)}</li>)}</ul>}
    </div>

    {overlaps.length > 0 ? <div role="group" aria-label="Cảnh báo nguy cơ chồng lấn" className="form-warning">Điều kiện này có thể chồng lấn với {overlaps.map((item) => item.key).join(', ')}: cùng Loại tài nguyên và các điều kiện không loại trừ nhau. Đây chỉ là cảnh báo nguy cơ từ giao diện; chỉ “Kiểm tra matching” trên máy chủ mới cho biết cấu hình thắng, hòa hay không khớp.</div> : null}

    <div className="criteria-preview" role="group" aria-label="Ngữ cảnh kiểm tra matching">
      <h4>Ngữ cảnh kiểm tra matching</h4>
      <p className="feature-note">Chọn ứng dụng và môi trường thật rồi nhập ID tài nguyên (res_id) và Class. env_type được máy chủ lấy từ môi trường; trình duyệt không tự đánh giá hay xếp hạng cấu hình.</p>
      <div className="field-grid">
        <label>Ứng dụng kiểm tra<select value={sample.app} onChange={(event) => setSample({ ...sample, app: event.target.value, env: '' })}><option value="">Chọn ứng dụng</option>{applications.items.map((app) => <option key={app.key} value={app.key}>{app.name !== app.key ? `${app.name} (${app.key})` : app.key}</option>)}</select></label>
        <label>Môi trường kiểm tra<select value={sample.env} disabled={!sampleApp} onChange={(event) => setSample({ ...sample, env: event.target.value })}><option value="">Chọn môi trường</option>{(sampleApp?.environments ?? []).map((env) => <option key={env.key} value={env.key}>{env.name !== env.key ? `${env.name} (${env.key})` : env.key}</option>)}</select></label>
        <label>ID tài nguyên kiểm tra (res_id)<input value={sample.resId} onChange={(event) => setSample({ ...sample, resId: event.target.value })} /></label>
        <label>Class kiểm tra<input value={sample.cls} onChange={(event) => setSample({ ...sample, cls: event.target.value })} /></label>
      </div>
      {missing.length > 0 ? <p>Cần thêm ngữ cảnh: {missing.join(', ')}. Chưa đủ ngữ cảnh nên chưa kiểm tra matching.</p> : problem ? <p>Hoàn thiện điều kiện áp dụng trước khi kiểm tra.</p> : null}
    </div>

    {matching ? <MatchingPreviewPanel candidate={matching} scopeKey={`${scopeKey}|${sampleEnv?.environmentType ?? ''}|${sampleEnv?.snapshotKey ?? ''}|apps:${applications.loading ? 'loading' : 'idle'}:${applications.error}|defs:${sources.loading ? 'loading' : 'idle'}:${sources.error}|${JSON.stringify(sources.items)}`} sample={{ applicationId: sample.app, environmentId: sampleApp && sampleEnv ? sample.env : '', resourceId: sample.resId, class: sample.cls }} /> : null}
  </section>;
}
