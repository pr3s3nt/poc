import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ResourceDefinitionsPage } from './ResourceDefinitionsPage';
import { backend, completeDefinition, field, ready, submitDefinition } from '../../test/definitionKit';

afterEach(() => vi.unstubAllGlobals());

// UC-03 V02-03: a Kubernetes module inherits its cluster access from the
// dependency binding. It takes no Driver Account, no redundant Connection
// selector and no account criterion. The provider target is a resource cluster pin.
describe('UC-03 V02 Kubernetes modules take no account', () => {
  it('shows the inherited cluster source instead of an account selector, and sends no account', async () => {
    const user = userEvent.setup();
    const { posted, gets } = backend();
    await ready(<ResourceDefinitionsPage />, render);
    await completeDefinition(user, { id: 'pg-in-cluster', type: 'postgres', module: 'postgres-k8s' });
    expect(screen.queryByLabelText('Tài khoản hạ tầng')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Vùng AWS')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Kết nối riêng cho cluster (tùy chọn)')).not.toBeInTheDocument();
    const section = screen.getByRole('region', { name: 'Tài khoản và nguồn phụ thuộc' });
    expect(section).toHaveTextContent('Module này không chọn tài khoản. Truy cập cluster được kế thừa từ tài nguyên phụ thuộc, không chọn lại tài khoản.');
    expect(section).toHaveTextContent('Nguồn cluster của provider kubernetes: cluster do node EKS cung cấp (class eks).');
    await submitDefinition(user);
    expect(await screen.findByRole('status')).toHaveTextContent('pg-in-cluster');
    expect(posted[0]).not.toHaveProperty('driverAccount');
    expect(posted[0]).not.toHaveProperty('connectionKey');
    expect(gets.some((url) => url.endsWith('/connections'))).toBe(false);
  });

  it('shows the EKS class source for a resource binding and offers no Environment target', async () => {
    const user = userEvent.setup();
    backend();
    await ready(<ResourceDefinitionsPage />, render);
    await completeDefinition(user, { id: 'pg-eks', type: 'postgres', module: 'postgres-k8s', binding: { cluster: 'eks' } });
    expect(screen.getByRole('region', { name: 'Tài khoản và nguồn phụ thuộc' })).toHaveTextContent('Nguồn cluster của provider kubernetes: cluster do node EKS cung cấp (class eks).');
    const target = field('Cluster đích của provider kubernetes');
    expect(within(target).getByRole('option', { name: 'Cluster do node EKS cung cấp (kế thừa từ tài nguyên phụ thuộc)' })).toBeInTheDocument();
    expect(within(target).queryByRole('option', { name: /Môi trường|Environment/i })).not.toBeInTheDocument();
  });

  it('has no account selector for AWS-free Types and does not need the account list to register them', async () => {
    const user = userEvent.setup();
    const { posted } = backend({ accountsFail: true });
    await ready(<ResourceDefinitionsPage />, render);
    await completeDefinition(user, { id: 'ns-no-account', type: 'k8s-namespace' });
    await submitDefinition(user);
    await waitFor(() => expect(posted).toHaveLength(1));
    expect(await screen.findByRole('status')).toHaveTextContent('ns-no-account');
  });
});
