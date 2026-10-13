import { api } from '../../shared/api/client';
import { fail, filled, flag, isObject, members, text } from './metadata';

export type Driver = 'terraform' | 'kubernetes';

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
// The exact verified pin of a registry module: the only choices the form offers
// (no trust root, URL or free text of its own).
export type CapabilitySource = { sourceId: string; moduleId: string; revision: string; subdirectory: string; contentSha256: string };
export type CapabilityProvider = { name: string; source: string; version: string; binding: 'aws' | 'kubernetes' | 'none' };
export type CapabilityOutput = { name: string; type?: string; sensitive: boolean };
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
};

const binding = (value: unknown): value is CapabilityProvider['binding'] => value === 'aws' || value === 'kubernetes' || value === 'none';

function variable(item: unknown): CapabilityVariable {
  if (!isObject(item) || !filled(item.name) || !text(item.type) || !flag(item.required) || !flag(item.hasDefault) || !flag(item.resourceParam) || (item.preset !== undefined && !text(item.preset))) return fail();
  return { name: item.name, type: item.type, required: item.required, hasDefault: item.hasDefault, resourceParam: item.resourceParam, ...(item.default !== undefined ? { default: item.default } : {}), ...(item.preset !== undefined ? { preset: item.preset } : {}) };
}
const pin = /^[0-9a-f]{64}$/;
function source(value: unknown): CapabilitySource {
  if (!isObject(value) || !filled(value.sourceId) || !filled(value.moduleId) || !filled(value.revision) || !filled(value.subdirectory) || !text(value.contentSha256) || !pin.test(value.contentSha256)) return fail();
  return { sourceId: value.sourceId, moduleId: value.moduleId, revision: value.revision, subdirectory: value.subdirectory, contentSha256: value.contentSha256 };
}

// Reads GET /resource-definition-capabilities strictly. `variables` and `executorOwned` are required
// arrays; `providerRequirements` and `outputs` are omitted by the server when empty. A Terraform pair
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
