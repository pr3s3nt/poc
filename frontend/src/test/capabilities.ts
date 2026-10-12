import type { Capability, CapabilityVariable } from '../features/platform/capabilities';

const v = (name: string, type: string, extra: Partial<CapabilityVariable> = {}): CapabilityVariable => ({ name, type, required: false, hasDefault: false, resourceParam: false, ...extra });
const infra = "${resources['vpc.default#@infra'].outputs";

// Mirrors GET /resource-definition-capabilities of the seeded backend (no secrets, no executor-owned variables).
export const capabilityFixture: Capability[] = [
  { resourceType: 'k8s-cluster', driverType: 'terraform', module: 'eks', executorOwned: ['region', 'tags'], variables: [
    v('kubernetes_version', 'string', { required: true }), v('name', 'string', { required: true, preset: '${context.infra.resourceName}' }), v('node_ami_type', 'string', { required: true }),
    v('node_capacity_type', 'string', { required: true }), v('node_disk_size', 'number', { required: true }), v('node_instance_type', 'string', { required: true }),
    v('subnet_ids', 'list(string)', { required: true, preset: `${infra}.subnetIds}` })] },
  { resourceType: 'k8s-namespace', driverType: 'kubernetes', module: '', executorOwned: [], variables: [v('name', 'string', { required: true, preset: '${context.env.namespace}' })] },
  { resourceType: 'postgres', driverType: 'kubernetes', module: '', executorOwned: [], variables: [
    v('database', 'string', { hasDefault: true, default: 'app', resourceParam: true }), v('image', 'string', { hasDefault: true, default: 'postgres:16-alpine' }),
    v('namespace', 'string'), v('storage', 'string', { hasDefault: true, default: '1Gi' }), v('username', 'string', { hasDefault: true, default: 'app', resourceParam: true })] },
  { resourceType: 'postgres', driverType: 'terraform', module: 'aurora', executorOwned: ['master_password', 'region', 'tags'], variables: [
    v('database', 'string', { required: true, resourceParam: true }), v('engine_version', 'string', { required: true }), v('max_capacity', 'number', { required: true }), v('min_capacity', 'number', { required: true }),
    v('name', 'string', { required: true, preset: '${context.infra.resourceName}' }), v('seconds_until_auto_pause', 'number', { hasDefault: true, default: 3600 }),
    v('subnet_ids', 'list(string)', { required: true, preset: `${infra}.subnetIds}` }), v('username', 'string', { required: true, resourceParam: true }),
    v('vpc_cidr', 'string', { required: true, preset: `${infra}.cidr}` }), v('vpc_id', 'string', { required: true, preset: `${infra}.id}` })] },
  { resourceType: 'vpc', driverType: 'terraform', module: 'vpc', executorOwned: ['region', 'tags'], variables: [
    v('az_count', 'number', { hasDefault: true, default: 2 }), v('cidr', 'string', { hasDefault: true, default: '10.42.0.0/16' }), v('name', 'string', { required: true, preset: '${context.infra.resourceName}' })] },
];
