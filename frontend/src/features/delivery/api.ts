
export type DeliverySelection = { mode: 'DIRECT' | 'CD'; bindingId?: string; bindingVersion?: number; selfHeal: boolean };
export type DeliveryView = { desired: DeliverySelection; applied: DeliverySelection; environmentVersion: number; configurationRevision: number; pending: boolean };
export type DeliveryBinding = { id: string; version: number; name: string; provider: string; status: string; capabilities: { selfHeal: boolean } };
