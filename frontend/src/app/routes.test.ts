import { describe, expect, it } from 'vitest';
import { href, parseRoute } from './routes';

describe('console routes', () => {
  it('parses the public application routes under the UI base path', () => {
    expect(parseRoute('/ui/sign-in')).toEqual({ name: 'sign-in' });
    expect(parseRoute('/ui/applications')).toEqual({ name: 'applications' });
    expect(parseRoute('/ui/applications/new')).toEqual({ name: 'create-application' });
    expect(parseRoute('/ui/applications/payment%20api')).toEqual({ name: 'application', applicationId: 'payment api' });
    expect(parseRoute('/ui/applications/payment%20api/settings')).toEqual({ name: 'settings', applicationId: 'payment api' });
    expect(parseRoute('/ui/applications/payment%20api/environments/staging/workloads/new')).toEqual({ name: 'workload', applicationId: 'payment api', environment: 'staging', workloadId: undefined });
    expect(parseRoute('/ui/applications/payment%20api/environments/production/workloads/backend')).toEqual({ name: 'workload', applicationId: 'payment api', environment: 'production', workloadId: 'backend' });
    expect(parseRoute('/ui/applications/payment%20api/environments/staging/deployments')).toEqual({ name: 'deployments', applicationId: 'payment api', environment: 'staging' });
    expect(parseRoute('/ui/applications/payment%20api/environments/staging/deployments/deploy-1')).toEqual({ name: 'deployment', applicationId: 'payment api', environment: 'staging', deploymentId: 'deploy-1' });
  });

  it('encodes application IDs when constructing a link', () => {
    expect(href({ name: 'application', applicationId: 'payment api' })).toBe('/ui/applications/payment%20api');
    expect(href({ name: 'settings', applicationId: 'payment api' })).toBe('/ui/applications/payment%20api/settings');
  });
});
