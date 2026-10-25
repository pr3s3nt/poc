import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { OverviewPanel } from './OverviewPanel';
import { HistoryPage } from './HistoryPage';
import { filledPoints, percentage, signalsOf, text } from './model';
import type { Snapshot } from './api';
import { href, parseRoute } from '../../app/routes';
import { bothConfigured } from '../../test/targets';

const stamp = Date.now();
const at = (offset = 0) => new Date(stamp + offset).toISOString();
function fixture(): Snapshot {
  const sample = { workloadId: 'api', timestamp: at(-15000), metricsAt: at(-15000), cpuMilli: 100, memoryBytes: 104857600, cpuLimitMilli: 500, memoryLimitBytes: null, readyPods: 1, totalPods: 1, restarts: 1 };
  return { state: 'RECEIVING', from: at(-3600000), to: at(), generatedAt: at(), stepSeconds: 30, workloads: ['api'], sources: [{ workloadId: 'api', logsCode: 'OK', metricsCode: 'OK', observedAt: at(), lastLogAt: at(-1000), lastMetricAt: at(-15000), storageLimited: false }], signalCount: 1, logsRetentionDays: 7, metricsRetentionDays: 15, hasHistory: true, logs: [{ id: 'one', workloadId: 'api', timestamp: at(-1000), podUid: 'pod-one', podName: 'api-abc', container: 'main', restartIndex: 0, level: 'ERROR', message: 'ERROR <script>plain text</script>' }], logCount: 1, errorCount: 1, histogram: [{ timestamp: at(-1000), count: 1 }], points: [sample], latest: [sample], peaks: [{ workloadId: 'api', metric: 'cpu', timestamp: at(-600000), value: 450, limit: 500, percent: 90 }], usagePeaks: [{ workloadId: 'api', metric: 'memory', timestamp: at(-15000), value: 104857600 }], events: [], eventCount: 0 };
}
afterEach(() => { vi.unstubAllGlobals(); window.history.replaceState({}, '', '/'); });
it('preserves drill-down scope in routes without sending UI focus to the API', async () => {
  const route = { name: 'observability' as const, applicationId: 'app/a', environment: 'production' as const, tab: 'logs' as const, query: { from: at(-3600000), to: at(), workload: 'api', level: 'ERROR', q: 'a & b', focus: 'cpu' as const } };
  expect(parseRoute(href(route))).toEqual(route);
  const requests: URL[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string) => { requests.push(new URL(input, 'http://localhost')); return Response.json(fixture()); }));
  render(<HistoryPage application={{ id: 'app/a', name: 'App', subdomain: 'app', environments: bothConfigured(), workloads: { staging: [], production: [] } }} environment="production" tab="logs" query={route.query} />);
  expect(await screen.findByText('ERROR <script>plain text</script>')).toBeInTheDocument();
  expect(document.querySelector('script')).toBeNull();
  expect(requests[0]?.searchParams.get('focus')).toBeNull();
  expect(requests[0]?.searchParams.get('q')).toBe('a & b');
  expect(requests[0]?.pathname).toContain('/app%2Fa/environments/production/');
});
it('shows actual overview facts and opens a filtered error view', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => Response.json(fixture())));
  const user = userEvent.setup(); render(<OverviewPanel applicationId="shop" environment="staging" />);
  const card = await screen.findByRole('button', { name: new RegExp(text('logErrors')) });
  expect(card).toHaveTextContent('1');
  expect(screen.getByText(text('recovered'))).toBeInTheDocument();
  await user.click(card);
  expect(parseRoute()).toMatchObject({ name: 'observability', applicationId: 'shop', environment: 'staging', tab: 'logs', query: { level: 'ERROR', from: fixture().from, to: fixture().to } });
});
it('ignores a late response from the previous Environment', async () => {
  let resolveOld!: (response: Response) => void;
  vi.stubGlobal('fetch', vi.fn((input: string) => input.includes('/staging/') ? new Promise<Response>(resolve => { resolveOld = resolve; }) : Promise.resolve(Response.json({ ...fixture(), state: 'NOT_DEPLOYED', sources: [], hasHistory: false }))));
  const view = render(<OverviewPanel applicationId="shop" environment="staging" />);
  await waitFor(() => expect(resolveOld).toBeTypeOf('function'));
  view.rerender(<OverviewPanel applicationId="shop" environment="production" />);
  await screen.findByText(text('noDeploy'));
  await act(async () => { resolveOld(Response.json(fixture())); });
  expect(screen.queryByText('ERROR <script>plain text</script>')).not.toBeInTheDocument();
  expect(screen.getByText(text('noDeploy'))).toBeInTheDocument();
});
it('keeps gaps null, uses absolute units without limits and never labels stale values as recovered', () => {
  const data = fixture();
  const points = filledPoints(data, 'api', 'cpu');
  expect(points.some(point => point.value === null)).toBe(true);
  expect(points.some(point => point.value === 100)).toBe(true);
  expect(percentage(0, 500)).toBe(0);
  expect(percentage(100, null)).toBeNull();
  data.latest[0]!.metricsAt = at(-300000);
  expect(signalsOf(data)[0]?.state).toBe(text('currentUnavailable'));
});

it('pauses at the latest followed interval without rewinding received logs', async () => {
  const original = fixture();
  const newer = { ...original.logs[0]!, id: 'newer', timestamp: at(30000), message: 'INFO new live record', level: 'INFO' };
  const now = vi.spyOn(Date, 'now').mockReturnValue(stamp);
  vi.stubGlobal('fetch', vi.fn(async (input: string) => {
    const url = new URL(input, 'http://localhost'), from = url.searchParams.get('from')!, to = url.searchParams.get('to')!;
    const advanced = Date.parse(to) > Date.parse(newer.timestamp);
    return Response.json({ ...original, from, to, logs: advanced ? [newer, ...original.logs] : original.logs, logCount: advanced ? 2 : 1 });
  }));
  const user = userEvent.setup();
  render(<HistoryPage application={{ id: 'shop', name: 'Shop', subdomain: 'shop', environments: bothConfigured(), workloads: { staging: [], production: [] } }} environment="staging" tab="logs" />);
  await screen.findByText(original.logs[0]!.message);
  now.mockReturnValue(stamp + 60000);
  await user.click(screen.getByRole('button', { name: text('follow') }));
  await screen.findByText(newer.message);
  now.mockReturnValue(stamp + 90000);
  await user.click(screen.getByRole('button', { name: text('pause') }));
  await waitFor(() => expect(screen.getByRole('button', { name: text('follow') })).toBeVisible());
  expect(await screen.findByText(newer.message)).toBeInTheDocument();
  now.mockRestore();
});
