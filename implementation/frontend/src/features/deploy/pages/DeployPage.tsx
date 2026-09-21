import { useEffect, useMemo, useReducer, useState } from 'react';
import { createDeployment, fetchApplications, fetchScoreSamples } from '../api/client';
import type { ApplicationSummary } from '../api/types';
import { ApiError } from '../../../shared/api/http';
import { Callout } from '../../../shared/ui/Callout';
import { navigate } from '../../../app/router';
import { deployDraftReducer, emptyDraft, validateDraft } from '../draft/reducer';

type LoadState =
  | { readonly phase: 'loading' }
  | { readonly phase: 'error'; readonly message: string }
  | {
      readonly phase: 'ready';
      readonly applications: readonly ApplicationSummary[];
      readonly order: readonly string[];
      readonly samples: Readonly<Record<string, unknown>>;
    };

export function DeployPage() {
  const [load, setLoad] = useState<LoadState>({ phase: 'loading' });
  const [draft, dispatch] = useReducer(deployDraftReducer, emptyDraft);
  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [showErrors, setShowErrors] = useState(false);

  useEffect(() => {
    let cancelled = false;
    Promise.all([fetchApplications(), fetchScoreSamples()])
      .then(([apps, samples]) => {
        if (cancelled) {
          return;
        }
        setLoad({ phase: 'ready', applications: apps.applications, order: samples.order, samples: samples.samples });
        const first = apps.applications[0];
        if (first) {
          dispatch({
            type: 'application-selected',
            applicationKey: first.key,
            environmentKey: first.environments[0]?.key ?? '',
          });
        }
        const firstWorkload = samples.order[0];
        if (firstWorkload) {
          dispatch({
            type: 'workload-selected',
            workloadId: firstWorkload,
            scoreText: JSON.stringify(samples.samples[firstWorkload] ?? {}, null, 2),
          });
        }
      })
      .catch((cause: unknown) => {
        if (cancelled) {
          return;
        }
        setLoad({ phase: 'error', message: cause instanceof Error ? cause.message : 'failed to load' });
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const validation = useMemo(() => validateDraft(draft), [draft]);

  if (load.phase === 'loading') {
    return <p data-testid="deploy-loading">Loading applications…</p>;
  }
  if (load.phase === 'error') {
    return <Callout tone="error" title="Could not load the deploy form">{load.message}</Callout>;
  }
  if (load.applications.length === 0) {
    return <Callout tone="info" title="No application is registered yet">Register an application before deploying.</Callout>;
  }

  const application = load.applications.find((app) => app.key === draft.applicationKey) ?? load.applications[0];

  const onSubmit = (event: React.FormEvent) => {
    event.preventDefault();
    setShowErrors(true);
    setSubmitError(null);
    if (!validation.valid) {
      return;
    }
    setSubmitting(true);
    createDeployment({
      applicationKey: draft.applicationKey,
      environmentKey: draft.environmentKey,
      workloadId: draft.workloadId,
      score: validation.score,
      actor: 'web-console',
    })
      .then((created) => {
        navigate({ name: 'deployment-details', deploymentId: created.deploymentId });
      })
      .catch((cause: unknown) => {
        const message =
          cause instanceof ApiError ? `${cause.message} (HTTP ${cause.status})` : 'deployment request failed';
        setSubmitError(message);
      })
      .finally(() => setSubmitting(false));
  };

  return (
    <section className="page">
      <h1>Deploy workload</h1>
      <p className="page-intro">
        Submit a Score document. The orchestrator plans the deployment, provisions the resource graph and applies the
        workload.
      </p>
      <form onSubmit={onSubmit} className="form">
        <label className="field">
          <span>Application</span>
          <select
            value={draft.applicationKey}
            onChange={(event) => {
              const next = load.applications.find((app) => app.key === event.target.value);
              dispatch({
                type: 'application-selected',
                applicationKey: event.target.value,
                environmentKey: next?.environments[0]?.key ?? '',
              });
            }}
          >
            {load.applications.map((app) => (
              <option key={app.key} value={app.key}>
                {app.name} ({app.executionProfile})
              </option>
            ))}
          </select>
        </label>

        <label className="field">
          <span>Environment</span>
          <select
            value={draft.environmentKey}
            onChange={(event) => dispatch({ type: 'environment-selected', environmentKey: event.target.value })}
          >
            {(application?.environments ?? []).map((env) => (
              <option key={env.key} value={env.key}>
                {env.name} — namespace {env.namespaceIdentity}
              </option>
            ))}
          </select>
        </label>

        <label className="field">
          <span>Workload</span>
          <select
            value={draft.workloadId}
            onChange={(event) =>
              dispatch({
                type: 'workload-selected',
                workloadId: event.target.value,
                scoreText: JSON.stringify(load.samples[event.target.value] ?? {}, null, 2),
              })
            }
          >
            {load.order.map((workload) => (
              <option key={workload} value={workload}>
                {workload}
              </option>
            ))}
          </select>
        </label>

        <label className="field">
          <span>Score document (JSON)</span>
          <textarea
            value={draft.scoreText}
            rows={18}
            spellCheck={false}
            onChange={(event) => dispatch({ type: 'score-edited', scoreText: event.target.value })}
          />
        </label>

        {showErrors && !validation.valid ? (
          <Callout tone="error" title="Fix the form before deploying">
            <ul>
              {validation.errors.map((error) => (
                <li key={error}>{error}</li>
              ))}
            </ul>
          </Callout>
        ) : null}

        {submitError ? <Callout tone="error" title="Deployment failed">{submitError}</Callout> : null}

        <button type="submit" disabled={submitting}>
          {submitting ? 'Deploying…' : 'Deploy'}
        </button>
      </form>
    </section>
  );
}
