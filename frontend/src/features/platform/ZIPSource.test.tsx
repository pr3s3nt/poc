import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { ZIPSource } from './ZIPSource';

afterEach(() => vi.unstubAllGlobals());

it('uploads multipart without a JSON header and invalidates stale file/path responses', async () => {
  const user = userEvent.setup();
  let resolve!: (response: Response) => void;
  const fetcher = vi.fn((_url: string, _init?: RequestInit) => new Promise<Response>((done) => { resolve = done; }));
  vi.stubGlobal('fetch', fetcher);
  const result = vi.fn();
  const view = render(<ZIPSource endpoint="/terraform-modules/upload" resourceType="postgres" disabled={false} parseResult={(value) => value} onResult={result} />);
  const button = screen.getByRole('button', { name: 'Tải lên và kiểm tra' });
  expect(button).toBeDisabled();
  await user.upload(screen.getByLabelText('Tệp ZIP'), new File(['zip'], 'modules.zip', { type: 'application/zip' }));
  await user.type(screen.getByLabelText('Thư mục trong ZIP'), 'modules/postgres');
  await user.click(button);
  const [url, init] = fetcher.mock.calls[0]!;
  expect(url).toBe('/api/v1/terraform-modules/upload');
  expect(new Headers(init?.headers).has('Content-Type')).toBe(false);
  const body = init?.body as FormData;
  expect(body.get('resourceType')).toBe('postgres');
  expect(body.get('path')).toBe('modules/postgres');
  expect((body.get('file') as File).name).toBe('modules.zip');
  await user.type(screen.getByLabelText('Thư mục trong ZIP'), '-new');
  resolve(Response.json({ stale: true }));
  await waitFor(() => expect(button).toBeEnabled());
  expect(result.mock.calls.every(([value]) => value === null)).toBe(true);
  expect(screen.queryByText('Đã xác minh ZIP.')).not.toBeInTheDocument();
  await user.click(button);
  view.unmount();
  resolve(Response.json({ stale: true }));
  await Promise.resolve();
  expect(result.mock.calls.every(([value]) => value === null)).toBe(true);
});

it('preserves the file and directory on safe errors and lets explicit retry verify it', async () => {
  const user = userEvent.setup();
  const fetcher = vi.fn().mockResolvedValueOnce(Response.json({ error: 'secret-source.tf' }, { status: 400 })).mockResolvedValueOnce(Response.json({ pin: 'verified' }));
  vi.stubGlobal('fetch', fetcher);
  const result = vi.fn();
  render(<ZIPSource endpoint="/render-bundles/upload" disabled={false} parseResult={(value) => value} onResult={result} />);
  await user.upload(screen.getByLabelText('Tệp ZIP'), new File(['zip'], 'bundle.zip'));
  await user.type(screen.getByLabelText('Thư mục trong ZIP'), 'bundles/standard');
  await user.click(screen.getByRole('button', { name: 'Tải lên và kiểm tra' }));
  expect(await screen.findByRole('alert')).not.toHaveTextContent('secret-source');
  expect(screen.getByLabelText('Thư mục trong ZIP')).toHaveValue('bundles/standard');
  await user.click(screen.getByRole('button', { name: 'Tải lên và kiểm tra' }));
  expect(await screen.findByText('Đã xác minh ZIP.')).toBeInTheDocument();
  expect(result).toHaveBeenLastCalledWith({ pin: 'verified' });
  fireEvent.change(screen.getByLabelText('Tệp ZIP'), { target: { files: [new File(['new'], 'new.zip')] } });
  expect(result).toHaveBeenLastCalledWith(null);
  expect(screen.queryByText('Đã xác minh ZIP.')).not.toBeInTheDocument();
});
