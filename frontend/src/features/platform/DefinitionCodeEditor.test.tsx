import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { stringify } from 'yaml';
import { ResourceDefinitionsPage } from './ResourceDefinitionsPage';
import { backend, field, type Body } from '../../test/definitionKit';
import { capabilityFixture } from '../../test/capabilities';
import { typeFixture } from '../../test/types';
import { editDocument } from './definitionEditDraft';

afterEach(() => vi.unstubAllGlobals());
const policy = { rootDomain: 'apps.example.com', scheme: 'http', gatewayName: 'public', gatewayNamespace: 'gateway-system' };
const endpoint = { key: 'public-endpoint', name: 'Public endpoint', version: 2, resourceType: 'endpoint', driverType: 'endpoint', driverInputs: { values: { variables: policy } }, provision: {}, criteria: [{ env_type: 'staging' }] };
const endpointType = { key: 'endpoint', inputs: [], outputs: [{ name: 'hostname', type: 'string' }, { name: 'url', type: 'string' }], classes: ['default'] };
const endpointCapability = { resourceType: 'endpoint', driverType: 'endpoint', module: '', variables: [], outputs: [{ name: 'hostname', type: 'string', sensitive: false }, { name: 'url', type: 'string', sensitive: false }], executorOwned: [], bindings: [], account: { required: false } };
const gatewayType = { key: 'k8s-gateway', inputs: [], outputs: [{ name: 'gateway', type: 'any', required: true }], classes: ['default', 'private'] };
const gatewayCapability = { ...endpointCapability, resourceType: 'k8s-gateway', driverType: 'echo', outputs: [{ name: 'gateway', type: 'any', sensitive: false }] };
const internalCapability = { resourceType: 'k8s-cluster', driverType: 'existing-cluster', module: '', variables: [], outputs: [{ name: 'cluster', type: 'any', sensitive: false }], executorOwned: [], bindings: [], account: { required: true, type: 'INTERNAL' } };
const internal = { key: 'cluster-lab', name: 'Cluster lab', version: 4, resourceType: 'k8s-cluster', driverType: 'existing-cluster', driverAccount: 'internal-cluster', driverInputs: { values: { variables: {} } }, provision: {}, criteria: [{ class: 'internal' }] };
const templateSource = { id: '11111111-1111-4111-8111-111111111111', name: 'custom-workload', version: 'a'.repeat(12), status: 'AVAILABLE', source: { kind: 'git', url: 'https://example.com/templates.git', path: '.', revision: 'a'.repeat(40), contentSha256: 'a'.repeat(64) }, inputsSchema: { type: 'object', additionalProperties: false, properties: {} }, engineContract: 'template-v1', inputContract: 'orchestrator.dev/template-input/v1', renderer: { type: 'template', version: '1.0.0' } };
const template = { key: 'workload-main', name: 'Workload main', version: 5, resourceType: 'workload', driverType: 'template', driverInputs: { values: { variables: { template_source: templateSource.id, template_inputs: {}, namespace: "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}", resource_references: {} } } }, provision: {}, criteria: [{}] };
const source = capabilityFixture.find(c => c.module === 'vpc')!.source!;
const terraform = { key: 'vpc-main', name: 'VPC main', version: 3, resourceType: 'vpc', driverType: 'terraform', driverAccount: 'aws-account', driverInputs: { values: { source: { source_id: source.sourceId, module_id: source.moduleId, revision: source.revision, subdirectory: source.subdirectory, content_sha256: source.contentSha256 }, variables: { name: '${context.infra.resourceName}', region: 'ap-southeast-1' }, input_mapping: {}, output_mapping: { id: 'id', cidr: 'cidr', subnetIds: 'subnetIds', privateSubnetIds: 'privateSubnetIds' }, provider_bindings: { aws: { target: 'account' } } } }, provision: {}, criteria: [{}] };
type Prepare = (body: Body) => Response | Promise<Response>;
async function open(definition: Body = endpoint, prepare?: Prepare, put?: () => Response | Promise<Response>) {
  const harness = backend({ definitions: [definition], put, types: [...typeFixture, endpointType, gatewayType], capabilities: [...capabilityFixture, endpointCapability, internalCapability, gatewayCapability, { ...endpointCapability, resourceType: 'workload', driverType: 'template' }] });
  const fetch = globalThis.fetch;
  const checks: Body[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    if (String(input).endsWith('/template-sources')) return Response.json({ templateSources: [templateSource] });
    if (String(input).endsWith('/import/prepare')) {
      const body = JSON.parse(String(init?.body)) as Body; checks.push(body);
      return prepare ? prepare(body) : Response.json({ definition: editDocument(body) });
    }
    return fetch(input, init);
  }));
  render(<ResourceDefinitionsPage />);
  const user = userEvent.setup();
  await user.click(await screen.findByRole('button', { name: 'Sửa cấu hình ' + String(definition.key) }));
  return { ...harness, user, checks };
}
const codeTab = () => screen.getByRole('tab', { name: 'YAML/JSON' });
const formTab = () => screen.getByRole('tab', { name: 'Biểu mẫu' });
const codeText = () => screen.getByRole('textbox', { name: 'Nội dung YAML/JSON' });
function writeCode(value: unknown, yaml = false) { fireEvent.change(codeText(), { target: { value: yaml ? stringify(value) : JSON.stringify(value, null, 2) } }); }

it('synchronizes form → YAML → form → code and saves one versioned update', async () => {
  const { user, puts, posted } = await open();
  await user.clear(field('Domain gốc')); await user.type(field('Domain gốc'), 'ui.example.com');
  await user.click(codeTab());
  const fromForm = JSON.parse((codeText() as HTMLTextAreaElement).value);
  expect(fromForm.driverInputs.values.variables.rootDomain).toBe('ui.example.com');
  expect(fromForm.version).toBeUndefined();
  const changed = { ...fromForm, name: 'From YAML', driverInputs: { values: { variables: { ...policy, rootDomain: 'code.example.com' } } } };
  writeCode(changed, true);
  await user.click(formTab());
  await waitFor(() => expect(formTab()).toHaveAttribute('aria-selected', 'true'));
  expect(field('Domain gốc')).toHaveValue('code.example.com');
  expect(field('Tên hiển thị')).toHaveValue('From YAML');
  await user.selectOptions(field('Giao thức'), 'https');
  await user.click(codeTab());
  const roundTrip = JSON.parse((codeText() as HTMLTextAreaElement).value);
  expect(roundTrip.driverInputs.values.variables).toMatchObject({ scheme: 'https', rootDomain: 'code.example.com' });
  await user.click(screen.getByRole('button', { name: 'Lưu thay đổi' }));
  await waitFor(() => expect(puts).toHaveLength(1));
  expect(puts[0]?.body).toMatchObject({ ...roundTrip, version: 2 });
  expect(posted).toHaveLength(0);
  expect(await within(screen.getByRole('tabpanel', { name: 'YAML/JSON' })).findByText(/Đã lưu cấu hình public-endpoint.*3/)).toBeVisible();
  await user.click(formTab());
  await waitFor(() => expect(formTab()).toHaveAttribute('aria-selected', 'true'));
  expect(field('Giao thức')).toHaveValue('https');
});

it.each([internal, terraform, template])('keeps $driverType account and exact pins through a name edit and round-trip', async definition => {
  const { user, puts } = await open(definition);
  await user.click(codeTab());
  const doc = JSON.parse((codeText() as HTMLTextAreaElement).value);
  writeCode({ ...doc, name: 'Renamed in code' });
  await user.click(formTab());
  await waitFor(() => expect(formTab()).toHaveAttribute('aria-selected', 'true'));
  expect(field('Tên hiển thị')).toHaveValue('Renamed in code');
  await user.click(codeTab());
  const roundTrip = JSON.parse((codeText() as HTMLTextAreaElement).value);
  expect(roundTrip.driverInputs).toEqual(definition.driverInputs);
  expect(roundTrip.driverAccount).toBe('driverAccount' in definition ? definition.driverAccount : undefined);
  await user.click(screen.getByRole('button', { name: 'Lưu thay đổi' }));
  await waitFor(() => expect(puts).toHaveLength(1));
  expect(puts[0]?.body).toMatchObject({ ...roundTrip, version: definition.version });
});

it('preserves stored code-only fields when saving a form change', async () => {
  const provision = { 'redis.custom': { params: { nested: { keep: ['a', false] } } } };
  const { user, puts } = await open({ ...endpoint, provision });
  await user.clear(field('Domain gốc')); await user.type(field('Domain gốc'), 'updated.example.com');
  await user.click(screen.getByRole('button', { name: 'Lưu thay đổi' }));
  await waitFor(() => expect(puts).toHaveLength(1));
  expect(puts[0]?.body.provision).toEqual(provision);
  expect(puts[0]?.body).toMatchObject({ driverInputs: { values: { variables: { rootDomain: 'updated.example.com' } } } });
});

it('keeps rejected system fields visible for correction and sends no PUT', async () => {
  const { user, puts, checks } = await open(endpoint, body => Object.hasOwn(body, 'version') ? Response.json({ code: 'INVALID_REQUEST' }, { status: 400 }) : Response.json({ definition: editDocument(body) }));
  await user.click(codeTab());
  const doc = { ...editDocument(endpoint), version: 999 };
  writeCode(doc);
  await user.click(screen.getByRole('button', { name: 'Lưu thay đổi' }));
  expect(await screen.findByRole('alert')).toBeVisible();
  expect(checks[0]).toMatchObject({ version: 999 });
  expect(JSON.parse((codeText() as HTMLTextAreaElement).value)).toEqual(doc);
  expect(puts).toHaveLength(0);
});

it('preserves invalid YAML and refuses changed identity without a mutation', async () => {
  const { user, puts, checks } = await open();
  await user.click(codeTab());
  const invalid = 'key: [broken'; fireEvent.change(codeText(), { target: { value: invalid } });
  await user.click(formTab());
  expect(await screen.findByRole('alert')).toBeVisible();
  expect(codeText()).toHaveValue(invalid); expect(codeTab()).toHaveAttribute('aria-selected', 'true');
  writeCode({ ...editDocument(endpoint), key: 'someone-else' });
  await user.click(screen.getByRole('button', { name: 'Lưu thay đổi' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('ID');
  expect(checks).toHaveLength(0); expect(puts).toHaveLength(0);
});

it('keeps valid code when the form cannot express it and saves the full document', async () => {
  const { user, puts } = await open();
  await user.click(codeTab());
  const doc = { ...editDocument(endpoint), provision: { 'redis.custom': { params: { nested: { keep: ['a', false] } } } } };
  writeCode(doc);
  await user.click(formTab());
  expect(await screen.findByRole('alert')).toHaveTextContent('chưa biểu diễn đầy đủ');
  expect(codeTab()).toHaveAttribute('aria-selected', 'true');
  expect(JSON.parse((codeText() as HTMLTextAreaElement).value)).toEqual(doc);
  await user.click(screen.getByRole('button', { name: 'Lưu thay đổi' }));
  await waitFor(() => expect(puts).toHaveLength(1));
  expect(puts[0]?.body.provision).toEqual(doc.provision);
});

it.each(['stale', 'network', 'uncertain'])('holds a %s code Save and never replays it', async outcome => {
  const { user, puts } = await open(endpoint, undefined, () => {
    if (outcome === 'network') throw new TypeError('connection lost');
    return Response.json({ code: outcome === 'stale' ? 'VERSION_CONFLICT' : 'COMMIT_UNCERTAIN' }, { status: outcome === 'stale' ? 409 : 503 });
  });
  await user.click(codeTab()); writeCode({ ...editDocument(endpoint), name: 'My draft' });
  await user.click(screen.getByRole('button', { name: 'Lưu thay đổi' }));
  await waitFor(() => expect(puts).toHaveLength(1));
  expect(await screen.findByRole('alert', { name: 'Cần tải lại cấu hình' })).toBeVisible();
  expect(codeText()).toHaveValue(JSON.stringify({ ...editDocument(endpoint), name: 'My draft' }, null, 2));
  expect(screen.getByRole('button', { name: 'Lưu thay đổi' })).toBeDisabled();
  expect(puts).toHaveLength(1);
});

it('saves the exact checked Git pin without resolving the authored branch again', async () => {
  let resolutions = 0;
  const { user, checks, puts } = await open(terraform, body => {
    const input = body.driverInputs as { values: { source: Record<string, unknown> } };
    if (input.values.source.ref) resolutions++;
    return Response.json({ definition: { ...body, driverInputs: terraform.driverInputs }, source: { url: 'https://example.com/modules.git', path: 'vpc', revision: source.revision } });
  });
  await user.click(codeTab());
  const authored = { ...editDocument(terraform), name: 'Pinned branch', driverInputs: { values: { ...terraform.driverInputs.values, source: { url: 'https://example.com/modules.git', path: 'vpc', ref: 'feature' } } } };
  writeCode(authored);
  await user.click(screen.getByRole('button', { name: 'Kiểm tra cấu hình' }));
  await waitFor(() => expect(screen.getByRole('button', { name: 'Kiểm tra cấu hình' })).toBeEnabled());
  expect(resolutions).toBe(1);
  await user.click(screen.getByRole('button', { name: 'Lưu thay đổi' }));
  await waitFor(() => expect(puts).toHaveLength(1));
  expect(checks).toHaveLength(1); expect(resolutions).toBe(1);
  expect(puts[0]?.body.driverInputs).toEqual(terraform.driverInputs);
});

it('discards a preparation reply after the editor unmounts', async () => {
  let finish: (r: Response) => void = () => {};
  const { user, puts } = await open(endpoint, () => new Promise<Response>(resolve => { finish = resolve; }));
  await user.click(codeTab());
  await user.click(screen.getByRole('button', { name: 'Kiểm tra cấu hình' }));
  // Unmounting a route invalidates the old generation before its reply arrives.
  const { cleanup } = await import('@testing-library/react'); cleanup();
  await act(async () => finish(Response.json({ definition: editDocument(endpoint) })));
  expect(puts).toHaveLength(0);
});

it.each(['echo', 'existing-gateway'])('synchronizes the %s Gateway class and code without changing its driver/layout', async driverType => {
  const userDefinition = { key: 'shared-gateway', name: 'Shared Gateway', version: 6, resourceType: 'k8s-gateway', driverType,
    driverInputs: { values: { variables: driverType === 'echo' ? { gateway: { ...policy, cluster: "${resources['k8s-cluster.internal#environments.@app.@env'].outputs.cluster}" } } : { ...policy, cluster: "${resources['k8s-cluster.internal#environments.@app.@env'].outputs.cluster}" } } }, provision: {}, criteria: [{}] };
  const { user, puts } = await open(userDefinition);
  expect(field('Nhóm cluster')).toHaveValue('internal');
  await user.click(codeTab());
  const doc = JSON.parse((codeText() as HTMLTextAreaElement).value);
  expect(doc.driverType).toBe(driverType);
  const metadata = driverType === 'echo' ? doc.driverInputs.values.variables.gateway : doc.driverInputs.values.variables;
  const reference = "${resources['k8s-cluster.eks#environments.@app.@env'].outputs.cluster}";
  metadata.cluster = reference;
  writeCode(doc, true);
  await user.click(formTab());
  await waitFor(() => expect(formTab()).toHaveAttribute('aria-selected', 'true'));
  expect(field('Nhóm cluster')).toHaveValue('eks');
  expect(field('Tên Gateway')).toHaveValue(policy.gatewayName);
  await user.clear(field('Domain gốc')); await user.type(field('Domain gốc'), 'changed.example.com');
  await user.click(codeTab());
  const roundTrip = JSON.parse((codeText() as HTMLTextAreaElement).value);
  const roundTripMetadata = driverType === 'echo' ? roundTrip.driverInputs.values.variables.gateway : roundTrip.driverInputs.values.variables;
  expect(roundTripMetadata).toMatchObject({ cluster: reference, rootDomain: 'changed.example.com' });
  expect(roundTrip.driverType).toBe(driverType);
  await user.click(screen.getByRole('button', { name: 'Lưu thay đổi' }));
  await waitFor(() => expect(puts).toHaveLength(1));
  expect(puts[0]?.body).toMatchObject({ ...roundTrip, version: 6 });
});

it('synchronizes a Gateway-bound endpoint class and code without duplicating Gateway policy', async () => {
  const reference = "${resources['k8s-gateway.default#environments.@app.@env'].outputs.gateway}";
  const { user, puts } = await open({ ...endpoint, driverInputs: { values: { variables: { gateway: reference } } } });
  expect(field('Nhóm Gateway')).toHaveValue('default');
  expect(screen.queryByLabelText('Domain gốc')).not.toBeInTheDocument();
  await user.selectOptions(field('Nhóm Gateway'), 'private');
  await user.click(codeTab());
  const doc = JSON.parse((codeText() as HTMLTextAreaElement).value);
  expect(doc.driverInputs.values.variables).toEqual({ gateway: "${resources['k8s-gateway.private#environments.@app.@env'].outputs.gateway}" });
  writeCode({ ...doc, driverInputs: { values: { variables: { gateway: reference } } } }, true);
  await user.click(formTab());
  await waitFor(() => expect(formTab()).toHaveAttribute('aria-selected', 'true'));
  expect(field('Nhóm Gateway')).toHaveValue('default');
  await user.click(screen.getByRole('button', { name: 'Lưu thay đổi' }));
  await waitFor(() => expect(puts).toHaveLength(1));
  expect(puts[0]?.body).toMatchObject({ driverType: 'endpoint', version: 2, driverInputs: { values: { variables: { gateway: reference } } } });
});
