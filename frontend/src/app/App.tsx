import { DeployPage } from '../features/deploy/pages/DeployPage';
import { DeploymentDetailsPage } from '../features/deployment-details/pages/DeploymentDetailsPage';
import { hrefFor, navigate, useRoute } from './router';

export function App() {
  const route = useRoute();
  return (
    <div className="shell">
      <header className="shell-header">
        <a
          className="brand"
          href={hrefFor({ name: 'deploy' })}
          onClick={(event) => {
            event.preventDefault();
            navigate({ name: 'deploy' });
          }}
        >
          Orchestrator Console
        </a>
        <span className="shell-scope">UC-06 · UC-08 · UC-09</span>
      </header>
      <main className="shell-main">
        {route.name === 'deploy' ? <DeployPage /> : null}
        {route.name === 'deployment-details' ? <DeploymentDetailsPage deploymentId={route.deploymentId} /> : null}
        {route.name === 'not-found' ? <p data-testid="not-found">No console page matches {route.path}.</p> : null}
      </main>
    </div>
  );
}
