import { api } from '../../shared/api/client';
import type { DeliveryBinding } from './api';

export type ConfigField = { key: string; labelKey: string; kind: 'text' | 'url' | 'textarea' | 'account' | 'choice'; required: boolean; default?: string; advanced?: boolean; readOnly?: boolean; visibleWhenKey?: string; visibleWhenValue?: string; choices?: { value: string; labelKey: string }[] };
export type Provider = { kind: string; fields: ConfigField[] };
export type BindingDetails = { binding: DeliveryBinding & { revision: number }; config: Record<string, string>; managementAccountKey: string; managementAccountVersion: number; credentialConfigured: boolean; usage: { applicationKey: string; environmentKey: string; desired: boolean; applied: boolean }[]; verifiedAt?: string; reasonCode?: string };
export type BindingInput = { name: string; provider: string; managementAccountKey: string; managementAccountVersion: number; config: Record<string, string>; writerUsername: string; writerPassword: string; retainWriterCredential?: boolean; previousVersion?: number; expectedRevision?: number };
export type BindingSummary = DeliveryBinding & { revision: number };
export const getProviders = async () => (await api<{ providers: Provider[] }>('/delivery-providers')).providers;
export const getBindings = async () => (await api<{ bindings: BindingSummary[] }>('/delivery-bindings')).bindings;
export const getBinding = (id: string, version: number) => api<BindingDetails>(`/delivery-bindings/${encodeURIComponent(id)}?version=${version}`);
export const createBinding = (input: BindingInput, id?: string) => api<BindingSummary>(id ? `/delivery-bindings/${encodeURIComponent(id)}/versions` : '/delivery-bindings', { method: 'POST', body: JSON.stringify(input) });
export const verifyBinding = (b: BindingSummary) => api<BindingSummary>(`/delivery-bindings/${encodeURIComponent(b.id)}/verify`, { method: 'POST', body: JSON.stringify({ version: b.version, expectedRevision: b.revision }) });
export type ConfigOption = { name: string; value: string; values: Record<string, string> };
export type Discovery = { options: Record<string, ConfigOption[]> };
export const inspectProvider = (provider: string, account: { key: string; version: number }, config: Record<string, string>) => api<Discovery>(`/delivery-providers/${encodeURIComponent(provider)}/inspect`, { method: 'POST', body: JSON.stringify({ managementAccountKey: account.key, managementAccountVersion: account.version, config }) });
