import { api } from '../../shared/api/client';

export type Driver = 'terraform' | 'kubernetes';

// Public, secret-free metadata of GET /resource-definition-capabilities (UC-03 BR-23).
export type CapabilityVariable = {
  name: string;
  type: string;
  required: boolean;
  hasDefault: boolean;
  default?: unknown;
  resourceParam: boolean;
  preset?: string;
};
export type Capability = {
  resourceType: string;
  driverType: Driver;
  module: string;
  variables: CapabilityVariable[];
  executorOwned: string[];
};

export const loadCapabilities = async (): Promise<Capability[]> => {
  const body = await api<{ capabilities?: Capability[] }>('/resource-definition-capabilities');
  return (body.capabilities ?? []).map((capability) => ({ ...capability, variables: capability.variables ?? [], executorOwned: capability.executorOwned ?? [] }));
};

export const pairKey = (resourceType: string, driver: string) => `${resourceType}|${driver}`;
export const capabilitiesForType = (capabilities: Capability[], resourceType: string): Capability[] => capabilities.filter((capability) => capability.resourceType === resourceType);
export const capabilityFor = (capabilities: Capability[], resourceType: string, driver: string): Capability | undefined => capabilities.find((capability) => capability.resourceType === resourceType && capability.driverType === driver);
