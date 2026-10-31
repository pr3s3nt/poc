import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { SettingsPage } from './SettingsPage';
import type { Application } from '../../shared/types/application';
import { configuredTarget } from '../../test/targets';
import { messageFor } from '../../shared/api/errors';

const application: Application = { id: 'catalog', name: 'Catalog', subdomain: 'catalog', environments: { staging: configuredTarget( { secretStoreKey: 'vault-a', secretStoreName: 'Vault A' }), production: configuredTarget() }, workloads: { staging: [], production: [] } };
const LABEL = 'Đổi tên và cập nhật tham chiếu';

const view = (over: Record<string, unknown> = {}) => ({
  applicationKey: 'catalog', environmentKey: 'staging', version: 4, environmentVersion: 7, draftVersion: 2,
  keys: [{ name: 'DB_URL', kind: 'VARIABLE', value: 'x', configured: true, usedBy: ['api', 'web'] }, { name: 'TOKEN', kind: 'SECRET', configured: true, usedBy: ['api', 'web'] }], ...over,
});

type Patch = { url: string; body: Record<string, unknown> };

function stub(onPatch: (patch: Patch) => Response | Promise<Response>, gets: () => unknown = () => view()) {
  const patches: Patch[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.includes("/environments/") && url.endsWith("/catalog")) return Response.json({catalog:{revision:0,status:"LEGACY"},environmentVersion:1,releases:[]});
    if (url.endsWith('/delivery-bindings')) return Response.json({ bindings: [] });
    if (url.endsWith('/delivery')) return Response.json({ desired: { mode: 'DIRECT', selfHeal: false }, applied: { mode: 'DIRECT', selfHeal: false }, pending: false, environmentVersion: 1, configurationRevision: 0 });
    if (init?.method === 'PATCH') { const patch = { url, body: JSON.parse(String(init.body)) as Record<string, unknown> }; patches.push(patch); return onPatch(patch); }
    if (url.endsWith('/secret-store-choices')) return Response.json({ secretStores: [] });
    if (url.endsWith('/application-connections')) return Response.json({ connections: [] });
    if (url.includes('/configuration')) return Response.json(gets());
    return Response.json({});
  }));
  return patches;
}

afterEach(() => { vi.unstubAllGlobals(); });

async function openRename(user: ReturnType<typeof userEvent.setup>, row = 0, name = 'DB_URL2') {
  await screen.findByText('DB_URL');
  await user.click(screen.getAllByRole('button', { name: 'Đổi tên' })[row]!);
  await user.clear(screen.getByLabelText('Tên key mới'));
  await user.type(screen.getByLabelText('Tên key mới'), name);
}

it('keeps rename-only by default: unchecked checkbox sends no opt-in or guards', async () => {
  const patches = stub(() => Response.json(view({ version: 5 })));
  const user = userEvent.setup();
  render(<SettingsPage application={application} />);
  await openRename(user);
  const box = screen.getByRole('checkbox', { name: LABEL });
  expect(box).not.toBeChecked();
  await user.click(screen.getByRole('button', { name: 'Tiếp tục' }));
  await waitFor(() => expect(patches).toHaveLength(1));
  expect(patches[0]!.body).toEqual({ newName: 'DB_URL2', version: 4 });
});

it('opt-in sends the same-snapshot guards, shows pending outcome and no rollout claim', async () => {
  const patches = stub(() => Response.json(view({ version: 5, draftVersion: 3, keys: [{ name: 'DB_URL2', kind: 'VARIABLE', value: 'x', configured: true, usedBy: ['api', 'web'] }] })));
  const user = userEvent.setup();
  render(<SettingsPage application={application} />);
  await openRename(user);
  await user.click(screen.getByRole('checkbox', { name: LABEL }));
  expect(screen.getByText(/cập nhật cùng lúc/)).toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Tiếp tục' }));
  await waitFor(() => expect(patches).toHaveLength(1));
  expect(patches[0]!.body).toEqual({ newName: 'DB_URL2', version: 4, updateReferences: true, environmentVersion: 7, draftVersion: 2 });
  const note = await screen.findByText(/Đã đổi tên DB_URL thành DB_URL2/);
  expect(note).toHaveTextContent('api, web');
  expect(note).toHaveTextContent('Chưa có gì được triển khai');
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  await user.click(screen.getAllByRole('button', { name: 'Đổi tên' })[0]!);
  expect(screen.getByRole('checkbox', { name: LABEL })).not.toBeChecked();
});

it('never substitutes missing versions with zero: opt-in is blocked until a full snapshot is loaded', async () => {
  const patches = stub(() => Response.json(view()), () => view({ environmentVersion: undefined, draftVersion: null }));
  const user = userEvent.setup();
  render(<SettingsPage application={application} />);
  await openRename(user);
  await user.click(screen.getByRole('checkbox', { name: LABEL }));
  expect(screen.getByRole('button', { name: 'Tiếp tục' })).toBeDisabled();
  expect(screen.getByText(/Chưa có đủ phiên bản/)).toBeInTheDocument();
  await user.click(screen.getByRole('checkbox', { name: LABEL }));
  expect(screen.getByRole('button', { name: 'Tiếp tục' })).toBeEnabled();
  expect(patches).toHaveLength(0);
});

it.each([
  ['REFERENCE_REWRITE_UNSAFE', 422, 'Không thể tự cập nhật tham chiếu'],
  ['DUPLICATE', 409, 'đã tồn tại'],
  ['ENVIRONMENT_BUSY', 409, 'Môi trường đang bận'],
])('%s keeps the dialog, checkbox and typed name; no replay', async (code, status, text) => {
  const patches = stub(() => Response.json({ error: 'backend sentence must not show', code }, { status }));
  const user = userEvent.setup();
  render(<SettingsPage application={application} />);
  await openRename(user);
  await user.click(screen.getByRole('checkbox', { name: LABEL }));
  await user.click(screen.getByRole('button', { name: 'Tiếp tục' }));
  expect(await screen.findByRole('alert')).toHaveTextContent(text);
  expect(screen.getByRole('alert')).not.toHaveTextContent('backend sentence');
  expect(screen.getByRole('dialog')).toBeInTheDocument();
  expect(screen.getByRole('checkbox', { name: LABEL })).toBeChecked();
  expect(screen.getByLabelText('Tên key mới')).toHaveValue('DB_URL2');
  expect(patches).toHaveLength(1);
});

it('stale conflict reloads the snapshot, keeps edits and requires an explicit resubmit with new guards', async () => {
  let reads = 0;
  const patches = stub(() => Response.json({ error: 's', code: 'STALE_VERSION' }, { status: 409 }), () => { reads += 1; return reads === 1 ? view() : view({ version: 4, environmentVersion: 8, draftVersion: 5 }); });
  const user = userEvent.setup();
  render(<SettingsPage application={application} />);
  await openRename(user);
  await user.click(screen.getByRole('checkbox', { name: LABEL }));
  await user.click(screen.getByRole('button', { name: 'Tiếp tục' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('Đã tải lại giá trị mới nhất');
  expect(screen.getByRole('checkbox', { name: LABEL })).toBeChecked();
  expect(patches).toHaveLength(1);
  await user.click(screen.getByRole('button', { name: 'Tiếp tục' }));
  await waitFor(() => expect(patches).toHaveLength(2));
  expect(patches[1]!.body).toMatchObject({ environmentVersion: 8, draftVersion: 5 });
});

it('an unknown outcome after an opt-in rename demands a reload and is never replayed', async () => {
  let fail = true;
  const patches = stub(() => { throw new TypeError('network'); }, () => { if (!fail) return view(); return view(); });
  const user = userEvent.setup();
  render(<SettingsPage application={application} />);
  await openRename(user);
  await user.click(screen.getByRole('checkbox', { name: LABEL }));
  await user.click(screen.getByRole('button', { name: 'Tiếp tục' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('Chưa rõ thay đổi đã được lưu hay chưa');
  expect(screen.getByRole('button', { name: 'Tiếp tục' })).toBeDisabled();
  fail = false;
  await user.click(screen.getByRole('button', { name: 'Tải lại cấu hình' }));
  await waitFor(() => expect(screen.getByRole('button', { name: 'Tiếp tục' })).toBeEnabled());
  expect(patches).toHaveLength(1);
});

it('a late reply from another Environment neither closes the new dialog nor shows its notice', async () => {
  let release: (response: Response) => void = () => undefined;
  const patches = stub(() => new Promise<Response>((resolve) => { release = resolve; }), () => view());
  const user = userEvent.setup();
  render(<SettingsPage application={application} />);
  await openRename(user);
  await user.click(screen.getByRole('checkbox', { name: LABEL }));
  await user.click(screen.getByRole('button', { name: 'Tiếp tục' }));
  await waitFor(() => expect(patches).toHaveLength(1));
  await user.click(screen.getByRole('tab', { name: 'Production' }));
  await screen.findByText('DB_URL');
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  release(Response.json(view({ version: 5 })));
  await new Promise((resolve) => setTimeout(resolve, 20));
  expect(screen.queryByText(/Đã đổi tên/)).not.toBeInTheDocument();
  await user.click(screen.getAllByRole('button', { name: 'Đổi tên' })[0]!);
  expect(screen.getByRole('checkbox', { name: LABEL })).not.toBeChecked();
});

it('secret rename is explicit too and never shows secret values', async () => {
  const patches = stub(() => Response.json(view({ version: 5 })));
  const user = userEvent.setup();
  render(<SettingsPage application={application} />);
  await screen.findByText('TOKEN');
  await user.click(screen.getAllByRole('button', { name: 'Đổi tên' })[1]!);
  await user.clear(screen.getByLabelText('Tên key mới'));
  await user.type(screen.getByLabelText('Tên key mới'), 'API_TOKEN');
  await user.click(screen.getByRole('checkbox', { name: LABEL }));
  await user.click(screen.getByRole('button', { name: 'Tiếp tục' }));
  await waitFor(() => expect(patches).toHaveLength(1));
  expect(patches[0]!.url).toContain('/keys/TOKEN');
  expect(JSON.stringify(patches[0]!.body)).not.toMatch(/secret|value/i);
});

it('delete has no reference checkbox', async () => {
  stub(() => Response.json(view()));
  const user = userEvent.setup();
  render(<SettingsPage application={application} />);
  await screen.findByText('DB_URL');
  await user.click(screen.getAllByRole('button', { name: 'Xóa' })[0]!);
  expect(screen.queryByRole('checkbox', { name: LABEL })).not.toBeInTheDocument();
});

it('maps REFERENCE_REWRITE_UNSAFE to fixed Vietnamese text', () => {
  const text = messageFor(422, 'REFERENCE_REWRITE_UNSAFE');
  expect(text).toContain('Không thể tự cập nhật tham chiếu');
  expect(text).not.toMatch(/[a-z]+\.[a-z]+\(/);
});

it('Preview CTA after a production rename opens production, never silently staging', async () => {
  const urls: string[] = [];
  const patches = stub(() => Response.json(view({ environmentKey: 'production', version: 5, keys: [{ name: 'DB_URL2', kind: 'VARIABLE', value: 'x', configured: true, usedBy: ['api'] }] })));
  const inner = globalThis.fetch;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => { urls.push(String(input)); return inner(input, init); }));
  const user = userEvent.setup();
  render(<SettingsPage application={application} initialEnvironment="production" />);
  await openRename(user);
  await user.click(screen.getByRole('checkbox', { name: LABEL }));
  await user.click(screen.getByRole('button', { name: 'Tiếp tục' }));
  await user.click(await screen.findByRole('button', { name: 'Đến Xem trước thay đổi' }));
  expect(patches).toHaveLength(1);
  expect(urls.some((url) => url.includes('/environments/production/configuration/keys/'))).toBe(true);
  expect(window.location.pathname + window.location.search).toBe('/ui/applications/catalog?environment=production');
});

it('after a stale reload the dialog names the current consumers and the success notice uses the reply, keeping name and checkbox', async () => {
  let reads = 0;
  const fresh = (usedBy: string[]) => view({ environmentVersion: 8, draftVersion: 5, keys: [{ name: 'DB_URL', kind: 'VARIABLE', value: 'x', configured: true, usedBy }] });
  let patched = 0;
  const patches = stub(() => {
    patched += 1;
    if (patched === 1) return Response.json({ error: 's', code: 'STALE_VERSION' }, { status: 409 });
    return Response.json(view({ version: 5, keys: [{ name: 'DB_URL2', kind: 'VARIABLE', value: 'x', configured: true, usedBy: ['api', 'web', 'worker'] }] }));
  }, () => { reads += 1; return reads === 1 ? view() : fresh(['api', 'web', 'worker']); });
  const user = userEvent.setup();
  render(<SettingsPage application={application} />);
  await openRename(user);
  expect(screen.getByRole('dialog')).toHaveTextContent('api, web.');
  await user.click(screen.getByRole('checkbox', { name: LABEL }));
  await user.click(screen.getByRole('button', { name: 'Tiếp tục' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('Đã tải lại giá trị mới nhất');
  expect(screen.getByRole('dialog')).toHaveTextContent('api, web, worker');
  expect(screen.getByRole('dialog')).not.toHaveTextContent('api, web.');
  expect(screen.getByLabelText('Tên key mới')).toHaveValue('DB_URL2');
  expect(screen.getByRole('checkbox', { name: LABEL })).toBeChecked();
  await user.click(screen.getByRole('button', { name: 'Tiếp tục' }));
  await waitFor(() => expect(patches).toHaveLength(2));
  expect(patches[1]!.body).toMatchObject({ environmentVersion: 8, draftVersion: 5 });
  const note = await screen.findByText(/Đã đổi tên DB_URL thành DB_URL2/);
  expect(note).toHaveTextContent('Workload bị ảnh hưởng: api, web, worker.');
});
