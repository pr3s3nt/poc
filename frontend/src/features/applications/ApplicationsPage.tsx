import type { Application } from '../../shared/types/application';
import { endpointFor } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { Status } from '../../shared/ui/Status';
import { navigate } from '../../app/routes';

export type ApplicationsLoadState = 'loading' | 'ready' | 'error';

export function ApplicationsPage({ applications, state = 'ready', onRetry }: { applications: readonly Application[]; state?: ApplicationsLoadState; onRetry?(): void }) {
  return <section className="page">
    <header className="page-header"><div><p className="eyebrow">Không gian làm việc</p><h1>Ứng dụng của bạn</h1><p>Tạo ứng dụng, sau đó triển khai lên staging hoặc production.</p></div><Button tone="primary" onClick={() => navigate({ name: 'create-application' })}>+ Tạo ứng dụng</Button></header>
    {state === 'loading' ? <div className="application-grid" role="status" aria-busy="true" aria-label="Đang tải ứng dụng"><div className="application-card-skeleton" /><div className="application-card-skeleton" /></div> :
      state === 'error' ? <div className="form-error" role="alert">Không thể tải danh sách ứng dụng.<Button onClick={onRetry}>Thử lại</Button></div> :
      applications.length === 0 ? <div className="empty-state"><div className="empty-icon">◇</div><h2>Bắt đầu với ứng dụng mới</h2><p>Tạo ứng dụng để có môi trường staging và production.</p><Button tone="primary" onClick={() => navigate({ name: 'create-application' })}>+ Tạo ứng dụng</Button></div> :
      <div className="application-grid">{applications.map((application) => <button className="application-card" key={application.id} onClick={() => navigate({ name: 'application', applicationId: application.id })}>
        <div className="card-title"><span className="application-icon">⌘</span><span><strong>{application.name}</strong><small>{endpointFor(application, 'production')}</small></span><span className="card-arrow">→</span></div>
        <div className="environment-summary">{application.availableEnvironments !== undefined && !application.availableEnvironments.some(key => key === 'staging' || key === 'production') ? <span><small>{application.availableEnvironments.join(', ') || 'Chưa có môi trường'}</small><Status tone="neutral">Môi trường cũ / chưa hỗ trợ</Status></span> : (['staging', 'production'] as const).filter(key => application.availableEnvironments === undefined || application.availableEnvironments.includes(key)).map(key => <span key={key}><small>{key}</small><Status tone="draft">Sẵn sàng cấu hình</Status></span>)}</div>
      </button>)}</div>}
  </section>;
}
