export type Route =
  | { name: 'sign-in' }
  | { name: 'applications' }
  | { name: 'create-application' }
  | { name: 'application'; applicationId: string }
  | { name: 'settings'; applicationId: string }
  | { name: 'workload'; applicationId: string; environment: 'staging' | 'production'; workloadId?: string };

const base = '/ui';

export function parseRoute(path = window.location.pathname): Route {
  const relative = path.startsWith(base) ? path.slice(base.length) || '/' : path;
  if (relative === '/' || relative === '/sign-in') return { name: 'sign-in' };
  if (relative === '/applications') return { name: 'applications' };
  if (relative === '/applications/new') return { name: 'create-application' };
  const settings = relative.match(/^\/applications\/([^/]+)\/settings$/);
  if (settings?.[1]) return { name: 'settings', applicationId: decodeURIComponent(settings[1]) };
  const workload = relative.match(/^\/applications\/([^/]+)\/environments\/(staging|production)\/workloads\/(new|[^/]+)$/);
  if (workload?.[1] && workload[2] && workload[3]) return { name: 'workload', applicationId: decodeURIComponent(workload[1]), environment: workload[2] as 'staging' | 'production', workloadId: workload[3] === 'new' ? undefined : decodeURIComponent(workload[3]) };
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
    case 'settings': return `${base}/applications/${encodeURIComponent(route.applicationId)}/settings`;
    case 'workload': return `${base}/applications/${encodeURIComponent(route.applicationId)}/environments/${route.environment}/workloads/${route.workloadId ? encodeURIComponent(route.workloadId) : 'new'}`;
  }
}

export function navigate(route: Route): void {
  window.history.pushState({}, '', href(route));
  window.dispatchEvent(new Event('orchestrator:navigate'));
}
