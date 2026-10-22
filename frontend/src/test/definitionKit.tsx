import type { ReactElement } from 'react';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { expect } from 'vitest';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';
import { capabilityFixture } from './capabilities';
import { typeFixture } from './types';

// Open creation through the same catalog CTA used by a Platform Engineer.
export function renderDefinition(element: ReactElement) {
  const view = render(element);
  const create = screen.queryByRole('button', { name: 'Tạo cấu hình' });
  if (create) fireEvent.click(create);
  // These fixtures exercise the explicitly selected registered-module path.
  const registered = screen.queryByRole('button', { name: 'Module đã đăng ký' });
  if (registered) fireEvent.click(registered);
  return view;
}

export type Body = Record<string, unknown>;
export type Backend = {
  types?: unknown[]; definitions?: Body[]; post?: () => Response | Promise<Response>; put?: () => Response | Promise<Response>; typesFail?: boolean;
  capabilitiesFail?: boolean; capabilities?: unknown[]; capabilityLists?: (() => Response | Promise<Response>)[];
  accounts?: Body[]; accountsFail?: boolean; accountLists?: (() => Response | Promise<Response>)[];
  connections?: Body[]; connectionsFail?: boolean; connectionLists?: (() => Response | Promise<Response>)[];
  definitionLists?: (() => Response)[]; applications?: Body[]; applicationsFail?: boolean; applicationLists?: (() => Response | Promise<Response>)[];
};
// Driver Accounts of the V01 catalog: one READY AWS account (the default choice of the AWS pairs) and one internal cluster.
export const defaultAccounts: Body[] = [
  { key: 'aws-account', name: 'AWS chính', type: 'AWS', authenticationType: 'AWS_ACCESS_KEY', status: 'READY', version: 3 },
  { key: 'aws-second', name: 'AWS thứ hai', type: 'AWS', authenticationType: 'AWS_ACCESS_KEY', status: 'READY', version: 1 },
  { key: 'aws-pending', name: 'AWS chưa xác minh', type: 'AWS', authenticationType: 'AWS_ACCESS_KEY', status: 'VERIFYING', version: 1 },
  { key: 'internal-cluster', name: 'Cluster nội bộ', type: 'INTERNAL', authenticationType: 'KUBECONFIG', status: 'READY', version: 1 },
];
export const defaultApplications = [
  { key: 'shop', name: 'Cửa hàng', environments: [{ key: 'staging', name: 'Staging', environmentType: 'staging' }, { key: 'production', name: 'Production', environmentType: 'production' }] },
  { key: 'blog', name: 'blog', environments: [{ key: 'dev', name: 'Development', environmentType: 'development' }] },
];

// Serves the Console lists of the Definition page from recorded backend metadata and records every request.
export function backend(options: Backend = {}) {
  const posted: Body[] = [];
  const puts: { url: string; body: Body }[] = [];
  const gets: string[] = [];
  const state = { ...options };
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (init?.method === 'POST') {
      posted.push(JSON.parse(String(init.body)) as Body);
      return (state.post ?? (() => Response.json({}, { status: 201 })))();
    }
    if (init?.method === 'PUT') {
      const body = JSON.parse(String(init.body)) as Body;
      puts.push({ url, body });
      return (state.put ?? (() => Response.json({ ...body, version: Number(body.version) + 1 }, { status: 200 })))();
    }
    gets.push(url);
    if (url.endsWith('/resource-definition-capabilities')) {
      const next = state.capabilityLists?.shift();
      if (next) return next();
      return state.capabilitiesFail ? Response.json({ code: 'INTERNAL_ERROR' }, { status: 500 }) : Response.json({ capabilities: state.capabilities ?? capabilityFixture });
    }
    if (url.endsWith('/resource-types')) return state.typesFail ? Response.json({ code: 'INTERNAL_ERROR' }, { status: 500 }) : Response.json({ resourceTypes: state.types ?? typeFixture });
    if (url.endsWith('/driver-accounts')) {
      const next = state.accountLists?.shift();
      if (next) return next();
      return state.accountsFail ? Response.json({ code: 'INTERNAL_ERROR' }, { status: 500 }) : Response.json({ accounts: state.accounts ?? defaultAccounts });
    }
    if (url.endsWith('/connections')) {
      const next = state.connectionLists?.shift();
      if (next) return next();
      return state.connectionsFail ? Response.json({ code: 'INTERNAL_ERROR' }, { status: 500 }) : Response.json({ connections: state.connections ?? [] });
    }
    if (url.endsWith('/applications')) {
      const next = state.applicationLists?.shift();
      if (next) return next();
      return state.applicationsFail ? Response.json({ error: 'boom' }, { status: 500 }) : Response.json({ applications: state.applications ?? defaultApplications });
    }
    const next = state.definitionLists?.shift();
    return next ? next() : Response.json({ resourceDefinitions: state.definitions ?? [] });
  }));
  return { posted, puts, gets, state };
}

export const field = (name: string) => screen.getByLabelText(name);
export const submitName = 'Đăng ký cấu hình tài nguyên';
export const submitDefinition = (user: ReturnType<typeof userEvent.setup>) => user.click(screen.getByRole('button', { name: submitName }));

type User = ReturnType<typeof userEvent.setup>;
export type Fill = { id: string; name?: string; type: string; module?: string; account?: string; region?: string; binding?: 'environment' | { cluster: string }; skipVariables?: string[] };

// Drives the real form to a registrable Definition of one verified module,
// the way a Platform Engineer does: name it, pick the pin, the Driver Account
// and the literal region when the pair needs them, map the contract
// explicitly, bind the provider and opt into the presets.
export async function openAdvanced(user: User) {
  const summary = screen.getByText('Thiết lập nâng cao · ánh xạ & tài nguyên liên quan');
  if (!summary.closest('details')?.open) await user.click(summary);
}

export async function completeDefinition(user: User, fill: Fill) {
  const configuration = screen.getByRole('tab', { name: 'Cấu hình' });
  if (configuration.getAttribute('aria-selected') !== 'true') await user.click(configuration);
  // Paste fixture text in one user action; keystroke-by-keystroke guards have
  // their own tests. This avoids spending the test deadline re-rendering IDs.
  for (const [label, text] of [['ID cấu hình (Definition ID)', fill.id], ['Tên hiển thị', fill.name ?? `Tên ${fill.id}`]] as const) {
    await user.click(field(label));
    await user.paste(text);
  }
  await user.selectOptions(field('Loại tài nguyên'), fill.type);
  await user.click(screen.getByRole('button', { name: 'Module đã đăng ký' }));
  const pins = field('Module đã kiểm chứng') as HTMLSelectElement;
  if (!pins.disabled) {
    const option = within(pins).getAllByRole('option').find((candidate) => candidate.textContent?.startsWith(`${fill.module} ·`));
    if (!option) throw new Error(`no pin ${fill.module}`);
    await user.selectOptions(pins, option);
  }
  const account = screen.queryByLabelText('Tài khoản hạ tầng') as HTMLSelectElement | null;
  if (account) {
    await waitFor(() => expect(within(account).getByRole('option', { name: /AWS chính/ })).toBeInTheDocument());
    await user.selectOptions(account, fill.account ?? 'aws-account');
    await user.click(field('Vùng AWS'));
    await user.paste(fill.region ?? 'us-east-1');
  }
  await openAdvanced(user);
  const inputs = screen.queryByRole('button', { name: 'Ghép đầu vào theo tên trùng' });
  if (inputs) await user.click(inputs);
  await user.click(screen.getByRole('button', { name: 'Ghép đầu ra theo tên trùng' }));
  for (const select of screen.queryAllByLabelText(/^Nhóm cluster của provider /)) {
    await user.selectOptions(select, fill.binding && typeof fill.binding === 'object' ? fill.binding.cluster : 'eks');
  }
  // The module's recorded presets are now explicit source choices made through the shared chooser.
  const capability = capabilityFixture.find((candidate) => candidate.resourceType === fill.type && (fill.module === undefined || candidate.module === fill.module));
  for (const variable of capability?.variables ?? []) {
    if (variable.preset && !fill.skipVariables?.includes(variable.name)) await provideSource(user, variable.name, variable.preset);
  }
}

// Drives the source chooser of one parameter card from a placeholder: context values and Resource Type outputs only.
export async function provideSource(user: User, name: string, placeholder: string) {
  const open = screen.queryByRole('button', { name: `Cung cấp giá trị ${name}` }) ?? screen.getByRole('button', { name: `Thay đổi ${name}` });
  await user.click(open);
  const dialog = within(screen.getByRole('dialog'));
  const context = /^\$\{context\.(.+)\}$/.exec(placeholder);
  if (context) {
    await user.click(dialog.getByRole('radio', { name: 'Lấy từ ngữ cảnh triển khai' }));
    await user.selectOptions(dialog.getByLabelText('Giá trị ngữ cảnh'), context[1] ?? '');
  } else {
    const reference = /^\$\{resources\['([^.]+)\.([^#]+)#(.+)'\]\.outputs\.(.+)\}$/.exec(placeholder);
    if (!reference) throw new Error(`unsupported source ${placeholder}`);
    await user.click(dialog.getByRole('radio', { name: 'Lấy từ tài nguyên khác' }));
    await user.selectOptions(dialog.getByLabelText('Loại tài nguyên nguồn'), reference[1] ?? '');
    await user.selectOptions(dialog.getByLabelText('Nhóm cấu hình nguồn'), reference[2] ?? '');
    await user.selectOptions(dialog.getByLabelText('Phạm vi nguồn'), reference[3] === 'environments.@app.@env' || reference[3] === '@infra' ? (reference[3] ?? '') : 'resource');
    await user.selectOptions(dialog.getByLabelText('Output cần lấy'), reference[4] ?? '');
  }
  await user.click(dialog.getByRole('button', { name: 'Áp dụng' }));
}

// Provides one literal through the shared chooser; a whole placeholder is pasted, never typed key by key.
export async function provideLiteral(user: User, name: string, text: string) {
  const open = screen.queryByRole('button', { name: `Cung cấp giá trị ${name}` }) ?? screen.getByRole('button', { name: `Thay đổi ${name}` });
  await user.click(open);
  const dialog = within(screen.getByRole('dialog'));
  await user.click(dialog.getByRole('radio', { name: 'Nhập giá trị' }));
  const input = dialog.getByLabelText(`Giá trị ${name}`);
  if (input.tagName === 'SELECT') await user.selectOptions(input, text);
  else { await user.clear(input); await user.click(input); await user.paste(text); }
  await user.click(dialog.getByRole('button', { name: 'Áp dụng' }));
}

export const card = (name: string) => document.querySelector(`[data-variable="${name}"]`) as HTMLElement;

export async function ready(element: React.ReactElement, render: (ui: React.ReactElement) => unknown) {
  render(element);
  await screen.findByText('Chưa có cấu hình tài nguyên nào.');
  await waitFor(() => expect(screen.getByRole('option', { name: 'vpc' })).toBeEnabled());
}
