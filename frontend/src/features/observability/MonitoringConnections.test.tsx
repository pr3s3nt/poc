import { render, screen, fireEvent } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { MonitoringConnections } from './MonitoringConnections';
afterEach(() => vi.unstubAllGlobals());
it('keeps a legacy connection with unspecified Loki retention editable', async () => {
 const connection = { id: 'one', name: 'Legacy source', revision: 1, clusterUid: 'cluster', prometheusUrl: 'https://prom.test', lokiUrl: 'https://loki.test', lokiRetentionDays: 0, rules: { restartWindow: 300000000000, restartThreshold: 3, availabilityFor: 300000000000, restartFor: 60000000000, oomFor: 60000000000 }, ruleLabels: {} };
 vi.stubGlobal('fetch', vi.fn(async () => Response.json({ connections: [connection] })));
 render(<MonitoringConnections />);
 fireEvent.click(await screen.findByRole('button', { name: 'Sửa' }));
 const retention = document.querySelector<HTMLInputElement>('#monitor-lokiRetention')!;
 expect(retention.value).toBe('3');
 expect(retention.validity.valid).toBe(true);
 expect(retention.closest('form')!.checkValidity()).toBe(true);
});
