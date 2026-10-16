import { describe, expect, it, vi } from 'vitest';
import { verifyCleanupAWSAbsence } from './terraform-v2-aws.mjs';

const input = { configFile: '/private/run/config.json', manifestFile: '/private/run/manifest.json' };

describe('AWS cleanup absence evidence', () => {
  it('calls the authoritative verifier once with the exact cleanup command and accepts its success record', () => {
    const v2ctl = vi.fn().mockReturnValue('layer aux verified absent\n');

    expect(verifyCleanupAWSAbsence(v2ctl, input)).toBe('layer aux verified absent');
    expect(v2ctl).toHaveBeenCalledTimes(1);
    expect(v2ctl).toHaveBeenCalledWith(input, 'aws-verify', '-config', input.configFile, '-manifest', input.manifestFile, '-layer', 'aux');
  });

  it.each([
    ['literal empty output', ''],
    ['failure text', 'FAIL layer aux is not verified absent'],
    ['unknown success text', 'success; no resources found'],
    ['success without the command newline', 'layer aux verified absent'],
    ['extra output around success', 'notice\nlayer aux verified absent\n'],
  ])('rejects %s as absence proof', (_name, output) => {
    expect(() => verifyCleanupAWSAbsence(() => output, input)).toThrow(/not verified absent/);
  });

  it('rejects a nonzero verifier command even if it emitted a success-looking line', () => {
    const v2ctl = () => {
      throw new Error('aws-verify exited with status 1');
    };
    expect(() => verifyCleanupAWSAbsence(v2ctl, input)).toThrow('aws-verify exited with status 1');
  });
});
