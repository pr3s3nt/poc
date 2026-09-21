import { describe, expect, it } from 'vitest';
import { hrefFor, parseRoute } from './router';

describe('parseRoute', () => {
  it('maps the console root to the deploy page', () => {
    expect(parseRoute('/ui/')).toEqual({ name: 'deploy' });
    expect(parseRoute('/ui')).toEqual({ name: 'deploy' });
  });

  it('maps a deployment path to the details route', () => {
    expect(parseRoute('/ui/deployments/dep-1')).toEqual({ name: 'deployment-details', deploymentId: 'dep-1' });
  });

  it('reports unknown paths', () => {
    expect(parseRoute('/ui/unknown/page')).toEqual({ name: 'not-found', path: '/ui/unknown/page' });
  });

  it('builds hrefs that the Go backend serves', () => {
    expect(hrefFor({ name: 'deploy' })).toBe('/ui/');
    expect(hrefFor({ name: 'deployment-details', deploymentId: 'dep 1' })).toBe('/ui/deployments/dep%201');
  });
});
