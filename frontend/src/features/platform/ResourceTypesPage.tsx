import { useRef, useState, type FormEvent } from 'react';
import { specificGuidance, fieldNote } from '../../shared/api/errors';
import { href, navigate } from '../../app/routes';
import { api, ApiError } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { own, supported } from './resourceSupport';
import { useCatalogList } from './useCatalogList';

type InputField = { name: string; type: string; required: boolean };
type OutputField = InputField & { secret: boolean };
type ResourceType = { key: string; inputs?: Partial<InputField>[] | null; outputs?: Partial<OutputField>[] | null };

const fieldTypes = ['string', 'number', 'bool', 'any'];
const newInput = (): InputField => ({ name: '', type: 'string', required: false });
const newOutput = (): OutputField => ({ name: '', type: 'string', required: false, secret: false });
const yesNo = (value: boolean | undefined) => (value === true ? 'Có' : 'Không');
const driverText = (driver: string, module: string) => (driver === 'terraform' ? `Terraform/${module}` : 'Kubernetes');

// Flags and types are copied exactly as the GET returned them; nothing is
// inferred from field names. A missing flag is false (the API omits false).
const copyInputs = (fields: Partial<InputField>[] | null | undefined): InputField[] => (fields ?? []).map((field) => ({ name: field.name ?? '', type: field.type ?? 'string', required: field.required === true }));
const copyOutputs = (fields: Partial<OutputField>[] | null | undefined): OutputField[] => (fields ?? []).map((field) => ({ name: field.name ?? '', type: field.type ?? 'string', required: field.required === true, secret: field.secret === true }));

// Specific guidance comes from the stable error code; raw backend text is never shown.
function describeFailure(reason: unknown): string {
  if (!(reason instanceof ApiError)) return 'Chưa nhận được phản hồi từ máy chủ nên chưa rõ kết quả: loại tài nguyên có thể đã được đăng ký. Hãy tải lại danh sách trước khi thử lại.';
  const field = fieldNote(reason.field);
  const specific = specificGuidance(reason.code, reason.message, reason.field);
  if (specific) return `Máy chủ từ chối loại tài nguyên. ${specific}`;
  switch (reason.status) {
    case 400: return `Máy chủ từ chối loại tài nguyên này. Kiểm tra ID (chữ thường, số, dấu gạch ngang; environment và service là ID dành riêng) và tên các tham số rồi thử lại.${field}`;
    case 401: return 'Phiên đăng nhập đã hết hạn. Hãy đăng nhập lại.';
    case 403: return 'Bạn không có quyền đăng ký loại tài nguyên. Chỉ Kỹ sư nền tảng hoặc Quản trị viên được đăng ký.';
    case 409: return 'ID loại tài nguyên đã tồn tại. Hãy đổi ID rồi thử lại.';
    case 413: return 'Nội dung loại tài nguyên quá lớn. Hãy rút gọn rồi thử lại.';
    default: return 'Máy chủ gặp lỗi khi đăng ký loại tài nguyên. Dữ liệu đã nhập được giữ lại; hãy thử lại sau.';
  }
}
const listFailure = () => 'Không tải được danh sách loại tài nguyên từ máy chủ.';

const loadTypes = async () => (await api<{ resourceTypes: ResourceType[] }>('/resource-types')).resourceTypes ?? [];

function SchemaTable({ label, fields, secret }: { label: string; fields: Partial<OutputField>[]; secret: boolean }) {
  return <table className="catalog-schema" aria-label={label}><thead><tr><th scope="col">Tên</th><th scope="col">Kiểu</th><th scope="col">Bắt buộc</th>{secret ? <th scope="col">Bí mật</th> : null}</tr></thead>
    <tbody>{fields.map((field, index) => <tr key={index}><td><code>{field.name}</code></td><td><code>{field.type}</code></td><td>{yesNo(field.required)}</td>{secret ? <td>{yesNo(field.secret)}</td> : null}</tr>)}</tbody></table>;
}

export function ResourceTypesPage() {
  const { items: types, loading, loadError, reload } = useCatalogList(loadTypes, listFailure);
  const [key, setKey] = useState('');
  const [inputs, setInputs] = useState<InputField[]>([]);
  const [outputs, setOutputs] = useState<OutputField[]>([]);
  const [copiedFrom, setCopiedFrom] = useState('');
  const [expanded, setExpanded] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const idField = useRef<HTMLInputElement>(null);

  // Copy never sends a request: it only fills the independent draft; the ID stays blank.
  function copySchema(source: ResourceType) {
    if (saving) return;
    setKey(''); setInputs(copyInputs(source.inputs)); setOutputs(copyOutputs(source.outputs));
    setCopiedFrom(source.key); setError(''); setNotice('');
    idField.current?.focus();
  }
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (saving) return;
    setError(''); setNotice(''); setSaving(true);
    // The backend owns the ID rule (UC-02 BR-05); send the ID exactly as typed.
    const submitted = key;
    try {
      await api('/resource-types', { method: 'POST', body: JSON.stringify({ key: submitted, inputs: inputs.map((field) => ({ ...field, name: field.name.trim() })), outputs: outputs.map((field) => ({ ...field, name: field.name.trim() })) }) });
    } catch (reason) { setError(describeFailure(reason)); setSaving(false); return; }
    // Committed: report it and reset the form before reloading the list.
    setNotice(`Đã đăng ký loại tài nguyên ${submitted}.`); setKey(''); setInputs([]); setOutputs([]); setCopiedFrom(''); setSaving(false);
    await reload();
  }
  function typeOptions(value: string) {
    return [...fieldTypes, ...(fieldTypes.includes(value) ? [] : [value])].map((option) => <option key={option}>{option}</option>);
  }
  function renderFields<T extends InputField>(label: string, noun: string, fields: T[], update: (fields: T[]) => void, create: () => T, secret: boolean) {
    const patch = (index: number, change: Partial<OutputField>) => update(fields.map((item, position) => position === index ? { ...item, ...change } : item));
    return <section className="catalog-fields"><div className="section-header"><h3>{label}</h3><Button type="button" onClick={() => update([...fields, create()])}>+ Thêm {noun}</Button></div>
      {fields.length === 0 ? <p className="muted">Chưa khai báo {noun}.</p> : null}
      {fields.map((field, index) => <div className="catalog-field-row" key={index}>
        <label>Tên<input aria-label={`${label} ${index + 1} tên`} value={field.name} required onChange={(event) => patch(index, { name: event.target.value })} /></label>
        <label>Kiểu<select aria-label={`${label} ${index + 1} kiểu`} value={field.type} onChange={(event) => patch(index, { type: event.target.value })}>{typeOptions(field.type)}</select></label>
        <label className="catalog-check"><input type="checkbox" aria-label={`${label} ${index + 1} bắt buộc`} checked={field.required} onChange={(event) => patch(index, { required: event.target.checked })} /> Bắt buộc</label>
        {secret ? <label className="catalog-check"><input type="checkbox" aria-label={`${label} ${index + 1} bí mật`} checked={'secret' in field && field.secret === true} onChange={(event) => patch(index, { secret: event.target.checked })} /> Bí mật</label> : null}
        <Button type="button" tone="quiet" aria-label={`Xóa ${label.toLowerCase()} ${index + 1}`} onClick={() => update(fields.filter((_, position) => position !== index))}>Xóa</Button>
      </div>)}
    </section>;
  }
  function renderEntry(type: ResourceType) {
    const rule = own(supported, type.key);
    const open = expanded === type.key;
    const inputList = type.inputs ?? [];
    const outputList = type.outputs ?? [];
    return <div key={type.key} className="catalog-entry catalog-entry-rich"><div className="section-header"><div><strong>{type.key}</strong> <span className="status-pill">{rule ? 'Có cách tạo hỗ trợ' : 'Chỉ có contract'}</span><br />
      <span>{inputList.length} đầu vào · {outputList.length} đầu ra</span>{rule ? <span> · Cách tạo: {rule.drivers.map((driver) => driverText(driver, rule.module)).join(' hoặc ')}</span> : null}</div>
      <div className="form-actions">
        <Button type="button" aria-expanded={open} aria-label={`${open ? 'Ẩn' : 'Xem'} schema ${type.key}`} onClick={() => setExpanded(open ? '' : type.key)}>{open ? 'Ẩn schema' : 'Xem schema'}</Button>
        {rule ? <a className="button" href={href({ name: 'resource-definitions', resourceType: type.key })} aria-label={`Tạo Cấu hình tài nguyên cho ${type.key}`} onClick={(event) => { event.preventDefault(); navigate({ name: 'resource-definitions', resourceType: type.key }); }}>Tạo Cấu hình tài nguyên</a> : null}
      </div></div>
      {type.key === 'workload' ? <p className="feature-note">Loại này mô tả workload của ứng dụng, được dựng bằng <a href={href({ name: 'rendering-templates' })} onClick={(event) => { event.preventDefault(); navigate({ name: 'rendering-templates' }); }}>Mẫu dựng ứng dụng</a>. Không tạo Cấu hình tài nguyên cho loại này.</p> : null}
      {open ? <section className="catalog-details" aria-label={`Schema của ${type.key}`}>
        <h3>Đầu vào</h3>{inputList.length === 0 ? <p className="muted">Chưa khai báo tham số đầu vào.</p> : <SchemaTable label={`Đầu vào của ${type.key}`} fields={inputList} secret={false} />}
        <h3>Đầu ra</h3>{outputList.length === 0 ? <p className="muted">Chưa khai báo tham số đầu ra.</p> : <SchemaTable label={`Đầu ra của ${type.key}`} fields={outputList} secret />}
        <div className="form-actions"><Button type="button" disabled={saving} aria-label={`Sao chép schema ${type.key}`} onClick={() => copySchema(type)}>Sao chép schema</Button></div>
      </section> : null}
    </div>;
  }
  return <section className="page"><header className="page-header"><div><p className="eyebrow">Nền tảng</p><h1>Loại tài nguyên</h1><p>Khai báo contract (đầu vào, đầu ra) dùng lại được cho Tổ chức này. Chỉ Kỹ sư nền tảng hoặc Quản trị viên được đăng ký.</p></div></header>
    <section className="content-panel"><div className="section-header"><h2>Loại tài nguyên đã đăng ký</h2></div>
      <p className="feature-note">“Có cách tạo hỗ trợ” nghĩa là hệ thống có cách tạo cho loại này; vẫn cần Cấu hình tài nguyên và đích hợp lệ nên chưa bảo đảm triển khai thành công. “Chỉ có contract” chỉ là mô tả schema, chưa có cách tạo.</p>
      {loadError ? <div className="form-error" role="alert" aria-label="Lỗi tải danh sách">{loadError} <Button type="button" onClick={() => void reload()}>Thử lại</Button></div> : loading ? <p role="status">Đang tải loại tài nguyên…</p> : types.length === 0 ? <p>Chưa có loại tài nguyên nào.</p> : <div className="catalog-list">{types.map(renderEntry)}</div>}</section>
    <section className="content-panel"><div className="section-header"><div><h2>Đăng ký loại tài nguyên</h2><p>Contract độc lập với nhà cung cấp hạ tầng. Đăng ký loại mới không thêm cách tạo tài nguyên. Loại đã đăng ký không sửa, xóa hay đổi phiên bản; muốn đổi schema hãy sao chép sang ID mới.</p></div></div>
      <form onSubmit={submit}><fieldset className="editor-grid form-fieldset" disabled={saving} aria-busy={saving}><label>ID loại tài nguyên<input ref={idField} value={key} required aria-describedby="resource-type-id-hint" onChange={(event) => setKey(event.target.value)} /></label>
        <p id="resource-type-id-hint" className="feature-note">Dùng chữ thường, số và dấu gạch ngang, ví dụ <code>postgres-ha</code>. ID không được tự sửa hộ. <code>environment</code> và <code>service</code> là ID dành riêng.</p>
        {copiedFrom ? <p className="feature-note">Bản nháp sao chép từ <code>{copiedFrom}</code>: nhập ID mới, kiểm tra hoặc sửa schema rồi đăng ký. Loại nguồn không bị thay đổi.</p> : null}
        {renderFields('Đầu vào', 'tham số đầu vào', inputs, setInputs, newInput, false)}{renderFields('Đầu ra', 'tham số đầu ra', outputs, setOutputs, newOutput, true)}
        {error ? <div className="form-error" role="alert">{error}</div> : null}{notice ? <div className="form-success" role="status">{notice}</div> : null}<div className="form-actions"><Button type="submit" tone="primary" disabled={saving}>{saving ? 'Đang đăng ký…' : 'Đăng ký loại tài nguyên'}</Button></div>
      </fieldset></form>
    </section>
  </section>;
}
