import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ResourceDefinitionsPage } from './ResourceDefinitionsPage';
import { RenderingTemplatesPage } from './RenderingTemplatesPage';
import { parseCapabilities, loadCapabilities } from './capabilities';
import { MetadataError, parseRenderBundles, parseTypeMetadata } from './metadata';
import { capabilityFixture } from '../../test/capabilities';
import { typeFixture } from '../../test/types';
import { backend, completeDefinition, field } from '../../test/definitionKit';

afterEach(() => vi.unstubAllGlobals());

const clone = <T,>(value: T): T => JSON.parse(JSON.stringify(value)) as T;
const vpc = () => clone(capabilityFixture.find((capability) => capability.module === 'vpc'));
const withVpc = (change: (capability: Record<string, unknown>) => void) => { const copy = clone(capabilityFixture) as unknown as Record<string, unknown>[]; const target = copy.find((capability) => capability.module === 'vpc') as Record<string, unknown>; change(target); return { capabilities: copy }; };

describe('strict capability metadata', () => {
  it('accepts the recorded response and treats server-omitted empty arrays as valid', () => {
    expect(parseCapabilities({ capabilities: capabilityFixture })).toHaveLength(capabilityFixture.length);
    const valid = withVpc((capability) => { delete capability.providerRequirements; delete capability.outputs; capability.variables = []; capability.executorOwned = []; });
    expect(parseCapabilities(valid).find((capability) => capability.module === 'vpc')).toMatchObject({ providerRequirements: [], outputs: [], variables: [], executorOwned: [] });
    expect(parseCapabilities({ capabilities: [] })).toEqual([]);
  });

  it.each<[string, unknown]>([
    ['no body', null], ['no capabilities', {}], ['capabilities not an array', { capabilities: {} }],
    ['variables missing', withVpc((c) => { delete c.variables; })], ['variables not an array', withVpc((c) => { c.variables = 'x'; })],
    ['executorOwned missing', withVpc((c) => { delete c.executorOwned; })], ['executorOwned member', withVpc((c) => { c.executorOwned = [3]; })],
    ['providerRequirements not an array', withVpc((c) => { c.providerRequirements = {}; })], ['provider binding unknown', withVpc((c) => { c.providerRequirements = [{ name: 'aws', source: 's', version: '1', binding: 'both' }]; })],
    ['outputs member', withVpc((c) => { c.outputs = [{ name: 'id' }]; })], ['variable without flags', withVpc((c) => { c.variables = [{ name: 'x', type: 'string' }]; })],
    ['null source', withVpc((c) => { c.source = null; })], ['terraform without source', withVpc((c) => { delete c.source; })],
    ['incomplete source', withVpc((c) => { c.source = { sourceId: 'platform', moduleId: 'vpc' }; })],
    ['empty revision', withVpc((c) => { (c.source as Record<string, unknown>).revision = ''; })],
    ['digest shape', withVpc((c) => { (c.source as Record<string, unknown>).contentSha256 = 'not-a-digest'; })],
    ['member not an object', { capabilities: [1] }],
  ])('rejects %s with a payload-free error', (_name, body) => {
    let error: unknown;
    try { parseCapabilities(body); } catch (reason) { error = reason; }
    expect(error).toBeInstanceOf(MetadataError);
    expect(String((error as Error).message)).not.toMatch(/platform|vpc|not-a-digest/);
  });
});

describe('strict Resource Type and bundle metadata', () => {
  it('accepts valid empty arrays, null schemas and rejects the rest', () => {
    expect(parseTypeMetadata({ resourceTypes: [{ key: 'cache', inputs: null, outputs: [], classes: [] }] })).toEqual([{ key: 'cache', inputs: [], outputs: [], classes: [] }]);
    expect(parseTypeMetadata({ resourceTypes: [] })).toEqual([]);
    for (const body of [{}, { resourceTypes: {} }, { resourceTypes: [{ key: 'a', inputs: [], outputs: [] }] }, { resourceTypes: [{ key: 'a', inputs: [], outputs: [], classes: 'x' }] }, { resourceTypes: [{ key: 'a', inputs: [], outputs: [], classes: [1] }] },
      { resourceTypes: [{ key: 'a', inputs: {}, outputs: [], classes: [] }] }, { resourceTypes: [{ key: 'a', inputs: [], outputs: [{ type: 'string' }], classes: [] }] }, { resourceTypes: [{ key: '', inputs: [], outputs: [], classes: [] }] }, { resourceTypes: [{ key: 'a', inputs: [], classes: [] }] }]) {
      expect(() => parseTypeMetadata(body)).toThrow(MetadataError);
    }
    expect(parseRenderBundles({ renderBundles: [] })).toEqual([]);
    for (const body of [{}, { renderBundles: {} }, { renderBundles: [{ id: 'b', version: '1' }] }, { renderBundles: [{ id: '', version: '1', status: 'AVAILABLE' }] }, { renderBundles: [null] }]) expect(() => parseRenderBundles(body)).toThrow(MetadataError);
  });
});

describe('malformed metadata never becomes ready metadata in the Definition form', () => {
  const secretLooking = 'MALFORMED-PAYLOAD-SENTINEL';
  it.each<[string, () => Record<string, unknown>]>([
    ['capabilities without variables', () => ({ capabilities: withVpc((c) => { delete c.variables; c.marker = secretLooking; }).capabilities })],
    ['capabilities with a null pin', () => ({ capabilities: withVpc((c) => { c.source = null; c.marker = secretLooking; }).capabilities })],
    ['capabilities missing', () => ({ marker: secretLooking })],
  ])('%s: a retryable generic error, no registration, edits kept, then valid metadata works', async (_name, malformed) => {
    const user = userEvent.setup();
    const { posted } = backend({ capabilityLists: [() => Response.json(malformed()), () => Response.json({ capabilities: capabilityFixture })] });
    render(<ResourceDefinitionsPage />);
    const alert = await screen.findByRole('alert', { name: 'Lỗi tải metadata module' });
    expect(alert).toHaveTextContent('Không tải được metadata module đã kiểm chứng từ máy chủ.');
    expect(document.body).not.toHaveTextContent(secretLooking);
    expect(screen.getByRole('button', { name: 'Đăng ký cấu hình tài nguyên' })).toBeDisabled();
    await user.type(field('ID cấu hình (Definition ID)'), 'kept-draft');
    expect(screen.queryByRole('option', { name: 'vpc' })).toBeNull();
    await user.click(within(alert).getByRole('button', { name: 'Thử lại' }));
    await waitFor(() => expect(screen.getByRole('option', { name: 'vpc' })).toBeEnabled());
    expect(field('ID cấu hình (Definition ID)')).toHaveValue('kept-draft');
    expect(posted).toEqual([]);
  });

  it.each<[string, unknown]>([
    ['classes missing', { resourceTypes: typeFixture.map((type) => ({ ...type, classes: undefined })) }],
    ['classes not strings', { resourceTypes: typeFixture.map((type) => ({ ...type, classes: [1] })) }],
    ['inputs not an array', { resourceTypes: typeFixture.map((type) => ({ ...type, inputs: {} })) }],
  ])('Resource Type metadata with %s blocks the form without a POST and recovers on retry', async (_name, malformed) => {
    const user = userEvent.setup();
    const { posted } = backend();
    let first = true;
    const original = globalThis.fetch;
    vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      if (String(input).endsWith('/resource-types') && first) { first = false; return Response.json(malformed); }
      return (original as typeof fetch)(input, init);
    }));
    render(<ResourceDefinitionsPage />);
    const alert = await screen.findByRole('alert', { name: 'Lỗi tải loại tài nguyên' });
    expect(screen.getByRole('button', { name: 'Đăng ký cấu hình tài nguyên' })).toBeDisabled();
    await user.type(field('ID cấu hình (Definition ID)'), 'kept');
    await user.click(within(alert).getByRole('button', { name: 'Thử lại' }));
    await waitFor(() => expect(screen.getByRole('option', { name: 'vpc' })).toBeEnabled());
    expect(field('ID cấu hình (Definition ID)')).toHaveValue('kept');
    expect(posted).toEqual([]);
  });

  it('a valid empty capabilities list is not an error: nothing is registrable, and nothing is posted', async () => {
    backend({ capabilities: [] });
    render(<ResourceDefinitionsPage />);
    await screen.findByText('Chưa có cấu hình tài nguyên nào.');
    expect(screen.queryByRole('alert', { name: 'Lỗi tải metadata module' })).toBeNull();
    expect(screen.getByRole('option', { name: 'vpc (chưa có module đã kiểm chứng)' })).toBeDisabled();
  });
});

describe('malformed bundle metadata in the rendering form', () => {
  it.each<[string, unknown]>([['missing list', {}], ['member without status', { renderBundles: [{ id: 'b', version: '1' }] }], ['not an array', { renderBundles: 'x' }]])('%s: generic retryable error, nothing registered', async (_name, malformed) => {
    const user = userEvent.setup();
    const posted: unknown[] = [];
    const bundleReplies = [() => Response.json(malformed), () => Response.json({ renderBundles: [{ id: 'bundle-web', version: '0.15.0', status: 'AVAILABLE' }] })];
    vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      if (init?.method === 'POST') { posted.push(init.body); return Response.json({}, { status: 201 }); }
      if (url.endsWith('/applications')) return Response.json({ applications: [] });
      if (url.endsWith('/render-bundles')) return (bundleReplies.shift() as () => Response)();
      return Response.json({ resourceDefinitions: [] });
    }));
    render(<RenderingTemplatesPage />);
    const alert = await screen.findByRole('alert', { name: 'Lỗi tải bundle' });
    expect(alert).toHaveTextContent('Không tải được danh sách bundle đã cài.');
    expect(screen.getByRole('button', { name: 'Đăng ký mẫu dựng ứng dụng' })).toBeDisabled();
    await user.type(screen.getByLabelText('ID mẫu'), 'kept');
    await user.click(within(alert).getByRole('button', { name: 'Thử lại tải bundle' }));
    await waitFor(() => expect(screen.getByLabelText('Bundle dựng ứng dụng')).toHaveValue('bundle-web'));
    expect(screen.getByLabelText('ID mẫu')).toHaveValue('kept');
    expect(posted).toEqual([]);
  });
});

it('loadCapabilities keeps the strict contract end to end', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => Response.json({ capabilities: [{ resourceType: 'vpc', driverType: 'terraform', module: 'vpc', variables: [], executorOwned: [] }] })));
  await expect(loadCapabilities()).rejects.toBeInstanceOf(MetadataError);
  expect(vpc()).toBeDefined();
  void completeDefinition;
});
