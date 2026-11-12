import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { DefinitionVersions } from './DefinitionVersions';
import { EnvironmentDefinitionVersions } from './EnvironmentDefinitionVersions';
const history = { generation: 2, activeId: 'r1', versions: [
  { id: 'r1', number: 1, definition: { driverType: 'terraform', driverInputs: { value: 1 } }, archived: false, createdAt: '' },
  { id: 'r2', number: 2, definition: { driverType: 'terraform', driverInputs: { value: 2 } }, archived: false, createdAt: '' },
] };
afterEach(() => vi.unstubAllGlobals());
it('shows proposed revisions and compares before explicit promotion without deploying', async () => {
  const requests: { url: string; method: string; body?: string }[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input, init?: RequestInit) => {
    requests.push({ url: String(input), method: init?.method ?? 'GET', body: init?.body as string });
    return Response.json({ history, usage: [] });
  }));
  const user = userEvent.setup();
  render(<DefinitionVersions definitionKey="pg" onClose={() => {}} />);
  expect((await screen.findByText('r2', { selector: 'strong' })).closest('.catalog-entry')).toHaveTextContent('Đề xuất');
  expect(screen.getByText('r1', { selector: 'strong' }).closest('.catalog-entry')).toHaveTextContent('Mặc định');
  await user.click(screen.getAllByRole('button', { name: 'So sánh' })[0]!);
  expect(screen.getByText('driverInputs')).toBeInTheDocument();
  const promote = screen.getAllByRole('button', { name: 'Đặt làm mặc định' });
  await user.click(promote[0]!);
  await waitFor(() => expect(requests.some(r => r.url.endsWith('/r2/promote'))).toBe(true));
  const action = requests.find(r => r.method === 'POST')!;
  expect(JSON.parse(action.body!)).toEqual({ expectedGeneration: 2 });
  expect(requests.every(r => !r.url.includes('/deploy'))).toBe(true);
});
const selection = { revision: 3, environmentVersion: 9, pending: false, appliedVerified: true, definitions: [
  { key: 'pg', resourceType: 'postgres', desiredRevisionId: 'r1', appliedRevisionId: 'r1', versions: [
    { id: 'r1', number: 1, active: true, archived: false, changes: [] },
    { id: 'r2', number: 2, active: false, archived: false, changes: ['driverInputs.values.source'] },
  ] },
] };
it('requires explicit Environment selection, shows differences and only saves desired state', async () => {
  const writes: { url: string; body: unknown }[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input, init?: RequestInit) => {
    if (init?.method) writes.push({ url: String(input), body: JSON.parse(String(init.body)) });
    return Response.json(selection);
  }));
  const user = userEvent.setup();
  render(<EnvironmentDefinitionVersions applicationId="shop" environment="staging" busy={false} />);
  expect(vi.mocked(fetch)).not.toHaveBeenCalled();
  await user.click(screen.getByText('Phiên bản tài nguyên'));
  const select = await screen.findByRole('combobox', { name: 'Phiên bản cho pg' });
  expect(select).toHaveValue('r1');
  await user.selectOptions(select, 'r2');
  expect(screen.getByText('driverInputs.values.source')).toBeInTheDocument();
  expect(screen.getByText(/Nâng cấp database/)).toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Chọn để nâng cấp' }));
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]?.body).toEqual({ definitionKey: 'pg', revisionId: 'r2', expectedRevision: 3, expectedEnvironmentVersion: 9 });
  expect(writes[0]?.url).toContain('/shop/environments/staging/definition-versions');
  expect(await screen.findByText('Đã lưu lựa chọn. Chưa triển khai.')).toBeInTheDocument();
});
it('requires reload after an uncertain or stale save and keeps provider configuration private', async () => {
  vi.stubGlobal('fetch', vi.fn(async (_input, init?: RequestInit) => init?.method ? Response.json({ code: 'STALE_VERSION' }, { status: 409 }) : Response.json(selection)));
  const user = userEvent.setup();
  render(<EnvironmentDefinitionVersions applicationId="shop" environment="production" busy={false} />);
  await user.click(screen.getByText('Phiên bản tài nguyên'));
  await user.selectOptions(await screen.findByRole('combobox', { name: 'Phiên bản cho pg' }), 'r2');
  await user.click(screen.getByRole('button', { name: 'Chọn để nâng cấp' }));
  await screen.findByRole('alert');
  expect(screen.getByRole('button', { name: 'Chọn để nâng cấp' })).toBeDisabled();
  await user.click(screen.getByRole('button', { name: 'Tải lại' }));
  expect(await screen.findByRole('combobox', { name: 'Phiên bản cho pg' })).toHaveValue('r1');
});
it('ignores a late Environment response after switching scope', async () => {
  let finish: (r: Response) => void = () => {};
  vi.stubGlobal('fetch', vi.fn(async input => String(input).includes('/staging/') ? new Promise<Response>(r => { finish = r; }) : Response.json({ ...selection, definitions: [] })));
  const view = render(<EnvironmentDefinitionVersions applicationId="shop" environment="staging" busy={false} />);
  const user = userEvent.setup();
  await user.click(screen.getByText('Phiên bản tài nguyên'));
  await waitFor(() => expect(fetch).toHaveBeenCalled());
  view.rerender(<EnvironmentDefinitionVersions applicationId="shop" environment="production" busy={false} />);
  expect(await screen.findByText('Chưa có phiên bản được ghi nhận.')).toBeInTheDocument();
  finish(Response.json(selection));
  await waitFor(() => expect(screen.queryByRole('combobox')).not.toBeInTheDocument());
});
