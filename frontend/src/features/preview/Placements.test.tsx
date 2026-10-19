import { render, screen, within } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { Placements } from './Placements';
import { ResourceIntents, rendererText } from './ResourceIntents';

describe('Preview target provenance (ADR-016)', () => {
  it('shows each node cluster Definition, Internal account and namespace without any credential field', () => {
    render(<Placements placements={[
      { node: 'workload.default#modules.frontend', cluster: 'k8s-cluster.internal#environments.shop.dev', clusterDefinition: 'cluster-lab', clusterDefinitionVersion: 2, account: 'lab', accountVersion: 4, namespace: 'k8s-namespace.default#environments.shop.dev', source: 'workload-reference' },
      { node: 'k8s-namespace.default#environments.shop.dev', cluster: 'k8s-cluster.internal#environments.shop.dev', clusterDefinition: 'cluster-lab', clusterDefinitionVersion: 2, account: 'lab', accountVersion: 4, source: 'provider-binding' },
    ]} />);
    const rows = within(screen.getByLabelText('Đích triển khai của từng thành phần')).getAllByRole('row');
    expect(rows).toHaveLength(3);
    expect(rows[1]).toHaveTextContent('cluster-lab (v2)');
    expect(rows[1]).toHaveTextContent('lab (v4)');
    expect(rows[1]).toHaveTextContent('tham chiếu của workload');
    expect(rows[2]).toHaveTextContent('provider binding của module');
    expect(document.body.textContent).not.toMatch(/kubeconfig|token|secret/i);
  });

  it('shows an EKS cluster created in the same execution with its own account and region and no live endpoint (ADR-017)', () => {
    render(<Placements placements={[
      { node: 'workload.default#modules.web', cluster: 'k8s-cluster.eks#environments.shop.dev', clusterDefinition: 'cluster-eks', clusterDefinitionVersion: 1, account: 'aws-a', accountVersion: 5, namespace: 'k8s-namespace.default#environments.shop.dev', source: 'workload-reference', provider: 'eks', region: 'us-east-1' },
    ]} />);
    const rows = within(screen.getByLabelText('Đích triển khai của từng thành phần')).getAllByRole('row');
    expect(rows[1]).toHaveTextContent('EKS tạo trong lần triển khai này');
    expect(rows[1]).toHaveTextContent('aws-a (v5)');
    expect(rows[1]).toHaveTextContent('us-east-1');
    expect(screen.getByText(/chưa tồn tại lúc xem trước: endpoint, xác thực và trạng thái Ready chỉ có sau khi tạo/)).toBeInTheDocument();
    expect(document.body.textContent).not.toMatch(/https:\/\/|kubeconfig|token|secret/i);
  });

  it('shows the producing account and region on a Kubernetes provider bound to a cluster the plan creates', () => {
    render(<ResourceIntents intents={[{ descriptor: 'postgres.in-cluster#shared.db', resourceType: 'postgres', class: 'in-cluster', scope: 'SHARED', ownership: 'ORCHESTRATOR', intent: 'CREATE', definitionKey: 'pg', driverType: 'terraform',
      providers: [{ name: 'kubernetes', kind: 'kubernetes', target: 'resource', provider: 'k8s-cluster.eks#environments.shop.dev', connectionKey: 'aws-a', region: 'us-east-1' }] }]} />);
    expect(screen.getByText(/kubernetes: cluster do node k8s-cluster\.eks#environments\.shop\.dev cung cấp · tài khoản aws-a · vùng us-east-1/)).toBeInTheDocument();
  });

  it('renders nothing when the plan has no placements (legacy or AWS plans)', () => {
    const { container } = render(<Placements placements={[]} />);
    expect(container).toBeEmptyDOMElement();
    expect(render(<Placements placements={undefined} />).container).toBeEmptyDOMElement();
  });

  it('marks the preexisting cluster and its account in the resource intents and shows the workload class', () => {
    render(<ResourceIntents intents={[{ descriptor: 'k8s-cluster.internal#environments.shop.dev', resourceType: 'k8s-cluster', class: 'internal', scope: 'ENVIRONMENT:shop.dev', ownership: 'PREEXISTING', intent: 'CREATE', definitionKey: 'cluster-lab', driverType: 'existing-cluster', driverAccount: 'lab', accountVersion: 4 }]} />);
    expect(screen.getByText(/Có sẵn \(không bao giờ bị tạo hoặc xóa\)/)).toBeInTheDocument();
    expect(screen.getByText(/existing-cluster · tài khoản lab v4/)).toBeInTheDocument();
    expect(rendererText({ definitionKey: 'workload-gold', class: 'gold', driverType: 'score-k8s', bundle: { version: '0.15.0' } })).toBe('score-k8s 0.15.0 (mẫu workload-gold, class gold)');
  });
});
