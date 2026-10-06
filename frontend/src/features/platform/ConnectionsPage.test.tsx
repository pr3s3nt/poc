import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ConnectionsPage } from './ConnectionsPage';

const kubeconfig = 'apiVersion: v1\nkind: Config\n# token: synthetic-secret-token\n';
const twoContexts = { contexts: [{ name: 'dev', cluster: 'dev-cluster', endpoint: 'https://dev.example' }, { name: 'prod', cluster: 'prod-cluster', endpoint: 'https://prod.example' }] };
const oneContext = { contexts: [{ name: 'kind-lab', cluster: 'kind-lab', endpoint: 'https://127.0.0.1:6443' }] };

type Handler = (url: string, body: Record<string, string>) => Response | Promise<Response>;

function stubApi(handlers: { inspect?: Handler; register?: Handler; list?: () => Response }) {
  const calls: { url: string; body: Record<string, string> }[] = [];
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
  return calls;
}

afterEach(() => vi.unstubAllGlobals());

async function pasteKubeconfig(user: ReturnType<typeof userEvent.setup>, text = kubeconfig) {
  await user.click(screen.getByLabelText('Paste kubeconfig'));
  await user.click(screen.getByLabelText('Kubeconfig content'));
  await user.paste(text);
}

describe('UC-04 Kubernetes connections', () => {
  it('registers a pasted kubeconfig with a chosen context and clears the credential', async () => {
    const user = userEvent.setup();
    let registered = false;
    const calls = stubApi({
      register: () => { registered = true; return Response.json({ key: 'lab-cluster', name: 'Lab cluster', kind: 'KUBERNETES', status: 'READY' }, { status: 201 }); },
      list: () => Response.json({ connections: registered ? [{ key: 'lab-cluster', name: 'Lab cluster', kind: 'KUBERNETES', status: 'READY', config: { cluster: 'prod-cluster', endpoint: 'https://prod.example' } }] : [] }),
    });
    render(<ConnectionsPage />);
    await screen.findByText('No connections registered yet.');
    expect(screen.queryByLabelText('Connection ID')).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/Cluster ID/)).not.toBeInTheDocument();
    await user.type(screen.getByLabelText('Connection name'), 'Lab cluster');
    await pasteKubeconfig(user);
    expect(screen.getByLabelText('Kubeconfig content')).toHaveClass('masked-text');
    await user.click(screen.getByRole('button', { name: 'Check and save' }));
    expect(screen.getByRole('alert')).toHaveTextContent('Inspect the kubeconfig and select a context');
    expect(calls).toHaveLength(0);
    await user.click(screen.getByRole('button', { name: 'Inspect kubeconfig' }));
    await user.selectOptions(await screen.findByLabelText('Context'), 'prod');
    const summary = screen.getByLabelText('Selected destination');
    expect(summary).toHaveTextContent('Cluster prod-cluster');
    expect(summary).toHaveTextContent('Endpoint https://prod.example');
    await user.click(screen.getByRole('button', { name: 'Check and save' }));
    expect(await screen.findByRole('status')).toHaveTextContent('Registered connection Lab cluster (lab-cluster).');
    const row = (await screen.findByText('Lab cluster')).closest('tr');
    expect(row).toHaveTextContent('lab-cluster');
    expect(row).toHaveTextContent('Endpoint https://prod.example');
    expect(calls[1]).toEqual({ url: '/api/v1/connections/kubernetes', body: { name: 'Lab cluster', kubeconfig, context: 'prod' } });
    expect(screen.getByLabelText('Connection name')).toHaveValue('');
    expect(screen.getByLabelText('Kubeconfig content')).toHaveValue('');
    expect(screen.queryByLabelText('Selected destination')).not.toBeInTheDocument();
    expect(document.body.textContent).not.toContain('synthetic-secret-token');
  });

  it('auto-selects a single context from an uploaded file without displaying it', async () => {
    const user = userEvent.setup();
    const calls = stubApi({ inspect: () => Response.json(oneContext) });
    render(<ConnectionsPage />);
    await screen.findByText('No connections registered yet.');
    await user.type(screen.getByLabelText('Connection name'), 'kind lab');
    await user.upload(screen.getByLabelText('Kubeconfig file'), new File([kubeconfig], 'config.txt', { type: 'text/plain' }));
    expect(await screen.findByText(/Selected file: config.txt/)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Inspect kubeconfig' }));
    expect(await screen.findByLabelText('Selected destination')).toHaveTextContent('Context kind-lab');
    expect(screen.queryByLabelText('Context')).not.toBeInTheDocument();
    expect(calls[0]?.body).toEqual({ kubeconfig });
    expect(document.body.textContent).not.toContain('synthetic-secret-token');
  });

  it('invalidates the context and ignores a stale inspection when the document changes', async () => {
    const user = userEvent.setup();
    let release: (response: Response) => void = () => undefined;
    stubApi({ inspect: () => new Promise<Response>((done) => { release = done; }) });
    render(<ConnectionsPage />);
    await screen.findByText('No connections registered yet.');
    await user.type(screen.getByLabelText('Connection name'), 'lab');
    await pasteKubeconfig(user);
    await user.click(screen.getByRole('button', { name: 'Inspect kubeconfig' }));
    expect(screen.getByRole('button', { name: 'Inspecting…' })).toBeDisabled();
    await user.click(screen.getByLabelText('Kubeconfig content'));
    await user.paste('# changed');
    release(Response.json(oneContext));
    await waitFor(() => expect(screen.getByRole('button', { name: 'Inspect kubeconfig' })).toBeEnabled());
    expect(screen.queryByLabelText('Selected destination')).not.toBeInTheDocument();
  });

  it('shows safe parser guidance and keeps the form', async () => {
    const user = userEvent.setup();
    stubApi({ inspect: () => Response.json({ error: 'connection: invalid registration: the selected context uses an unsupported authentication method' }, { status: 400 }) });
    render(<ConnectionsPage />);
    await screen.findByText('No connections registered yet.');
    await user.type(screen.getByLabelText('Connection name'), 'eks');
    await pasteKubeconfig(user);
    await user.click(screen.getByRole('button', { name: 'Inspect kubeconfig' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('unsupported authentication method');
    expect(screen.getByLabelText('Connection name')).toHaveValue('eks');
    expect(screen.getByLabelText('Kubeconfig content')).toHaveValue(kubeconfig);
  });

  it('reports an unknown network outcome without resubmitting', async () => {
    const user = userEvent.setup();
    const calls = stubApi({ inspect: () => Response.json(oneContext), register: () => { throw new TypeError('network down'); } });
    render(<ConnectionsPage />);
    await screen.findByText('No connections registered yet.');
    await user.type(screen.getByLabelText('Connection name'), 'lab');
    await pasteKubeconfig(user);
    await user.click(screen.getByRole('button', { name: 'Inspect kubeconfig' }));
    await screen.findByLabelText('Selected destination');
    await user.click(screen.getByRole('button', { name: 'Check and save' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('may have been saved');
    expect(calls.filter((call) => !call.url.endsWith('/inspect'))).toHaveLength(1);
    expect(screen.getByLabelText('Connection name')).toHaveValue('lab');
  });
});
