import { requireConfig } from './bootstrap-config.mjs';
export const executionProfile = config => config.runtime?.profile ?? (config.schemaVersion === 3 ? 'console' : 'dev-test');
export const isDevTest = config => executionProfile(config) === 'dev-test';
export function requireDevTest(config) { requireConfig(isDevTest(config), 'Native CLI operations require explicit dev-test profile'); }
