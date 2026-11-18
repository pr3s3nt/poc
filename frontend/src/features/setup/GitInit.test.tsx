import { render, screen } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { GitInit } from './GitInit';

afterEach(() => vi.unstubAllGlobals());
it('previews before confirmation, imports Types first, and invalidates review on edit', async () => {
  const calls: string[] = [];
  vi.stubGlobal('fetch', vi.fn(async (url: string) => {
    calls.push(url);
    if (url.endsWith('/discover')) return Response.json({ url: 'https://git.example.com/platform', revision: 'a'.repeat(40), documents: [
      { file: 'definitions/edge.yaml', kind: 'resource-definitions', content: { key: 'edge', resourceType: 'endpoint' } },
      { file: 'types/edge.yaml', kind: 'resource-types', content: { key: 'endpoint', outputs: [] } },
    ] });
    if (url.endsWith('/preview')) return Response.json({ documents: [ { file: 'definitions/edge.yaml', definition: { key: 'edge' }, status: 'NEW' }, { file: 'types/edge.yaml', type: { key: 'endpoint' }, status: 'NEW' } ] });
    return Response.json({ status: 'CREATED' });
  }));
  const user = userEvent.setup();
  render(<GitInit onImported={vi.fn()} />);
  await user.type(screen.getByLabelText('URL repository Git'), 'https://git.example.com/platform');
  await user.type(screen.getByLabelText('Branch, tag hoặc commit'), 'main');
  await user.click(screen.getByRole('button', { name: 'Đọc repository' }));
  await user.click(await screen.findByRole('button', { name: 'Kiểm tra trước khi nhập' }));
  expect(await screen.findByRole('button', { name: 'Xác nhận nhập tài nguyên' })).toBeEnabled();
  expect(calls.some(url => url.endsWith('/import'))).toBe(false);
  await user.click(screen.getByRole('button', { name: 'Xác nhận nhập tài nguyên' }));
  await screen.findAllByText('Đã tạo');
  expect(calls.filter(url => url.endsWith('/import'))).toEqual(['/api/v1/resource-types/import', '/api/v1/resource-definitions/import']);
  await user.click(screen.getAllByRole('checkbox')[0]!);
  expect(screen.queryByRole('button', { name: 'Xác nhận nhập tài nguyên' })).not.toBeInTheDocument();
});
it('keeps conflict review visible and blocks confirmation', async () => {
  vi.stubGlobal('fetch', vi.fn(async (url: string) => Response.json(url.endsWith('/discover') ? { url: 'https://git.example.com/platform', revision: 'b'.repeat(40), documents: [{ file: 'type.yaml', kind: 'resource-types', content: { key: 'queue', outputs: [] } }] } : { documents: [{ file: 'type.yaml', type: { key: 'queue' }, status: 'CONFLICT', fields: ['outputs'] }] })));
  const user = userEvent.setup(); render(<GitInit onImported={vi.fn()} />);
  await user.type(screen.getByLabelText('URL repository Git'), 'https://git.example.com/platform');
  await user.type(screen.getByLabelText('Branch, tag hoặc commit'), 'main');
  await user.click(screen.getByRole('button', { name: 'Đọc repository' }));
  await user.click(await screen.findByRole('button', { name: 'Kiểm tra trước khi nhập' }));
  expect(await screen.findByRole('button', { name: 'Xác nhận nhập tài nguyên' })).toBeDisabled();
  expect(screen.getByText('Xung đột')).toBeInTheDocument();
});
