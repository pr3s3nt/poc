import { describe, expect, it } from 'vitest';
import { awsPinSetMatches } from './terraform-v2-aws-definitions.mjs';

const intents = (modules) => modules.map((module) => ({
  module: { module },
  providers: [{ name: 'aws', target: 'account', connectionKey: 'aws-run', region: 'us-east-1' }],
}));

describe('scenario AWS provider pin contracts', () => {
  it('requires exactly the AWS modules authored in V05, A02 and A03', () => {
    expect(awsPinSetMatches('V05', intents(['vpc', 'eks']), 'aws-run', 'us-east-1')).toBe(true);
    expect(awsPinSetMatches('V05', intents(['vpc', 'eks', 'postgres-k8s']), 'aws-run', 'us-east-1')).toBe(false);
    expect(awsPinSetMatches('V05', intents(['vpc']), 'aws-run', 'us-east-1')).toBe(false);
    expect(awsPinSetMatches('A02', intents(['vpc', 'eks', 'aurora', 'elasticache']), 'aws-run', 'us-east-1')).toBe(true);
    expect(awsPinSetMatches('A02', intents(['vpc', 'eks', 'aurora']), 'aws-run', 'us-east-1')).toBe(false);
    expect(awsPinSetMatches('A02', intents(['vpc', 'eks', 'aurora', 'elasticache', 'redis-k8s']), 'aws-run', 'us-east-1')).toBe(false);
    expect(awsPinSetMatches('A03', intents(['vpc', 'eks', 'aurora']), 'aws-run', 'us-east-1')).toBe(true);
    expect(awsPinSetMatches('A03', intents(['vpc', 'eks']), 'aws-run', 'us-east-1')).toBe(false);
    expect(awsPinSetMatches('A03', intents(['vpc', 'eks', 'aurora', 'elasticache']), 'aws-run', 'us-east-1')).toBe(false);
  });

  it('rejects a wrong account, region, or provider target on any expected module', () => {
    for (const mutate of [
      (value) => { value[0].providers[0].connectionKey = 'other'; },
      (value) => { value[0].providers[0].region = 'eu-west-1'; },
      (value) => { value[0].providers[0].target = 'resource'; },
    ]) {
      const value = intents(['vpc', 'eks']);
      mutate(value);
      expect(awsPinSetMatches('V05', value, 'aws-run', 'us-east-1')).toBe(false);
    }
  });
});
