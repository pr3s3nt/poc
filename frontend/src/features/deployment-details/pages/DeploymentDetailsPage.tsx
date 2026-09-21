import { useEffect, useState } from 'react';
import { fetchDeployment } from '../../deploy/api/client';
import type { DeploymentView } from '../../deploy/api/types';
import { Callout } from '../../../shared/ui/Callout';
import { StatusBadge } from '../../../shared/ui/StatusBadge';
import { hrefFor, navigate } from '../../../app/router';

interface DeploymentDetailsPageProps {
  readonly deploymentId: string;
}

type ViewState =
  | { readonly phase: 'loading' }
  | { readonly phase: 'error'; readonly message: string }
  | { readonly phase: 'ready'; readonly view: DeploymentView };

function renderOutputValue(value: unknown): string {
  if (typeof value === 'string') {
    return value;
  }
  return JSON.stringify(value);
}

export function DeploymentDetailsPage({ deploymentId }: DeploymentDetailsPageProps) {
  const [state, setState] = useState<ViewState>({ phase: 'loading' });

  useEffect(() => {
    let cancelled = false;
    setState({ phase: 'loading' });
    fetchDeployment(deploymentId)
      .then((view) => {
        if (!cancelled) {
          setState({ phase: 'ready', view });
        }
      })
      .catch((cause: unknown) => {
        if (!cancelled) {
          setState({ phase: 'error', message: cause instanceof Error ? cause.message : 'failed to load' });
        }
      });
    return () => {
      cancelled = true;
    };
  }, [deploymentId]);

  if (state.phase === 'loading') {
    return <p data-testid="details-loading">Loading deployment…</p>;
  }
  if (state.phase === 'error') {
    return <Callout tone="error" title="Could not load the deployment">{state.message}</Callout>;
  }

  const { view } = state;
  const batches = view.batches ?? [];
  const nodes = view.graph?.nodes ?? [];
  const edges = view.graph?.edges ?? [];

  return (
    <section className="page">
      <a
        className="back-link"
        href={hrefFor({ name: 'deploy' })}
        onClick={(event) => {
          event.preventDefault();
          navigate({ name: 'deploy' });
        }}
      >
        ← Deploy another workload
      </a>

      <h1>
        Deployment <span className="mono">{view.deployment.workloadId}</span> <StatusBadge status={view.deployment.status} />
      </h1>
      <dl className="summary">
        <div>
          <dt>Deployment ID</dt>
          <dd className="mono">{view.deployment.id}</dd>
        </div>
        <div>
          <dt>Application / Environment</dt>
          <dd>
            {view.deployment.applicationKey} / {view.deployment.environmentKey}
          </dd>
        </div>
        <div>
          <dt>Execution profile</dt>
          <dd>{view.deployment.executionProfile}</dd>
        </div>
        <div>
          <dt>Action</dt>
          <dd>{view.deployment.action}</dd>
        </div>
        <div>
          <dt>Plan hash</dt>
          <dd className="mono">{view.planHash}</dd>
        </div>
        <div>
          <dt>Started</dt>
          <dd>{view.deployment.startedAt}</dd>
        </div>
      </dl>

      {view.deployment.failureReason ? (
        <Callout tone="error" title="Failure reason">{view.deployment.failureReason}</Callout>
      ) : null}

      <h2>Execution batches</h2>
      {batches.length === 0 ? (
        <p>No resource batch was scheduled.</p>
      ) : (
        <ol className="batches">
          {batches.map((batch, index) => (
            <li key={`batch-${index}`}>
              <span className="batch-label">Batch {index}</span>
              <ul>
                {batch.map((descriptor) => (
                  <li key={descriptor} className="mono">
                    {descriptor}
                  </li>
                ))}
              </ul>
            </li>
          ))}
        </ol>
      )}

      <h2>Resource graph</h2>
      <table>
        <thead>
          <tr>
            <th>Descriptor</th>
            <th>Kind</th>
            <th>Type</th>
            <th>Origins</th>
          </tr>
        </thead>
        <tbody>
          {nodes.map((node) => (
            <tr key={node.descriptor}>
              <td className="mono">{node.descriptor}</td>
              <td>{node.kind}</td>
              <td>{node.resourceType}</td>
              <td>{node.origins.join(', ')}</td>
            </tr>
          ))}
        </tbody>
      </table>
      <p className="hint">{edges.length} consumer → provider edges.</p>

      <h2>Active resources</h2>
      <table data-testid="resources-table">
        <thead>
          <tr>
            <th>Descriptor</th>
            <th>Definition</th>
            <th>Status</th>
            <th>Batch</th>
            <th>Outputs</th>
          </tr>
        </thead>
        <tbody>
          {view.resources.map((row) => (
            <tr key={row.descriptor}>
              <td className="mono">{row.descriptor}</td>
              <td>{row.definitionKey}</td>
              <td>
                <StatusBadge status={row.status} />
              </td>
              <td>{row.batchIndex}</td>
              <td>
                <ul className="outputs">
                  {Object.entries(row.outputs).map(([key, value]) => (
                    <li key={key}>
                      <span className="mono">{key}</span>: <span className="mono">{renderOutputValue(value)}</span>
                    </li>
                  ))}
                </ul>
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      <h2>Workloads</h2>
      <table data-testid="workloads-table">
        <thead>
          <tr>
            <th>Workload</th>
            <th>Status</th>
            <th>Namespace</th>
            <th>Manifest digest</th>
          </tr>
        </thead>
        <tbody>
          {view.workloads.map((row) => (
            <tr key={row.workloadId}>
              <td>{row.workloadId}</td>
              <td>
                <StatusBadge status={row.status} />
              </td>
              <td className="mono">{String(row.targetRef['namespace'] ?? '')}</td>
              <td className="mono">{row.manifestDigest.slice(0, 12)}</td>
            </tr>
          ))}
        </tbody>
      </table>

      <h2>Deployment set</h2>
      <p>Modules: {Object.keys(view.deploymentSet.modules).join(', ') || 'none'}</p>
      <ul>
        {Object.entries(view.deploymentSet.shared).map(([id, entry]) => (
          <li key={id}>
            <span className="mono">{id}</span> — type <span className="mono">{entry.type}</span>, class{' '}
            <span className="mono">{entry.class}</span>
            {entry.params ? <> , params <span className="mono">{JSON.stringify(entry.params)}</span></> : null}
          </li>
        ))}
      </ul>
    </section>
  );
}
