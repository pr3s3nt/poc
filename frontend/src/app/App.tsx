import { useCallback, useEffect, useState } from 'react';
import { AppShell } from './AppShell';
import { parseRoute, navigate, type Route } from './routes';
import { SignInPage } from '../features/auth/SignInPage';
import { ApplicationsPage } from '../features/applications/ApplicationsPage';
import { CreateApplicationPage } from '../features/applications/CreateApplicationPage';
import { ApplicationHomePage } from '../features/applications/ApplicationHomePage';
import { SettingsPage } from '../features/configuration/SettingsPage';
import { WorkloadEditorPage } from '../features/workloads/WorkloadEditorPage';
import type { Application } from '../shared/types/application';
import { api } from '../shared/api/client';

type APIApplication = { key: string; name: string; subdomain: string };
type ApplicationsResponse = { applications: APIApplication[] };

export function App() {
  const [route, setRoute] = useState<Route>(() => parseRoute());
  const [signedIn, setSignedIn] = useState(false);
  const [applications, setApplications] = useState<readonly Application[]>([]);
  useEffect(() => { const update = () => setRoute(parseRoute()); window.addEventListener('popstate', update); window.addEventListener('orchestrator:navigate', update); return () => { window.removeEventListener('popstate', update); window.removeEventListener('orchestrator:navigate', update); }; }, []);
  function mapApplication(application: APIApplication): Application { return { id: application.key, name: application.name, subdomain: application.subdomain, workloads: { staging: [], production: [] } }; }
  const loadApplications = useCallback(async () => { const response = await api<ApplicationsResponse>('/applications'); setApplications(response.applications.map(mapApplication)); }, []);
  useEffect(() => { api('/auth/session').then(() => { setSignedIn(true); return loadApplications(); }).catch(() => setSignedIn(false)); }, [loadApplications]);
  async function signOut() { await api('/auth/sign-out', { method: 'POST' }); setSignedIn(false); navigate({ name: 'sign-in' }); }
  async function signIn(username: string, password: string) { await api('/auth/sign-in', { method: 'POST', body: JSON.stringify({ username, password }) }); await loadApplications(); setSignedIn(true); navigate({ name: 'applications' }); }
  async function createApplication(name: string, subdomain: string) { const response = await api<{ application: APIApplication }>('/applications', { method: 'POST', body: JSON.stringify({ name, subdomain }) }); const application = mapApplication(response.application); setApplications((current) => [...current, application]); return application.id; }
  if (!signedIn || route.name === 'sign-in') return <SignInPage onSuccess={signIn} />;
  const application = 'applicationId' in route ? applications.find((item) => item.id === route.applicationId) : undefined;
  return <AppShell onSignOut={signOut}>{route.name === 'applications' ? <ApplicationsPage applications={applications} /> : null}{route.name === 'create-application' ? <CreateApplicationPage onCreate={createApplication} /> : null}{route.name === 'application' && application ? <ApplicationHomePage application={application} /> : null}{route.name === 'settings' && application ? <SettingsPage application={application} /> : null}{route.name === 'workload' && application ? <WorkloadEditorPage application={application} environment={route.environment} workloadId={route.workloadId} /> : null}{'applicationId' in route && !application ? <section className="page"><h1>Application not found</h1></section> : null}</AppShell>;
}
