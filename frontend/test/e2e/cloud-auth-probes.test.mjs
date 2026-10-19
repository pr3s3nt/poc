import { describe, expect, it } from 'vitest';
import { authenticationRejected } from './cloud-auth-probes.mjs';

describe('live dependency credential denial classification', () => {
  it.each([
    ['postgres', { code: 2, stdout: '', stderr: 'FATAL: password authentication failed for user "app"' }],
    ['redis', { code: 1, stdout: '', stderr: 'AUTH failed: WRONGPASS invalid username-password pair or user is disabled.' }],
    ['redis', { code: 1, stdout: 'NOAUTH Authentication required.', stderr: '' }],
  ])('accepts actual %s authentication denial only after a failed client exit', (kind, result) => {
    expect(authenticationRejected(result, kind)).toBe(true);
  });
  it.each([
    { code: 0, stdout: 'PONG', stderr: 'WRONGPASS' },
    { code: -1, stdout: '', stderr: 'NOAUTH' },
    { code: 1, stdout: '', stderr: 'connection refused' },
    { code: 1, stdout: '', stderr: 'WRONGPASS; TLS handshake certificate failure' },
    { code: 1, stdout: '', stderr: 'NOAUTH; connection timeout' },
    { code: 1, stdout: '', stderr: 'Forbidden' },
    { code: 1, stdout: '', stderr: '' },
    {}, null,
  ])('does not turn success, incomplete evidence or transport failures into PASS', (result) => {
    expect(authenticationRejected(result, 'postgres')).toBe(false);
    expect(authenticationRejected(result, 'redis')).toBe(false);
  });
  it('rejects unknown probe contracts', () => {
    expect(() => authenticationRejected({ code: 1, stderr: 'NOAUTH' }, 'other')).toThrow('unsupported');
  });
});
