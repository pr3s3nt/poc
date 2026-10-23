import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { EnvironmentResources } from './EnvironmentResources';
import { apiError, stubApi } from '../../test/api';
import { rootId, rootName } from '../../test/resources';

afterEach(() => vi.unstubAllGlobals());

const summary = (over = {}) => ({ id: rootId, descriptor: rootName, type: 'postgres', class: 'default', resourceId: 'shared.db', status: 'UNREFERENCED', scope: 'SHARED', generation: 0, module: 'aurora', ...over });

describe('Environment resources list', () => {
  it('lists an UNREFERENCED resource after its Workload was removed and opens its detail', async () => {
    stubApi(() => Response.json({ resources: [summary(), summary({ id: 'b', descriptor: 'redis.default#shared.cache', status: 'READY', type: 'redis' })] }));
    const user = userEvent.setup();
    render(<EnvironmentResources applicationId="shop" environment="staging" />);
    expect(screen.getByRole('status')).toHaveTextContent('Đang tải tài nguyên…');
    const entry = await screen.findByRole('button', { name: new RegExp(rootName) });
    expect(entry).toHaveTextContent('Không còn được tham chiếu');
    await user.click(entry);
    expect(window.location.pathname).toBe(`/ui/applications/shop/environments/staging/resources/${rootId}`);
  });

  it('distinguishes an empty list from a failed load', async () => {
    let fail = true;
    const calls = stubApi(() => fail ? apiError(500, 'INTERNAL_ERROR') : Response.json({ resources: [] }));
    const user = userEvent.setup();
    render(<EnvironmentResources applicationId="shop" environment="staging" />);
    expect(await screen.findByRole('alert')).toHaveTextContent('Không thể tải danh sách tài nguyên.');
    expect(screen.queryByText(/Chưa có tài nguyên nào/)).not.toBeInTheDocument();
    fail = false;
    await user.click(screen.getByRole('button', { name: 'Thử lại' }));
    expect(await screen.findByText('Chưa có tài nguyên nào trong staging.')).toBeInTheDocument();
    expect(calls).toHaveLength(2);
  });

  it('treats a malformed list as an error, not as an empty list', async () => {
    stubApi(() => Response.json({ resources: [{ id: 'x' }] }));
    render(<EnvironmentResources applicationId="shop" environment="staging" />);
    expect(await screen.findByRole('alert')).toBeInTheDocument();
  });

  it('drops a late reply for another Environment', async () => {
    const pending: Record<string, (r: Response) => void> = {};
    stubApi((c) => new Promise<Response>((resolve) => { pending[c.url] = resolve; }) as unknown as Response);
    const view = render(<EnvironmentResources applicationId="shop" environment="staging" />);
    await waitFor(() => expect(Object.keys(pending)).toHaveLength(1));
    view.rerender(<EnvironmentResources applicationId="shop" environment="production" />);
    await waitFor(() => expect(Object.keys(pending)).toHaveLength(2));
    const [stagingUrl, productionUrl] = Object.keys(pending) as [string, string];
    expect(stagingUrl).toContain('/environments/staging/');
    await act(async () => { pending[productionUrl]?.(Response.json({ resources: [summary({ descriptor: 'postgres.default#shared.prod' })] })); });
    await act(async () => { pending[stagingUrl]?.(Response.json({ resources: [summary({ descriptor: 'postgres.default#shared.STAGING-LATE' })] })); });
    expect(await screen.findByText('postgres.default#shared.prod')).toBeInTheDocument();
    expect(screen.queryByText('postgres.default#shared.STAGING-LATE')).not.toBeInTheDocument();
  });
});

it('keeps a 503 unavailable inventory distinct from an empty list and allows an explicit read retry', async () => {
  let unavailable = true;
  const calls = stubApi(() => unavailable ? apiError(503, 'SERVICE_UNAVAILABLE') : Response.json({ resources: [] }));
  const user = userEvent.setup();
  render(<EnvironmentResources applicationId="shop" environment="staging" />);
  expect(await screen.findByText('Danh sách tài nguyên hiện chưa khả dụng')).toBeInTheDocument();
  expect(screen.queryByText(/Chưa có tài nguyên nào/)).not.toBeInTheDocument();
  expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  unavailable = false;
  await user.click(screen.getByRole('button', { name: 'Kiểm tra lại' }));
  expect(await screen.findByText('Chưa có tài nguyên nào trong staging.')).toBeInTheDocument();
  expect(calls).toHaveLength(2);
});
