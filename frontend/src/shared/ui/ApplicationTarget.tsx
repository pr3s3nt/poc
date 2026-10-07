import { navigate } from '../../app/routes';
import type { Application, EnvironmentKey } from '../types/application';

// Shows the selected Environment's saved execution target (ADR-011). Each
// Environment has its own; there is no Application-wide target.
export function ApplicationTarget({ application, environment }: { application: Application; environment: EnvironmentKey }) {
  const target = application.environments[environment];
  if (!target.configured) {
    return <p className="application-target" aria-label="Execution target">{environment === 'staging' ? 'Staging' : 'Production'} has no execution connection yet · <a href="#" onClick={(event) => { event.preventDefault(); navigate({ name: 'settings', applicationId: application.id, environment }); }}>Choose one in Settings</a></p>;
  }
  return <p className="application-target" aria-label="Execution target">{environment === 'staging' ? 'Staging' : 'Production'} · Connection <strong>{target.connectionName || target.connectionKey}</strong> ({target.connectionKey}) · profile <strong>{target.profile}</strong>{target.region ? <> · region <strong>{target.region}</strong></> : null}</p>;
}
