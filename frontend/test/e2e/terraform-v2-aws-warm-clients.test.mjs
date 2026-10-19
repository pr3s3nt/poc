import { describe, expect, it } from 'vitest';
import { warmTechnicalClients } from './terraform-v2-aws-definitions.mjs';

describe('AWS technical client lifecycle', () => {
  it('waits for real PostgreSQL and Redis readiness in order before handing control to job submission', async () => {
    const calls = [];
    await warmTechnicalClients({
      psql: async (sql) => { calls.push(`postgres:${sql}`); return '1'; },
      redis: async (...command) => { calls.push(`redis:${command.join(' ')}`); return 'PONG'; },
    });
    calls.push('job:submit');
    expect(calls).toEqual(['postgres:SELECT 1', 'redis:PING', 'job:submit']);
  });

  it('fails readiness instead of proceeding with a cold or unreachable Redis client', async () => {
    const calls = [];
    await expect(warmTechnicalClients({
      psql: async () => { calls.push('postgres'); },
      redis: async () => { calls.push('redis'); return 'NOAUTH'; },
    })).rejects.toThrow('Redis technical client did not become ready');
    expect(calls).toEqual(['postgres', 'redis']);
  });
});
