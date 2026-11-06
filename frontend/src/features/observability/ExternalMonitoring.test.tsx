import { act, render, screen } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { ExternalMonitoring } from './ExternalMonitoring';
vi.mock('./OverviewPanel', () => ({ OverviewPanel: () => <p>native-history</p> }));
afterEach(() => vi.unstubAllGlobals());
it('selects native only when the environment is explicitly unbound', async () => {
 vi.stubGlobal('fetch', vi.fn(async () => Response.json({ state: 'NOT_CONFIGURED', workloads: [], alerts: [] })));
 render(<ExternalMonitoring applicationId="app" environment="staging" />);
 expect(await screen.findByText('native-history')).toBeInTheDocument();
});
it('does not switch to native or invent healthy values when external queries fail', async () => {
 vi.stubGlobal('fetch', vi.fn(async () => Response.json({ code: 'SERVICE_UNAVAILABLE' }, { status: 503 })));
 render(<ExternalMonitoring applicationId="app" environment="staging" />);
 await screen.findByRole('alert');
 expect(screen.queryByText('native-history')).not.toBeInTheDocument();
});
it('discards a late response from a previous environment', async () => {
 let finish: (r: Response) => void = () => {};
 vi.stubGlobal('fetch', vi.fn(async (input: unknown) => String(input).includes('/staging/') ? new Promise<Response>(r => { finish = r; }) : Response.json({ state: 'NOT_DEPLOYED', workloads: [], alerts: [] })));
 const view = render(<ExternalMonitoring applicationId="app" environment="staging" />);
 view.rerender(<ExternalMonitoring applicationId="app" environment="production" />);
 await screen.findByText(/Prometheus · production/);
 await act(async () => finish(Response.json({ state: 'NOT_CONFIGURED', workloads: [], alerts: [] })));
 expect(screen.queryByText('native-history')).not.toBeInTheDocument();
});
it('uses native when the bound metrics source explicitly selects native', async () => {
 vi.stubGlobal('fetch', vi.fn(async () => Response.json({ source: 'native', state: 'METRICS_NATIVE', workloads: [], alerts: [] })));
 render(<ExternalMonitoring applicationId="app" environment="staging" />);
 expect(await screen.findByText('native-history')).toBeInTheDocument();
});
