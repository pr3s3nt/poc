import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { CatalogPage } from './CatalogPage';
afterEach(() => vi.unstubAllGlobals());
it('keeps legacy sources and release usage readable without Catalog mutations', async () => {
 const requests: string[] = [];
 vi.stubGlobal('fetch',vi.fn(async (url: string, init?: RequestInit) => {
  requests.push(init?.method ?? 'GET');
  return Response.json(url.endsWith('/releases')
   ? {releases:[{tag:'v1.0.0',commit:'abc',usage:[{applicationKey:'shop',environmentKey:'staging',applied:true}]}]}
   : {sources:[{id:'company',branch:'main',code:'READY'}]});
 }));
 const user=userEvent.setup();render(<CatalogPage />);
 expect(await screen.findByText('company')).toBeVisible();
 expect(screen.queryByRole('textbox')).not.toBeInTheDocument();
 await user.click(screen.getByRole('button',{name:'Các phiên bản'}));
 expect(await screen.findByText('v1.0.0')).toBeVisible();
 expect(screen.getByText(/shop · staging/)).toBeVisible();
 expect(requests).toEqual(['GET','GET']);
});
