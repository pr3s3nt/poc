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

  it('routes a resource detail inside its Environment and encodes the ID', () => {
    expect(parseRoute('/ui/applications/shop/environments/staging/resources/abc-1')).toEqual({ name: 'resource', applicationId: 'shop', environment: 'staging', resourceId: 'abc-1' });
    expect(parseRoute('/ui/applications/shop/environments/qa/resources/abc-1')).not.toMatchObject({ name: 'resource' });
    expect(href({ name: 'resource', applicationId: 'shop', environment: 'production', resourceId: 'a/b' })).toBe('/ui/applications/shop/environments/production/resources/a%2Fb');
    expect(parseRoute(href({ name: 'resource', applicationId: 'shop', environment: 'production', resourceId: 'a/b' }))).toEqual({ name: 'resource', applicationId: 'shop', environment: 'production', resourceId: 'a/b' });
  });

  it('routes the effectiveness report with its scope and period (UC-17)', () => {
    expect(parseRoute('/ui/platform/effectiveness')).toEqual({ name: 'effectiveness' });
    expect(parseRoute('/ui/platform/effectiveness?application=a%2Fb&workload=api&days=7')).toEqual({ name: 'effectiveness', application: 'a/b', workload: 'api', days: 7 });
    expect(parseRoute('/ui/platform/effectiveness?workload=api&days=12')).toEqual({ name: 'effectiveness' });
    expect(href({ name: 'effectiveness', application: 'a/b', workload: 'api', days: 90 })).toBe('/ui/platform/effectiveness?application=a%2Fb&workload=api&days=90');
    expect(href({ name: 'effectiveness', workload: 'api' })).toBe('/ui/platform/effectiveness');
  });

  it('routes infrastructure accounts with an optional selected account', () => {
    expect(parseRoute('/ui/platform/driver-accounts')).toEqual({ name: 'driver-accounts' });
    expect(parseRoute('/ui/platform/driver-accounts?account=lab-aws')).toEqual({ name: 'driver-accounts', account: 'lab-aws' });
    expect(href({ name: 'driver-accounts' })).toBe('/ui/platform/driver-accounts');
    expect(href({ name: 'driver-accounts', account: 'a/b' })).toBe('/ui/platform/driver-accounts?account=a%2Fb');
    expect(parseRoute('/ui/platform/connections')).toEqual({ name: 'driver-accounts' });
  });

  it('routes the Admin user management and the account page (UC-00)', () => {
    expect(parseRoute('/ui/platform/users')).toEqual({ name: 'users' });
    expect(href({ name: 'users' })).toBe('/ui/platform/users');
    expect(parseRoute(href({ name: 'users' }))).toEqual({ name: 'users' });
    expect(parseRoute('/ui/account')).toEqual({ name: 'account' });
    expect(href({ name: 'account' })).toBe('/ui/account');
    expect(parseRoute(href({ name: 'account' }))).toEqual({ name: 'account' });
  });

  it('retires standalone Definition authoring routes', () => {
    expect(parseRoute('/ui/platform/rendering-templates')).toEqual({ name: 'applications' });
    expect(parseRoute('/ui/platform/resource-definitions')).toEqual({ name: 'applications' });
    expect(parseRoute('/ui/platform/catalog')).toEqual({ name: 'catalog' });
  });

  it('reads the query of the current location when the path has none, and keeps settings parsing', () => {
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
