export type Route =
  | { name: 'sign-in' }
  | { name: 'applications' }
  | { name: 'create-application' }
  | { name: 'application'; applicationId: string };

const base = '/ui';

export function parseRoute(path = window.location.pathname): Route {
  const relative = path.startsWith(base) ? path.slice(base.length) || '/' : path;
  if (relative === '/' || relative === '/sign-in') return { name: 'sign-in' };
  if (relative === '/applications') return { name: 'applications' };
  if (relative === '/applications/new') return { name: 'create-application' };
  const match = relative.match(/^\/applications\/([^/]+)$/);
  const applicationId = match?.[1];
  return applicationId ? { name: 'application', applicationId: decodeURIComponent(applicationId) } : { name: 'applications' };
}

export function href(route: Route): string {
  switch (route.name) {
    case 'sign-in': return `${base}/sign-in`;
    case 'applications': return `${base}/applications`;
    case 'create-application': return `${base}/applications/new`;
    case 'application': return `${base}/applications/${encodeURIComponent(route.applicationId)}`;
  }
}

export function navigate(route: Route): void {
  window.history.pushState({}, '', href(route));
  window.dispatchEvent(new Event('orchestrator:navigate'));
}
