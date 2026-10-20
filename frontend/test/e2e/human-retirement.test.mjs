import { describe, expect, it } from 'vitest';
import { createApplication, setEnvironmentConnections } from './human.mjs';

describe('retired Environment Connection recording helpers', () => {
  it('refuses old imports before touching a page or API', async () => {
    const untouched = new Proxy({}, { get() { throw new Error('must not access UI'); } });
    await expect(setEnvironmentConnections(untouched, 'app', { staging: 'old' })).rejects.toThrow(/Historical.*retired/);
    await expect(createApplication(untouched, 'app', 'app', { connections: { staging: 'old' } })).rejects.toThrow(/Historical.*retired/);
  });
});
