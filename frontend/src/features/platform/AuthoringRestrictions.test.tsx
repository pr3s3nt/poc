import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useState } from 'react';
import { afterEach, expect, it, vi } from 'vitest';
import { TemplateDriverFields, emptyTemplateDriver } from './TemplateDriverFields';
import { ResourceDefinitionsPage } from './ResourceDefinitionsPage';
import { WorkloadEditorPage } from '../workloads/WorkloadEditorPage';
import { backend } from '../../test/definitionKit';
import { bothConfigured } from '../../test/targets';

afterEach(() => vi.unstubAllGlobals());

const source = (kind: 'git' | 'inline', id: string) => ({
  id, name: `${kind}-template`, version: 'a'.repeat(12), status: 'AVAILABLE',
  source: { kind, url: kind === 'git' ? 'https://example.com/templates.git' : 'inline:sha256:example', path: '.', revision: 'a'.repeat(40), contentSha256: 'b'.repeat(64) },
  inputsSchema: { type: 'object', properties: {} }, engineContract: 'template-v1',
  inputContract: 'orchestrator.dev/template-input/v1', renderer: { type: 'template', version: '1.0.0' },
});
const git = source('git', '11111111-1111-4111-8111-111111111111');
const inline = source('inline', '22222222-2222-4222-8222-222222222222');

function TemplateForm({ editing = false }: { editing?: boolean }) {
  const [value, setValue] = useState(() => ({ ...emptyTemplateDriver(), source: editing ? inline.id : '' }));
  return <><TemplateDriverFields value={value} onChange={setValue} disabled={false} editing={editing} /><output aria-label="selection">{value.source}</output></>;
}

it('offers only Git template authoring and verified Git choices', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => Response.json({ templateSources: [git, inline] })));
  render(<TemplateForm />);
  await screen.findByRole('option', { name: /git-template/ });
  expect(screen.getByLabelText('Git HTTPS URL')).toBeVisible();
  for (const name of ['Nhập trực tiếp', 'ZIP', 'Nguồn đã đăng ký']) expect(screen.queryByRole('button', { name })).not.toBeInTheDocument();
  expect(screen.queryByRole('option', { name: /inline-template/ })).not.toBeInTheDocument();
  await userEvent.setup().selectOptions(screen.getByLabelText('Nguồn đã xác thực'), git.id);
  expect(screen.getByLabelText('selection')).toHaveTextContent(git.id);
});

it('preserves an existing non-Git template and restricts replacement to Git', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => Response.json({ templateSources: [git, inline] })));
  render(<TemplateForm editing />);
  await screen.findByText('Inline');
  expect(screen.getByLabelText('selection')).toHaveTextContent(inline.id);
  await userEvent.setup().click(screen.getByRole('button', { name: /Đổi nguồn/ }));
  expect(screen.getByLabelText('Git HTTPS URL')).toBeVisible();
  expect(screen.getByRole('option', { name: /inline-template/ })).toBeDisabled();
  expect(screen.getByLabelText('selection')).toHaveTextContent(inline.id);
});

it('offers Git only when creating a Terraform Definition', async () => {
  backend();
  render(<ResourceDefinitionsPage initialResourceType="vpc" />);
  await waitFor(() => expect(screen.getByLabelText('Loại tài nguyên')).toHaveValue('vpc'));
  const choices = screen.getByRole('group', { name: 'Nguồn module Terraform' });
  expect(within(choices).getAllByRole('button')).toHaveLength(1);
  expect(within(choices).getByRole('button', { name: 'Thêm từ Git' })).toBeVisible();
  expect(screen.getByLabelText('Git HTTPS URL')).toBeVisible();
  expect(screen.getByRole('button', { name: 'Đăng ký cấu hình tài nguyên' })).toBeDisabled();
});

it('keeps Workload import hidden while allowing the scoped .env file picker', async () => {
  vi.stubGlobal('fetch', vi.fn(async (input, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/configuration')) return Response.json({ keys: [] });
    if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: [] });
    if (url.endsWith('/workloads/resolve-defaults')) return Response.json({ workload: JSON.parse(String(init?.body)).workload, sources: {} });
    return Response.json({ draftVersion: 0, workloads: [] });
  }));
  const { container } = render(<WorkloadEditorPage application={{ id: 'app', name: 'App', environments: bothConfigured(), workloads: { staging: [], production: [] } }} environment="staging" />);
  await screen.findByRole('heading', { name: 'Thông tin cơ bản' });
  expect(screen.getByLabelText('Tên Workload')).toBeEnabled();
  expect(screen.queryByRole('button', { name: 'Nhập Workload' })).not.toBeInTheDocument();
  expect(container.querySelectorAll('input[type="file"]')).toHaveLength(1);
  expect(screen.getByLabelText('Chọn file UTF-8')).toHaveAttribute('accept', '.env,text/plain');
});
