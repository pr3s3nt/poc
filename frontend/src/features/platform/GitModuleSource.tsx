import { useEffect, useRef, useState } from 'react';
import { api } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { parseCapabilities, type Capability } from './capabilities';

type Props = { resourceType: string; active: boolean; disabled: boolean; onResult: (capability: Capability | null) => void };

export function GitModuleSource({ resourceType, active, disabled, onResult }: Props) {
  const [url, setURL] = useState('');
  const [path, setPath] = useState('');
  const [ref, setRef] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [verified, setVerified] = useState('');
  const request = useRef({ version: 0 });
  const callback = useRef(onResult);
  callback.current = onResult;

  useEffect(() => {
    request.current.version++;
    setBusy(false); setError(''); setVerified(''); callback.current(null);
    const tracker = request.current;
    return () => { tracker.version++; };
  }, [resourceType, active]);

  const change = (setter: (value: string) => void, value: string) => {
    request.current.version++; setBusy(false); setError(''); setVerified(''); callback.current(null); setter(value);
  };

  async function inspect() {
    const current = ++request.current.version;
    setBusy(true); setError(''); setVerified(''); callback.current(null);
    try {
      const response = await api<{ capability: unknown }>('/terraform-modules/inspect', { method: 'POST', body: JSON.stringify({ resourceType, url: url.trim(), path: path.trim() || '.', ref: ref.trim() }) });
      const capability = parseCapabilities({ capabilities: [response.capability] })[0];
      if (!capability || capability.resourceType !== resourceType || capability.driverType !== 'terraform' || !capability.source) throw new Error('invalid metadata');
      if (current !== request.current.version) return;
      callback.current(capability);
      setVerified(`${ref.trim()} → ${capability.source.revision.slice(0, 12)}`);
    } catch {
      if (current === request.current.version) setError('Không kiểm tra được module. Kiểm tra Git HTTPS URL, thư mục, tag/commit và khả năng truy cập repository. Module phải tương thích provider của nền tảng. Dữ liệu đã nhập được giữ lại.');
    } finally {
      if (current === request.current.version) setBusy(false);
    }
  }

  return <section hidden={!active} className="catalog-fields" aria-label="Thêm module từ Git">
    <p>Nhập repository HTTPS công khai hoặc nguồn truy cập được không cần credential. Chưa hỗ trợ đăng nhập Git private hoặc SSH.</p>
    <div className="field-grid">
      <label>Git HTTPS URL<input type="url" value={url} disabled={disabled} spellCheck={false} placeholder="https://github.com/company/infra-modules.git" onChange={(event) => change(setURL, event.target.value)} /></label>
      <label>Thư mục module<input value={path} disabled={disabled} spellCheck={false} placeholder=". (thư mục gốc)" onChange={(event) => change(setPath, event.target.value)} /></label>
      <label>Tag hoặc commit<input value={ref} disabled={disabled} spellCheck={false} placeholder="v1.2.0 hoặc full commit" onChange={(event) => change(setRef, event.target.value)} /></label>
    </div>
    <Button type="button" disabled={disabled || busy || !resourceType || !url.trim() || !ref.trim()} onClick={() => { void inspect(); }}>{busy ? 'Đang kiểm tra module…' : 'Kiểm tra module'}</Button>
    {busy ? <p role="status">Đang tải và kiểm tra module…</p> : null}
    {error ? <p className="form-error" role="alert">{error}</p> : null}
    {verified ? <p className="form-success" role="status">Đã xác minh: {verified}. Phiên bản được cố định; hãy cấu hình các trường bên dưới trước khi lưu.</p> : null}
  </section>;
}
