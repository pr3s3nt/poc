import { useState, type FormEvent } from 'react';
import { navigate } from '../../app/routes';
import { ApiError } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { platformDomain } from '../../shared/types/application';

// Mirrors the backend DNS-label rule (UC-01 BR-03); the backend stays authoritative.
const subdomainPattern = /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/;

type FieldErrors = { name?: string; subdomain?: string };

const nameRequired = 'Nhập tên ứng dụng.';
const subdomainRequired = 'Nhập subdomain.';
const subdomainInvalid = 'Chỉ dùng chữ thường không dấu, số và dấu gạch ngang; bắt đầu và kết thúc bằng chữ hoặc số, tối đa 63 ký tự.';

// Input aid only: the backend stays authoritative for the ID and for uniqueness.
export function suggestSubdomain(name: string): string {
  return name
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .replace(/[đĐ]/g, 'd')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+/, '')
    .slice(0, 63)
    .replace(/-+$/, '');
}

function describeCreateError(err: unknown): { fields: FieldErrors; form?: string } {
  if (err instanceof ApiError && (err.field === 'name' || err.field === 'subdomain')) {
    if (err.status === 409) return { fields: err.field === 'name' ? { name: 'Tên ứng dụng này đã tồn tại.' } : { subdomain: 'Subdomain này đã được sử dụng.' } };
    if (err.status === 400) return { fields: err.field === 'name' ? { name: nameRequired } : { subdomain: subdomainInvalid } };
  }
  return { fields: {}, form: 'Không thể tạo ứng dụng. Hãy thử lại.' };
}

// UC-01: only Name and Subdomain. Each Environment chooses its own execution
// connection later, in Environment Settings (ADR-011).
export function CreateApplicationPage({ onCreate }: { onCreate(name: string, subdomain: string): Promise<string> }) {
  const [name, setName] = useState('');
  const [subdomain, setSubdomain] = useState('');
  const [subdomainEdited, setSubdomainEdited] = useState(false);
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({});
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const normalized = subdomain.trim().toLowerCase();
  function changeName(value: string) {
    setName(value);
    if (!subdomainEdited) setSubdomain(suggestSubdomain(value));
  }
  function submit(event: FormEvent) {
    event.preventDefault();
    if (submitting) return;
    const errors: FieldErrors = {
      name: name.trim() ? undefined : nameRequired,
      subdomain: !normalized ? subdomainRequired : subdomainPattern.test(normalized) ? undefined : subdomainInvalid,
    };
    setFieldErrors(errors);
    setError(null);
    if (errors.name || errors.subdomain) return;
    setSubmitting(true);
    onCreate(name.trim(), normalized)
      .then((id) => navigate({ name: 'application', applicationId: id }))
      .catch((err: unknown) => { const described = describeCreateError(err); setFieldErrors(described.fields); setError(described.form ?? null); setSubmitting(false); });
  }
  const host = normalized || 'ten-ung-dung';
  return <section className="page narrow-page"><button className="back-link" onClick={() => navigate({ name: 'applications' })}>← Ứng dụng</button><header className="form-header"><p className="eyebrow">Ứng dụng mới</p><h1>Tạo ứng dụng</h1><p>Hệ thống tự tạo môi trường staging và production. Chọn kết nối cho từng môi trường sau, trong Cài đặt môi trường.</p></header>
    <form className="create-form" aria-label="Tạo ứng dụng" onSubmit={submit} noValidate>{error ? <div className="form-error" role="alert">{error}</div> : null}
      <label htmlFor="application-name">Tên ứng dụng</label><input id="application-name" placeholder="Ví dụ: Thanh toán" value={name} disabled={submitting} aria-invalid={Boolean(fieldErrors.name)} onChange={(event) => changeName(event.target.value)} />{fieldErrors.name ? <span className="field-error">{fieldErrors.name}</span> : null}
      <label htmlFor="application-subdomain">Subdomain</label><input id="application-subdomain" placeholder="Ví dụ: thanh-toan" value={subdomain} disabled={submitting} aria-invalid={Boolean(fieldErrors.subdomain)} onChange={(event) => { setSubdomainEdited(true); setSubdomain(event.target.value); }} />{fieldErrors.subdomain ? <span className="field-error">{fieldErrors.subdomain}</span> : null}<small>Gợi ý từ tên ứng dụng, bạn có thể sửa. Chỉ dùng chữ thường không dấu, số và dấu gạch ngang.</small>
      <div className="url-preview" role="group" aria-label="Endpoint sẽ được tạo"><span>Endpoint sẽ được tạo</span><strong>staging.{host}.{platformDomain}</strong><strong>{host}.{platformDomain}</strong></div>
      <div className="form-actions"><Button type="button" onClick={() => navigate({ name: 'applications' })}>Hủy</Button><Button tone="primary" type="submit" disabled={submitting}>{submitting ? 'Đang tạo ứng dụng…' : 'Tạo ứng dụng'}</Button></div>
    </form>
  </section>;
}
