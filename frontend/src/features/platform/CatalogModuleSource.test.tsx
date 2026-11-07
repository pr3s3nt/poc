// Legacy paths stay covered with switches enabled; production defaults have separate coverage.
vi.mock('../../shared/authoringFeatures', () => ({ authoringFeatures: { workloadImport: true, nonGitDefinitionSources: true } }));
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { CatalogModuleSource } from './CatalogModuleSource';
import { ResourceDefinitionsPage } from './ResourceDefinitionsPage';
import { capabilityFixture } from '../../test/capabilities';
import { backend, completeDefinition, field, submitDefinition } from '../../test/definitionKit';

const capability = capabilityFixture.find(c => c.module === 'vpc')!;
const selection = { sourceId: 'company', tag: 'v1.0.0', componentId: 'network' };
afterEach(() => vi.unstubAllGlobals());
function routes(url: string) {
  if (url.endsWith('/sources')) return Response.json({ sources: [{ id: 'company' }, { id: 'other' }] });
  if (url.endsWith('/releases')) return Response.json({ releases: [{ tag: 'v2.0.0' }, { tag: 'v1.0.0' }] });
  return Response.json({ modules: [{ componentId: 'network', capability }] });
}
async function choose(user: ReturnType<typeof userEvent.setup>) {
  await waitFor(() => expect(field('Nguồn Catalog')).toBeEnabled());
  await user.selectOptions(field('Nguồn Catalog'), 'company');
  await waitFor(() => expect(field('Phiên bản Catalog')).toBeEnabled());
  expect(field('Phiên bản Catalog')).toHaveValue('');
  await user.selectOptions(field('Phiên bản Catalog'), 'v1.0.0');
  await waitFor(() => expect(field('Module trong Catalog')).toBeEnabled());
  await user.selectOptions(field('Module trong Catalog'), 'network');
}

describe('Catalog module selection', () => {
  it('requires an explicit version, returns its verified pin and clears selection on version change', async () => {
    vi.stubGlobal('fetch', vi.fn(async url => routes(String(url))));
    const result = vi.fn(); const user = userEvent.setup();
    render(<CatalogModuleSource resourceType="vpc" disabled={false} onResult={result} />);
    await choose(user);
    await waitFor(() => expect(result).toHaveBeenLastCalledWith(capability, selection));
    await waitFor(() => expect(field('Phiên bản Catalog')).toBeEnabled());
    await user.selectOptions(field('Phiên bản Catalog'), 'v2.0.0');
    expect(result).toHaveBeenLastCalledWith(null);
    await waitFor(() => expect(field('Module trong Catalog')).toBeEnabled());
    expect(field('Module trong Catalog')).toHaveValue('');
  });

  it('restores the saved version even when a newer version exists', async () => {
    vi.stubGlobal('fetch', vi.fn(async url => routes(String(url))));
    const result = vi.fn();
    render(<CatalogModuleSource resourceType="vpc" disabled={false} initialSelection={selection} onResult={result} />);
    await waitFor(() => expect(result).toHaveBeenLastCalledWith(capability, selection));
    expect(field('Phiên bản Catalog')).toHaveValue('v1.0.0');
    expect(field('Module trong Catalog')).toHaveValue('network');
  });

  it('shows unavailable/empty modules and retries failed metadata without selecting a pin', async () => {
    let fail = true;
    vi.stubGlobal('fetch', vi.fn(async url => String(url).includes('/modules?') ? fail ? Response.json({}, { status: 503 }) : Response.json({ modules: [] }) : routes(String(url))));
    const result = vi.fn(); const user = userEvent.setup();
    render(<CatalogModuleSource resourceType="vpc" disabled={false} initialSelection={selection} onResult={result} />);
    expect(await screen.findByRole('alert')).toHaveTextContent('Không thể tải module');
    fail = false;
    await user.click(screen.getByRole('button', { name: 'Thử lại' }));
    expect(await screen.findByText('Phiên bản này không có module cho loại tài nguyên đã chọn.')).toBeInTheDocument();
    expect(field('Module trong Catalog')).toBeDisabled();
    expect(result).toHaveBeenLastCalledWith(null);
  });

  it('ignores a late response after the Resource Type changes', async () => {
    let finish: (r: Response) => void = () => {};
    vi.stubGlobal('fetch', vi.fn(async url => String(url).includes('resourceType=vpc') ? new Promise<Response>(resolve => { finish = resolve; }) : String(url).includes('/modules?') ? Response.json({ modules: [] }) : routes(String(url))));
    const result = vi.fn();
    const view = render(<CatalogModuleSource resourceType="vpc" disabled={false} initialSelection={selection} onResult={result} />);
    await waitFor(() => expect(vi.mocked(fetch).mock.calls.some(c => String(c[0]).includes('resourceType=vpc'))).toBe(true));
    view.rerender(<CatalogModuleSource resourceType="postgres" disabled={false} initialSelection={selection} onResult={result} />);
    await screen.findByText('Phiên bản này không có module cho loại tài nguyên đã chọn.');
    finish(Response.json({ modules: [{ componentId: 'network', capability }] }));
    await waitFor(() => expect(field('Module trong Catalog')).toBeDisabled());
    expect(result.mock.calls.every(c => c[0] === null)).toBe(true);
  });

  it('saves a Definition with Catalog provenance, the exact pin and the existing form configuration', async () => {
    const { posted, state } = backend();
    const fallback = globalThis.fetch;
    vi.stubGlobal('fetch', vi.fn(async (input, init) => String(input).includes('/platform/catalog/') ? routes(String(input)) : fallback(input, init)));
    const user = userEvent.setup();
    const view = render(<ResourceDefinitionsPage initialResourceType="vpc" />);
    await waitFor(() => expect(field('Loại tài nguyên')).toHaveValue('vpc'));
    await completeDefinition(user, { id: 'catalog-network', type: 'vpc' });
    await user.click(screen.getByRole('button', { name: 'Catalog' }));
    await choose(user);
    await waitFor(() => expect(screen.getByRole('button', { name: 'Đăng ký cấu hình tài nguyên' })).toBeEnabled());
    await submitDefinition(user);
    await waitFor(() => expect(posted).toHaveLength(1));
    const inputs = posted[0]!.driverInputs as { catalog: unknown; values: { source: Record<string, string> } };
    expect(inputs.catalog).toEqual(selection);
    expect(inputs.values.source.revision).toBe(capability.source?.revision);
    expect(inputs.values.source.content_sha256).toBe(capability.source?.contentSha256);
    view.unmount();
    state.definitions = [{ ...posted[0]!, version: 1 }];
    render(<ResourceDefinitionsPage />);
    await user.click(await screen.findByRole('button', { name: 'Sửa cấu hình catalog-network' }));
    const summary=screen.getByLabelText('Nguồn đã lưu');
    expect(summary).toHaveTextContent('Catalog');expect(summary).toHaveTextContent('v1.0.0');
    expect(screen.queryByLabelText('Module trong Catalog')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button',{name:'Đổi nguồn'}));
    await waitFor(() => expect(field('Module trong Catalog')).toHaveValue('network'));
    expect(field('Phiên bản Catalog')).toHaveValue('v1.0.0');

  });
});
