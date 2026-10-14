import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { CleanupProgress } from './CleanupProgress';
import { apiError, stubApi } from '../../test/api';
import { depA, operation, rootId, rootName, wire } from '../../test/resources';

afterEach(() => vi.unstubAllGlobals());

const forOp = (id: string, ...responses: Array<Response | (() => Response)>) => {
  let n = 0;
  return (c: { url: string }) => {
    if (!c.url.endsWith(`/resource-cleanups/${id}`)) return undefined;
    const next = responses[Math.min(n, responses.length - 1)];
    n += 1;
    return typeof next === 'function' ? next() : next?.clone();
  };
};

describe('cleanup progress (UC-09 RES-03)', () => {
  it('polls only the given operation, shows the stages and stops at a terminal state, settling once', async () => {
    const settled = vi.fn();
    const calls = stubApi(forOp('op-1',
      Response.json(wire.cleanup(operation({ stage: 'CHECKING' }))),
      Response.json(wire.cleanup(operation({ stage: 'DESTROYING' }))),
      Response.json(wire.cleanup(operation({ status: 'SUCCEEDED', stage: 'VERIFYING', terminal: 'SUCCEEDED', allVerified: true, resources: [{ id: rootId, descriptor: rootName, currentStatus: 'DESTROYED', outcome: 'DESTROYED', verified: true, remaining: [], remainingUnknown: false }] }))),
    ));
    render(<CleanupProgress applicationId="shop" environment="staging" operationId="op-1" intervalMs={10} onSettled={settled} />);
    expect(screen.getByRole('status')).toHaveTextContent('Đang tải tiến trình xóa…');
    expect(await screen.findByText(/Trạng thái:/)).toHaveTextContent('Đang chạy · Đang kiểm tra');
    expect(await screen.findByText(/đã được xác minh là không còn/)).toBeInTheDocument();
    await new Promise((resolve) => setTimeout(resolve, 60));
    expect(calls).toHaveLength(3);
    expect(settled).toHaveBeenCalledTimes(1);
    expect(calls.every((c) => c.method === 'GET' && c.url.includes('/applications/shop/environments/staging/resource-cleanups/op-1'))).toBe(true);
  });

  it('never calls success while a confirmed resource is unverified, even when the operation says SUCCEEDED', async () => {
    stubApi(forOp('op-1', Response.json(wire.cleanup(operation({ status: 'SUCCEEDED', allVerified: false, resources: [{ id: rootId, descriptor: rootName, currentStatus: 'UNREFERENCED', outcome: 'UNKNOWN', verified: false, remaining: [], remainingUnknown: true }] })))));
    render(<CleanupProgress applicationId="shop" environment="staging" operationId="op-1" />);
    expect(await screen.findByRole('alert')).toHaveTextContent('bằng chứng từng tài nguyên thiếu hoặc mâu thuẫn');
    expect(screen.queryByText(/đã được xác minh là không còn/)).not.toBeInTheDocument();
    expect(screen.getByText('Chưa rõ — không thể coi là đã hết')).toBeInTheDocument();
  });

  it.each([
    ['empty per-resource proof', { status: 'SUCCEEDED', allVerified: true, resources: [] }],
    ['a row that is not verified', { status: 'SUCCEEDED', allVerified: true, resources: [{ id: rootId, descriptor: rootName, currentStatus: 'DESTROYED', outcome: 'DESTROYED', verified: false, remaining: [], remainingUnknown: false }] }],
    ['an unknown remainder', { status: 'SUCCEEDED', allVerified: true, resources: [{ id: rootId, descriptor: rootName, currentStatus: 'DESTROYED', outcome: 'DESTROYED', verified: true, remaining: [], remainingUnknown: true }] }],
    ['an outcome that is not DESTROYED', { status: 'SUCCEEDED', allVerified: true, resources: [{ id: rootId, descriptor: rootName, currentStatus: 'DESTROYED', outcome: 'UNKNOWN', verified: true, remaining: [], remainingUnknown: false }] }],
    ['remaining objects', { status: 'SUCCEEDED', allVerified: true, remainingIds: ['x=1'], resources: [{ id: rootId, descriptor: rootName, currentStatus: 'DESTROYED', outcome: 'DESTROYED', verified: true, remaining: [], remainingUnknown: false }] }],
    ['a root missing from the proof', { status: 'SUCCEEDED', allVerified: true, resources: [{ id: depA, descriptor: 'vpc.default#x', currentStatus: 'DESTROYED', outcome: 'DESTROYED', verified: true, remaining: [], remainingUnknown: false }] }],
  ])('does not show success for SUCCEEDED/allVerified with %s', async (_name, over) => {
    stubApi(forOp('op-1', Response.json(wire.cleanup(operation(over as never)))));
    render(<CleanupProgress applicationId="shop" environment="staging" operationId="op-1" intervalMs={10} />);
    expect(await screen.findByRole('alert')).toHaveTextContent('bằng chứng từng tài nguyên thiếu hoặc mâu thuẫn');
    expect(screen.queryByText(/đã được xác minh là không còn/)).not.toBeInTheDocument();
  });

  it('keeps an unknown status indeterminate: it keeps polling, never settles and says the outcome is unknown', async () => {
    const settled = vi.fn();
    const calls = stubApi(forOp('op-1', Response.json(wire.cleanup(operation({ status: 'MYSTERY' })))));
    render(<CleanupProgress applicationId="shop" environment="staging" operationId="op-1" intervalMs={10} onSettled={settled} />);
    expect(await screen.findByRole('alert')).toHaveTextContent('Trạng thái thao tác xóa không nhận ra được');
    await waitFor(() => expect(calls.length).toBeGreaterThan(3));
    expect(settled).not.toHaveBeenCalled();
    expect(screen.queryByText(/đã được xác minh là không còn/)).not.toBeInTheDocument();
  });

  it('an interrupted operation is held: it never settles the page', async () => {
    const settled = vi.fn();
    stubApi(forOp('op-1', Response.json(wire.cleanup(operation({ status: 'INTERRUPTED' })))));
    render(<CleanupProgress applicationId="shop" environment="staging" operationId="op-1" intervalMs={10} onSettled={settled} />);
    await screen.findByRole('alert');
    await new Promise((resolve) => setTimeout(resolve, 80));
    expect(settled).not.toHaveBeenCalled();
  });

  it('shows an interrupted operation as needing attention, neither deleted nor intact', async () => {
    stubApi(forOp('op-1', Response.json(wire.cleanup(operation({ status: 'INTERRUPTED', terminal: 'NEEDS_ATTENTION', resources: [{ id: rootId, descriptor: rootName, currentStatus: 'DESTROYING', outcome: 'NEEDS_ATTENTION', verified: false, remaining: [], remainingUnknown: true, reasonCode: 'NEEDS_ATTENTION' }] })))));
    render(<CleanupProgress applicationId="shop" environment="staging" operationId="op-1" intervalMs={10} />);
    expect(await screen.findByRole('alert')).toHaveTextContent('chưa rõ kết quả');
    expect(screen.getByText('Chưa rõ kết quả; cần kiểm tra thủ công')).toBeInTheDocument();
  });

  it('maps unknown stages, outcomes and reason codes to generic text', async () => {
    stubApi(forOp('op-1', Response.json(wire.cleanup(operation({ status: 'weird', stage: 'password=hunter2', failureCode: 'RAW', resources: [{ id: rootId, descriptor: rootName, currentStatus: 'x', outcome: 'hunter2-outcome', verified: false, remaining: [], remainingUnknown: true, reasonCode: 'RAW ERROR hunter2' }] })))));
    render(<CleanupProgress applicationId="shop" environment="staging" operationId="op-1" />);
    await screen.findByText(/Trạng thái:/);
    expect(document.body).not.toHaveTextContent('hunter2');
    expect(document.body).not.toHaveTextContent('RAW');
    expect(screen.getByText(/Trạng thái:/)).toHaveTextContent('Trạng thái không rõ');
  });

  it('keeps the last progress on a read failure and retries only on request', async () => {
    let fail = false;
    const calls = stubApi(forOp('op-1', () => fail ? apiError(500, 'INTERNAL_ERROR') : Response.json(wire.cleanup(operation({ stage: 'DESTROYING' })))));
    const user = userEvent.setup();
    render(<CleanupProgress applicationId="shop" environment="staging" operationId="op-1" intervalMs={15} />);
    expect(await screen.findByText(/Trạng thái:/)).toHaveTextContent('Đang chạy · Đang xóa');
    fail = true;
    expect(await screen.findByRole('alert')).toHaveTextContent('Không đọc được tiến trình xóa');
    expect(screen.getByText(/Trạng thái:/)).toHaveTextContent('Đang chạy · Đang xóa');
    expect(screen.queryByText(/đã được xác minh là không còn/)).not.toBeInTheDocument();
    const count = calls.length;
    await new Promise((resolve) => setTimeout(resolve, 60));
    expect(calls.length).toBe(count);
    fail = false;
    await user.click(screen.getByRole('button', { name: 'Tải lại tiến trình' }));
    await waitFor(() => expect(screen.queryByRole('alert')).not.toBeInTheDocument());
  });

  it('drops a late reply for a previous operation or scope', async () => {
    const pending: Record<string, (r: Response) => void> = {};
    stubApi((c) => new Promise<Response>((resolve) => { pending[c.url] = resolve; }) as unknown as Response);
    const view = render(<CleanupProgress applicationId="shop" environment="staging" operationId="op-old" />);
    await waitFor(() => expect(Object.keys(pending)).toHaveLength(1));
    view.rerender(<CleanupProgress applicationId="shop" environment="staging" operationId="op-new" />);
    await waitFor(() => expect(Object.keys(pending)).toHaveLength(2));
    const [oldUrl, newUrl] = Object.keys(pending) as [string, string];
    await act(async () => { pending[newUrl]?.(Response.json(wire.cleanup(operation({ operationId: 'op-new', status: 'FAILED', resources: [{ id: depA, descriptor: 'vpc.default#new', currentStatus: 'UNREFERENCED', outcome: 'REMAINING', verified: false, remaining: ['x=new'], remainingUnknown: false }] })))); });
    await act(async () => { pending[oldUrl]?.(Response.json(wire.cleanup(operation({ operationId: 'op-old', status: 'SUCCEEDED', allVerified: true, resources: [{ id: rootId, descriptor: 'OLD-RESOURCE', currentStatus: 'DESTROYED', outcome: 'DESTROYED', verified: true, remaining: [], remainingUnknown: false }] })))); });
    expect(screen.getByText('vpc.default#new')).toBeInTheDocument();
    expect(screen.queryByText('OLD-RESOURCE')).not.toBeInTheDocument();
    expect(screen.queryByText(/đã được xác minh là không còn/)).not.toBeInTheDocument();
  });
});
