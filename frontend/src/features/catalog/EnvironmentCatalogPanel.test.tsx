import { render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { EnvironmentCatalogPanel } from './EnvironmentCatalogPanel';
afterEach(() => vi.unstubAllGlobals());
it('shows existing applied and desired pins without offering selection or writes', async () => {
  const fetcher = vi.fn(async () => Response.json({catalog:{applied:{sourceId:'company',tag:'v1.0.0'},desired:{sourceId:'company',tag:'v2.0.0'},status:'PENDING'},releases:[]}));
  vi.stubGlobal('fetch',fetcher);
  render(<EnvironmentCatalogPanel applicationId="shop" environment="staging" busy={false} onSaved={vi.fn()} />);
  expect(await screen.findByText('company · v2.0.0')).toBeVisible();
  expect(screen.getByText('company · v1.0.0')).toBeVisible();
  expect(screen.queryByRole('combobox')).not.toBeInTheDocument();
  expect(screen.queryByRole('button',{name:'Lưu lựa chọn'})).not.toBeInTheDocument();
  expect(fetcher).toHaveBeenCalledTimes(1);
});
it('hides the Catalog panel when no selection exists', async () => {
  const fetcher=vi.fn(async () => Response.json({catalog:{status:'LEGACY'},releases:[]}));vi.stubGlobal('fetch',fetcher);
  const {container}=render(<EnvironmentCatalogPanel applicationId="shop" environment="staging" busy={false} onSaved={vi.fn()} />);
  await waitFor(()=>expect(fetcher).toHaveBeenCalledOnce());
  expect(container).toBeEmptyDOMElement();
});
