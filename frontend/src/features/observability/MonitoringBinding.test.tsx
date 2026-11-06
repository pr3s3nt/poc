import { render, screen, act } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { MonitoringBinding } from './MonitoringBinding';
afterEach(() => vi.unstubAllGlobals());
it('names each source control independently from its option contents', async () => {
 vi.stubGlobal('fetch', vi.fn(async (input: unknown) => Response.json(String(input).includes('/connections') ? { connections: [{ id: 'one', name: 'Owned source', verifiedAt: '2026-11-07', prometheusUrl: 'https://prom.test', lokiUrl: 'https://loki.test' }] } : { binding: null })));
 render(<MonitoringBinding applicationId="app" environment="staging" busy={false} />);
 expect(await screen.findByRole('combobox', { name: 'Kết nối giám sát' })).toBeInTheDocument();
 expect(screen.getByRole('combobox', { name: 'Nguồn metrics' })).toBeInTheDocument();
 expect(screen.getByRole('combobox', { name: 'Nguồn logs' })).toBeInTheDocument();
});
it('discards late binding reads after environment changes', async () => {
 let finish: (r: Response) => void = () => {};
 vi.stubGlobal('fetch', vi.fn(async (input: unknown) => String(input).includes('/connections') ? Response.json({ connections: [{ id: 'one', name: 'Owned source' }] }) : String(input).includes('/staging/') ? new Promise<Response>(r => { finish = r; }) : Response.json({ binding: null })));
 const view = render(<MonitoringBinding applicationId="app" environment="staging" busy={false} />);
 view.rerender(<MonitoringBinding applicationId="app" environment="production" busy={false} />);
 await screen.findByRole('combobox', { name: 'Kết nối giám sát' });
 await act(async () => finish(Response.json({ binding: { connectionId: 'one', revision: 7 } })));
 expect(screen.getByRole('combobox', { name: 'Kết nối giám sát' })).toHaveValue('');
});
