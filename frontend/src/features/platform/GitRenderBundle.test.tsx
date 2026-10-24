import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { RenderingTemplatesPage } from './RenderingTemplatesPage';

// ADR-021 Git render bundle authoring on Mẫu dựng ứng dụng (UC-03 RB-01..06).
type Body = Record<string, unknown>;
const nsRef = "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}";
const clusterRef = "${resources['k8s-cluster.internal#environments.@app.@env'].outputs.cluster}";
const schema = { type: 'object', additionalProperties: false, properties: {
  team: { type: 'string', title: 'Team', pattern: '^[a-z0-9-]+$' },
  defaultReplicas: { type: 'integer', title: 'Default replicas', minimum: 1, maximum: 20, default: 1 },
  runAsNonRoot: { type: 'boolean', title: 'Run as non-root', default: false },
} };
const gitBundle = (id: string, revision: string, extra: Body = {}) => ({ id, name: 'standard-workload', version: revision.slice(0, 12), status: 'AVAILABLE',
  source: { kind: 'git', url: 'https://github.com/pr3s3nt/poc.git', path: 'backend/render-bundles/standard', revision, contentSha256: 'c'.repeat(64) },
  inputContract: 'orchestrator.dev/render-input/v1', engineContract: 'git-render-v1', renderer: { type: 'score-k8s', version: '0.15.0' }, inputsSchema: schema, ...extra });
const idA = '11111111-1111-4111-8111-111111111111';
const idB = '22222222-2222-4222-8222-222222222222';
const bundleA = gitBundle(idA, 'a'.repeat(40));
const bundleB = gitBundle(idB, 'b'.repeat(40), { inputsSchema: { type: 'object', additionalProperties: false, properties: { tier: { type: 'string', enum: ['gold', 'silver'] } } } });

type Options = { bundles?: Body[]; definitions?: Body[]; inspect?: (body: Body) => Response | Promise<Response>; details?: Record<string, Body> };
function backend(options: Options = {}) {
  const posted: Body[] = [];
  const puts: Body[] = [];
  const inspections: Body[] = [];
  const gets: string[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/render-bundles/inspect')) { const body = JSON.parse(String(init?.body)) as Body; inspections.push(body); return (options.inspect ?? (() => Response.json({ renderBundle: bundleA })))(body); }
    if (init?.method === 'PUT') { const body = JSON.parse(String(init.body)) as Body; puts.push(body); return Response.json({ ...body, version: Number(body.version) + 1 }); }
    if (init?.method === 'POST' && url.endsWith('/resource-definitions')) { posted.push(JSON.parse(String(init.body)) as Body); return Response.json({}, { status: 201 }); }
    if (init?.method === 'POST') return Response.json({ status: 'NO_MATCH', candidates: [], snapshot: { marker: 'm', environmentVersion: 1 } });
    gets.push(url);
    if (url.endsWith('/applications')) return Response.json({ applications: [] });
    if (url.endsWith('/resource-definition-capabilities')) return Response.json({ capabilities: [{ resourceType: 'workload', driverType: 'score-k8s', module: '', variables: [
      { name: 'render_bundle', type: 'string', required: true, hasDefault: false, resourceParam: false },
      { name: 'namespace', type: 'string', required: true, hasDefault: false, resourceParam: false, preset: nsRef },
      { name: 'cluster', type: 'string', required: true, hasDefault: false, resourceParam: false, preset: clusterRef }], executorOwned: [], account: { required: false }, bindings: [] }] });
    if (url.endsWith('/render-bundles')) return Response.json({ renderBundles: options.bundles ?? [] });
    const detail = Object.entries(options.details ?? {}).find(([id]) => url.endsWith(`/render-bundles/${id}`));
    if (detail) return Response.json({ renderBundle: detail[1] });
    return Response.json({ resourceDefinitions: options.definitions ?? [] });
  }));
  return { posted, puts, inspections, gets };
}

afterEach(() => vi.unstubAllGlobals());
const submit = () => screen.getByRole('button', { name: 'Đăng ký mẫu dựng ứng dụng' });

async function inspectA(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText('Git HTTPS URL'), 'https://github.com/pr3s3nt/poc.git');
  await user.type(screen.getByLabelText('Thư mục bundle'), 'backend/render-bundles/standard');
  await user.type(screen.getByLabelText('Tag hoặc commit'), 'test/manifest-bundle/a');
  await user.click(screen.getByRole('button', { name: 'Kiểm tra bundle' }));
  expect(await screen.findByText('Đã xác minh: standard-workload · commit aaaaaaaaaaaa')).toBeInTheDocument();
}
async function identity(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText('ID mẫu'), 'web-git');
  await user.type(screen.getByLabelText('Tên hiển thị'), 'Mẫu Git');
  await user.click(await screen.findByRole('button', { name: 'Dùng gợi ý namespace' }));
  await user.click(screen.getByRole('button', { name: 'Dùng gợi ý cluster' }));
}

describe('Git render bundle authoring (ADR-021)', () => {
  it('defaults to Git, works with no installed bundle, and saves the verified pin with typed inputs', async () => {
    const user = userEvent.setup();
    const { posted, inspections } = backend();
    render(<RenderingTemplatesPage />);
    await screen.findByText('Chưa có mẫu dựng ứng dụng nào.');
    expect(screen.getByRole('button', { name: 'Thêm từ Git' })).toHaveAttribute('aria-pressed', 'true');
    await identity(user);
    expect(submit()).toBeDisabled();
    await inspectA(user);
    expect(inspections).toEqual([{ url: 'https://github.com/pr3s3nt/poc.git', path: 'backend/render-bundles/standard', ref: 'test/manifest-bundle/a' }]);
    // Defaults are hints, not saved values: an untouched field is omitted.
    expect(screen.getByText(/Mặc định: 1\./)).toBeInTheDocument();
    await user.type(screen.getByLabelText('Team'), 'payments');
    await user.click(screen.getByLabelText('Run as non-root'));
    await user.click(screen.getByLabelText('Run as non-root'));
    await user.click(submit());
    expect(await screen.findByRole('status', {}, { timeout: 3000 })).toBeInTheDocument();
    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toMatchObject({ driverType: 'score-k8s', driverInputs: { values: { variables: { render_bundle: idA, namespace: nsRef, cluster: clusterRef, render_inputs: { team: 'payments', runAsNonRoot: false } } } } });
    expect((posted[0]?.driverInputs as { values: { variables: { render_inputs: Body } } }).values.variables.render_inputs).not.toHaveProperty('defaultReplicas');
    expect(JSON.stringify(posted[0])).not.toMatch(/contentSha256|binary|engine/);
  });

  it('keeps typed inputs lossless through the advanced JSON and blocks invalid input', async () => {
    const user = userEvent.setup();
    const { posted } = backend();
    render(<RenderingTemplatesPage />);
    await screen.findByText('Chưa có mẫu dựng ứng dụng nào.');
    await identity(user);
    await inspectA(user);
    await user.type(screen.getByLabelText('Default replicas'), '0');
    expect(screen.getByText('Giá trị nằm ngoài giới hạn cho phép.')).toBeInTheDocument();
    expect(submit()).toBeDisabled();
    await user.clear(screen.getByLabelText('Default replicas'));
    await user.type(screen.getByLabelText('Default replicas'), '3');
    await user.click(screen.getByRole('button', { name: 'JSON nâng cao' }));
    const json = screen.getByLabelText('JSON nâng cao của mẫu dựng ứng dụng') as HTMLTextAreaElement;
    expect(JSON.parse(json.value).variables.render_inputs).toEqual({ defaultReplicas: 3 });
    await user.click(screen.getByRole('button', { name: 'Biểu mẫu' }));
    expect(screen.getByLabelText('Default replicas')).toHaveValue('3');
    await user.click(submit());
    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toMatchObject({ driverInputs: { values: { variables: { render_inputs: { defaultReplicas: 3 } } } } });
  });

  it('discards a late inspection reply after the source changed and shows safe error codes', async () => {
    const user = userEvent.setup();
    let release: (response: Response) => void = () => undefined;
    let calls = 0;
    backend({ inspect: () => { calls++; return calls === 1 ? new Promise<Response>((done) => { release = done; }) : Response.json({ code: 'RENDER_BUNDLE_INVALID', params: { file: 'bundle.yaml' }, error: 'raw yaml: secret' }, { status: 400 }); } });
    render(<RenderingTemplatesPage />);
    await screen.findByText('Chưa có mẫu dựng ứng dụng nào.');
    await user.type(screen.getByLabelText('Git HTTPS URL'), 'https://github.com/pr3s3nt/poc.git');
    await user.type(screen.getByLabelText('Tag hoặc commit'), 'main');
    await user.click(screen.getByRole('button', { name: 'Kiểm tra bundle' }));
    await user.type(screen.getByLabelText('Tag hoặc commit'), '-2');
    release(Response.json({ renderBundle: bundleA }));
    await waitFor(() => expect(screen.queryByText(/Đã xác minh: standard-workload/)).not.toBeInTheDocument());
    await user.click(screen.getByRole('button', { name: 'Kiểm tra bundle' }));
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('Bundle không đúng định dạng phiên bản 1. Tệp: bundle.yaml.');
    expect(alert).not.toHaveTextContent('raw yaml');
    expect(screen.getByLabelText('Tag hoặc commit')).toHaveValue('main-2');
  });

  it('reopens a saved Git pin without inspection, keeps per-pin drafts and selects an older verified pin explicitly', async () => {
    const user = userEvent.setup();
    const stored = { key: 'web-git', name: 'Mẫu Git', version: 4, resourceType: 'workload', driverType: 'score-k8s', criteria: [{}],
      driverInputs: { values: { variables: { render_bundle: idA, namespace: nsRef, cluster: clusterRef, render_inputs: { team: 'payments' } } } } };
    const { puts, inspections } = backend({ bundles: [bundleA, bundleB], definitions: [stored] });
    render(<RenderingTemplatesPage />);
    expect(await screen.findByText(/Bundle: standard-workload @ aaaaaaaaaaaa/)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Sửa mẫu web-git' }));
    expect(screen.getByRole('button', { name: 'Bundle đã xác minh' })).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getByLabelText('Bundle dựng ứng dụng')).toHaveValue(idA);
    expect(screen.getByLabelText('Team')).toHaveValue('payments');
    // Switching to B (another schema) keeps the A draft and shows incompatible fields instead of dropping them.
    await user.selectOptions(screen.getByLabelText('Bundle dựng ứng dụng'), idB);
    const warning = await screen.findByRole('alert', { name: 'Đầu vào không tương thích' });
    expect(warning).toHaveTextContent('team');
    await user.click(within(warning).getByRole('button', { name: 'Bỏ các trường không tương thích' }));
    await user.selectOptions(screen.getByLabelText('tier'), '"gold"');
    // Back to A: its own draft is restored.
    await user.selectOptions(screen.getByLabelText('Bundle dựng ứng dụng'), idA);
    expect(screen.getByLabelText('Team')).toHaveValue('payments');
    await user.click(screen.getByRole('button', { name: 'Lưu thay đổi' }));
    await waitFor(() => expect(puts).toHaveLength(1));
    expect(puts[0]).toMatchObject({ version: 4, driverInputs: { values: { variables: { render_bundle: idA, render_inputs: { team: 'payments' } } } } });
    expect(inspections).toEqual([]);
  });

  it('prefills the saved source when an edit switches to Git for a new ref', async () => {
    const user = userEvent.setup();
    const stored = { key: 'web-git', name: 'Mẫu Git', version: 1, resourceType: 'workload', driverType: 'score-k8s', criteria: [{}], driverInputs: { values: { variables: { render_bundle: idA, namespace: nsRef, cluster: clusterRef } } } };
    backend({ bundles: [bundleA], definitions: [stored] });
    render(<RenderingTemplatesPage />);
    await screen.findByText('Mẫu Git');
    await user.click(screen.getByRole('button', { name: 'Sửa mẫu web-git' }));
    await waitFor(() => expect(screen.getByLabelText('Bundle dựng ứng dụng')).toHaveValue(idA));
    await user.click(screen.getByRole('button', { name: 'Thêm từ Git' }));
    expect(screen.getByLabelText('Git HTTPS URL')).toHaveValue(bundleA.source.url);
    expect(screen.getByLabelText('Thư mục bundle')).toHaveValue(bundleA.source.path);
    expect(screen.getByLabelText('Tag hoặc commit')).toHaveValue('');
  });

  it('offers re-inspection of an unavailable saved pin with its exact commit', async () => {
    const user = userEvent.setup();
    const unavailable = { ...bundleA, status: 'UNAVAILABLE' };
    const stored = { key: 'web-git', name: 'Mẫu Git', version: 1, resourceType: 'workload', driverType: 'score-k8s', criteria: [{}], driverInputs: { values: { variables: { render_bundle: idA, namespace: nsRef, cluster: clusterRef } } } };
    const { inspections } = backend({ bundles: [unavailable], definitions: [stored] });
    render(<RenderingTemplatesPage />);
    await screen.findByText('Mẫu Git');
    await user.click(screen.getByRole('button', { name: 'Sửa mẫu web-git' }));
    const alert = await screen.findByRole('alert', { name: 'Bundle chưa khả dụng' });
    expect(screen.getByRole('button', { name: 'Lưu thay đổi' })).toBeDisabled();
    await user.click(within(alert).getByRole('button', { name: 'Kiểm tra lại' }));
    await waitFor(() => expect(inspections).toEqual([{ url: bundleA.source.url, path: bundleA.source.path, ref: 'a'.repeat(40) }]));
  });
});
