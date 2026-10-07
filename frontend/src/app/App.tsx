import { useCallback, useEffect, useState } from 'react';
import { AppShell } from './AppShell';
import { parseRoute, navigate, type Route } from './routes';
import { SignInPage } from '../features/auth/SignInPage';
import { ApplicationsPage, type ApplicationsLoadState } from '../features/applications/ApplicationsPage';
import { CreateApplicationPage } from '../features/applications/CreateApplicationPage';
import { ApplicationHomePage } from '../features/applications/ApplicationHomePage';
import { SettingsPage } from '../features/configuration/SettingsPage';
import { WorkloadEditorPage } from '../features/workloads/WorkloadEditorPage';
import { DeploymentHistoryPage } from '../features/deployments/DeploymentHistoryPage';
import { DeploymentDetailsPage } from '../features/deployments/DeploymentDetailsPage';
import { ScorePreviewPage } from '../features/preview/ScorePreviewPage';
import { ResourceTypesPage } from '../features/platform/ResourceTypesPage';
import { ResourceDefinitionsPage } from '../features/platform/ResourceDefinitionsPage';
import { ConnectionsPage } from '../features/platform/ConnectionsPage';
import type { Application } from '../shared/types/application';
import { api, ApiError, sessionExpiredEvent } from '../shared/api/client';
import { Button } from '../shared/ui/Button';

type APIApplication = { key: string; name: string; subdomain: string; connectionKey: string; executionProfile: string; region?: string };
type ApplicationsResponse = { applications: APIApplication[] };
type SessionResponse = { user: { Username?: string; Role?: string } };

type SessionState = 'restoring' | 'signed-in' | 'signed-out';

export function App() {
  const [route, setRoute] = useState<Route>(() => parseRoute());
  const [session, setSession] = useState<SessionState>('restoring');
  const [sessionEnded, setSessionEnded] = useState(false);
  const [identity, setIdentity] = useState({ username: 'developer', role: 'DEVELOPER' });
  const [applications, setApplications] = useState<readonly Application[]>([]);
  const [applicationsState, setApplicationsState] = useState<ApplicationsLoadState>('loading');
  const [createdApplicationId, setCreatedApplicationId] = useState<string>();
  useEffect(() => { const update = () => setRoute(parseRoute()); window.addEventListener('popstate', update); window.addEventListener('orchestrator:navigate', update); return () => { window.removeEventListener('popstate', update); window.removeEventListener('orchestrator:navigate', update); }; }, []);
  function mapApplication(application: APIApplication): Application { return { id: application.key, name: application.name, subdomain: application.subdomain, connectionKey: application.connectionKey, profile: application.executionProfile, region: application.region, workloads: { staging: [], production: [] } }; }
  const loadApplications = useCallback(async () => {
    setApplicationsState('loading');
    try { const response = await api<ApplicationsResponse>('/applications'); setApplications(response.applications.map(mapApplication)); setApplicationsState('ready'); }
    catch (err) { if (!(err instanceof ApiError && err.status === 401)) setApplicationsState('error'); }
  }, []);
  const endSession = useCallback((expired: boolean) => { setSession('signed-out'); setSessionEnded(expired); setApplications([]); setApplicationsState('loading'); navigate({ name: 'sign-in' }); }, []);
  useEffect(() => { api<SessionResponse>('/auth/session').then((response) => { setIdentity({ username: response.user.Username ?? 'developer', role: response.user.Role ?? 'DEVELOPER' }); setSession('signed-in'); return loadApplications(); }).catch(() => setSession('signed-out')); }, [loadApplications]);
  useEffect(() => { const expire = () => endSession(true); window.addEventListener(sessionExpiredEvent, expire); return () => window.removeEventListener(sessionExpiredEvent, expire); }, [endSession]);
  useEffect(() => { if (session === 'signed-in' && route.name === 'sign-in') navigate({ name: 'applications' }); }, [session, route.name]);
  useEffect(() => { if (route.name !== 'application') setCreatedApplicationId(undefined); }, [route.name]);
  async function signOut() { try { await api('/auth/sign-out', { method: 'POST' }); } catch { /* local context is cleared regardless */ } endSession(false); }
  async function signIn(username: string, password: string) { const response = await api<SessionResponse>('/auth/sign-in', { method: 'POST', body: JSON.stringify({ username, password }) }); setIdentity({ username: response.user.Username ?? username, role: response.user.Role ?? 'DEVELOPER' }); setSessionEnded(false); setSession('signed-in'); navigate({ name: 'applications' }); void loadApplications(); }
  async function createApplication(name: string, subdomain: string, connectionKey: string) { const response = await api<{ application: APIApplication }>('/applications', { method: 'POST', body: JSON.stringify({ name, subdomain, connectionKey }) }); const application = mapApplication(response.application); setApplications((current) => [...current, application]); setCreatedApplicationId(application.id); return application.id; }
  if (session === 'restoring') return <div className="page" role="status" aria-busy="true">Loading Orchestrator…</div>;
  if (session !== 'signed-in' || route.name === 'sign-in') return <SignInPage onSuccess={signIn} notice={sessionEnded ? 'Your session ended. Sign in again to continue.' : undefined} />;
  const application = 'applicationId' in route ? applications.find((item) => item.id === route.applicationId) : undefined;
  const isPlatformEngineer = identity.role === 'PLATFORM_ENGINEER' || identity.role === 'ADMIN';
  return <AppShell onSignOut={signOut} username={identity.username} role={identity.role} activeRoute={route.name}>{route.name === 'applications' ? <ApplicationsPage applications={applications} state={applicationsState} onRetry={() => void loadApplications()} /> : null}{route.name === 'resource-types' && isPlatformEngineer ? <ResourceTypesPage /> : null}{route.name === 'resource-definitions' && isPlatformEngineer ? <ResourceDefinitionsPage /> : null}{route.name === 'connections' && isPlatformEngineer ? <ConnectionsPage /> : null}{route.name === 'create-application' ? <CreateApplicationPage onCreate={createApplication} /> : null}{route.name === 'application' && application ? <ApplicationHomePage application={application} created={createdApplicationId === application.id} /> : null}{route.name === 'settings' && application ? <SettingsPage application={application} /> : null}{route.name === 'workload' && application ? <WorkloadEditorPage application={application} environment={route.environment} workloadId={route.workloadId} /> : null}{route.name === 'score-preview' && application ? <ScorePreviewPage application={application} environment={route.environment} /> : null}{route.name === 'deployments' && application ? <DeploymentHistoryPage application={application} environment={route.environment} /> : null}{route.name === 'deployment' && application ? <DeploymentDetailsPage application={application} environment={route.environment} deploymentId={route.deploymentId} /> : null}{'applicationId' in route && !application ? applicationsState === 'loading' ? <section className="page" role="status" aria-busy="true">Loading application…</section> : applicationsState === 'error' ? <section className="page"><div className="form-error" role="alert">Could not load applications.<Button onClick={() => void loadApplications()}>Retry</Button></div></section> : <section className="page"><h1>Application not found</h1></section> : null}</AppShell>;
}
