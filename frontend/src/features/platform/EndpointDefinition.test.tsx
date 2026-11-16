import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ResourceDefinitionsPage } from './ResourceDefinitionsPage';
import { endpointFromVariables, endpointToVariables, emptyEndpointDriver } from './EndpointDriverFields';
import { capabilityFixture } from '../../test/capabilities';
import { typeFixture } from '../../test/types';
import { backend, field } from '../../test/definitionKit';

// EP-02: the built-in endpoint driver has literal routing policy, no account and no module.
afterEach(() => vi.unstubAllGlobals());

const endpointType = { key: 'endpoint', inputs: [{ name: 'name', type: 'string', required: true }, { name: 'port', type: 'string', required: true }, { name: 'path', type: 'string', required: false }], outputs: [{ name: 'hostname', type: 'string' }, { name: 'url', type: 'string' }], classes: ['default'] };
const endpointCapability = { resourceType: 'endpoint', driverType: 'endpoint', module: '', variables: [{ name: 'rootDomain', type: 'string', required: true, hasDefault: false, resourceParam: false }], executorOwned: [], bindings: [], account: { required: false }, outputs: [{ name: 'hostname', type: 'string', sensitive: false }, { name: 'url', type: 'string', sensitive: false }] };
const stored = { key: 'public-endpoint', name: 'Public endpoint', version: 2, resourceType: 'endpoint', driverType: 'endpoint', driverInputs: { values: { variables: { rootDomain: 'apps.example.com', scheme: 'http', gatewayName: 'public', gatewayNamespace: 'gateway-system', reservedNames: ['www', 'api'] } } }, provision: {}, criteria: [{ env_type: 'staging' }] };
const section = () => screen.getByRole('region', { name: 'Đăng ký cấu hình tài nguyên' });

async function open(options: Parameters<typeof backend>[0] = {}) {
  const harness = backend({ capabilities: [...capabilityFixture, endpointCapability], types: [...typeFixture, endpointType], ...options });
  render(<ResourceDefinitionsPage />);
  await waitFor(() => expect(screen.getByRole('button', { name: 'Tạo cấu hình' })).toBeEnabled());
  return harness;
}

describe('endpoint Definition in the shared editor', () => {
  it('registers a Gateway reference without duplicating infrastructure policy', async () => {
    const user = userEvent.setup();
    const { posted } = await open();
    await user.click(screen.getByRole('button', { name: 'Tạo cấu hình' }));
    await waitFor(() => expect(screen.getByRole('option', { name: 'endpoint' })).toBeEnabled());
    await user.selectOptions(field('Loại tài nguyên'), 'endpoint');
    expect(screen.queryByRole('region', { name: 'Module Terraform' })).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Tài khoản hạ tầng')).not.toBeInTheDocument();
    await user.type(within(section()).getByLabelText('ID cấu hình (Definition ID)'), 'public-endpoint');
    await user.type(within(section()).getByLabelText('Tên hiển thị'), 'Public endpoint');
    expect(screen.queryByLabelText('Domain gốc')).not.toBeInTheDocument();
    await user.clear(field('Tham chiếu Gateway'));
    await user.type(field('Tham chiếu Gateway'), 'not-a-reference');
    await user.click(within(section()).getByRole('button', { name: 'Đăng ký cấu hình tài nguyên' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('k8s-gateway');
    expect(posted).toEqual([]);
    const reference = "${resources['k8s-gateway.default#environments.@app.@env'].outputs.gateway}";
    await user.clear(field('Tham chiếu Gateway'));
    await user.click(field('Tham chiếu Gateway'));
    await user.paste(reference);
    await user.click(within(section()).getByRole('button', { name: 'Đăng ký cấu hình tài nguyên' }));
    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toEqual({ key: 'public-endpoint', name: 'Public endpoint', resourceType: 'endpoint', driverType: 'endpoint', driverInputs: { values: { variables: { gateway: reference } } }, provision: {}, criteria: [{}] });
  });

  it('edits a stored endpoint policy with its version', async () => {
    const user = userEvent.setup();
    const { puts } = await open({ definitions: [stored] });
    await user.click(await screen.findByRole('button', { name: 'Sửa cấu hình public-endpoint' }));
    expect(field('Domain gốc')).toHaveValue('apps.example.com');
    expect(field('Giao thức')).toHaveValue('http');
    expect(field('Tên đã dùng ngoài hệ thống (tùy chọn)')).toHaveValue('www, api');
    await user.selectOptions(field('Giao thức'), 'https');
    await user.click(screen.getByRole('button', { name: 'Lưu thay đổi' }));
    await waitFor(() => expect(puts).toHaveLength(1));
    expect(puts[0]?.body).toMatchObject({ key: 'public-endpoint', driverType: 'endpoint', version: 2, driverInputs: { values: { variables: { scheme: 'https', rootDomain: 'apps.example.com', reservedNames: ['www', 'api'] } } }, criteria: [{ env_type: 'staging' }] });
  });
});

describe('endpoint policy conversion', () => {
  it('round-trips and rejects invalid labels', () => {
    const draft = endpointFromVariables(stored.driverInputs.values.variables);
    expect(endpointToVariables(draft).variables).toEqual(stored.driverInputs.values.variables);
    expect(endpointToVariables({ ...draft, gatewayName: 'Public' }).problem).toBeTruthy();
    expect(endpointToVariables({ ...draft, reservedNames: 'a.b' }).problem).toBeTruthy();
    expect(endpointToVariables(emptyEndpointDriver()).problem).toBeTruthy();
  });
});

it('registers a separate existing Gateway with its cluster and policy', async () => {
  const user = userEvent.setup();
  const gatewayType = { key: 'k8s-gateway', inputs: [], outputs: [{name:'gateway',type:'any'}],classes:['default'] };
  const gatewayCapability = { ...endpointCapability, resourceType:'k8s-gateway',driverType:'existing-gateway',outputs:[{name:'gateway',type:'any',sensitive:false}] };
  const {posted} = await open({ types:[...typeFixture,endpointType,gatewayType],capabilities:[...capabilityFixture,endpointCapability,gatewayCapability] });
  await user.click(screen.getByRole('button',{name:'Tạo cấu hình'}));
  await user.selectOptions(field('Loại tài nguyên'),'k8s-gateway');
  await user.type(within(section()).getByLabelText('ID cấu hình (Definition ID)'),'existing-edge');
  await user.type(within(section()).getByLabelText('Tên hiển thị'),'Existing edge');
  await user.type(field('Domain gốc'),'apps.example.com');
  await user.type(field('Tên Gateway'),'public');
  await user.type(field('Namespace của Gateway'),'gateway-system');
  expect(screen.queryByRole('region',{name:'Module Terraform'})).not.toBeInTheDocument();
  await user.click(within(section()).getByRole('button',{name:'Đăng ký cấu hình tài nguyên'}));
  await waitFor(()=>expect(posted).toHaveLength(1));
  expect(posted[0]).toMatchObject({ resourceType:'k8s-gateway',driverType:'existing-gateway',driverInputs:{values:{variables:{gatewayName:'public',gatewayNamespace:'gateway-system',cluster:"${resources['k8s-cluster.internal#environments.@app.@env'].outputs.cluster}"}}} });
});
