import { act, cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { DefinitionDocumentDownload } from './DefinitionDocumentDownload';
import { downloadDocument } from './catalogDocument';

vi.mock('./catalogDocument', () => ({ downloadDocument: vi.fn() }));
afterEach(() => { cleanup(); vi.unstubAllGlobals(); vi.clearAllMocks(); });

it('downloads the server projection of the selected immutable revision', async () => {
  const doc = { key: 'mongo', driverInputs: { values: { source: { url: 'https://example.com/repo', ref: 'a'.repeat(40), path: 'terraform/mongodb' } } } };
  const fetch = vi.fn().mockResolvedValue(Response.json(doc)); vi.stubGlobal('fetch', fetch);
  render(<DefinitionDocumentDownload definitionKey="mongo" label="r2" revisionId="revision-2" />);
  await userEvent.setup().click(screen.getByRole('button', { name: 'Tải file · r2' }));
  expect(fetch.mock.calls[0]?.[0]).toBe('/api/v1/resource-definitions/mongo/document?revision=revision-2');
  expect(downloadDocument).toHaveBeenCalledWith('resource-definitions', doc, 'r2');
});

it('requests the displayed list version instead of silently exporting a newer one', async () => {
  const fetch = vi.fn().mockResolvedValue(Response.json({ key: 'mongo' })); vi.stubGlobal('fetch', fetch);
  render(<DefinitionDocumentDownload definitionKey="mongo" label="v1" version={1} />);
  await userEvent.setup().click(screen.getByRole('button', { name: 'Tải file · v1' }));
  expect(fetch.mock.calls[0]?.[0]).toBe('/api/v1/resource-definitions/mongo/document?version=1');
});

it('discards an old download on scope change and reports a safe failure without exporting stale metadata', async () => {
  let resolve: (r: Response) => void = () => {};
  const fetch = vi.fn().mockImplementationOnce(() => new Promise<Response>(r => { resolve = r; })).mockResolvedValueOnce(Response.json({ error: 'raw provider credential' }, { status: 404 }));
  vi.stubGlobal('fetch', fetch);
  const { rerender } = render(<DefinitionDocumentDownload definitionKey="first" label="v1" />);
  const user = userEvent.setup();
  await user.click(screen.getByRole('button', { name: 'Tải file · v1' }));
  rerender(<DefinitionDocumentDownload definitionKey="second" label="v1" />);
  await act(async () => { resolve(Response.json({ key: 'first' })); });
  expect(downloadDocument).not.toHaveBeenCalled();
  await user.click(screen.getByRole('button', { name: 'Tải file · v1' }));
  expect(await screen.findByRole('alert')).not.toHaveTextContent('raw provider credential');
  expect(downloadDocument).not.toHaveBeenCalled();
});
