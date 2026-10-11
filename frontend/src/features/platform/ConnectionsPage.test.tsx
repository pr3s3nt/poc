import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ConnectionsPage } from './ConnectionsPage';

const kubeconfig = 'apiVersion: v1\nkind: Config\n# token: synthetic-secret-token\n';
const otherKubeconfig = 'apiVersion: v1\nkind: Config\n# second document\n';
const twoContexts = { contexts: [{ name: 'dev', cluster: 'dev-cluster', endpoint: 'https://dev.example' }, { name: 'prod', cluster: 'prod-cluster', endpoint: 'https://prod.example' }] };
const oneContext = { contexts: [{ name: 'kind-lab', cluster: 'kind-cluster', endpoint: 'https://127.0.0.1:6443' }] };
const otherContext = { contexts: [{ name: 'other-ctx', cluster: 'other-cluster', endpoint: 'https://other.example' }] };
const emptyList = 'Chưa có kết nối nào được đăng ký.';

type Handler = (url: string, body: Record<string, string>) => Response | Promise<Response>;
type Call = { url: string; body: Record<string, string> };

function stubApi(handlers: { inspect?: Handler; register?: Handler; list?: () => Response }) {
  const calls: Call[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (init?.method === 'POST') {
      const body = JSON.parse(String(init.body)) as Record<string, string>;
      calls.push({ url, body });
      if (url.endsWith('/inspect')) return (handlers.inspect ?? (() => Response.json(twoContexts)))(url, body);
      return (handlers.register ?? (() => Response.json({ key: 'lab', name: 'Lab', kind: 'KUBERNETES', status: 'READY' }, { status: 201 })))(url, body);
    }
    return (handlers.list ?? (() => Response.json({ connections: [] })))();
  }));
  return { inspects: () => calls.filter((call) => call.url.endsWith('/inspect')), registers: () => calls.filter((call) => !call.url.endsWith('/inspect')), calls };
}

// Controllable inspect answers, keyed by the submitted document.
function pendingInspects() {
  const waiting: { text: string; release: (response: Response) => void }[] = [];
  const handler: Handler = (_url, body) => new Promise<Response>((release) => { waiting.push({ text: body.kubeconfig ?? '', release }); });
  return { waiting, handler };
}

afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks(); });

async function pasteKubeconfig(user: ReturnType<typeof userEvent.setup>, text = kubeconfig) {
  await user.click(screen.getByLabelText('Dán nội dung kubeconfig'));
  await user.click(screen.getByLabelText('Nội dung kubeconfig'));
  await user.paste(text);
}
const upload = (user: ReturnType<typeof userEvent.setup>, text = kubeconfig, name = 'config.txt') => user.upload(screen.getByLabelText('File kubeconfig'), new File([text], name, { type: 'text/plain' }));
const nameField = () => screen.getByLabelText('Tên kết nối');
const submitButton = () => screen.getByRole('button', { name: 'Kiểm tra và lưu' });
const slow = { timeout: 3000 };

describe('UC-04 Kubernetes connections (T06)', () => {
  it('inspects an uploaded file automatically, selects the single context and suggests a name without registering', async () => {
    const user = userEvent.setup();
    const api = stubApi({ inspect: () => Response.json(oneContext) });
    render(<ConnectionsPage />);
    await screen.findByText(emptyList);
    expect(screen.queryByRole('button', { name: /Kiểm tra lại/ })).not.toBeInTheDocument();
    await upload(user);
    expect(await screen.findByLabelText('Đích đã chọn')).toHaveTextContent('Context kind-lab');
    expect(screen.getByText(/Đã chọn file: config.txt/)).toBeInTheDocument();
    expect(screen.queryByLabelText('Context')).not.toBeInTheDocument();
    expect(nameField()).toHaveValue('kind-cluster');
    expect(api.inspects()).toHaveLength(1);
    expect(api.inspects()[0]?.body).toEqual({ kubeconfig });
    expect(api.registers()).toHaveLength(0);
    expect(document.body.textContent).not.toContain('synthetic-secret-token');
  });

  it('waits for a pasted document to settle and inspects once, keeping the content masked', async () => {
    const user = userEvent.setup();
    const api = stubApi({ inspect: () => Response.json(oneContext) });
    render(<ConnectionsPage />);
    await screen.findByText(emptyList);
    await user.click(screen.getByLabelText('Dán nội dung kubeconfig'));
    await user.type(screen.getByLabelText('Nội dung kubeconfig'), 'apiVersion: v1');
    expect(api.inspects()).toHaveLength(0);
    expect(screen.getByLabelText('Nội dung kubeconfig')).toHaveClass('masked-text');
    expect(await screen.findByLabelText('Đích đã chọn', undefined, slow)).toHaveTextContent('Cluster kind-cluster');
    expect(api.inspects()).toHaveLength(1);
    expect(api.inspects()[0]?.body).toEqual({ kubeconfig: 'apiVersion: v1' });
    expect(api.registers()).toHaveLength(0);
    await user.click(screen.getByLabelText('Hiện nội dung'));
    expect(screen.getByLabelText('Nội dung kubeconfig')).not.toHaveClass('masked-text');
  });

  it('never guesses a context from current-context when there are several', async () => {
    const user = userEvent.setup();
    const api = stubApi({ inspect: () => Response.json({ currentContext: 'prod', ...twoContexts }) });
    render(<ConnectionsPage />);
    await screen.findByText(emptyList);
    await pasteKubeconfig(user);
    const select = await screen.findByLabelText('Context', undefined, slow);
    expect(select).toHaveValue('');
    expect(screen.queryByLabelText('Đích đã chọn')).not.toBeInTheDocument();
    expect(nameField()).toHaveValue('');
    // The required name and context block the native submit: nothing is sent.
    await user.click(submitButton());
    await user.type(nameField(), 'tay');
    await user.click(submitButton());
    expect(api.registers()).toHaveLength(0);
    await user.selectOptions(select, 'prod');
    expect(screen.getByLabelText('Đích đã chọn')).toHaveTextContent('Endpoint https://prod.example');
    expect(nameField()).toHaveValue('tay');
  });

  it('follows the selected context with an untouched name and never overwrites a user edit, even a blank one', async () => {
    const user = userEvent.setup();
    stubApi({});
    render(<ConnectionsPage />);
    await screen.findByText(emptyList);
    await pasteKubeconfig(user);
    const select = await screen.findByLabelText('Context', undefined, slow);
    await user.selectOptions(select, 'dev');
    expect(nameField()).toHaveValue('dev-cluster');
    await user.selectOptions(select, 'prod');
    expect(nameField()).toHaveValue('prod-cluster');
    await user.clear(nameField());
    await user.selectOptions(select, 'dev');
    expect(nameField()).toHaveValue('');
    await user.type(nameField(), 'Tên của tôi');
    await user.selectOptions(select, 'prod');
    expect(nameField()).toHaveValue('Tên của tôi');
  });

  it('keeps a user-edited name when a late reply or a different file arrives and drops an untouched suggestion on document change', async () => {
    const user = userEvent.setup();
    const pending = pendingInspects();
    stubApi({ inspect: pending.handler });
    render(<ConnectionsPage />);
    await screen.findByText(emptyList);
    await user.type(nameField(), 'Tên riêng');
    await upload(user);
    await waitFor(() => expect(pending.waiting).toHaveLength(1));
    await act(async () => { pending.waiting[0]?.release(Response.json(oneContext)); });
    expect(await screen.findByLabelText('Đích đã chọn')).toBeInTheDocument();
    expect(nameField()).toHaveValue('Tên riêng');
    await upload(user, otherKubeconfig, 'other.txt');
    expect(screen.queryByLabelText('Đích đã chọn')).not.toBeInTheDocument();
    await waitFor(() => expect(pending.waiting).toHaveLength(2));
    await act(async () => { pending.waiting[1]?.release(Response.json(otherContext)); });
    expect(await screen.findByLabelText('Đích đã chọn')).toHaveTextContent('other-cluster');
    expect(nameField()).toHaveValue('Tên riêng');
  });

  it('removes an untouched suggestion as soon as the document changes', async () => {
    const user = userEvent.setup();
    stubApi({ inspect: () => Response.json(oneContext) });
    render(<ConnectionsPage />);
    await screen.findByText(emptyList);
    await upload(user);
    await waitFor(() => expect(nameField()).toHaveValue('kind-cluster'));
    await user.click(screen.getByLabelText('Dán nội dung kubeconfig'));
    expect(nameField()).toHaveValue('');
    expect(screen.queryByLabelText('Đích đã chọn')).not.toBeInTheDocument();
  });

  it('ignores a stale inspection when the pasted document changes while it is pending', async () => {
    const user = userEvent.setup();
    const pending = pendingInspects();
    stubApi({ inspect: pending.handler });
    render(<ConnectionsPage />);
    await screen.findByText(emptyList);
    await pasteKubeconfig(user);
    await waitFor(() => expect(pending.waiting).toHaveLength(1), slow);
    expect(screen.getByRole('status')).toHaveTextContent('Đang kiểm tra kubeconfig…');
    await user.click(screen.getByLabelText('Nội dung kubeconfig'));
    await user.paste('# changed');
    expect(screen.queryByText('Đang kiểm tra kubeconfig…')).not.toBeInTheDocument();
    await act(async () => { pending.waiting[0]?.release(Response.json(oneContext)); });
    expect(screen.queryByLabelText('Đích đã chọn')).not.toBeInTheDocument();
    expect(nameField()).toHaveValue('');
    await waitFor(() => expect(pending.waiting).toHaveLength(2), slow);
    expect(pending.waiting[1]?.text).toBe(`${kubeconfig}# changed`);
    await act(async () => { pending.waiting[1]?.release(Response.json(otherContext)); });
    expect(await screen.findByLabelText('Đích đã chọn')).toHaveTextContent('other-cluster');
    expect(nameField()).toHaveValue('other-cluster');
  });

  it('ignores a file read that finishes after the document source changed', async () => {
    const readers: { onload: () => void; result: string }[] = [];
    class SlowReader {
      result = kubeconfig;
      onload: () => void = () => undefined;
      onerror: () => void = () => undefined;
      readAsText() { readers.push(this); }
    }
    vi.stubGlobal('FileReader', SlowReader);
    const user = userEvent.setup();
    const api = stubApi({ inspect: () => Response.json(oneContext) });
    render(<ConnectionsPage />);
    await screen.findByText(emptyList);
    await upload(user);
    expect(screen.getByRole('status')).toHaveTextContent('Đang đọc file kubeconfig…');
    await waitFor(() => expect(readers).toHaveLength(1));
    await user.click(screen.getByLabelText('Dán nội dung kubeconfig'));
    expect(screen.queryByText('Đang đọc file kubeconfig…')).not.toBeInTheDocument();
    await act(async () => { readers[0]?.onload(); });
    expect(api.inspects()).toHaveLength(0);
    expect(screen.queryByLabelText('Đích đã chọn')).not.toBeInTheDocument();
    expect(screen.getByLabelText('Nội dung kubeconfig')).toHaveValue('');
    expect(nameField()).toHaveValue('');
  });

  it('shows Vietnamese guidance, retries only when asked and ignores a stale retry reply', async () => {
    const user = userEvent.setup();
    const pending = pendingInspects();
    let failing = true;
    const api = stubApi({
      inspect: (url, body) => failing
        ? Response.json({ error: 'connection: invalid registration: raw parser detail token-leak' }, { status: 400 })
        : pending.handler(url, body),
    });
    render(<ConnectionsPage />);
    await screen.findByText(emptyList);
    await user.type(nameField(), 'eks');
    await upload(user);
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('Không đọc được kubeconfig');
    expect(document.body.textContent).not.toMatch(/raw parser detail|invalid registration|token-leak/);
    expect(nameField()).toHaveValue('eks');
    await new Promise((resolve) => setTimeout(resolve, 800));
    expect(api.inspects()).toHaveLength(1);
    failing = false;
    await user.click(screen.getByRole('button', { name: 'Kiểm tra lại kubeconfig' }));
    await waitFor(() => expect(pending.waiting).toHaveLength(1));
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Kiểm tra lại kubeconfig' })).not.toBeInTheDocument();
    await upload(user, otherKubeconfig, 'other.txt');
    await waitFor(() => expect(pending.waiting).toHaveLength(2));
    await act(async () => { pending.waiting[0]?.release(Response.json(oneContext)); });
    expect(screen.queryByLabelText('Đích đã chọn')).not.toBeInTheDocument();
    await act(async () => { pending.waiting[1]?.release(Response.json(otherContext)); });
    expect(await screen.findByLabelText('Đích đã chọn')).toHaveTextContent('other-cluster');
    expect(api.registers()).toHaveLength(0);
  });

  it('ignores a pending inspection after unmount', async () => {
    const user = userEvent.setup();
    const pending = pendingInspects();
    const errors = vi.spyOn(console, 'error').mockImplementation(() => undefined);
    stubApi({ inspect: pending.handler });
    const view = render(<ConnectionsPage />);
    await screen.findByText(emptyList);
    await upload(user);
    await waitFor(() => expect(pending.waiting).toHaveLength(1));
    view.unmount();
    await act(async () => { pending.waiting[0]?.release(Response.json(oneContext)); });
    expect(errors).not.toHaveBeenCalled();
  });

  it('cancels a debounce timer on unmount so no inspection is sent', async () => {
    const user = userEvent.setup();
    const api = stubApi({ inspect: () => Response.json(oneContext) });
    const view = render(<ConnectionsPage />);
    await screen.findByText(emptyList);
    await pasteKubeconfig(user);
    view.unmount();
    await new Promise((resolve) => setTimeout(resolve, 800));
    expect(api.inspects()).toHaveLength(0);
  });

  it('rejects an oversized file without reading or inspecting it', async () => {
    const user = userEvent.setup();
    const api = stubApi({});
    render(<ConnectionsPage />);
    await screen.findByText(emptyList);
    await upload(user, 'x'.repeat((1 << 20) + 1), 'big.txt');
    expect(await screen.findByRole('alert')).toHaveTextContent('File kubeconfig lớn hơn 1 MiB.');
    expect(api.inspects()).toHaveLength(0);
  });

  it('rejects an oversized pasted document without inspecting it', async () => {
    const api = stubApi({});
    render(<ConnectionsPage />);
    await screen.findByText(emptyList);
    fireEvent.click(screen.getByLabelText('Dán nội dung kubeconfig'));
    fireEvent.change(screen.getByLabelText('Nội dung kubeconfig'), { target: { value: 'x'.repeat((1 << 20) + 1) } });
    expect(await screen.findByRole('alert', undefined, slow)).toHaveTextContent('Kubeconfig lớn hơn 1 MiB.');
    expect(api.inspects()).toHaveLength(0);
  });

  it('registers only on the explicit action, sends no UI state and clears everything on success', async () => {
    const user = userEvent.setup();
    let registered = false;
    const api = stubApi({
      inspect: () => Response.json(oneContext),
      register: () => { registered = true; return Response.json({ key: 'lab-cluster', name: 'Lab cluster', kind: 'KUBERNETES', status: 'READY' }, { status: 201 }); },
      list: () => Response.json({ connections: registered ? [{ key: 'lab-cluster', name: 'Lab cluster', kind: 'KUBERNETES', status: 'READY', config: { cluster: 'kind-cluster', endpoint: 'https://127.0.0.1:6443' } }] : [] }),
    });
    render(<ConnectionsPage />);
    await screen.findByText(emptyList);
    await pasteKubeconfig(user);
    await screen.findByLabelText('Đích đã chọn', undefined, slow);
    expect(api.registers()).toHaveLength(0);
    await user.clear(nameField());
    await user.type(nameField(), 'Lab cluster');
    await user.click(submitButton());
    expect(await screen.findByRole('status')).toHaveTextContent('Đã đăng ký kết nối Lab cluster (lab-cluster).');
    expect(api.registers()).toEqual([{ url: '/api/v1/connections/kubernetes', body: { name: 'Lab cluster', kubeconfig, context: 'kind-lab' } }]);
    const row = (await screen.findByText('Lab cluster')).closest('tr');
    expect(row).toHaveTextContent('Sẵn sàng');
    expect(nameField()).toHaveValue('');
    expect(screen.getByLabelText('Nội dung kubeconfig')).toHaveValue('');
    expect(screen.queryByLabelText('Đích đã chọn')).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain('synthetic-secret-token');
    // The name suggestion is available again for the next document.
    await pasteKubeconfig(user, otherKubeconfig);
    await waitFor(() => expect(nameField()).toHaveValue('kind-cluster'), slow);
  });

  it('shows fixed guidance for a verification failure without raw provider text and keeps the draft', async () => {
    const user = userEvent.setup();
    stubApi({ inspect: () => Response.json(oneContext), register: () => Response.json({ error: 'connection: verification failed: x509: provider says token synthetic-secret-token' }, { status: 422 }) });
    render(<ConnectionsPage />);
    await screen.findByText(emptyList);
    await pasteKubeconfig(user);
    await screen.findByLabelText('Đích đã chọn', undefined, slow);
    await user.click(submitButton());
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('Không xác minh được cluster nên chưa lưu kết nối.');
    expect(document.body.textContent).not.toMatch(/x509|provider says|verification failed/);
    expect(nameField()).toHaveValue('kind-cluster');
    expect(screen.getByLabelText('Nội dung kubeconfig')).toHaveValue(kubeconfig);
    expect(screen.getByLabelText('Nội dung kubeconfig')).toHaveClass('masked-text');
    expect(submitButton()).toBeEnabled();
  });

  it('reports an unknown network outcome without resubmitting and reloads only the list', async () => {
    const user = userEvent.setup();
    const api = stubApi({ inspect: () => Response.json(oneContext), register: () => { throw new TypeError('network down'); } });
    render(<ConnectionsPage />);
    await screen.findByText(emptyList);
    await user.type(nameField(), 'lab');
    await pasteKubeconfig(user);
    await screen.findByLabelText('Đích đã chọn', undefined, slow);
    await user.click(submitButton());
    expect(await screen.findByRole('alert')).toHaveTextContent('chưa biết kết nối đã được lưu hay chưa');
    expect(api.registers()).toHaveLength(1);
    expect(nameField()).toHaveValue('lab');
    expect(document.body.textContent).not.toContain('network down');
  });

  it('shows a Vietnamese list failure with retry instead of raw text', async () => {
    const user = userEvent.setup();
    let fail = true;
    stubApi({ list: () => fail ? Response.json({ error: 'dial tcp raw failure' }, { status: 500 }) : Response.json({ connections: [] }) });
    render(<ConnectionsPage />);
    const alert = await screen.findByRole('alert', { name: 'Lỗi tải danh sách' });
    expect(alert).toHaveTextContent('Không tải được danh sách kết nối.');
    expect(document.body.textContent).not.toContain('raw failure');
    fail = false;
    await user.click(screen.getByRole('button', { name: 'Thử lại' }));
    expect(await screen.findByText(emptyList)).toBeInTheDocument();
  });
});
