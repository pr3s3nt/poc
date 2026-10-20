import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ResourceDetailPage } from './ResourceDetailPage';
import type { Application } from '../../shared/types/application';
import { bothConfigured } from '../../test/targets';
import { apiError, stubApi } from '../../test/api';
import { detail, operation, preview, rootId, rootName, wire } from '../../test/resources';

const application: Application = { id: 'shop', name: 'Shop', subdomain: 'shop', environments: bothConfigured(), workloads: { staging: [], production: [] } };
const isDetail = (url: string) => url.endsWith(`/resources/${rootId}`);

afterEach(() => vi.unstubAllGlobals());

describe('resource detail (UC-09 RES-01/02)', () => {
  it('shows the safe metadata of an UNREFERENCED resource, hides secret outputs and offers deletion to a Platform Engineer', async () => {
    stubApi((c) => c.method === 'GET' && isDetail(c.url) ? Response.json(wire.resource(detail())) : undefined);
    render(<ResourceDetailPage application={application} environment="staging" resourceId={rootId} canDelete />);
    expect(screen.getByRole('status')).toHaveTextContent('Đang tải chi tiết tài nguyên…');
    expect(await screen.findByTestId('resource-name')).toHaveTextContent(rootName);
    expect(screen.getByText('Không còn được tham chiếu')).toBeInTheDocument();
    expect(screen.getByText('aurora · rev1')).toBeInTheDocument();
    expect(screen.getByText('db.internal')).toBeInTheDocument();
    expect(screen.getByText('Đã che')).toBeInTheDocument();
    expect(screen.queryByText('***redacted***')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Xóa tài nguyên' })).toBeEnabled();
  });

  it('shows the original target of the resource and never a substitute for an unknown one', async () => {
    stubApi((c) => isDetail(c.url) ? Response.json(wire.resource(detail())) : undefined);
    const view = render(<ResourceDetailPage application={application} environment="staging" resourceId={rootId} canDelete />);
    const target = await screen.findByTestId('original-target');
    expect(target).toHaveTextContent('Original AWS (aws-original) · AWS · tài khoản 123456789012 · eu-west-1 · thế hệ 0');
    view.unmount();
    stubApi((c) => isDetail(c.url) ? Response.json(wire.resource(detail({ target: { known: false, generation: 2 } }))) : undefined);
    render(<ResourceDetailPage application={application} environment="staging" resourceId={rootId} canDelete />);
    expect(await screen.findByTestId('original-target')).toHaveTextContent('Console không thay bằng đích hiện tại của môi trường');
  });

  it('explains missing deletion infrastructure instead of offering a deletion that can only end in a 503', async () => {
    stubApi((c) => isDetail(c.url) ? Response.json(wire.resource(detail({ eligibility: { eligible: false, reasons: ['INFRASTRUCTURE_UNAVAILABLE'] } }))) : undefined);
    render(<ResourceDetailPage application={application} environment="staging" resourceId={rootId} canDelete />);
    expect(await screen.findByRole('button', { name: 'Xóa tài nguyên' })).toBeDisabled();
    expect(screen.getByTestId('delete-blockers')).toHaveTextContent('chưa được cấu hình trên máy chủ');
  });

  it('never shows the delete action to a Developer and sends no mutation', async () => {
    const calls = stubApi((c) => isDetail(c.url) ? Response.json(wire.resource(detail())) : undefined);
    render(<ResourceDetailPage application={application} environment="staging" resourceId={rootId} canDelete={false} />);
    await screen.findByTestId('resource-name');
    expect(screen.queryByRole('button', { name: 'Xóa tài nguyên' })).not.toBeInTheDocument();
    expect(screen.getByText('Chỉ Kỹ sư nền tảng hoặc Quản trị viên được xóa tài nguyên.')).toBeInTheDocument();
    expect(calls.every((c) => c.method === 'GET')).toBe(true);
  });

  it('disables the action and explains each blocker', async () => {
    stubApi(() => Response.json(wire.resource(detail({ status: 'READY', consumers: [{ kind: 'WORKLOAD', name: 'api' }], eligibility: { eligible: false, reasons: ['NOT_UNREFERENCED', 'HAS_CONSUMERS', 'ENVIRONMENT_BUSY'] } }))));
    render(<ResourceDetailPage application={application} environment="staging" resourceId={rootId} canDelete />);
    expect(await screen.findByRole('button', { name: 'Xóa tài nguyên' })).toBeDisabled();
    const note = screen.getByTestId('delete-blockers');
    expect(note).toHaveTextContent('chưa ở trạng thái không còn được tham chiếu');
    expect(note).toHaveTextContent('Vẫn còn Workload hoặc tài nguyên khác đang dùng');
    expect(note).toHaveTextContent('Môi trường đang bận');
    expect(screen.getByText(/Workload hiện tại/)).toBeInTheDocument();
  });

  it('keeps a failed load apart from an empty resource and retries only on request', async () => {
    let fail = true;
    const calls = stubApi((c) => isDetail(c.url) ? (fail ? apiError(500, 'INTERNAL_ERROR') : Response.json(wire.resource(detail()))) : undefined);
    const user = userEvent.setup();
    render(<ResourceDetailPage application={application} environment="staging" resourceId={rootId} canDelete />);
    expect(await screen.findByRole('alert')).toHaveTextContent('Không thể tải chi tiết tài nguyên.');
    expect(screen.queryByTestId('resource-name')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Xóa tài nguyên' })).not.toBeInTheDocument();
    expect(calls).toHaveLength(1);
    fail = false;
    await user.click(screen.getByRole('button', { name: 'Thử lại' }));
    expect(await screen.findByTestId('resource-name')).toBeInTheDocument();
    expect(calls).toHaveLength(2);
  });

  it('reports a missing or foreign resource as not found, without a retry', async () => {
    stubApi(() => apiError(404, 'NOT_FOUND'));
    render(<ResourceDetailPage application={application} environment="staging" resourceId={rootId} canDelete />);
    expect(await screen.findByRole('heading', { name: 'Không tìm thấy tài nguyên' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Thử lại' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Xóa tài nguyên' })).not.toBeInTheDocument();
  });

  it('rejects a malformed response instead of showing an empty resource', async () => {
    stubApi(() => Response.json({ resource: { id: rootId } }));
    render(<ResourceDetailPage application={application} environment="staging" resourceId={rootId} canDelete />);
    expect(await screen.findByRole('alert')).toHaveTextContent('Không thể tải chi tiết tài nguyên.');
    expect(screen.queryByTestId('resource-name')).not.toBeInTheDocument();
  });

  it('drops a late reply for another resource or Environment', async () => {
    const resolvers: Record<string, (r: Response) => void> = {};
    stubApi((c) => { if (c.method !== 'GET') return undefined; return new Promise<Response>((resolve) => { resolvers[c.url] = resolve; }) as unknown as Response; });
    const view = render(<ResourceDetailPage application={application} environment="staging" resourceId={rootId} canDelete />);
    await waitFor(() => expect(Object.keys(resolvers)).toHaveLength(1));
    const stagingUrl = Object.keys(resolvers)[0] as string;
    view.rerender(<ResourceDetailPage application={application} environment="production" resourceId={rootId} canDelete />);
    await waitFor(() => expect(Object.keys(resolvers)).toHaveLength(2));
    resolvers[Object.keys(resolvers)[1] as string]?.(Response.json(wire.resource(detail({ environment: 'production', descriptor: 'redis.default#shared.prod' }))));
    expect(await screen.findByTestId('resource-name')).toHaveTextContent('redis.default#shared.prod');
    resolvers[stagingUrl]?.(Response.json(wire.resource(detail({ descriptor: 'postgres.default#shared.STALE' }))));
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(screen.getByTestId('resource-name')).toHaveTextContent('redis.default#shared.prod');
    expect(screen.queryByText('postgres.default#shared.STALE')).not.toBeInTheDocument();
  });

  it('keeps deletion blocked while the latest operation is of unknown or held status', async () => {
    for (const status of ['MYSTERY', 'INTERRUPTED', 'RECOVERING']) {
      stubApi((c) => isDetail(c.url) ? Response.json(wire.resource(detail({ cleanup: { operationId: 'op-3', status, stage: 'CHECKING' } }))) : c.url.endsWith('/resource-cleanups/op-3') ? Response.json(wire.cleanup(operation({ operationId: 'op-3', status }))) : undefined);
      const view = render(<ResourceDetailPage application={application} environment="staging" resourceId={rootId} canDelete pollMs={500} />);
      expect(await screen.findByRole('button', { name: 'Xóa tài nguyên' })).toBeDisabled();
      expect(screen.getByTestId('delete-blockers')).toHaveTextContent('Một thao tác xóa đang chạy');
      view.unmount();
    }
  });

  it('shows a deleted resource as history without a delete action and resumes the progress of its operation after a reload', async () => {
    stubApi((c) => {
      if (isDetail(c.url)) return Response.json(wire.resource(detail({ status: 'DESTROYED', eligibility: { eligible: false, reasons: ['NOT_UNREFERENCED'] }, cleanup: { operationId: 'op-9', status: 'SUCCEEDED', stage: 'VERIFYING' } })));
      if (c.url.endsWith('/resource-cleanups/op-9')) return Response.json(wire.cleanup(operation({ operationId: 'op-9', status: 'SUCCEEDED', stage: 'VERIFYING', allVerified: true, resources: [{ id: rootId, descriptor: rootName, currentStatus: 'DESTROYED', outcome: 'DESTROYED', verified: true, remaining: [], remainingUnknown: false }] })));
      return undefined;
    });
    render(<ResourceDetailPage application={application} environment="staging" resourceId={rootId} canDelete pollMs={10} />);
    expect(await screen.findByText('Tài nguyên này đã được xóa và xác minh; lịch sử được giữ lại.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Xóa tài nguyên' })).toBeDisabled();
    const progress = await screen.findByTestId('cleanup-progress');
    expect(await within(progress).findByText(/đã được xác minh là không còn/)).toBeInTheDocument();
  });

  it('reloads the detail once when an operation settles instead of looping through remounts', async () => {
    let detailReads = 0;
    stubApi((c) => {
      if (isDetail(c.url)) { detailReads += 1; return Response.json(wire.resource(detail({ status: 'DESTROYED', eligibility: { eligible: false, reasons: ['NOT_UNREFERENCED'] }, cleanup: { operationId: 'op-9', status: 'SUCCEEDED', stage: 'VERIFYING' } }))); }
      if (c.url.endsWith('/resource-cleanups/op-9')) return Response.json(wire.cleanup(operation({ operationId: 'op-9', status: 'SUCCEEDED', stage: 'VERIFYING', allVerified: true, resources: [{ id: rootId, descriptor: rootName, currentStatus: 'DESTROYED', outcome: 'DESTROYED', verified: true, remaining: [], remainingUnknown: false }] })));
      return undefined;
    });
    render(<ResourceDetailPage application={application} environment="staging" resourceId={rootId} canDelete pollMs={10} />);
    const progress = await screen.findByTestId('cleanup-progress');
    expect(await within(progress).findByText(/đã được xác minh là không còn/)).toBeInTheDocument();
    await new Promise((resolve) => setTimeout(resolve, 300));
    expect(detailReads).toBe(2); // the first load and exactly one reload after the operation settled
    expect(screen.getByTestId('cleanup-progress')).toHaveTextContent('đã được xác minh là không còn');
  });

  it('closes the dialog and never shows or reopens anything of the old scope when the resource, Environment or permission changes', async () => {
    const otherId = '00000000-0000-4000-8000-0000000000ff';
    const late: Record<string, (r: Response) => void> = {};
    const calls = stubApi((c) => {
      if (c.method === 'POST' && c.url.endsWith('/deletion-preview')) return Response.json(wire.preview(preview()));
      if (c.method === 'GET' && c.url.includes('/environments/staging/resources/') && c.url.endsWith(otherId)) return new Promise<Response>((resolve) => { late[c.url] = resolve; }) as unknown as Response;
      if (c.method === 'GET' && c.url.includes('/environments/production/resources/')) return apiError(404, 'NOT_FOUND');
      return isDetail(c.url) ? Response.json(wire.resource(detail())) : undefined;
    });
    const view = render(<ResourceDetailPage application={application} environment="staging" resourceId={rootId} canDelete />);
    await userEvent.click(await screen.findByRole('button', { name: 'Xóa tài nguyên' }));
    expect(await screen.findByRole('dialog')).toBeInTheDocument();
    const previews = () => calls.filter((c) => c.url.endsWith('/deletion-preview')).length;
    expect(previews()).toBe(1);

    // Another resource of the same Environment: the dialog closes at once, the old detail is gone, and when
    // the new detail finally loads no deletion preview is started on its own.
    view.rerender(<ResourceDetailPage application={application} environment="staging" resourceId={otherId} canDelete />);
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(screen.queryByTestId('resource-name')).not.toBeInTheDocument();
    expect(screen.getByRole('status')).toHaveTextContent('Đang tải chi tiết tài nguyên…');
    await waitFor(() => expect(Object.keys(late)).toHaveLength(1));
    late[Object.keys(late)[0] as string]?.(Response.json(wire.resource(detail({ id: otherId, descriptor: 'redis.default#shared.other' }))));
    expect(await screen.findByTestId('resource-name')).toHaveTextContent('redis.default#shared.other');
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(previews()).toBe(1);

    // The SAME resource id under another Environment is a different resource: loading, then not found. The
    // staging detail is never shown for it.
    view.rerender(<ResourceDetailPage application={application} environment="production" resourceId={otherId} canDelete />);
    expect(screen.queryByTestId('resource-name')).not.toBeInTheDocument();
    expect(await screen.findByText('Không tìm thấy tài nguyên')).toBeInTheDocument();
    expect(screen.queryByText('redis.default#shared.other')).not.toBeInTheDocument();

    // Back to an eligible resource: open, then lose the right to delete. The dialog closes and stays closed.
    view.rerender(<ResourceDetailPage application={application} environment="staging" resourceId={rootId} canDelete />);
    await userEvent.click(await screen.findByRole('button', { name: 'Xóa tài nguyên' }));
    expect(await screen.findByRole('dialog')).toBeInTheDocument();
    view.rerender(<ResourceDetailPage application={application} environment="staging" resourceId={rootId} canDelete={false} />);
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    view.rerender(<ResourceDetailPage application={application} environment="staging" resourceId={rootId} canDelete />);
    await screen.findByRole('button', { name: 'Xóa tài nguyên' });
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('drops a late old-scope reply that arrives after the scope changed', async () => {
    const pending: Record<string, (r: Response) => void> = {};
    stubApi((c) => (c.method === 'GET' ? new Promise<Response>((resolve) => { pending[c.url] = resolve; }) as unknown as Response : undefined));
    const view = render(<ResourceDetailPage application={application} environment="staging" resourceId={rootId} canDelete />);
    await waitFor(() => expect(Object.keys(pending)).toHaveLength(1));
    const oldUrl = Object.keys(pending)[0] as string;
    view.rerender(<ResourceDetailPage application={application} environment="production" resourceId={rootId} canDelete />);
    await waitFor(() => expect(Object.keys(pending)).toHaveLength(2));
    pending[oldUrl]?.(Response.json(wire.resource(detail({ descriptor: 'postgres.default#shared.STALE' }))));
    await new Promise((resolve) => setTimeout(resolve, 30));
    expect(screen.queryByText('postgres.default#shared.STALE')).not.toBeInTheDocument();
    expect(screen.getByRole('status')).toHaveTextContent('Đang tải chi tiết tài nguyên…');
  });
});
