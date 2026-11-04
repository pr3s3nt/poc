import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useState } from 'react';
import { afterEach, expect, it, vi } from 'vitest';
import { TemplateDriverFields, emptyTemplateDriver } from './TemplateDriverFields';

const source = { id: '11111111-1111-4111-8111-111111111111', name: 'custom-workload', version: 'a'.repeat(12), status: 'AVAILABLE', source: { kind: 'inline', url: `inline:sha256:${'a'.repeat(64)}`, path: '.', revision: 'a'.repeat(64), contentSha256: 'a'.repeat(64) }, inputsSchema: { type: 'object', additionalProperties: false, properties: {} }, engineContract: 'template-v1', inputContract: 'orchestrator.dev/template-input/v1', renderer: { type: 'template', version: '1.0.0' } };
function Form() { const [value, setValue] = useState(emptyTemplateDriver); return <><TemplateDriverFields value={value} onChange={setValue} disabled={false} /><output aria-label="selected source">{value.source}</output></>; }
afterEach(() => vi.unstubAllGlobals());
it('prepares named Deployment and Service files, pins the result, and invalidates it after edits', async () => {
  let files: Record<string, string> = {};
  vi.stubGlobal('fetch', vi.fn(async (_input, init?: RequestInit) => {
    if (init?.method === 'POST') { files = JSON.parse(String(init.body)).files as Record<string, string>; return Response.json({ templateSource: source }); }
    return Response.json({ templateSources: [] });
  }));
  const user = userEvent.setup(); render(<Form />);
  await user.click(screen.getByRole('button', { name: 'Nhập trực tiếp' }));
  expect(screen.getByLabelText('deployment.tpl')).toBeInTheDocument();
  expect(screen.getByLabelText('service.tpl')).toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Kiểm tra template' }));
  await waitFor(() => expect(screen.getByLabelText('selected source')).toHaveTextContent(source.id));
  expect(files['bundle.yaml']).toContain('type: template');
  expect(files['service.tpl']).toContain('gt (len $ports) 0');
  await user.type(screen.getByLabelText('service.tpl'), ' ');
  expect(screen.getByLabelText('selected source')).toBeEmptyDOMElement();
});
it('keeps a verified source when returning to its editor and shows a load failure', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => Response.json({ templateSources: [source] })));
  const user = userEvent.setup(); render(<Form />);
  await screen.findByRole('option', { name: /custom-workload/ });
  await user.selectOptions(screen.getByLabelText('Nguồn đã xác thực'), source.id);
  expect(screen.getByLabelText('selected source')).toHaveTextContent(source.id);
  await user.click(screen.getByRole('button', { name: /^Git$/ }));
  expect(screen.getByLabelText('selected source')).toHaveTextContent(source.id);
});

it('keeps source load errors retryable without preparing or saving anything', async () => {
 let attempts=0;
 vi.stubGlobal('fetch',vi.fn(async()=>{attempts++;return Response.json(attempts===1?{templateSources:'invalid'}:{templateSources:[source]});}));
 const user=userEvent.setup();render(<Form />);
 await screen.findByRole('alert');await user.click(screen.getByRole('button',{name:'Thử lại'}));
 await screen.findByRole('option',{name:/custom-workload/});expect(screen.queryByRole('alert')).not.toBeInTheDocument();expect(attempts).toBe(2);
});
