import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { WorkloadEditorPage } from './WorkloadEditorPage';
import { bothConfigured } from '../../test/targets';

const application = { id: 'app', name: 'App', environments: bothConfigured(), workloads: { staging: [], production: [] } };
const native = { apiVersion: 'orchestrator.dev/workload/v1', metadata: { name: 'api' }, containers: { main: { image: 'example/api:1' } } };
const file = (content: string, name: string) => Object.assign(new File([content], name, { type: 'text/yaml' }), { text: async () => content });
afterEach(() => vi.unstubAllGlobals());
function fixture(parse: (body: Record<string, string>) => Promise<Response>) {
 const calls: Record<string, string>[] = [];
 vi.stubGlobal('fetch', vi.fn(async (input, init?: RequestInit) => {
  const url = String(input);
  if (url.endsWith('/configuration')) return Response.json({ keys: [] });
  if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: [] });
  if (url.endsWith('/workloads/resolve-defaults')) return Response.json({ workload: JSON.parse(String(init?.body)).workload, sources: {} });
  if (url.endsWith('/workloads/parse')) { const body = JSON.parse(String(init?.body)) as Record<string,string>; calls.push(body); return parse(body); }
  return Response.json({ draftVersion: 0, workloads: [] });
 }));
 return calls;
}
it('imports Score and its extension together and blocks stale content after an invalid replacement', async () => {
 const calls = fixture(async body => body.content === 'invalid' ? Response.json({ code: 'INVALID_WORKLOAD' }, { status: 400 }) : Response.json({ workload: native }));
 const user = userEvent.setup(); render(<WorkloadEditorPage application={application} environment="staging" />);
 await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
 await user.click(screen.getByRole('button', { name: 'Nhập Workload' }));
 await user.upload(screen.getByLabelText('File extension (tùy chọn)'), file('apiVersion: humanitec.org/v1b1\nprofile: backend', 'humanitec.score.yaml'));
 expect(calls).toHaveLength(0);
 await user.upload(screen.getByLabelText('File Workload'), file('apiVersion: score.dev/v1b1', 'score.yaml'));
 await screen.findByText('api', { selector: 'strong' });
 expect(calls[0]?.extensions).toContain('profile: backend');
 expect(screen.getByRole('button', { name: 'Lưu draft Workload' })).toBeEnabled();
 await user.upload(screen.getByLabelText('File Workload'), file('invalid', 'bad.yaml'));
 await screen.findByRole('alert');
 expect(screen.queryByText('api', { selector: 'strong' })).not.toBeInTheDocument();
 expect(screen.getByRole('button', { name: 'Lưu draft Workload' })).toBeDisabled();
});
it('ignores an older parse response after the user replaces the Score file', async () => {
 let resolveOld!: (response: Response) => void;
 fixture(body => body.content === 'old' ? new Promise(resolve => { resolveOld = resolve; }) : Promise.resolve(Response.json({ workload: { ...native, metadata: { name: 'new-api' } } })));
 const user = userEvent.setup(); render(<WorkloadEditorPage application={application} environment="staging" />);
 await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
 await user.click(screen.getByRole('button', { name: 'Nhập Workload' }));
 await user.upload(screen.getByLabelText('File Workload'), file('old', 'old.yaml'));
 await waitFor(() => expect(resolveOld).toBeDefined());
 await user.upload(screen.getByLabelText('File Workload'), file('new', 'new.yaml'));
 await screen.findByText('new-api', { selector: 'strong' });
 await act(async () => resolveOld(Response.json({ workload: native })));
 expect(screen.getByText('new-api', { selector: 'strong' })).toBeVisible();
 expect(screen.queryByText('api', { selector: 'strong' })).not.toBeInTheDocument();
});
it('exports the current form edits and downloads both documents', async () => {
 let exported: Record<string, unknown> | undefined;
 vi.stubGlobal('fetch', vi.fn(async (input, init?: RequestInit) => {
  const url = String(input);
  if (url.endsWith('/configuration')) return Response.json({ keys: [] });
  if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: [] });
  if (url.endsWith('/workloads/resolve-defaults')) return Response.json({ workload: JSON.parse(String(init?.body)).workload, sources: {} });
  if (url.endsWith('/workloads/export')) { exported = JSON.parse(String(init?.body)).workload as Record<string, unknown>; return Response.json({ score: 'apiVersion: score.dev/v1b1', extensions: 'apiVersion: humanitec.org/v1b1' }); }
  return Response.json({ draftVersion: 0, workloads: [] });
 }));
 const NativeURL = URL;
 vi.stubGlobal('URL', class extends NativeURL { static createObjectURL = vi.fn(() => 'blob:score'); static revokeObjectURL = vi.fn(); });
 const downloads: string[] = [];
 const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function(this: HTMLAnchorElement) { downloads.push(this.download); });
 try {
  const user = userEvent.setup(); render(<WorkloadEditorPage application={application} environment="staging" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  await user.type(screen.getByLabelText('Tên Workload'), 'api');
  await user.type(screen.getByLabelText('Ảnh Container'), 'example/api:edited');
  await user.click(screen.getByRole('button', { name: 'Tải Score + extension' }));
  await waitFor(() => expect(downloads).toEqual(['score.yaml', 'humanitec.score.yaml']));
  expect((exported?.containers as Record<string, { image: string }>).main?.image).toBe('example/api:edited');
 } finally { click.mockRestore(); }
});
