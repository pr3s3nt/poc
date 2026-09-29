import { useCallback, useEffect, useState } from 'react';
import { AppShell } from './AppShell';
import { parseRoute, navigate, type Route } from './routes';
import { SignInPage } from '../features/auth/SignInPage';
import { ApplicationsPage } from '../features/applications/ApplicationsPage';
import { CreateApplicationPage } from '../features/applications/CreateApplicationPage';
import { ApplicationHomePage } from '../features/applications/ApplicationHomePage';
import { SettingsPage } from '../features/configuration/SettingsPage';
import { WorkloadEditorPage } from '../features/workloads/WorkloadEditorPage';
import { DeploymentHistoryPage } from '../features/deployments/DeploymentHistoryPage';
import { DeploymentDetailsPage } from '../features/deployments/DeploymentDetailsPage';
import { ResourceTypesPage } from '../features/platform/ResourceTypesPage';
import { ResourceDefinitionsPage } from '../features/platform/ResourceDefinitionsPage';
import type { Application } from '../shared/types/application';
import { api } from '../shared/api/client';

type APIApplication = { key: string; name: string; subdomain: string };
type ApplicationsResponse = { applications: APIApplication[] };
type SessionResponse = { user: { Username?: string; Role?: string } };

export function App() {
  const [route, setRoute] = useState<Route>(() => parseRoute());
  const [signedIn, setSignedIn] = useState(false);
  const [identity, setIdentity] = useState({ username: 'developer', role: 'DEVELOPER' });
  const [applications, setApplications] = useState<readonly Application[]>([]);
  useEffect(() => { const update = () => setRoute(parseRoute()); window.addEventListener('popstate', update); window.addEventListener('orchestrator:navigate', update); return () => { window.removeEventListener('popstate', update); window.removeEventListener('orchestrator:navigate', update); }; }, []);
  function mapApplication(application: APIApplication): Application { return { id: application.key, name: application.name, subdomain: application.subdomain, workloads: { staging: [], production: [] } }; }
  const loadApplications = useCallback(async () => { const response = await api<ApplicationsResponse>('/applications'); setApplications(response.applications.map(mapApplication)); }, []);
  useEffect(() => { api<SessionResponse>('/auth/session').then((response) => { setIdentity({ username: response.user.Username ?? 'developer', role: response.user.Role ?? 'DEVELOPER' }); setSignedIn(true); return loadApplications(); }).catch(() => setSignedIn(false)); }, [loadApplications]);
  async function signOut() { await api('/auth/sign-out', { method: 'POST' }); setSignedIn(false); navigate({ name: 'sign-in' }); }
  async function signIn(username: string, password: string) { const response = await api<SessionResponse>('/auth/sign-in', { method: 'POST', body: JSON.stringify({ username, password }) }); setIdentity({ username: response.user.Username ?? username, role: response.user.Role ?? 'DEVELOPER' }); await loadApplications(); setSignedIn(true); navigate({ name: 'applications' }); }
  async function createApplication(name: string, subdomain: string) { const response = await api<{ application: APIApplication }>('/applications', { method: 'POST', body: JSON.stringify({ name, subdomain }) }); const application = mapApplication(response.application); setApplications((current) => [...current, application]); return application.id; }
  if (!signedIn || route.name === 'sign-in') return <SignInPage onSuccess={signIn} />;
  const application = 'applicationId' in route ? applications.find((item) => item.id === route.applicationId) : undefined;
  const isPlatformEngineer = identity.role === 'PLATFORM_ENGINEER' || identity.role === 'ADMIN';
  return <AppShell onSignOut={signOut} username={identity.username} role={identity.role}>{route.name === 'applications' ? <ApplicationsPage applications={applications} /> : null}{route.name === 'resource-types' && isPlatformEngineer ? <ResourceTypesPage /> : null}{route.name === 'resource-definitions' && isPlatformEngineer ? <ResourceDefinitionsPage /> : null}{route.name === 'create-application' ? <CreateApplicationPage onCreate={createApplication} /> : null}{route.name === 'application' && application ? <ApplicationHomePage application={application} /> : null}{route.name === 'settings' && application ? <SettingsPage application={application} /> : null}{route.name === 'workload' && application ? <WorkloadEditorPage application={application} environment={route.environment} workloadId={route.workloadId} /> : null}{route.name === 'deployments' && application ? <DeploymentHistoryPage application={application} environment={route.environment} /> : null}{route.name === 'deployment' && application ? <DeploymentDetailsPage application={application} environment={route.environment} deploymentId={route.deploymentId} /> : null}{'applicationId' in route && !application ? <section className="page"><h1>Application not found</h1></section> : null}</AppShell>;
}
