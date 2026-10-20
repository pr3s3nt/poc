import { api } from '../../shared/api/client';
import { fail, filled, flag, isObject, members, text } from './metadata';

// existing-cluster and score-k8s are platform pairs without a Terraform module pin (ADR-016).
export type Driver = 'terraform' | 'kubernetes' | 'existing-cluster' | 'score-k8s';

// Public, secret-free metadata of GET /resource-definition-capabilities (UC-03 BR-23, OC-04d).
export type CapabilityVariable = {
  name: string;
  type: string;
  required: boolean;
  hasDefault: boolean;
  default?: unknown;
  resourceParam: boolean;
  preset?: string;
};
// The exact verified execution pin, including an optional safe Git origin.
export type CapabilitySource = { sourceId: string; moduleId: string; revision: string; subdirectory: string; contentSha256: string; repositoryUrl?: string };
export type CapabilityProvider = { name: string; source: string; version: string; binding: 'aws' | 'kubernetes' | 'none' };
export type CapabilityOutput = { name: string; type?: string; sensitive: boolean };
// Account requirement and provider binding targets the server publishes for a
// pair (UC-03 V02-03): the Console never guesses a driver's account needs.
export type CapabilityAccount = { required: boolean; type?: string; regionVariable?: string };
export type CapabilityBinding = { provider: string; kind: 'aws' | 'kubernetes'; targets: { target: 'account' | 'resource' }[] };
export type Capability = {
  resourceType: string;
  driverType: Driver;
  module: string;
  source?: CapabilitySource;
  terraformVersion?: string;
  providerRequirements: CapabilityProvider[];
  outputs: CapabilityOutput[];
  variables: CapabilityVariable[];
  executorOwned: string[];
  account: CapabilityAccount;
  bindings: CapabilityBinding[];
};

const binding = (value: unknown): value is CapabilityProvider['binding'] => value === 'aws' || value === 'kubernetes' || value === 'none';

function variable(item: unknown): CapabilityVariable {
  if (!isObject(item) || !filled(item.name) || !text(item.type) || !flag(item.required) || !flag(item.hasDefault) || !flag(item.resourceParam) || (item.preset !== undefined && !text(item.preset))) return fail();
  return { name: item.name, type: item.type, required: item.required, hasDefault: item.hasDefault, resourceParam: item.resourceParam, ...(item.default !== undefined ? { default: item.default } : {}), ...(item.preset !== undefined ? { preset: item.preset } : {}) };
}
const targetNames = ['account', 'resource'] as const;
function account(value: unknown): CapabilityAccount {
  if (!isObject(value) || !flag(value.required) || (value.type !== undefined && !text(value.type)) || (value.regionVariable !== undefined && !text(value.regionVariable))) return fail();
  if (value.required && (!filled(value.type) || (value.type === 'AWS' && !filled(value.regionVariable)))) return fail();
  return { required: value.required, ...(value.type !== undefined ? { type: value.type } : {}), ...(value.regionVariable !== undefined ? { regionVariable: value.regionVariable } : {}) };
}
function bindingMeta(item: unknown): CapabilityBinding {
  if (!isObject(item) || !filled(item.provider) || (item.kind !== 'aws' && item.kind !== 'kubernetes')) return fail();
  const targets = members(item.targets, (entry) => {
    if (!isObject(entry) || typeof entry.target !== 'string' || !(targetNames as readonly string[]).includes(entry.target) || entry.legacy !== undefined) return fail();
    return { target: entry.target as CapabilityBinding['targets'][number]['target'] };
  });
  return { provider: item.provider, kind: item.kind, targets };
}
const pin = /^[0-9a-f]{64}$/;
function source(value: unknown): CapabilitySource {
  if (!isObject(value) || !filled(value.sourceId) || !filled(value.moduleId) || !filled(value.revision) || !filled(value.subdirectory) || !text(value.contentSha256) || !pin.test(value.contentSha256)) return fail();
  return { sourceId: value.sourceId, moduleId: value.moduleId, revision: value.revision, subdirectory: value.subdirectory, contentSha256: value.contentSha256, ...(value.repositoryUrl !== undefined ? { repositoryUrl: filled(value.repositoryUrl) ? value.repositoryUrl : fail() } : {}) };
}

// Reads GET /resource-definition-capabilities strictly. `variables`, `executorOwned`, `account` and `bindings` are required; `providerRequirements` and `outputs` are omitted by the server when empty. A Terraform pair
// must carry a complete module pin: anything else makes the whole metadata unavailable.
export function parseCapabilities(body: unknown): Capability[] {
  if (!isObject(body)) return fail();
  return members(body.capabilities, (item) => {
    if (!isObject(item) || !filled(item.resourceType) || !filled(item.driverType) || !text(item.module)) return fail();
    if (item.terraformVersion !== undefined && !text(item.terraformVersion)) return fail();
    const driverType = item.driverType as Driver;
    if (driverType === 'terraform' && item.source === undefined) return fail();
    return {
      resourceType: item.resourceType, driverType, module: item.module,
      ...(item.source !== undefined ? { source: source(item.source) } : {}),
      ...(item.terraformVersion !== undefined ? { terraformVersion: item.terraformVersion } : {}),
      providerRequirements: members(item.providerRequirements, (provider) => (isObject(provider) && filled(provider.name) && text(provider.source) && text(provider.version) && binding(provider.binding) ? { name: provider.name, source: provider.source, version: provider.version, binding: provider.binding } : fail()), true),
      outputs: members(item.outputs, (output) => (isObject(output) && filled(output.name) && flag(output.sensitive) && (output.type === undefined || text(output.type)) ? { name: output.name, sensitive: output.sensitive, ...(output.type !== undefined ? { type: output.type } : {}) } : fail()), true),
      variables: members(item.variables, variable),
      executorOwned: members(item.executorOwned, (name) => (filled(name) ? name : fail())),
      account: account(item.account),
      bindings: members(item.bindings, bindingMeta),
    };
  });
}

export const loadCapabilities = async (): Promise<Capability[]> => parseCapabilities(await api<unknown>('/resource-definition-capabilities'));

export const pairKey = (resourceType: string, driver: string) => `${resourceType}|${driver}`;
// One verified module pin is one choice: the same module at another revision is another choice.
export const capabilityKey = (capability: Capability) => [capability.resourceType, capability.driverType, capability.module, capability.source?.sourceId ?? '', capability.source?.revision ?? '', capability.source?.subdirectory ?? '', capability.source?.contentSha256 ?? ''].join('|');
export const capabilitiesForType = (capabilities: Capability[], resourceType: string): Capability[] => capabilities.filter((capability) => capability.resourceType === resourceType);
export const capabilityFor = (capabilities: Capability[], resourceType: string, driver: string): Capability | undefined => capabilities.find((capability) => capability.resourceType === resourceType && capability.driverType === driver);
// Only registry-verified Terraform pins can be registered from the form.
export const isVerifiedPin = (capability: Capability): boolean => capability.driverType === 'terraform' && capability.source !== undefined && Object.values(capability.source).every((part) => typeof part === 'string' && part !== '');
