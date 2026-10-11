import type { KeyKind } from '../configuration/api';
import { Button } from '../../shared/ui/Button';
import { selectKey, unselectKey, type Binding, type KeyCatalog } from './bindings';

type Props = { container: string; bindings: Binding[]; catalog: KeyCatalog; disabled: boolean; onChange: (bindings: Binding[]) => void };
type Mapping = { binding: Binding; index: number };

// Per-container checklist of existing Application variables and secrets. Only
// key names and container names are shown; catalog values are never rendered.
export function ApplicationKeyPicker({ container, bindings, catalog, disabled, onChange }: Props) {
  const mappings: Mapping[] = bindings.flatMap((binding, index) => binding.source === 'KEY' ? [{ binding, index }] : []);
  const update = (index: number, next: Binding) => onChange(bindings.map((item, i) => i === index ? next : item));
  const remove = (index: number) => onChange(bindings.filter((_, i) => i !== index));

  if (catalog.status !== 'ready') return <div className="key-picker">
    <p>{catalog.status === 'loading' ? 'Đang tải biến và bí mật của ứng dụng…' : 'Không tải được biến và bí mật của ứng dụng. Dùng nút Thử lại ở trên; các key đã chọn được giữ nguyên.'}</p>
    {mappings.length ? <ul aria-label={`Key ứng dụng đã chọn cho ${container}`}>{mappings.map(({ binding, index }) => <li key={index}><code>{binding.key}</code> thành <code>{binding.name}</code></li>)}</ul> : null}
  </div>;

  const known = new Set(catalog.keys.map((key) => key.name));
  const unavailable = mappings.filter(({ binding }) => !known.has(binding.key));

  const group = (kind: KeyKind, title: string, empty: string) => {
    const keys = catalog.keys.filter((key) => key.kind === kind);
    return <fieldset className="key-group" aria-label={`${title} cho ${container}`}>
      <legend>{title}</legend>
      {keys.length ? <ul>{keys.map((key) => {
        const selected = mappings.filter(({ binding }) => binding.key === key.name);
        return <li key={key.name}>
          <label className="key-option"><input type="checkbox" checked={selected.length > 0} disabled={disabled} onChange={(event) => onChange(event.target.checked ? selectKey(bindings, key.name) : unselectKey(bindings, key.name))} /><code>{key.name}</code></label>
          {key.configured ? null : <small className="muted"> · chưa đặt giá trị trong môi trường này</small>}
          {selected.map(({ binding, index }, n) => {
            const suffix = selected.length > 1 ? ` (${n + 1})` : '';
            return <div className="key-mapping" key={index}>
              {binding.custom ? <label>Tên trong Container<input aria-label={`Tên trong Container cho ${key.name}${suffix}`} value={binding.name} disabled={disabled} onChange={(event) => update(index, { ...binding, name: event.target.value })} /></label> : <span>Tên trong Container <code>{binding.name}</code></span>}
              <label className="key-option"><input type="checkbox" aria-label={`Dùng tên Container khác cho ${key.name}${suffix}`} checked={binding.custom} disabled={disabled} onChange={(event) => update(index, { ...binding, custom: event.target.checked, name: event.target.checked ? binding.name : key.name })} />Dùng tên Container khác</label>
              {selected.length > 1 ? <Button tone="quiet" disabled={disabled} aria-label={`Xóa tên Container ${binding.name || '(trống)'} cho ${key.name}`} onClick={() => remove(index)}>Xóa tên</Button> : null}
            </div>;
          })}
        </li>;
      })}</ul> : <p>{empty}</p>}
    </fieldset>;
  };

  return <div className="key-picker">
    {group('VARIABLE', 'Biến của ứng dụng', 'Môi trường này chưa có biến của ứng dụng. Thêm trong cài đặt Biến & Bí mật.')}
    {group('SECRET', 'Bí mật của ứng dụng', 'Môi trường này chưa có bí mật của ứng dụng. Thêm trong cài đặt Biến & Bí mật.')}
    {unavailable.length ? <div className="form-error" role="group" aria-label={`Key ứng dụng không còn tồn tại cho ${container}`}>
      <p>Các key này không còn trong môi trường này. Hãy xóa chúng hoặc chọn key có sẵn trước khi lưu.</p>
      <ul>{unavailable.map(({ binding, index }) => <li key={index}><code>{binding.key}</code> với tên Container <code>{binding.name}</code> <Button tone="quiet" disabled={disabled} aria-label={`Xóa key không còn tồn tại ${binding.key} với tên ${binding.name}`} onClick={() => remove(index)}>Xóa</Button></li>)}</ul>
    </div> : null}
  </div>;
}
