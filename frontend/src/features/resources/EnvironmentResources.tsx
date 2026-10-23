import { useEffect, useRef, useState } from 'react';
import { ApiError } from '../../shared/api/client';
import { navigate } from '../../app/routes';
import type { EnvironmentKey } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { Status } from '../../shared/ui/Status';
import { listResources, type ResourceSummary } from './api';
import { resourceStatusLabel, scopeLabel } from './labels';

/**
 * Resources of the selected Environment, the navigation entry to each detail. A resource that lost
 * its last Workload stays listed as UNREFERENCED (nothing destroys it). A failed load is an error
 * with retry, never an empty list, and a reply for another Environment is ignored.
 */
export function EnvironmentResources({ applicationId, environment, refreshKey }: { applicationId: string; environment: EnvironmentKey; refreshKey?: string }) {
  const [items, setItems] = useState<ResourceSummary[]>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [unavailable, setUnavailable] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const latest = useRef(0);
  useEffect(() => {
    const current = ++latest.current;
    setLoading(true); setError(''); setUnavailable(false); setItems(undefined);
    listResources(applicationId, environment).then((list) => { if (current === latest.current) setItems(list); })
      .catch((failure: unknown) => { if (current !== latest.current) return; if (failure instanceof ApiError && failure.status === 503 && failure.code === 'SERVICE_UNAVAILABLE') setUnavailable(true); else setError('Không thể tải danh sách tài nguyên.'); })
      .finally(() => { if (current === latest.current) setLoading(false); });
    return () => { latest.current += 1; };
  }, [applicationId, environment, refreshKey, attempt]);
  return <section className="content-panel" aria-label="Tài nguyên của môi trường"><div className="section-header"><div><h2>Tài nguyên</h2><p>Tài nguyên hạ tầng do Orchestrator quản lý trong {environment}, kể cả tài nguyên không còn được Workload nào dùng.</p></div><Button onClick={() => setAttempt((n) => n + 1)}>Tải lại</Button></div>
    {loading ? <p role="status" aria-busy="true">Đang tải tài nguyên…</p> : unavailable ? <div role="status" className="form-warning"><strong>Danh sách tài nguyên hiện chưa khả dụng</strong><p>Backend chưa cung cấp được dữ liệu cho mục này. Chưa thể kết luận tài nguyên trống hay trạng thái triển khai từ mục này. Hãy kiểm tra cấu hình dịch vụ và thử lại.</p><Button onClick={() => setAttempt(n => n + 1)}>Kiểm tra lại</Button></div> : error ? <div role="alert" className="form-error">{error} <Button onClick={() => setAttempt((n) => n + 1)}>Thử lại</Button></div> : items && items.length === 0 ? <p>Chưa có tài nguyên nào trong {environment}.</p> : items ? <div className="deployment-list">{items.map((item) => <button className="deployment-entry" key={item.id} onClick={() => navigate({ name: 'resource', applicationId, environment, resourceId: item.id })}><span><strong>{item.descriptor}</strong><small>{item.type} · {item.class} · {scopeLabel(item.scope)} · thế hệ {item.generation}</small></span><Status tone={item.status === 'READY' ? 'good' : item.status === 'UNREFERENCED' ? 'draft' : 'neutral'}>{resourceStatusLabel(item.status)}</Status></button>)}</div> : null}
  </section>;
}
