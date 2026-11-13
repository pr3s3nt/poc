import { render, screen, within } from '@testing-library/react';
import { afterEach, expect, it } from 'vitest';
import viCatalog from '../../../public/locales/vi.json';
import enCatalog from '../../../public/locales/en.json';
import { setCatalogs } from '../../shared/i18n';
import { parseEnvfile } from '../workloads/envfile';
import type { RowContext } from '../workloads/bindings';
import { environmentExample, secretExample, syntaxExample, UserGuidePage } from './UserGuidePage';

afterEach(() => setCatalogs('vi', viCatalog));

it.each([['vi', viCatalog], ['en', enCatalog]] as const)('publishes the selected article, with working contents in %s', (locale, catalog) => {
  setCatalogs(locale, catalog);
  render(<UserGuidePage />);
  expect(screen.getAllByRole('heading', { level: 1 })).toHaveLength(1);
  expect(screen.getAllByRole('article')).toHaveLength(1);
  for (const link of within(screen.getByRole('navigation')).getAllByRole('link')) {
    const href = link.getAttribute('href')!;
    if (!href.startsWith('#')) continue;
    const target = document.getElementById(href.slice(1));
    expect(target).not.toBeNull();
    expect(target).toHaveAttribute('tabindex', '-1');
  }
  expect(screen.queryByRole('button')).not.toBeInTheDocument();
  expect(document.body.textContent).not.toContain('guide.');
});

it.each([['vi', viCatalog], ['en', enCatalog]] as const)('renders the Workload article and resolves all its anchors in %s', (locale, catalog) => {
  setCatalogs(locale, catalog);
  render(<UserGuidePage topic="workloads" />);
  expect(screen.getAllByRole('article')).toHaveLength(1);
  expect(screen.getByRole('heading', { level: 2, name: catalog['guide.workload.title'] })).toBeVisible();
  expect(screen.queryByRole('heading', { name: catalog['guide.environment.title'] })).not.toBeInTheDocument();
  for (const link of within(screen.getByRole('navigation')).getAllByRole('link')) {
    const href = link.getAttribute('href')!;
    if (href.startsWith('#')) expect(document.getElementById(href.slice(1))).toHaveAttribute('tabindex', '-1');
  }
  expect(within(screen.getByRole('navigation')).getByRole('link', { name: catalog['guide.workload.title'] })).toHaveAttribute('aria-current', 'page');
  expect(document.body.textContent).not.toContain('guide.');
  expect(screen.queryByRole('button')).not.toBeInTheDocument();
});

it('keeps published examples valid against the actual .env parser', () => {
  const context: RowContext = {
    catalog: { status: 'ready', version: 1, keys: [
      { name: 'REGION', kind: 'VARIABLE', configured: true, usedBy: [] },
      { name: 'API_TOKEN', kind: 'SECRET', configured: true, usedBy: [] },
    ] },
    dependencies: [{ alias: 'db', type: 'postgres' }],
    outputs: alias => alias === 'db' ? ['host', 'password'] : undefined,
    services: [{ id: 'backend', ports: ['http'] }],
  };
  const reference = parseEnvfile(environmentExample, context);
  expect(reference.problems).toEqual([]);
  expect(reference.bindings.map(row => row.source)).toEqual(['LITERAL', 'LITERAL', 'LITERAL', 'LITERAL', 'VARIABLE', 'SECRET', 'RESOURCE', 'RESOURCE', 'SERVICE']);
  const secret = parseEnvfile(secretExample, context);
  expect(secret.problems).toEqual([]);
  expect(secret.bindings[0]).toMatchObject({ source: 'LITERAL', secretDraft: true });
  const syntax = parseEnvfile(syntaxExample, context);
  expect(syntax.problems).toEqual([]);
  expect(syntax.bindings.map(row => row.value)).toEqual(['  Demo API  ', 'abc # 123', 'a=b=c', '', 'line 1\nline 2', 'C:\\data\\files', '@team.example']);
});
