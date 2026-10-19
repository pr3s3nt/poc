import { render, screen, within } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { DependencyGraph } from './DependencyGraph';

const nodes = [
  { descriptor: 'k8s-cluster.internal#environments.shop.dev', kind: 'resource', origins: ['resource-reference'] },
  { descriptor: 'k8s-namespace.default#environments.shop.dev', kind: 'resource', origins: ['provider-binding'] },
  { descriptor: 'postgres.in-cluster#shared.db', kind: 'resource', origins: ['shared-dependency'], paramKeys: ['database'] },
  { descriptor: 'workload.default#modules.backend', kind: 'workload', origins: ['implicit-workload'] },
  { descriptor: 'audit.default#environments.shop.dev-audit', kind: 'resource', origins: ['co-provision'] },
];
const edges = [
  { consumer: 'k8s-namespace.default#environments.shop.dev', provider: 'k8s-cluster.internal#environments.shop.dev', reason: 'provider-binding' },
  { consumer: 'postgres.in-cluster#shared.db', provider: 'k8s-namespace.default#environments.shop.dev', reason: 'resource-reference' },
  { consumer: 'workload.default#modules.backend', provider: 'postgres.in-cluster#shared.db', reason: 'workload-placeholder' },
];

describe('DependencyGraph (ADR-017)', () => {
  it('labels every ordering edge with its reason and keeps the raw code as data', () => {
    render(<DependencyGraph nodes={nodes} edges={edges} />);
    expect(screen.getByText(/provider Kubernetes của module chạy trên cluster này/)).toBeInTheDocument();
    expect(screen.getByText(/tham chiếu output của tài nguyên khác/)).toBeInTheDocument();
    expect(screen.getByText(/workload đọc output của tài nguyên/)).toBeInTheDocument();
    expect(document.querySelectorAll('li[data-reason]')).toHaveLength(3);
    expect(document.querySelector('li[data-reason="provider-binding"]')).not.toBeNull();
  });

  it('shows a node with no ordering edge as selected only, never as an edge', () => {
    render(<DependencyGraph nodes={nodes} edges={edges} />);
    const heading = screen.getByRole('heading', { name: 'Chỉ được chọn, không có thứ tự thực thi' });
    const group = heading.nextElementSibling as HTMLElement;
    expect(within(group).getByText(/audit\.default#environments\.shop\.dev-audit/)).toBeInTheDocument();
    expect(within(group).getByText(/tài nguyên đi kèm/)).toBeInTheDocument();
    // The selection-only node appears in no dependency line.
    const ordering = Array.from(document.querySelectorAll('li[data-reason]')).map((li) => li.textContent ?? '');
    expect(ordering.some((line) => line.includes('audit.default'))).toBe(false);
  });

  it('names the consumers a failed provider held back with a fixed-vocabulary code', () => {
    render(<DependencyGraph nodes={nodes} edges={edges} blocked={[
      { consumer: 'workload.default#modules.backend', provider: 'postgres.in-cluster#shared.db', reason: 'workload-placeholder', code: 'REJECTED_BEFORE_RUN' },
      { consumer: 'x', provider: 'y', reason: 'hostile secretRef', code: 'vault://hvs.hostile-token' },
    ]} />);
    const section = screen.getByRole('region', { name: 'Thành phần bị chặn' });
    expect(within(section).getAllByText(/không chạy/)).toHaveLength(2);
    expect(within(section).getByText(/bị từ chối trước khi module chạy/)).toBeInTheDocument();
    // Unknown reason or code text from the server is never echoed.
    expect(section.textContent).not.toMatch(/vault:\/\/|hvs\.|secretRef/);
    expect(within(section).getByText(/lỗi không xác định/)).toBeInTheDocument();
  });

  it('renders no blocked section when nothing is held back', () => {
    render(<DependencyGraph nodes={nodes} edges={edges} blocked={[]} />);
    expect(screen.queryByRole('region', { name: 'Thành phần bị chặn' })).toBeNull();
  });
});
