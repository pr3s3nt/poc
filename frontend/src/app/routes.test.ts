import { describe, expect, it } from 'vitest';
import { href, parseRoute } from './routes';

describe('console routes', () => {
  it('parses the public application routes under the UI base path', () => {
    expect(parseRoute('/ui/sign-in')).toEqual({ name: 'sign-in' });
    expect(parseRoute('/ui/applications')).toEqual({ name: 'applications' });
    expect(parseRoute('/ui/applications/new')).toEqual({ name: 'create-application' });
    expect(parseRoute('/ui/applications/payment%20api')).toEqual({ name: 'application', applicationId: 'payment api' });
  });

  it('encodes application IDs when constructing a link', () => {
    expect(href({ name: 'application', applicationId: 'payment api' })).toBe('/ui/applications/payment%20api');
  });
});
