import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { GitModuleSource } from './GitModuleSource';
import { ResourceDefinitionsPage } from './ResourceDefinitionsPage';
import { backend, field, submitDefinition } from '../../test/definitionKit';
import { capabilityFixture } from '../../test/capabilities';

afterEach(() => vi.unstubAllGlobals());
const capability = () => ({ ...capabilityFixture.find((item) => item.module === 'vpc')!, source: { ...capabilityFixture.find((item) => item.module === 'vpc')!.source!, repositoryUrl: 'https://example.com/modules.git' } });
async function fill(user: ReturnType<typeof userEvent.setup>) {
  await user.type(field('Git HTTPS URL'), 'https://example.com/modules.git');
  await user.type(field('Thư mục module'), 'modules/vpc');
  await user.type(field('Tag hoặc commit'), 'v1.0.0');
}

it('inspects a source without a digest and uses its verified metadata in the existing save form', async () => {
  const user = userEvent.setup();
  const { posted } = backend();
  const original = globalThis.fetch;
  const inspected: unknown[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    if (String(input).endsWith('/terraform-modules/inspect')) {
      inspected.push(JSON.parse(String(init?.body)));
      return Response.json({ capability: capability() });
    }
    return original(input, init);
  }));
  render(<ResourceDefinitionsPage />);
  await screen.findByRole('option', { name: 'vpc' });
  await user.type(field('ID cấu hình (Definition ID)'), 'git-vpc');
  await user.type(field('Tên hiển thị'), 'Git VPC');
  await user.selectOptions(field('Loại tài nguyên'), 'vpc');
  await user.selectOptions(field('Nguồn module'), 'git');
  await fill(user);
  expect(screen.getByRole('button', { name: 'Đăng ký cấu hình tài nguyên' })).toBeDisabled();
  await user.click(screen.getByRole('button', { name: 'Kiểm tra module' }));
  await screen.findByText(/Đã xác minh: v1.0.0/);
  expect(inspected).toEqual([{ resourceType: 'vpc', url: 'https://example.com/modules.git', path: 'modules/vpc', ref: 'v1.0.0' }]);
  await user.selectOptions(field('Tài khoản hạ tầng'), 'aws-account');
  await user.type(field('Vùng AWS'), 'us-east-1');
  await user.click(screen.getByRole('button', { name: 'Ghép đầu ra theo tên trùng' }));
  await user.click(screen.getByRole('button', { name: 'Dùng preset cho name' }));
  await submitDefinition(user);
  await screen.findByText('Đã đăng ký cấu hình tài nguyên git-vpc.');
  expect(posted).toHaveLength(1);
  const values = (posted[0]?.driverInputs as { values: Record<string, unknown> }).values;
  expect(values.source).toMatchObject({ revision: capability().source.revision, content_sha256: capability().source.contentSha256 });
});

it('discards a late inspection after edits, preserves source text on errors and invalidates on Type or mode changes', async () => {
  const user = userEvent.setup();
  let finish!: (response: Response) => void;
  vi.stubGlobal('fetch', vi.fn(() => new Promise<Response>((resolve) => { finish = resolve; })));
  const result = vi.fn();
  const props = { resourceType: 'vpc', active: true, disabled: false, onResult: result };
  const view = render(<GitModuleSource {...props} />);
  await fill(user);
  await user.click(screen.getByRole('button', { name: 'Kiểm tra module' }));
  fireEvent.change(field('Tag hoặc commit'), { target: { value: 'v2.0.0' } });
  finish(Response.json({ capability: capability() }));
  await waitFor(() => expect(screen.queryByText(/Đã xác minh/)).not.toBeInTheDocument());
  expect(result.mock.calls.every(([value]) => value === null)).toBe(true);
  vi.stubGlobal('fetch', vi.fn(async () => Response.json({ error: 'secret diagnostic' }, { status: 400 })));
  await user.click(screen.getByRole('button', { name: 'Kiểm tra module' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('Dữ liệu đã nhập được giữ lại');
  expect(screen.queryByText(/secret diagnostic/)).not.toBeInTheDocument();
  expect(field('Tag hoặc commit')).toHaveValue('v2.0.0');
  view.rerender(<GitModuleSource {...props} resourceType="redis" />);
  expect(result).toHaveBeenLastCalledWith(null);
  view.rerender(<GitModuleSource {...props} active={false} />);
  expect(result).toHaveBeenLastCalledWith(null);
});
