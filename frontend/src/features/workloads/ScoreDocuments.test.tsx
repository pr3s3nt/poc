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
  await waitFor(() => expect(URL.revokeObjectURL).toHaveBeenCalledTimes(2), { timeout: 2000 });
 } finally { click.mockRestore(); }
});
it('accepts pasted Score and extension and requires checking each edit before Save or export', async () => {
 const calls = fixture(async body => body.content === 'invalid' ? Response.json({ code: 'SCORE_SCHEMA_INVALID' }, { status: 400 }) : Response.json({ workload: native }));
 const user = userEvent.setup(); render(<WorkloadEditorPage application={application} environment="staging" />);
 await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
 await user.click(screen.getByRole('button', { name: 'Nhập Workload' }));
 const score = screen.getByRole('textbox', { name: 'Nội dung Score (YAML/JSON)' });
 const extension = screen.getByRole('textbox', { name: 'Nội dung extension (tùy chọn)' });
 await user.click(score); await user.paste('apiVersion: score.dev/v1b1\nmetadata: {name: api}');
 await user.click(extension); await user.paste('apiVersion: humanitec.org/v1b1\nprofile: backend');
 expect(calls).toHaveLength(0);
 expect(screen.getByRole('button', { name: 'Lưu draft Workload' })).toBeDisabled();
 expect(screen.getByRole('button', { name: 'Tải Score + extension' })).toBeDisabled();
 await user.click(screen.getByRole('button', { name: 'Kiểm tra' }));
 await screen.findByText('api', { selector: 'strong' });
 expect(calls[0]?.content).toContain('metadata: {name: api}');
 expect(calls[0]?.extensions).toContain('profile: backend');
 expect(screen.getByRole('button', { name: 'Lưu draft Workload' })).toBeEnabled();
 await user.clear(score); await user.paste('invalid');
 expect(screen.queryByText('api', { selector: 'strong' })).not.toBeInTheDocument();
 expect(screen.getByRole('button', { name: 'Lưu draft Workload' })).toBeDisabled();
 await user.click(screen.getByRole('button', { name: 'Kiểm tra' }));
 await screen.findByRole('alert');
 expect(score).toHaveValue('invalid');
 expect(screen.getByRole('button', { name: 'Tải Score + extension' })).toBeDisabled();
});
it('loads selected files into editable text and omits a manually cleared extension', async () => {
 const calls = fixture(async () => Response.json({ workload: native }));
 const user = userEvent.setup(); render(<WorkloadEditorPage application={application} environment="staging" />);
 await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
 await user.click(screen.getByRole('button', { name: 'Nhập Workload' }));
 await user.upload(screen.getByLabelText('File extension (tùy chọn)'), file('profile: backend', 'extension.yaml'));
 await user.upload(screen.getByLabelText('File Workload'), file('score from file', 'score.yaml'));
 await screen.findByText('api', { selector: 'strong' });
 expect(screen.getByRole('textbox', { name: 'Nội dung Score (YAML/JSON)' })).toHaveValue('score from file');
 const extension = screen.getByRole('textbox', { name: 'Nội dung extension (tùy chọn)' });
 expect(extension).toHaveValue('profile: backend');
 await user.clear(extension);
 await user.click(screen.getByRole('button', { name: 'Kiểm tra' }));
 await screen.findByText('api', { selector: 'strong' });
 expect(calls.at(-1)?.extensions).toBeUndefined();
});

function synchronizedFixture(options: {
 stored?: Record<string, unknown>;
 exportDocument?: (workload: Record<string, unknown>) => Promise<Response>;
 parseDocument?: (body: Record<string, string>) => Promise<Response>;
} = {}) {
 const exports: Record<string, unknown>[] = [], parses: Record<string, string>[] = [], saves: Record<string, unknown>[] = [];
 const stored = options.stored ?? native;
 vi.stubGlobal('fetch', vi.fn(async (input, init?: RequestInit) => {
  const url = String(input);
  if (url.endsWith('/configuration')) return Response.json({ keys: [{ name: 'API_TOKEN', kind: 'SECRET', configured: true }] });
  if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: [] });
  if (url.endsWith('/workloads/resolve-defaults')) return Response.json({ workload: JSON.parse(String(init?.body)).workload, sources: {} });
  if (url.endsWith('/workloads/export')) {
   const workload = JSON.parse(String(init?.body)).workload as Record<string, unknown>; exports.push(workload);
   return options.exportDocument ? options.exportDocument(workload) : Response.json({ score: JSON.stringify(workload), extensions: 'profile: backend' });
  }
  if (url.endsWith('/workloads/parse')) {
   const body = JSON.parse(String(init?.body)) as Record<string, string>; parses.push(body);
   return options.parseDocument ? options.parseDocument(body) : Response.json({ workload: JSON.parse(String(body.content)) });
  }
  if (init?.method === 'PUT') { saves.push(JSON.parse(String(init.body)) as Record<string, unknown>); return Response.json({}); }
  return Response.json({ draftVersion: 3, workloads: [{ id: 'api', workload: stored }] });
 }));
 return { exports, parses, saves };
}
const formTab = () => screen.getByRole('button', { name: 'Nhập bằng form' });
const documentTab = () => screen.getByRole('button', { name: 'Nhập Workload' });
const scoreInput = () => screen.getByRole('textbox', { name: 'Nội dung Score (YAML/JSON)' });

it('synchronizes loaded data and unsaved form/YAML changes while preserving secret refs, defaults, pins and explicit zero', async () => {
 const pin = { id: 'backend', version: '1.2.0', sha256: 'a'.repeat(64) };
 const stored = { ...native, replicas: 0, resources: { env: { type: 'environment' } }, containers: { main: { image: 'example/api:1', variables: { TOKEN: '${resources.env.API_TOKEN}' } } }, workloadExecution: { version: 1, kind: 'Deployment', automountServiceAccountToken: false, template: pin, templateDefaults: { environment: 'staging', profile: { replicas: 1 } } } };
 const calls = synchronizedFixture({ stored });
 const user = userEvent.setup(); render(<WorkloadEditorPage application={application} environment="staging" workloadId="api" />);
 await screen.findByDisplayValue('example/api:1');
 await user.click(documentTab());
 await waitFor(() => expect(scoreInput()).toHaveValue(JSON.stringify(stored)));
 expect(screen.getByRole('textbox', { name: 'Nội dung extension (tùy chọn)' })).toHaveValue('profile: backend');
 await user.click(formTab()); await screen.findByDisplayValue('example/api:1');
 await user.clear(screen.getByLabelText('Ảnh Container')); await user.type(screen.getByLabelText('Ảnh Container'), 'example/api:2');
 await user.click(documentTab());
 await waitFor(() => expect(String((scoreInput() as HTMLTextAreaElement).value)).toContain('example/api:2'));
 const exported = calls.exports.at(-1)!;
 expect(exported).toMatchObject({ replicas: 0, workloadExecution: { automountServiceAccountToken: false, template: pin, templateDefaults: stored.workloadExecution.templateDefaults }, containers: { main: { variables: { TOKEN: '${resources.env.API_TOKEN}' } } } });
 const changed = { ...exported, containers: { main: { ...(exported.containers as typeof stored.containers).main, image: 'example/api:3' } } };
 await user.clear(scoreInput()); await user.click(scoreInput()); await user.paste(JSON.stringify(changed));
 await user.click(formTab()); await screen.findByDisplayValue('example/api:3');
 expect(calls.parses.at(-1)?.extensions).toBe('profile: backend');
 await user.click(screen.getByRole('button', { name: 'Lưu draft Workload' }));
 await waitFor(() => expect(calls.saves).toHaveLength(1));
 expect(calls.saves[0]?.workload).toEqual(changed);
});

it('keeps invalid YAML on its tab and blocks stale writes until a valid return to the form', async () => {
 const calls = synchronizedFixture({ parseDocument: async body => body.content === 'broken: [' ? Response.json({ code: 'SCORE_SCHEMA_INVALID' }, { status: 400 }) : Response.json({ workload: native }) });
 const user = userEvent.setup(); render(<WorkloadEditorPage application={application} environment="staging" workloadId="api" />);
 await screen.findByDisplayValue('example/api:1'); await user.click(documentTab()); await waitFor(() => expect(scoreInput()).not.toHaveValue(''));
 await user.clear(scoreInput()); await user.paste('broken: ['); await user.click(formTab());
 await screen.findByRole('alert'); expect(scoreInput()).toHaveValue('broken: [');
 expect(screen.getByRole('button', { name: 'Lưu draft Workload' })).toBeDisabled();
 expect(screen.getByRole('button', { name: 'Tải Score + extension' })).toBeDisabled(); expect(calls.saves).toHaveLength(0);
 await user.click(documentTab()); expect(scoreInput()).toHaveValue('broken: [');
 await user.clear(scoreInput()); await user.paste('fixed'); await user.click(formTab()); await screen.findByDisplayValue('example/api:1');
 expect(screen.getByRole('button', { name: 'Lưu draft Workload' })).toBeEnabled();
});

it('keeps form edits when export fails and allows retry without silently serializing an unfinished form', async () => {
 let reject = true;
 const calls = synchronizedFixture({ exportDocument: async workload => reject ? Response.json({ code: 'SCORE_UNSUPPORTED' }, { status: 400 }) : Response.json({ score: JSON.stringify(workload), extensions: '' }) });
 const user = userEvent.setup(); render(<WorkloadEditorPage application={application} environment="staging" workloadId="api" />);
 await screen.findByDisplayValue('example/api:1');
 await user.clear(screen.getByLabelText('Ảnh Container')); await user.type(screen.getByLabelText('Ảnh Container'), 'example/api:edited');
 await user.click(documentTab()); await screen.findByRole('alert');
 expect(screen.getByLabelText('Ảnh Container')).toHaveValue('example/api:edited'); expect(screen.queryByRole('textbox', { name: 'Nội dung Score (YAML/JSON)' })).not.toBeInTheDocument();
 reject = false; await user.click(documentTab()); await waitFor(() => expect(String((scoreInput() as HTMLTextAreaElement).value)).toContain('example/api:edited'));
 await user.click(formTab()); await screen.findByDisplayValue('example/api:edited');
 await user.click(screen.getByRole('button', { name: '+ Thêm biến' })); const count = calls.exports.length; await user.click(documentTab());
 expect(screen.getByLabelText('Tên biến, dòng 1')).toHaveValue(''); expect(calls.exports).toHaveLength(count);
});

it('locks conflicting actions during export and ignores a reply for a form edited while it was pending', async () => {
 let resolve!: (response: Response) => void;
 const calls = synchronizedFixture({ exportDocument: () => new Promise(r => { resolve = r; }) });
 const user = userEvent.setup(); render(<WorkloadEditorPage application={application} environment="staging" workloadId="api" />);
 await screen.findByDisplayValue('example/api:1'); await user.click(documentTab()); await waitFor(() => expect(resolve).toBeDefined());
 expect(formTab()).toBeDisabled(); expect(documentTab()).toBeDisabled(); expect(screen.getByRole('button', { name: 'Lưu draft Workload' })).toBeDisabled(); expect(screen.getByRole('button', { name: 'Tải Score + extension' })).toBeDisabled();
 await user.clear(screen.getByLabelText('Ảnh Container')); await user.type(screen.getByLabelText('Ảnh Container'), 'example/api:newer');
 await act(async () => resolve(Response.json({ score: 'outdated', extensions: '' })));
 await waitFor(() => expect(documentTab()).toBeEnabled()); expect(screen.getByLabelText('Ảnh Container')).toHaveValue('example/api:newer'); expect(screen.queryByRole('textbox', { name: 'Nội dung Score (YAML/JSON)' })).not.toBeInTheDocument(); expect(calls.saves).toHaveLength(0);
});

it('ignores a document export arriving after the Environment changes', async () => {
 let resolve!: (response: Response) => void;
 synchronizedFixture({ exportDocument: () => new Promise(r => { resolve = r; }) });
 const user = userEvent.setup(); const view = render(<WorkloadEditorPage application={application} environment="staging" workloadId="api" />);
 await screen.findByDisplayValue('example/api:1'); await user.click(documentTab()); await waitFor(() => expect(resolve).toBeDefined());
 view.rerender(<WorkloadEditorPage application={application} environment="production" workloadId="api" />); await screen.findByDisplayValue('example/api:1');
 await act(async () => resolve(Response.json({ score: 'staging-document', extensions: 'old extension' })));
 expect(documentTab()).toBeEnabled(); expect(screen.queryByRole('textbox', { name: 'Nội dung Score (YAML/JSON)' })).not.toBeInTheDocument();
});

it('populates a saved advanced document and keeps the form locked when it would drop configuration', async () => {
 const advanced = { ...native, resources: { env: { type: 'environment' } } };
 const calls = synchronizedFixture({ stored: advanced }); const user = userEvent.setup();
 render(<WorkloadEditorPage application={application} environment="staging" workloadId="api" />);
 await waitFor(() => expect(scoreInput()).toHaveValue(JSON.stringify(advanced)));
 expect(formTab()).toBeDisabled(); await user.click(screen.getByRole('button', { name: 'Kiểm tra' }));
 await waitFor(() => expect(screen.getByRole('button', { name: 'Lưu draft Workload' })).toBeEnabled());
 expect(formTab()).toBeDisabled(); expect(calls.parses).toHaveLength(1); expect(calls.exports[0]).toEqual(advanced);
});

it('keeps an unfinished Service row instead of dropping it while opening YAML', async () => {
 const calls = synchronizedFixture(); const user = userEvent.setup();
 render(<WorkloadEditorPage application={application} environment="staging" workloadId="api" />);
 await screen.findByDisplayValue('example/api:1');
 await user.click(screen.getByRole('button', { name: '+ Thêm cổng' }));
 await user.type(screen.getByLabelText('Tên cổng Service'), 'http');
 await user.click(documentTab());
 expect(await screen.findByRole('alert')).toHaveTextContent('Hãy sửa tên và số cổng Service');
 expect(screen.getByLabelText('Tên cổng Service')).toHaveValue('http'); expect(calls.exports).toHaveLength(0);
});
