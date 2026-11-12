import { afterEach, expect, it, vi } from 'vitest';
import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { CatalogDocumentImport } from './CatalogDocumentImport';
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
it('retains a conflict draft, reports field paths and never reloads as a write', async () => {
  const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify({ status: 'CONFLICT', fields: ['name'] }), { status: 200 }));
  vi.stubGlobal('fetch', fetch);
  const reload = vi.fn(); const user = userEvent.setup();
  render(<CatalogDocumentImport kind="resource-definitions" onRegistered={reload} />);
  await user.click(screen.getByText('Nhập YAML/JSON'));
  const editor = screen.getByLabelText('Nội dung YAML/JSON', { exact: true });
  await user.type(editor, 'key: example'); await user.click(screen.getByRole('button', { name: 'Đăng ký' }));
  expect(await screen.findByRole('status')).toHaveTextContent('CONFLICT: name');
  expect(editor).toHaveValue('key: example'); expect(reload).not.toHaveBeenCalled();
  expect(fetch.mock.calls[0]?.[1].body).toBe('{"key":"example"}');
});
it('rejects duplicate keys before making a request and preserves editable content', async () => {
  const fetch = vi.fn(); vi.stubGlobal('fetch', fetch); const user = userEvent.setup();
  render(<CatalogDocumentImport kind="resource-types" onRegistered={vi.fn()} />);
  await user.click(screen.getByText('Nhập YAML/JSON'));
  const editor = screen.getByLabelText('Nội dung YAML/JSON', { exact: true });
  await user.type(editor, 'key: a\nkey: b'); await user.click(screen.getByRole('button', { name: 'Đăng ký' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('key trùng');
  expect(editor).toHaveValue('key: a\nkey: b'); expect(fetch).not.toHaveBeenCalled();
});
