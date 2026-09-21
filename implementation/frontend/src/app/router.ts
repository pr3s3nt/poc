// Minimal History API router. The console has two routes, so a router library
// would add dependency weight without adding behaviour.
import { useEffect, useState } from 'react';

export const UI_BASE = '/ui';

export type Route =
  | { readonly name: 'deploy' }
  | { readonly name: 'deployment-details'; readonly deploymentId: string }
  | { readonly name: 'not-found'; readonly path: string };

export function parseRoute(pathname: string): Route {
  const trimmed = pathname.replace(/\/+$/, '');
  const relative = trimmed.startsWith(UI_BASE) ? trimmed.slice(UI_BASE.length) : trimmed;
  if (relative === '' || relative === '/deploy') {
    return { name: 'deploy' };
  }
  const match = /^\/deployments\/([^/]+)$/.exec(relative);
  if (match && match[1]) {
    return { name: 'deployment-details', deploymentId: decodeURIComponent(match[1]) };
  }
  return { name: 'not-found', path: pathname };
}

export function hrefFor(route: Route): string {
  switch (route.name) {
    case 'deploy':
      return `${UI_BASE}/`;
    case 'deployment-details':
      return `${UI_BASE}/deployments/${encodeURIComponent(route.deploymentId)}`;
    default:
      return `${UI_BASE}/`;
  }
}

const NAVIGATION_EVENT = 'orchestrator:navigate';

export function navigate(route: Route): void {
  window.history.pushState({}, '', hrefFor(route));
  window.dispatchEvent(new Event(NAVIGATION_EVENT));
}

export function useRoute(): Route {
  const [route, setRoute] = useState<Route>(() => parseRoute(window.location.pathname));
  useEffect(() => {
    const update = () => setRoute(parseRoute(window.location.pathname));
    window.addEventListener('popstate', update);
    window.addEventListener(NAVIGATION_EVENT, update);
    return () => {
      window.removeEventListener('popstate', update);
      window.removeEventListener(NAVIGATION_EVENT, update);
    };
  }, []);
  return route;
}
