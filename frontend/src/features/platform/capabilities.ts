import { api } from '../../shared/api/client';

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

export const loadCapabilities = async (): Promise<Capability[]> => {
  const body = await api<{ capabilities?: Capability[] }>('/resource-definition-capabilities');
  return (body.capabilities ?? []).map((capability) => ({ ...capability, variables: capability.variables ?? [], executorOwned: capability.executorOwned ?? [], providerRequirements: capability.providerRequirements ?? [], outputs: capability.outputs ?? [] }));
};

export const pairKey = (resourceType: string, driver: string) => `${resourceType}|${driver}`;
// One verified module pin is one choice: the same module at another revision is another choice.
export const capabilityKey = (capability: Capability) => [capability.resourceType, capability.driverType, capability.module, capability.source?.sourceId ?? '', capability.source?.revision ?? '', capability.source?.subdirectory ?? '', capability.source?.contentSha256 ?? ''].join('|');
export const capabilitiesForType = (capabilities: Capability[], resourceType: string): Capability[] => capabilities.filter((capability) => capability.resourceType === resourceType);
export const capabilityFor = (capabilities: Capability[], resourceType: string, driver: string): Capability | undefined => capabilities.find((capability) => capability.resourceType === resourceType && capability.driverType === driver);
// Only registry-verified Terraform pins can be registered from the form.
export const isVerifiedPin = (capability: Capability): boolean => capability.driverType === 'terraform' && capability.source !== undefined;
