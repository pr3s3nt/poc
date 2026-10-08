// Proves the recorder's secret scan: the naive serialized-HTML check cannot work
// on a form being filled in (React mirrors the typed token into the password
// input's value attribute), while the structural scan accepts exactly that field
// and nothing else. Uses the real Secret stores page.
import { createElement } from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { SecretStoresPage } from '../../src/features/platform/SecretStoresPage';
import { scanDocument } from './secret-scan.mjs';

const extras = [];
const attach = (node) => { document.body.append(node); extras.push(node); return node; };
afterEach(() => {
  vi.unstubAllGlobals();
  document.title = '';
  window.localStorage.clear();
  document.cookie = 'c=; expires=Thu, 01 Jan 1970 00:00:00 GMT';
  for (const node of extras.splice(0)) node.remove();
});
const TOKEN = 'hvs.CAESIFAKE-scoped-token-0123456789';

async function filledForm() {
  vi.stubGlobal('fetch', vi.fn(async () => Response.json({ secretStores: [] })));
  const user = userEvent.setup();
  render(createElement(SecretStoresPage));
  await screen.findByText('No secret store is registered yet.');
  await user.type(screen.getByLabelText(/^Token/), TOKEN);
  return screen.getByLabelText(/^Token/);
}
const scan = (allow = ['input[type="password"]']) => scanDocument({ values: [TOKEN], allow });

it('explains the failure: serialized HTML contains the token only through the typed password input value', async () => {
  const field = await filledForm();
  expect(field).toHaveAttribute('type', 'password');
  expect(document.body.outerHTML).toContain(TOKEN);
  expect(field.getAttribute('value')).toBe(TOKEN);
  expect(document.body.textContent).not.toContain(TOKEN);
});

it('accepts the token only inside the active password input', async () => {
  await filledForm();
  expect(scan()).toEqual({ leaks: [], allowedFields: 1 });
});

it('flags the same page when nothing is exempt, naming the location and not the secret', async () => {
  await filledForm();
  const { leaks } = scan([]);
  expect(leaks).toEqual(['attribute value of <input>', 'field value of <input>']);
  expect(JSON.stringify(leaks)).not.toContain(TOKEN);
});

it('flags leaks outside the permitted field: text, other attributes, other fields, title, storage, cookie', async () => {
  const field = await filledForm();
  const text = document.createElement('p'); text.textContent = `echo ${TOKEN}`; attach(text);
  const other = document.createElement('div'); other.setAttribute('data-debug', TOKEN); attach(other);
  const plain = document.createElement('input'); plain.value = TOKEN; attach(plain);
  field.setAttribute('placeholder', TOKEN);
  document.title = `t ${TOKEN}`;
  window.localStorage.setItem('k', TOKEN);
  document.cookie = `c=${TOKEN}`;
  const { leaks } = scan();
  for (const expected of ['page text', 'attribute data-debug of <div>', 'field value of <input>', 'attribute placeholder of <input>', 'document title', 'localStorage', 'cookie']) expect(leaks).toContain(expected);
});

it('does not exempt a non-password input and is clean once the token is cleared after submission', async () => {
  const field = await filledForm();
  const text = document.createElement('input'); text.type = 'text'; text.className = 'plain'; attach(text);
  text.setAttribute('value', TOKEN);
  const { leaks } = scan(['input.plain']);
  expect(leaks).toContain('attribute value of <input>');
  text.remove();
  const user = userEvent.setup();
  await user.clear(field);
  expect(scan([])).toEqual({ leaks: [], allowedFields: 0 });
});
