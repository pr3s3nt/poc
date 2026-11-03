import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { CDKubernetesConnectionDialog } from './CDKubernetesConnectionDialog';

afterEach(() => vi.unstubAllGlobals());

it('registers the inspected kubeconfig context through ordinary accounts and returns the reusable account', async () => {
  const created = { key: 'new-cluster', name: 'New cluster', type: 'INTERNAL', status: 'READY', version: 1 };
  const onCreated = vi.fn();
  const writes: unknown[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    if (String(input).endsWith('/inspect')) return Response.json({ contexts: [{ name: 'chosen-context', cluster: 'cluster-a', endpoint: 'https://cluster-a.invalid' }] });
    writes.push(JSON.parse(String(init?.body)));
    return Response.json(created, { status: 201 });
  }));
  const user = userEvent.setup();
  render(<CDKubernetesConnectionDialog onClose={vi.fn()} onCreated={onCreated} onUncertain={vi.fn()} />);
  await user.type(screen.getByLabelText('Tên tài khoản'), 'New cluster');
  await user.click(screen.getByRole('radio', { name: 'Dán nội dung kubeconfig' }));
  await user.type(screen.getByLabelText('Nội dung kubeconfig'), 'private-kubeconfig');
  await screen.findByText('chosen-context');
  await user.click(screen.getByRole('button', { name: 'Kiểm tra và lưu' }));
  await waitFor(() => expect(onCreated).toHaveBeenCalledWith(created));
  expect(writes).toEqual([{ name: 'New cluster', kubeconfig: 'private-kubeconfig', context: 'chosen-context' }]);
});

it('clears kubeconfig and prevents replay after a lost registration response', async () => {
  const onUncertain = vi.fn();
  let writes = 0;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
    if (String(input).endsWith('/inspect')) return Response.json({ contexts: [{ name: 'chosen-context', cluster: 'cluster-a', endpoint: 'https://cluster-a.invalid' }] });
    writes++;
    throw new TypeError('response lost');
  }));
  const user = userEvent.setup();
  render(<CDKubernetesConnectionDialog onClose={vi.fn()} onCreated={vi.fn()} onUncertain={onUncertain} />);
  await user.type(screen.getByLabelText('Tên tài khoản'), 'New cluster');
  await user.click(screen.getByRole('radio', { name: 'Dán nội dung kubeconfig' }));
  await user.type(screen.getByLabelText('Nội dung kubeconfig'), 'private-kubeconfig');
  await screen.findByText('chosen-context');
  await user.click(screen.getByRole('button', { name: 'Kiểm tra và lưu' }));
  await waitFor(() => expect(onUncertain).toHaveBeenCalledOnce());
  expect(screen.getByLabelText('Nội dung kubeconfig')).toHaveValue('');
  expect(screen.getByRole('button', { name: 'Kiểm tra và lưu' })).toBeDisabled();
  expect(writes).toBe(1);
});
