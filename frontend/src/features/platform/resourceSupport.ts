export type Driver = 'terraform' | 'kubernetes';

// Resource Types the runtime can create today, with the drivers it accepts and
// the embedded Terraform module each one maps to (backend catalog policy,
// backend/internal/application/catalog/service.go). Metadata authority
// replaces this allowlist in T31.
export const supported: Record<string, { drivers: Driver[]; module: string }> = {
  vpc: { drivers: ['terraform'], module: 'vpc' },
  'k8s-cluster': { drivers: ['terraform'], module: 'eks' },
  'k8s-namespace': { drivers: ['kubernetes'], module: '' },
  postgres: { drivers: ['kubernetes', 'terraform'], module: 'aurora' },
};

// Keys come from users and the API ("constructor" is a valid ID): own-property lookups only.
export const own = <T,>(table: Record<string, T>, key: string): T | undefined => (Object.hasOwn(table, key) ? table[key] : undefined);
export const isSupportedType = (key: string): boolean => own(supported, key) !== undefined;
