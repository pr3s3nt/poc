import type { EnvironmentTarget } from '../shared/types/application';

/** Environment Settings fixture: version, Secret Store and resource target generation only. */
export function configuredTarget(overrides: Partial<EnvironmentTarget> = {}): EnvironmentTarget {
  return { version: 2, secretStoreKey: '', targetGeneration: 0, draftVersion: 0, ...overrides };
}

export function bothConfigured() {
  return { staging: configuredTarget(), production: configuredTarget() };
}
