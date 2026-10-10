import { useId, useState } from 'react';
import { Button } from '../../shared/ui/Button';

// Shared "Điều kiện áp dụng" editor (UC-03 T02B). It edits the five-field
// Matching Criteria contract and serializes it unchanged; modes are only a
// friendlier projection. The draft is controlled by the owner so the editor
// can serve Resource Definition and renderer forms alike.

export type CriterionField = 'env_type' | 'app_id' | 'env_id' | 'res_id' | 'class';
export type Criterion = Record<CriterionField, string>;
export type CriteriaMode = 'all' | 'env_type' | 'app_env' | 'advanced';
export type CriteriaDraft = { mode: CriteriaMode; rows: Criterion[] };
export type EnvironmentOption = { key: string; name: string; environmentType: string; executionProfile?: string };
export type ApplicationOption = { key: string; name: string; environments: EnvironmentOption[] };
export type CriteriaSource = { key: string; resourceType: string; executionProfile?: string; criteria?: Partial<Criterion>[] };
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

export function CriteriaEditor({ draft, onChange, applications, sources, resourceType = '', profile = '', profileLabel = '' }: {
  draft: CriteriaDraft;
  onChange(next: CriteriaDraft): void;
  applications: ListState<ApplicationOption>;
  /** Other definitions: copy source and overlap candidates. */
  sources: ListState<CriteriaSource>;
  /** Type and executionProfile of the form being edited ('' profile = shared). */
  resourceType?: string;
  profile?: string;
  profileLabel?: string;
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

  const overlaps = resourceType === '' || problem !== '' ? [] : sources.items.filter((source) => source.resourceType === resourceType
    && ((source.executionProfile ?? '') === profile || (source.executionProfile ?? '') === '' || profile === '')
    && (source.criteria ?? []).some((other) => criteria.some((mine) => canOverlap(mine, other as Record<string, string>))));

  const sampleApp = applications.items.find((app) => app.key === sample.app);
  const sampleEnv = sampleApp?.environments.find((env) => env.key === sample.env);
  const missing = [sampleApp && sampleEnv ? '' : 'ứng dụng và môi trường mẫu', sample.resId.trim() ? '' : 'ID tài nguyên (res_id)', sample.cls.trim() ? '' : 'Class'].filter(Boolean);
  const sampleProfile = sampleEnv?.executionProfile ?? '';
  const profileState: 'match' | 'differs' | 'unknown' = profile === '' || sampleProfile === profile ? 'match' : sampleProfile === '' ? 'unknown' : 'differs';
  const context: Record<string, string> = { env_type: sampleEnv?.environmentType ?? '', app_id: sample.app, env_id: sample.env, res_id: sample.resId.trim(), class: sample.cls.trim() };
  const applies = (item: Record<string, string>) => fields.every(({ key }) => !item[key] || item[key] === context[key]);

  return <section className="catalog-fields criteria-editor" aria-labelledby={`${id}-title`}>
    <div className="section-header"><div><h3 id={`${id}-title`}>Điều kiện áp dụng</h3><p>Chọn nơi cấu hình này được xét. Hệ thống vẫn dùng năm field chuẩn; chế độ chỉ giúp nhập nhanh hơn.</p></div></div>
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

    <div className="criteria-copy field-grid"><label>Sao chép điều kiện từ cấu hình khác<select value={copyFrom} onChange={(event) => setCopyFrom(event.target.value)}><option value="">{sources.loading && sources.items.length === 0 ? 'Đang tải cấu hình…' : sources.items.length === 0 ? 'Chưa có cấu hình để sao chép' : 'Chọn cấu hình'}</option>{sources.items.map((source) => <option key={source.key} value={source.key}>{source.key} ({source.resourceType})</option>)}</select></label>
      <div className="form-actions"><Button type="button" disabled={copyFrom === ''} onClick={copy}>Sao chép điều kiện</Button></div></div>
    <p className="feature-note">Sao chép chỉ lấy một bản độc lập tại thời điểm bấm; sửa cấu hình nguồn sau đó không làm đổi điều kiện này.</p>

    <div className="criteria-summary" role="group" aria-label="Tóm tắt phạm vi áp dụng">
      <h4>Phạm vi áp dụng</h4>
      {problem ? <p>Chưa đủ thông tin để tóm tắt phạm vi.</p> : <ul>{criteria.map((item, index) => <li key={index}>{describeRow(item, applications.items)}</li>)}</ul>}
      <p>Phạm vi triển khai (executionProfile): <strong>{profileLabel || (profile === '' ? 'Dùng chung' : profile)}</strong>. Đây là bộ lọc riêng, không phải Loại môi trường (env_type).</p>
    </div>

    {overlaps.length > 0 ? <div role="group" aria-label="Cảnh báo nguy cơ chồng lấn" className="form-warning">Điều kiện này có thể chồng lấn với {overlaps.map((item) => item.key).join(', ')}: cùng Loại tài nguyên, cùng điều kiện lọc theo phạm vi triển khai và các điều kiện không loại trừ nhau. Đây chỉ là cảnh báo nguy cơ từ giao diện, chưa phải kết quả matching chính xác.</div> : null}

    <div className="criteria-preview" role="group" aria-label="Xem trước phạm vi (trợ giúp ở giao diện)">
      <h4>Xem trước phạm vi</h4>
      <p className="feature-note">Chỉ là trợ giúp trong trình duyệt: không lưu, không tạo tài nguyên và không phải kết quả matching chính xác.</p>
      <div className="field-grid">
        <label>Ứng dụng mẫu<select value={sample.app} onChange={(event) => setSample({ ...sample, app: event.target.value, env: '' })}><option value="">Chọn ứng dụng</option>{applications.items.map((app) => <option key={app.key} value={app.key}>{app.name !== app.key ? `${app.name} (${app.key})` : app.key}</option>)}</select></label>
        <label>Môi trường mẫu<select value={sample.env} disabled={!sampleApp} onChange={(event) => setSample({ ...sample, env: event.target.value })}><option value="">Chọn môi trường</option>{(sampleApp?.environments ?? []).map((env) => <option key={env.key} value={env.key}>{env.name !== env.key ? `${env.name} (${env.key})` : env.key}</option>)}</select></label>
        <label>ID tài nguyên mẫu (res_id)<input value={sample.resId} onChange={(event) => setSample({ ...sample, resId: event.target.value })} /></label>
        <label>Class mẫu<input value={sample.cls} onChange={(event) => setSample({ ...sample, cls: event.target.value })} /></label>
      </div>
      {missing.length > 0 ? <p>Cần thêm context: {missing.join(', ')}. Chưa đủ context nên chưa đánh giá điều kiện.</p>
        : problem ? <p>Hoàn thiện điều kiện áp dụng trước khi xem trước.</p>
        : profileState === 'differs' ? <p>Môi trường mẫu dùng phạm vi triển khai (executionProfile) {sampleProfile}, khác phạm vi {profile} của cấu hình này nên cấu hình không được xét ở môi trường đó, dù năm field điều kiện có khớp hay không.</p>
        : <><p>Context mẫu: loại môi trường (env_type) {context.env_type || '(không có)'}, ứng dụng {context.app_id}, môi trường {context.env_id}.</p>
          {profileState === 'unknown' ? <p>Chưa biết phạm vi triển khai của môi trường mẫu (chưa cấu hình?). Thỏa điều kiện chưa có nghĩa là cấu hình được xét.</p> : <p>Phạm vi triển khai (executionProfile) của môi trường mẫu phù hợp với cấu hình này; đây là bộ lọc riêng, tách khỏi năm field điều kiện.</p>}<ul>{criteria.map((item, index) => <li key={index}>Điều kiện {index + 1}: {applies(item) ? 'thỏa context mẫu' : 'không thỏa context mẫu'}</li>)}</ul></>}
    </div>
  </section>;
}
