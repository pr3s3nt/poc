import { useEffect, useState } from 'react';
import { navigate } from '../../app/routes';
import type { Application, EnvironmentKey } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { Status } from '../../shared/ui/Status';
import { getDeployment, type DeploymentDetail } from './api';
import { ApiError } from '../../shared/api/client';

const date = (value?: string) => value ? new Date(value).toLocaleString() : '—';

export function DeploymentDetailsPage({ application, environment, deploymentId }: { application: Application; environment: EnvironmentKey; deploymentId: string }) {
  const [detail, setDetail] = useState<DeploymentDetail>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [notFound, setNotFound] = useState(false);
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    let cancelled = false;
    setLoading(true); setError(''); setNotFound(false); setDetail(undefined);
    getDeployment(application.id, environment, deploymentId).then((value) => { if (!cancelled) setDetail(value); })
      .catch((err: unknown) => {
        if (cancelled) return;
        if (err instanceof ApiError && err.status === 404) setNotFound(true);
        else setError('Could not load deployment details.');
      })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [application.id, environment, deploymentId, attempt]);
  return <section className="page">
    <button className="back-link" onClick={() => navigate({ name: 'deployments', applicationId: application.id, environment })}>← {environment} deployment history</button>
    <header className="page-header"><div><p className="eyebrow">{application.name} · {environment}</p><h1>Deployment details</h1><p>Persisted plan and execution state.</p></div><Button onClick={() => setAttempt((value) => value + 1)}>Refresh</Button></header>
    {loading ? <p role="status" aria-busy="true">Loading deployment…</p> : notFound ? <section className="content-panel"><h2>Deployment not found</h2><p>This deployment does not exist in the selected Application and Environment.</p></section> : error ? <div role="alert" className="form-error">{error} <Button onClick={() => setAttempt((value) => value + 1)}>Retry</Button></div> : detail ? <>
      <section className="content-panel"><div className="section-header"><div><h2>{detail.deployment.workloadId} · {detail.deployment.action.toLowerCase()}</h2><p>{detail.deployment.id}</p></div><Status tone={detail.deployment.status === 'SUCCEEDED' ? 'good' : detail.deployment.status === 'FAILED' ? 'draft' : 'neutral'}>{detail.deployment.status}</Status></div>
        {detail.deployment.failureReason ? <div className="form-error" role="alert">{detail.deployment.failureReason}</div> : null}
        <dl className="detail-grid"><div><dt>Started</dt><dd>{date(detail.deployment.startedAt)}</dd></div><div><dt>Finished</dt><dd>{date(detail.deployment.finishedAt)}</dd></div><div><dt>Triggered by</dt><dd>{detail.deployment.actorRef || '—'}</dd></div><div><dt>Deployment Set</dt><dd>{detail.deploymentSetId || '—'}</dd></div></dl></section>
      <section className="content-panel"><h2>Workloads</h2>{detail.workloads.length ? <div className="detail-list">{detail.workloads.map((item) => <div key={item.workloadId}><strong>{item.workloadId}</strong><span>{item.status}</span><small>{item.manifestDigest}</small></div>)}</div> : <p>No workload status was recorded.</p>}</section>
      <section className="content-panel"><h2>Resources</h2>{detail.resources.length ? <div className="detail-list">{detail.resources.map((item) => <div key={item.descriptor}><strong>{item.resourceType || item.descriptor}</strong><span>{item.status} · {item.definitionKey || 'no definition'}</span><small>{item.descriptor}</small>{Object.keys(item.outputs ?? {}).length ? <small>Outputs: {Object.entries(item.outputs).map(([key, value]) => `${key}=${typeof value === 'string' || typeof value === 'number' ? value : '[value]'}`).join(', ')}</small> : null}</div>)}</div> : <p>No resource status was recorded.</p>}</section>
      <section className="content-panel"><h2>Provision plan</h2>{!detail.planHash ? <p>No provision plan was saved for this deployment.</p> : <><p>{detail.graph?.nodes?.length ?? 0} graph nodes · {detail.graph?.edges?.length ?? 0} dependencies · plan {detail.planHash.slice(0, 12)}</p>
        {detail.graph?.nodes?.length ? <><h3>Nodes</h3><ul>{detail.graph.nodes.map((node) => <li key={node.descriptor}>{node.descriptor} · {node.kind}{detail.matches?.[node.descriptor]?.definitionKey ? ` · ${detail.matches?.[node.descriptor]?.definitionKey}` : ''}</li>)}</ul></> : null}
        {detail.graph?.edges?.length ? <><h3>Dependencies</h3><ul>{detail.graph.edges.map((edge, index) => <li key={`${edge.consumer}-${edge.provider}-${index}`}>{edge.consumer} → {edge.provider}</li>)}</ul></> : null}
        <h3>Provision batches</h3>{detail.batches?.length ? <ol>{detail.batches.map((batch, index) => <li key={index}>{batch.join(', ')}</li>)}</ol> : <p>No provision batches were recorded.</p>}</>}</section>
    </> : null}
  </section>;
}
