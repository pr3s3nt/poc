import { screen, waitFor, within } from '@testing-library/react';
import { expect } from 'vitest';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';
import { capabilityFixture } from './capabilities';
import { typeFixture } from './types';

export type Body = Record<string, unknown>;
export type Backend = {
  types?: unknown[]; definitions?: Body[]; post?: () => Response; typesFail?: boolean;
  capabilitiesFail?: boolean; capabilities?: unknown[]; capabilityLists?: (() => Response | Promise<Response>)[];
  connections?: Body[]; connectionsFail?: boolean; connectionLists?: (() => Response | Promise<Response>)[];
  definitionLists?: (() => Response)[]; applications?: Body[]; applicationsFail?: boolean; applicationLists?: (() => Response | Promise<Response>)[];
};
export const defaultApplications = [
  { key: 'shop', name: 'Cửa hàng', environments: [{ key: 'staging', name: 'Staging', environmentType: 'staging' }, { key: 'production', name: 'Production', environmentType: 'production' }] },
  { key: 'blog', name: 'blog', environments: [{ key: 'dev', name: 'Development', environmentType: 'development' }] },
];

// Serves the Console lists of the Definition page from recorded backend metadata and records every request.
export function backend(options: Backend = {}) {
  const posted: Body[] = [];
  const gets: string[] = [];
  const state = { ...options };
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (init?.method === 'POST') {
      posted.push(JSON.parse(String(init.body)) as Body);
      return (state.post ?? (() => Response.json({}, { status: 201 })))();
    }
    gets.push(url);
    if (url.endsWith('/resource-definition-capabilities')) {
      const next = state.capabilityLists?.shift();
      if (next) return next();
      return state.capabilitiesFail ? Response.json({ code: 'INTERNAL_ERROR' }, { status: 500 }) : Response.json({ capabilities: state.capabilities ?? capabilityFixture });
    }
    if (url.endsWith('/resource-types')) return state.typesFail ? Response.json({ code: 'INTERNAL_ERROR' }, { status: 500 }) : Response.json({ resourceTypes: state.types ?? typeFixture });
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
  return { posted, gets, state };
}

export const field = (name: string) => screen.getByLabelText(name);
export const submitName = 'Đăng ký cấu hình tài nguyên';
export const submitDefinition = (user: ReturnType<typeof userEvent.setup>) => user.click(screen.getByRole('button', { name: submitName }));

type User = ReturnType<typeof userEvent.setup>;
export type Fill = { id: string; type: string; module?: string; profile?: string; binding?: 'environment' | { cluster: string }; skipVariables?: string[] };

// Drives the real form to a registrable Definition of one verified module,
// the way a Platform Engineer does: pick the pin and the scope, map the
// contract explicitly, bind the provider and opt into the presets.
export async function completeDefinition(user: User, fill: Fill) {
  await user.type(field('ID cấu hình'), fill.id);
  await user.selectOptions(field('Loại tài nguyên'), fill.type);
  const pins = field('Module đã kiểm chứng') as HTMLSelectElement;
  if (!pins.disabled) {
    const option = within(pins).getAllByRole('option').find((candidate) => candidate.textContent?.startsWith(`${fill.module} ·`));
    if (!option) throw new Error(`no pin ${fill.module}`);
    await user.selectOptions(pins, option);
  }
  const scope = field('Phạm vi triển khai') as HTMLSelectElement;
  if (!scope.disabled) await user.selectOptions(scope, fill.profile ?? 'internal-k8s');
  const inputs = screen.queryByRole('button', { name: 'Ghép đầu vào theo tên trùng' });
  if (inputs) await user.click(inputs);
  await user.click(screen.getByRole('button', { name: 'Ghép đầu ra theo tên trùng' }));
  for (const select of screen.queryAllByLabelText(/^Cluster đích của provider /)) {
    if (fill.binding && typeof fill.binding === 'object') {
      await user.selectOptions(select, 'resource');
      await user.selectOptions(screen.getByLabelText(/^Class cluster EKS của provider /), fill.binding.cluster);
    } else await user.selectOptions(select, 'environment');
  }
  for (const preset of screen.queryAllByRole('button', { name: /^Dùng preset cho / })) {
    const name = preset.getAttribute('aria-label')?.replace('Dùng preset cho ', '') ?? '';
    if (!fill.skipVariables?.includes(name)) await user.click(preset);
  }
}

export async function ready(element: React.ReactElement, render: (ui: React.ReactElement) => unknown) {
  render(element);
  await screen.findByText('Chưa có cấu hình tài nguyên nào.');
  await waitFor(() => expect(screen.getByRole('option', { name: 'vpc' })).toBeEnabled());
}
