import { act, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { DeleteResourceDialog } from './DeleteResourceDialog';
import { ResourceDetailPage } from './ResourceDetailPage';
import type { Application } from '../../shared/types/application';
import { bothConfigured } from '../../test/targets';
import { apiError, stubApi, type Call } from '../../test/api';
import { candidate, depA, depC, detail, operation, preview, rootId, rootName, wire } from '../../test/resources';

const application: Application = { id: 'shop', name: 'Shop', subdomain: 'shop', environments: bothConfigured('internal-cluster'), workloads: { staging: [], production: [] } };
const isPreview = (c: Call) => c.method === 'POST' && c.url.endsWith('/deletion-preview');
const isDeletion = (c: Call) => c.method === 'POST' && c.url.endsWith('/deletions');
const deletions = (calls: Call[]) => calls.filter(isDeletion);

afterEach(() => vi.unstubAllGlobals());

function open(props: Partial<React.ComponentProps<typeof DeleteResourceDialog>> = {}) {
  const handlers = { onClose: vi.fn(), onAccepted: vi.fn(), onLost: vi.fn() };
  const view = render(<DeleteResourceDialog applicationId="shop" environment="staging" resourceId={rootId} {...handlers} {...props} />);
  return { ...handlers, view };
}

const checkbox = (name: RegExp) => screen.getByRole('checkbox', { name });
const confirmButton = () => screen.getByRole('button', { name: /Xác nhận xóa vĩnh viễn|Đang gửi yêu cầu xóa/ });

describe('deletion dialog (UC-07 DEL-MS-02..04)', () => {
  it('is an accessible modal that shows the scope, the permanent data-loss warning and ticks only the root', async () => {
    stubApi((c) => isPreview(c) ? Response.json(wire.preview(preview())) : undefined);
    open();
    const dialog = screen.getByRole('dialog', { name: 'Xóa tài nguyên' });
    expect(dialog).toHaveAttribute('aria-modal', 'true');
    expect(screen.getByRole('status')).toHaveTextContent('Đang tải phạm vi xóa…');
    expect(await screen.findByTestId('root-name')).toHaveTextContent(rootName);
    expect(within(dialog).getByText(/Xóa vĩnh viễn dữ liệu, không thể hoàn tác/)).toBeInTheDocument();
    expect(within(dialog).getByText('shop')).toBeInTheDocument();
    expect(within(dialog).getByText('staging')).toBeInTheDocument();
    expect(checkbox(/tài nguyên gốc/)).toBeChecked();
    expect(checkbox(/tài nguyên gốc/)).toBeDisabled();
    expect(checkbox(/vpc\.default/)).not.toBeChecked();
    expect(checkbox(/k8s-secret/)).not.toBeChecked();
    expect(checkbox(/k8s-namespace/)).not.toBeChecked();
    expect(checkbox(/k8s-namespace/)).toBeDisabled();
    expect(screen.getByRole('heading', { name: 'Sẽ xóa 1 tài nguyên' })).toBeInTheDocument();
    expect(confirmButton()).toBeDisabled();
  });

  it('locks a dependency held outside the set and says why, and shows who uses it', async () => {
    stubApi((c) => isPreview(c) ? Response.json(wire.preview(preview())) : undefined);
    open();
    await screen.findByTestId('root-name');
    const reason = screen.getByText(/Không thể chọn:/);
    expect(reason).toHaveTextContent('Vẫn còn Workload hoặc tài nguyên khác đang dùng');
    expect(reason).toHaveTextContent('redis.default#shared.cache');
    expect(checkbox(/k8s-namespace/)).toHaveAccessibleDescription(/Không thể chọn/);
  });

  it('lists exactly the ticked items, never ticks anything on its own and keeps the unticked ones out of the request', async () => {
    const calls = stubApi((c) => isPreview(c) ? Response.json(wire.preview(preview())) : isDeletion(c) ? Response.json(wire.admission(), { status: 202 }) : undefined);
    const user = userEvent.setup();
    const { onAccepted } = open();
    await screen.findByTestId('root-name');
    await user.click(checkbox(/vpc\.default/));
    expect(checkbox(/vpc\.default/)).toBeChecked();
    expect(checkbox(/k8s-secret/)).not.toBeChecked();
    const list = screen.getByRole('region', { name: 'Các tài nguyên sẽ bị xóa' });
    expect(within(list).getByRole('heading')).toHaveTextContent('Sẽ xóa 2 tài nguyên');
    expect(within(list).getAllByRole('listitem').map((li) => li.textContent)).toEqual([rootName, 'vpc.default#environments.shop.vpc']);
    await user.type(screen.getByLabelText('Nhập lại tên tài nguyên'), rootName);
    await user.click(confirmButton());
    await waitFor(() => expect(onAccepted).toHaveBeenCalledWith({ operationId: 'op-1', status: 'ACTIVE', stage: 'REQUESTED', replayed: false }));
    expect(deletions(calls)).toHaveLength(1);
    expect(deletions(calls)[0]?.body).toEqual({ token: preview().token, confirmationName: rootName, selectedResourceIds: [rootId, depA] });
  });

  it('keeps the confirmation locked until the exact name is typed (no trim, no case folding)', async () => {
    stubApi((c) => isPreview(c) ? Response.json(wire.preview(preview())) : undefined);
    const user = userEvent.setup();
    open();
    await screen.findByTestId('root-name');
    const input = screen.getByLabelText('Nhập lại tên tài nguyên');
    for (const wrong of [rootName.toUpperCase(), ` ${rootName}`, `${rootName} `, rootName.slice(0, -1), 'khác']) {
      await user.clear(input);
      await user.type(input, wrong);
      expect(confirmButton()).toBeDisabled();
    }
    await user.clear(input);
    await user.type(input, rootName);
    expect(confirmButton()).toBeEnabled();
  });

  it('cascades unticks with an explanation and never adds a selection', async () => {
    const mid = candidate({ id: 'mid', descriptor: 'vpc.default#environments.shop.mid' });
    const leaf = candidate({ id: 'leaf', descriptor: 'k8s-namespace.default#environments.shop.leaf', requiredBy: ['mid'] });
    stubApi((c) => isPreview(c) ? Response.json(wire.preview(preview({ candidates: [mid, leaf] }))) : undefined);
    const user = userEvent.setup();
    open();
    await screen.findByTestId('root-name');
    // The leaf waits for mid and is offered nothing until mid is ticked.
    expect(checkbox(/leaf/)).toBeDisabled();
    expect(screen.getByText(/cần chọn trước vpc\.default#environments\.shop\.mid/)).toBeInTheDocument();
    await user.click(checkbox(/environments\.shop\.mid/));
    expect(checkbox(/leaf/)).toBeEnabled();
    await user.click(checkbox(/leaf/));
    expect(screen.getByRole('heading', { name: 'Sẽ xóa 3 tài nguyên' })).toBeInTheDocument();
    await user.click(checkbox(/environments\.shop\.mid/));
    expect(checkbox(/environments\.shop\.mid/)).not.toBeChecked();
    expect(checkbox(/leaf/)).not.toBeChecked();
    expect(checkbox(/leaf/)).toBeDisabled();
    expect(screen.getByRole('status', { name: '' })).toHaveTextContent('Đã tự bỏ chọn k8s-namespace.default#environments.shop.leaf');
    expect(screen.getByRole('status', { name: '' })).toHaveTextContent('Không có mục nào được tự chọn thêm');
    expect(screen.getByRole('heading', { name: 'Sẽ xóa 1 tài nguyên' })).toBeInTheDocument();
    // Ticking mid again does not bring the leaf back.
    await user.click(checkbox(/environments\.shop\.mid/));
    expect(checkbox(/leaf/)).not.toBeChecked();
  });

  it('clears the typed name whenever the scope changes, so a confirmation never covers a different set', async () => {
    stubApi((c) => isPreview(c) ? Response.json(wire.preview(preview())) : undefined);
    const user = userEvent.setup();
    open();
    await screen.findByTestId('root-name');
    await user.type(screen.getByLabelText('Nhập lại tên tài nguyên'), rootName);
    expect(confirmButton()).toBeEnabled();
    await user.click(checkbox(/vpc\.default/));
    expect(screen.getByLabelText('Nhập lại tên tài nguyên')).toHaveValue('');
    expect(confirmButton()).toBeDisabled();
  });

  it('cancel and Escape send no deletion', async () => {
    const calls = stubApi((c) => isPreview(c) ? Response.json(wire.preview(preview())) : undefined);
    const user = userEvent.setup();
    const { onClose } = open();
    await screen.findByTestId('root-name');
    await user.type(screen.getByLabelText('Nhập lại tên tài nguyên'), rootName);
    await user.click(screen.getByRole('button', { name: 'Hủy' }));
    expect(onClose).toHaveBeenCalledTimes(1);
    await user.keyboard('{Escape}');
    expect(onClose).toHaveBeenCalledTimes(2);
    expect(deletions(calls)).toHaveLength(0);
  });

  it('reports a failed preview as an error with retry, not as an empty scope', async () => {
    let fail = true;
    const calls = stubApi((c) => isPreview(c) ? (fail ? apiError(409, 'CLEANUP_REFUSED', { reason: 'HAS_CONSUMERS' }) : Response.json(wire.preview(preview()))) : undefined);
    const user = userEvent.setup();
    open();
    expect(await screen.findByRole('alert')).toHaveTextContent('Vẫn còn Workload hoặc tài nguyên khác đang dùng');
    expect(screen.queryByTestId('root-name')).not.toBeInTheDocument();
    expect(confirmButton).toThrow();
    fail = false;
    await user.click(screen.getByRole('button', { name: 'Thử lại' }));
    expect(await screen.findByTestId('root-name')).toBeInTheDocument();
    expect(calls.filter(isPreview)).toHaveLength(2);
  });

  it('treats a wrong name rejected by the server as a retryable input error and never echoes the typed text', async () => {
    const calls = stubApi((c) => isPreview(c) ? Response.json(wire.preview(preview())) : isDeletion(c) ? apiError(400, 'RESOURCE_CONFIRMATION_MISMATCH', undefined, 'confirmationName') : undefined);
    const user = userEvent.setup();
    const { onAccepted } = open();
    await screen.findByTestId('root-name');
    await user.type(screen.getByLabelText('Nhập lại tên tài nguyên'), rootName);
    await user.click(confirmButton());
    expect(await screen.findByRole('alert')).toHaveTextContent('Tên nhập lại không khớp tên tài nguyên');
    expect(document.body).not.toHaveTextContent('raw server sentence');
    expect(screen.getByLabelText('Nhập lại tên tài nguyên')).toHaveValue('');
    expect(confirmButton()).toBeDisabled();
    expect(onAccepted).not.toHaveBeenCalled();
    expect(deletions(calls)).toHaveLength(1);
  });

  it.each([
    ['STALE_PREVIEW', 409, undefined, 'Bản xem trước đã cũ'],
    ['ENVIRONMENT_BUSY', 409, undefined, 'Môi trường đang bận'],
    ['CLEANUP_REFUSED', 409, { reason: 'HAS_CONSUMERS' }, 'Vẫn còn Workload hoặc tài nguyên khác đang dùng'],
  ])('%s makes the dialog stale: no automatic retry, the old token and name are not reused, a new review is explicit', async (code, status, params, text) => {
    let phase = 0;
    const calls = stubApi((c) => {
      if (isPreview(c)) { phase += 1; return Response.json(wire.preview(preview({ token: `rd1.4102444800.${String(phase).repeat(64)}` }))); }
      if (isDeletion(c)) return apiError(status, code, params as Record<string, string> | undefined);
      return undefined;
    });
    const user = userEvent.setup();
    open();
    await screen.findByTestId('root-name');
    await user.click(checkbox(/vpc\.default/));
    await user.type(screen.getByLabelText('Nhập lại tên tài nguyên'), rootName);
    await user.click(confirmButton());
    expect(await screen.findByRole('alert')).toHaveTextContent(text);
    expect(screen.queryByLabelText('Nhập lại tên tài nguyên')).not.toBeInTheDocument();
    expect(deletions(calls)).toHaveLength(1);
    await user.click(screen.getByRole('button', { name: 'Xem lại phạm vi' }));
    await screen.findByTestId('root-name');
    expect(calls.filter(isPreview)).toHaveLength(2);
    expect(deletions(calls)).toHaveLength(1);
    expect(screen.getByLabelText('Nhập lại tên tài nguyên')).toHaveValue('');
    // The earlier valid tick is kept for review, nothing new is ticked, and the new token is the only one used.
    expect(checkbox(/vpc\.default/)).toBeChecked();
    expect(checkbox(/k8s-secret/)).not.toBeChecked();
    expect(confirmButton()).toBeDisabled();
  });

  it('after a lost response never resends and only reloads the persisted state on request', async () => {
    const calls = stubApi((c) => isPreview(c) ? Response.json(wire.preview(preview())) : undefined);
    vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const call = { method: init?.method ?? 'GET', url: String(input), body: init?.body ? JSON.parse(String(init.body)) as unknown : undefined };
      calls.push(call);
      if (isPreview(call)) return Response.json(wire.preview(preview()));
      if (isDeletion(call)) throw new TypeError('network down');
      return Response.json({}, { status: 404 });
    }));
    const user = userEvent.setup();
    const { onLost, onAccepted } = open();
    await screen.findByTestId('root-name');
    await user.type(screen.getByLabelText('Nhập lại tên tài nguyên'), rootName);
    await user.click(confirmButton());
    expect(await screen.findByRole('alert')).toHaveTextContent('chưa rõ yêu cầu xóa đã được nhận hay chưa');
    expect(deletions(calls)).toHaveLength(1);
    expect(onAccepted).not.toHaveBeenCalled();
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(deletions(calls)).toHaveLength(1);
    await user.click(screen.getByRole('button', { name: 'Tải lại trạng thái tài nguyên' }));
    expect(onLost).toHaveBeenCalledTimes(1);
    expect(deletions(calls)).toHaveLength(1);
  });

  it('ignores a preview that arrives after the dialog was closed', async () => {
    let release: (r: Response) => void = () => undefined;
    stubApi((c) => isPreview(c) ? new Promise<Response>((resolve) => { release = resolve; }) as unknown as Response : undefined);
    const { view } = open();
    view.unmount();
    await act(async () => { release(Response.json(wire.preview(preview()))); });
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('treats an expired preview as stale and keeps the confirmation locked', async () => {
    const calls = stubApi((c) => isPreview(c) ? Response.json(wire.preview(preview({ expiresAt: '2026-01-01T00:00:00Z' }))) : undefined);
    open({ now: () => Date.parse('2026-01-02T00:00:00Z') });
    expect(await screen.findByRole('alert')).toHaveTextContent('đã hết hạn');
    expect(screen.getByRole('button', { name: 'Xem lại phạm vi' })).toBeInTheDocument();
    expect(screen.queryByLabelText('Nhập lại tên tài nguyên')).not.toBeInTheDocument();
    expect(deletions(calls)).toHaveLength(0);
  });

  it('traps focus inside the dialog and starts on the dialog itself', async () => {
    stubApi((c) => isPreview(c) ? Response.json(wire.preview(preview())) : undefined);
    const user = userEvent.setup();
    open();
    await screen.findByTestId('root-name');
    expect(screen.getByRole('dialog')).toHaveFocus();
    for (let i = 0; i < 12; i += 1) await user.tab();
    expect(screen.getByRole('dialog')).toContainElement(document.activeElement as HTMLElement);
    await user.tab({ shift: true });
    expect(screen.getByRole('dialog')).toContainElement(document.activeElement as HTMLElement);
  });
});

describe('deletion from the resource page', () => {
  const isDetail = (c: Call) => c.method === 'GET' && c.url.endsWith(`/resources/${rootId}`);

  it('shows the accepted operation, follows its persisted progress and reports success only after every resource is verified absent', async () => {
    let polls = 0;
    const calls = stubApi(
      (c) => isDetail(c) ? Response.json(wire.resource(detail())) : undefined,
      (c) => isPreview(c) ? Response.json(wire.preview(preview())) : undefined,
      (c) => isDeletion(c) ? Response.json(wire.admission('op-7'), { status: 202 }) : undefined,
      (c) => {
        if (!c.url.endsWith('/resource-cleanups/op-7')) return undefined;
        polls += 1;
        if (polls === 1) return Response.json(wire.cleanup(operation({ operationId: 'op-7', stage: 'CHECKING' })));
        if (polls === 2) return Response.json(wire.cleanup(operation({ operationId: 'op-7', stage: 'VERIFYING' })));
        return Response.json(wire.cleanup(operation({ operationId: 'op-7', status: 'SUCCEEDED', stage: 'VERIFYING', terminal: 'SUCCEEDED', allVerified: true, resources: [{ id: rootId, descriptor: rootName, status: 'DESTROYED', outcome: 'DESTROYED', verified: true, remaining: [], remainingUnknown: false }] })));
      },
    );
    const user = userEvent.setup();
    render(<ResourceDetailPage application={application} environment="staging" resourceId={rootId} canDelete pollMs={15} />);
    await user.click(await screen.findByRole('button', { name: 'Xóa tài nguyên' }));
    await screen.findByTestId('root-name');
    await user.type(screen.getByLabelText('Nhập lại tên tài nguyên'), rootName);
    await user.click(confirmButton());
    const progress = await screen.findByTestId('cleanup-progress');
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(within(progress).queryByText(/đã được xác minh là không còn/)).not.toBeInTheDocument();
    expect(await within(progress).findByText(/đã được xác minh là không còn/)).toBeInTheDocument();
    expect(deletions(calls)).toHaveLength(1);
    // Polling stops once the operation is terminal.
    const seen = polls;
    await new Promise((resolve) => setTimeout(resolve, 80));
    expect(polls).toBe(seen);
  });

  it('shows a partial failure truthfully: remaining objects, no success, no automatic retry', async () => {
    let admitted = false;
    const calls = stubApi(
      (c) => isDetail(c) ? Response.json(wire.resource(detail(admitted ? { cleanup: { operationId: 'op-8', status: 'FAILED', stage: 'VERIFYING' }, eligibility: { eligible: true, reasons: [] } } : {}))) : undefined,
      (c) => isPreview(c) ? Response.json(wire.preview(preview())) : undefined,
      (c) => { if (!isDeletion(c)) return undefined; admitted = true; return Response.json(wire.admission('op-8'), { status: 202 }); },
      (c) => c.url.endsWith('/resource-cleanups/op-8') ? Response.json(wire.cleanup(operation({ operationId: 'op-8', status: 'FAILED', stage: 'VERIFYING', terminal: 'FAILED', failureCode: 'CLEANUP_INCOMPLETE', remainingIds: ['aws_vpc.main=vpc-1'],
        resources: [
          { id: rootId, descriptor: rootName, status: 'FAILED', outcome: 'REMAINING', verified: false, remaining: ['aws_db_instance.main=db-1'], remainingUnknown: false, reasonCode: 'DESTROY_FAILED' },
          { id: depA, descriptor: 'vpc.default#environments.shop.vpc', status: 'UNREFERENCED', outcome: 'RETAINED', verified: false, remaining: ['aws_vpc.main=vpc-1'], remainingUnknown: false, reasonCode: 'RETAINED_CONSUMER' },
          { id: depC, descriptor: 'k8s-secret.default#environments.shop.cfg', status: 'UNREFERENCED', outcome: 'UNKNOWN', verified: false, remaining: [], remainingUnknown: true },
        ] }))) : undefined,
    );
    const user = userEvent.setup();
    render(<ResourceDetailPage application={application} environment="staging" resourceId={rootId} canDelete pollMs={15} />);
    await user.click(await screen.findByRole('button', { name: 'Xóa tài nguyên' }));
    await screen.findByTestId('root-name');
    await user.click(checkbox(/vpc\.default/));
    await user.type(screen.getByLabelText('Nhập lại tên tài nguyên'), rootName);
    await user.click(confirmButton());
    const progress = await screen.findByTestId('cleanup-progress');
    expect(await within(progress).findByRole('alert')).toHaveTextContent('Chưa xóa hết tài nguyên đã chọn.');
    expect(within(progress).queryByText(/đã được xác minh là không còn/)).not.toBeInTheDocument();
    expect(within(progress).getByText('aws_db_instance.main=db-1')).toBeInTheDocument();
    expect(within(progress).getByText('Được giữ lại (không xóa)')).toBeInTheDocument();
    expect(within(progress).getByText('Chưa rõ — không thể coi là đã hết')).toBeInTheDocument();
    expect(within(progress).getAllByText('Chưa').length).toBeGreaterThan(0);
    await new Promise((resolve) => setTimeout(resolve, 60));
    expect(deletions(calls)).toHaveLength(1);
  });

  it('after a lost response an explicit reload shows the admitted operation and nothing is sent twice', async () => {
    let admitted = false;
    const calls: Call[] = [];
    vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const call: Call = { method: init?.method ?? 'GET', url: String(input), body: init?.body ? JSON.parse(String(init.body)) as unknown : undefined };
      calls.push(call);
      if (isDetail(call)) return Response.json(wire.resource(detail(admitted ? { cleanup: { operationId: 'op-5', status: 'ACTIVE', stage: 'CHECKING' } } : {})));
      if (isPreview(call)) return Response.json(wire.preview(preview()));
      if (isDeletion(call)) { admitted = true; throw new TypeError('connection reset after the server accepted'); }
      if (call.url.endsWith('/resource-cleanups/op-5')) return Response.json(wire.cleanup(operation({ operationId: 'op-5', stage: 'CHECKING' })));
      return Response.json({}, { status: 404 });
    }));
    const user = userEvent.setup();
    render(<ResourceDetailPage application={application} environment="staging" resourceId={rootId} canDelete pollMs={15} />);
    await user.click(await screen.findByRole('button', { name: 'Xóa tài nguyên' }));
    await screen.findByTestId('root-name');
    await user.type(screen.getByLabelText('Nhập lại tên tài nguyên'), rootName);
    await user.click(confirmButton());
    await screen.findByRole('alert');
    await user.click(screen.getByRole('button', { name: 'Tải lại trạng thái tài nguyên' }));
    expect(await screen.findByTestId('cleanup-progress')).toBeInTheDocument();
    expect(screen.getByTestId('delete-blockers')).toHaveTextContent('Một thao tác xóa đang chạy');
    expect(screen.getByRole('button', { name: 'Xóa tài nguyên' })).toBeDisabled();
    expect(deletions(calls)).toHaveLength(1);
  });
});
