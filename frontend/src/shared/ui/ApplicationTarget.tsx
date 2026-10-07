import type { Application } from '../types/application';

// Shows the saved Application binding (UC-01 BR-08); it applies to both Environments.
export function ApplicationTarget({ application }: { application: Application }) {
  return <p className="application-target" aria-label="Execution target">Connection <strong>{application.connectionKey}</strong> · profile <strong>{application.profile}</strong>{application.region ? <> · region <strong>{application.region}</strong></> : null} · both environments</p>;
}
