import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { unconfiguredTarget, type Application } from '../../shared/types/application';
import { ScorePreviewPage } from './ScorePreviewPage';
import { parseScoreText } from './parseScore';
import { bothConfigured, configuredTarget } from '../../test/targets';

const application: Application = { id: 'shop', name: 'Shop', subdomain: 'shop', environments: bothConfigured('internal-cluster'), workloads: { staging: [], production: [] } };
const yamlScore = 'apiVersion: score.dev/v1b1\nmetadata:\n  name: api\ncontainers:\n  main:\n    image: example.invalid/api:v1\n';
const result = (overrides: Record<string, unknown> = {}) => ({
  applicationKey: 'shop', environmentKey: 'staging', baseSetId: 'set-1', baseVersion: 3, runId: 'run-7', workloadId: 'api', action: 'deploy',
  planHash: '0123456789abcdef0123', delta: { modules: { add: { api: {} } } },
  candidateSet: { modules: { api: {}, web: { spec: { containers: { main: { variables: { TOKEN: '***redacted***' } } } } } }, shared: {} },
  graph: { nodes: [{ descriptor: 'k8s-namespace.default#environments.shop.staging', kind: 'resource', resourceType: 'k8s-namespace', class: 'default', origins: [], paramKeys: [], bindings: {} }, { descriptor: 'workload.api', kind: 'workload', resourceType: 'workload', class: 'default', origins: [], paramKeys: ['size'], bindings: {} }], edges: [{ consumer: 'workload.api', provider: 'k8s-namespace.default#environments.shop.staging', reason: 'execution-profile' }] },
  matches: [{ descriptor: 'k8s-namespace.default#environments.shop.staging', definitionKey: 'namespace-kubernetes', driverType: 'kubernetes', specificity: 0 }],
  batches: [['k8s-namespace.default#environments.shop.staging']],
  classification: { existing: [], new: ['k8s-namespace.default#environments.shop.staging'], unreferenced: [] },
  ...overrides,
});

afterEach(() => vi.unstubAllGlobals());

async function fill(user: ReturnType<typeof userEvent.setup>, score = yamlScore) {
  await user.type(screen.getByLabelText('Workload ID'), 'api');
  await user.type(screen.getByLabelText('Run ID'), 'run-7');
  fireEvent.change(screen.getByLabelText('Score after (YAML or JSON)'), { target: { value: score } });
}

it('parses YAML locally and shows every planning artifact without save or deploy controls', async () => {
  const fetcher = vi.fn(async () => Response.json(result()));
  vi.stubGlobal('fetch', fetcher);
  const user = userEvent.setup();
  render(<ScorePreviewPage application={application} environment="staging" />);
  expect(screen.getByText(/Do not paste secret values/)).toBeInTheDocument();
  expect(screen.queryByLabelText('Score before (YAML or JSON)')).not.toBeInTheDocument();
  await fill(user);
  await user.click(screen.getByRole('button', { name: 'Preview' }));

  const region = await screen.findByRole('region', { name: 'Score preview result' });
  const [url, init] = fetcher.mock.calls[0] as unknown as [string, RequestInit];
  expect(url).toBe('/api/v1/applications/shop/environments/staging/score-preview');
  expect(JSON.parse(String(init.body))).toEqual({ workloadId: 'api', action: 'deploy', runId: 'run-7', scoreAfter: { apiVersion: 'score.dev/v1b1', metadata: { name: 'api' }, containers: { main: { image: 'example.invalid/api:v1' } } } });
  expect(region).toHaveTextContent('set-1');
  expect(region).toHaveTextContent('0123456789abcdef');
  expect(region).toHaveTextContent('add module api');
  expect(region).toHaveTextContent('Batch 1: k8s-namespace.default#environments.shop.staging');
  expect(region).toHaveTextContent('namespace-kubernetes');
  expect(region).toHaveTextContent('workload.api → k8s-namespace.default#environments.shop.staging');
  expect(region).toHaveTextContent('Sanitized view');
  expect(screen.queryByRole('button', { name: /deploy|save/i })).not.toBeInTheDocument();
});

it('keeps input and blocks the request on local validation errors', async () => {
  const fetcher = vi.fn();
  vi.stubGlobal('fetch', fetcher);
  const user = userEvent.setup();
  render(<ScorePreviewPage application={application} environment="staging" />);
  await user.selectOptions(screen.getByLabelText('Action'), 'update');
  fireEvent.change(screen.getByLabelText('Score after (YAML or JSON)'), { target: { value: 'containers: [' } });
  await user.click(screen.getByRole('button', { name: 'Preview' }));
  const alert = screen.getByRole('alert');
  expect(alert).toHaveTextContent('Workload ID is required.');
  expect(alert).toHaveTextContent('Run ID is required.');
  expect(alert).toHaveTextContent('Score before is required.');
  expect(alert).toHaveTextContent('Score after is not valid YAML or JSON');
  expect(screen.getByLabelText('Score after (YAML or JSON)')).toHaveValue('containers: [');
  expect(fetcher).not.toHaveBeenCalled();
});

it('shows server validation as actionable and failures with retry, never a stale result', async () => {
  let call = 0;
  vi.stubGlobal('fetch', vi.fn(async () => {
    call += 1;
    if (call === 1) return Response.json({ error: 'scoreAfter metadata.name "web" does not match workloadId "api"' }, { status: 400 });
    if (call === 2) return Response.json({ error: 'preview failed; retry' }, { status: 500 });
    return Response.json(result({ delta: {} }));
  }));
  const user = userEvent.setup();
  render(<ScorePreviewPage application={application} environment="staging" />);
  await fill(user);
  await user.click(screen.getByRole('button', { name: 'Preview' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('Cannot preview: scoreAfter metadata.name "web" does not match workloadId "api"');
  await user.click(screen.getByRole('button', { name: 'Preview' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('Preview failed. Try again.');
  await user.click(screen.getByRole('button', { name: 'Retry' }));
  expect(await screen.findByText(/No workload change/)).toBeInTheDocument();
  expect(screen.queryByRole('alert')).not.toBeInTheDocument();
});

it('explains an UNCONFIGURED Environment instead of showing a plan', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => Response.json({ error: 'the Environment has no execution connection; set one in Environment Settings', field: 'connectionKey', code: 'ENVIRONMENT_UNCONFIGURED' }, { status: 422 })));
  const user = userEvent.setup();
  render(<ScorePreviewPage application={{ ...application, environments: { staging: unconfiguredTarget, production: configuredTarget('lab') } }} environment="staging" />);
  expect(screen.getByLabelText('Execution target')).toHaveTextContent('Staging has no execution connection yet');
  await fill(user);
  await user.click(screen.getByRole('button', { name: 'Preview' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('Open Environment settings for staging');
  expect(screen.queryByText(/No workload change/)).not.toBeInTheDocument();
});

it('disables duplicate submits and ignores a response for input that changed', async () => {
  let resolve: (value: Response) => void = () => undefined;
  const fetcher = vi.fn(() => new Promise<Response>((done) => { resolve = done; }));
  vi.stubGlobal('fetch', fetcher);
  const user = userEvent.setup();
  render(<ScorePreviewPage application={application} environment="staging" />);
  await fill(user);
  await user.click(screen.getByRole('button', { name: 'Preview' }));
  expect(screen.getByRole('button', { name: 'Calculating preview…' })).toBeDisabled();
  await user.type(screen.getByLabelText('Run ID'), '8');
  resolve(Response.json(result()));
  await waitFor(() => expect(screen.getByRole('button', { name: 'Preview' })).toBeEnabled());
  expect(screen.queryByRole('region', { name: 'Score preview result' })).not.toBeInTheDocument();
  expect(fetcher).toHaveBeenCalledTimes(1);
});

it('clears the result when the Environment scope changes', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => Response.json(result())));
  const user = userEvent.setup();
  const { rerender } = render(<ScorePreviewPage application={application} environment="staging" />);
  await fill(user);
  await user.click(screen.getByRole('button', { name: 'Preview' }));
  await screen.findByRole('region', { name: 'Score preview result' });
  rerender(<ScorePreviewPage application={application} environment="production" />);
  expect(screen.queryByRole('region', { name: 'Score preview result' })).not.toBeInTheDocument();
  expect(screen.getByText('Preview Score').closest('header')).toHaveTextContent('production');
});

it('ignores a late response after the Environment scope changed', async () => {
  let resolve: (value: Response) => void = () => undefined;
  vi.stubGlobal('fetch', vi.fn(() => new Promise<Response>((done) => { resolve = done; })));
  const user = userEvent.setup();
  const { rerender } = render(<ScorePreviewPage application={application} environment="staging" />);
  await fill(user);
  await user.click(screen.getByRole('button', { name: 'Preview' }));
  rerender(<ScorePreviewPage application={application} environment="production" />);
  resolve(Response.json(result()));
  await waitFor(() => expect(screen.getByRole('button', { name: 'Preview' })).toBeEnabled());
  expect(screen.queryByRole('region', { name: 'Score preview result' })).not.toBeInTheDocument();
});

it('shows the full plan hash and labels documents as a sanitized view', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => Response.json(result())));
  const user = userEvent.setup();
  render(<ScorePreviewPage application={application} environment="staging" />);
  await fill(user);
  await user.click(screen.getByRole('button', { name: 'Preview' }));
  expect(await screen.findByTitle('0123456789abcdef0123')).toHaveTextContent('0123456789abcdef0123');
  expect(screen.getByRole('heading', { name: 'Documents (sanitized view)' })).toBeInTheDocument();
  expect(screen.getByText('Candidate Deployment Set — sanitized view, not executable')).toBeInTheDocument();
});

it('rejects YAML alias bombs and self-references with actionable messages', () => {
  const bomb = ['a: &a [x, x, x, x, x, x, x, x, x]', 'b: &b [*a, *a, *a, *a, *a, *a, *a, *a, *a]', 'c: &c [*b, *b, *b, *b, *b, *b, *b, *b, *b]', 'd: [*c, *c, *c, *c, *c, *c, *c, *c, *c]'].join('\n');
  const bombResult = parseScoreText(bomb);
  expect(bombResult.ok).toBe(false);
  expect(!bombResult.ok && bombResult.error).toMatch(/aliases/);
  const selfReference = parseScoreText('metadata: &m\n  name: api\n  self: *m\n');
  expect(selfReference.ok).toBe(false);
  expect(!selfReference.ok && selfReference.error).toMatch(/self-references|aliases/);
  expect(parseScoreText('base: &b {image: x}\nother: *b')).toEqual({ ok: true, score: { base: { image: 'x' }, other: { image: 'x' } } });
});

it('accepts one JSON or YAML Score object only', () => {
  expect(parseScoreText('{"metadata":{"name":"api"}}')).toEqual({ ok: true, score: { metadata: { name: 'api' } } });
  expect(parseScoreText('a: 1\n---\nb: 2')).toEqual({ ok: false, error: 'must contain exactly one document.' });
  expect(parseScoreText('- a')).toEqual({ ok: false, error: 'must be a Score object.' });
  expect(parseScoreText('   ')).toEqual({ ok: false, error: 'is required.' });
});

it('shows the Environment connection binding that planning will use', () => {
  render(<ScorePreviewPage application={{ ...application, environments: { staging: configuredTarget('internal-cluster'), production: configuredTarget('lab') } }} environment="production" />);
  expect(screen.getByLabelText('Execution target')).toHaveTextContent('Production · Connection lab (lab) · profile internal-k8s');
});
