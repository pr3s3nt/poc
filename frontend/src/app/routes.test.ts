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
    expect(parseRoute('/ui/applications/payment%20api/environments/production/preview')).toEqual({ name: 'score-preview', applicationId: 'payment api', environment: 'production' });
    expect(href({ name: 'score-preview', applicationId: 'payment api', environment: 'staging' })).toBe('/ui/applications/payment%20api/environments/staging/preview');
    expect(parseRoute('/ui/applications/payment%20api/environments/staging/deployments')).toEqual({ name: 'deployments', applicationId: 'payment api', environment: 'staging' });
    expect(parseRoute('/ui/applications/payment%20api/environments/staging/deployments/deploy-1')).toEqual({ name: 'deployment', applicationId: 'payment api', environment: 'staging', deploymentId: 'deploy-1' });
  });

  it('routes the rendering templates page', () => {
    expect(parseRoute('/ui/platform/rendering-templates')).toEqual({ name: 'rendering-templates' });
    expect(href({ name: 'rendering-templates' })).toBe('/ui/platform/rendering-templates');
  });

  it('carries the resourceType query of the definitions page and drops an empty one', () => {
    expect(parseRoute('/ui/platform/resource-definitions')).toEqual({ name: 'resource-definitions' });
    expect(parseRoute('/ui/platform/resource-definitions?resourceType=postgres')).toEqual({ name: 'resource-definitions', resourceType: 'postgres' });
    expect(parseRoute('/ui/platform/resource-definitions?resourceType=')).toEqual({ name: 'resource-definitions' });
    expect(parseRoute('/ui/platform/resource-definitions?other=1')).toEqual({ name: 'resource-definitions' });
    // Raw value: validation against the catalog belongs to the page.
    expect(parseRoute('/ui/platform/resource-definitions?resourceType=constructor')).toEqual({ name: 'resource-definitions', resourceType: 'constructor' });
    expect(parseRoute('/ui/platform/resource-definitions?resourceType=a%20b')).toEqual({ name: 'resource-definitions', resourceType: 'a b' });
    expect(href({ name: 'resource-definitions' })).toBe('/ui/platform/resource-definitions');
    expect(href({ name: 'resource-definitions', resourceType: 'k8s-cluster' })).toBe('/ui/platform/resource-definitions?resourceType=k8s-cluster');
    expect(href({ name: 'resource-definitions', resourceType: 'a b&c' })).toBe('/ui/platform/resource-definitions?resourceType=a%20b%26c');
  });

  it('reads the query of the current location when the path has none, and keeps settings parsing', () => {
    window.history.replaceState({}, '', '/ui/platform/resource-definitions?resourceType=vpc');
    expect(parseRoute()).toEqual({ name: 'resource-definitions', resourceType: 'vpc' });
    window.history.replaceState({}, '', '/ui/applications/shop/settings?environment=production');
    expect(parseRoute()).toEqual({ name: 'settings', applicationId: 'shop', environment: 'production' });
    expect(parseRoute('/ui/applications/shop/settings?environment=staging')).toEqual({ name: 'settings', applicationId: 'shop', environment: 'staging' });
    window.history.replaceState({}, '', '/');
  });

  it('encodes application IDs when constructing a link', () => {
    expect(href({ name: 'application', applicationId: 'payment api' })).toBe('/ui/applications/payment%20api');
    expect(href({ name: 'settings', applicationId: 'payment api' })).toBe('/ui/applications/payment%20api/settings');
    expect(parseRoute('/ui/applications/shop?environment=production')).toEqual({ name: 'application', applicationId: 'shop', environment: 'production' });
    expect(parseRoute('/ui/applications/shop?environment=bogus')).toEqual({ name: 'application', applicationId: 'shop' });
    expect(href({ name: 'application', applicationId: 'shop', environment: 'production' })).toBe('/ui/applications/shop?environment=production');
    expect(href({ name: 'application', applicationId: 'shop' })).toBe('/ui/applications/shop');
    expect(href({ name: 'settings', applicationId: 'payment', environment: 'production' })).toBe('/ui/applications/payment/settings?environment=production');
  });
});
