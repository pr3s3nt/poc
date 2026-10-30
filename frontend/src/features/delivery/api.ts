import { api } from '../../shared/api/client';
import type { EnvironmentKey } from '../../shared/types/application';

export type DeliverySelection = { mode: 'DIRECT' | 'CD'; bindingId?: string; bindingVersion?: number; selfHeal: boolean };
export type DeliveryView = { desired: DeliverySelection; applied: DeliverySelection; environmentVersion: number; configurationRevision: number; pending: boolean };
export type DeliveryBinding = { id: string; version: number; name: string; provider: string; status: string; capabilities: { selfHeal: boolean } };
const path = (app: string, env: EnvironmentKey) => `/applications/${encodeURIComponent(app)}/environments/${env}/delivery`;
export const getDelivery = async (app: string, env: EnvironmentKey) => {
  const view = await api<DeliveryView>(path(app, env));
  if (!view?.desired?.mode || !view.applied?.mode || !Number.isInteger(view.environmentVersion) || !Number.isInteger(view.configurationRevision)) throw new Error('Invalid delivery response');
  return view;
};
export const listDeliveryBindings = async () => (await api<{ bindings?: DeliveryBinding[] }>('/delivery-bindings')).bindings ?? [];
export const saveDelivery = (app: string, env: EnvironmentKey, selection: DeliverySelection, view: DeliveryView) => api<DeliveryView>(path(app, env), {
  method: 'PUT', body: JSON.stringify({ ...selection, expectedEnvironmentVersion: view.environmentVersion, expectedConfigurationRevision: view.configurationRevision }),
});
