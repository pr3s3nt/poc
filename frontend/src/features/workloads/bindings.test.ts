import { expect, it } from 'vitest';
import { escapeLiteral, hasUnescapedPlaceholder, readBinding, unescapeLiteral } from './bindings';

// The backend expands ${...} and turns $${...} into a literal ${...}; the form text must never be evaluated.
const expand = (value: string): string => value.replace(/\$\$?\{[^}]*\}/g, (match) => (match.startsWith('$$') ? match.slice(1) : '<RESOLVED>'));

it.each([
  '', 'debug', '  padded  ', '${resources.env.KEY}', '$${already}', 'a ${x} b $${y} c', '$', '${', '$${', '}{', 'multi\nline', '$$$${x}', '${a}${b}',
])('escapes %j so the Score expander gives the text back and nothing resolves', (text) => {
  const stored = escapeLiteral(text);
  expect(hasUnescapedPlaceholder(stored)).toBe(false);
  expect(expand(stored)).toBe(text);
  expect(unescapeLiteral(stored)).toBe(text);
});

it('classifies Score variables into references, literals and unrepresentable values', () => {
  const resources = { env: { type: 'environment' }, svc_a_http: { type: 'service', params: { workload: 'a', port: 'http' } }, db: { type: 'postgres' } };
  expect(readBinding('A', '${resources.env.KEY}', resources)).toMatchObject({ source: 'KEY', key: 'KEY' });
  expect(readBinding('B', '${resources.db.host}', resources)).toMatchObject({ source: 'RESOURCE', alias: 'db', output: 'host' });
  expect(readBinding('C', '${resources.svc_a_http.url}', resources)).toMatchObject({ source: 'SERVICE', alias: 'svc_a_http', workload: 'a', port: 'http' });
  expect(readBinding('D', '', resources)).toMatchObject({ source: 'LITERAL', value: '' });
  expect(readBinding('E', 'x $${y}', resources)).toMatchObject({ source: 'LITERAL', value: 'x ${y}' });
  expect(readBinding('F', 'https://${resources.env.KEY}/x', resources)).toBeUndefined();
  expect(readBinding('G', '${resources.env.}', resources)).toBeUndefined();
  expect(readBinding('H', '${context.env.namespace}', resources)).toBeUndefined();
  expect(readBinding('I', 5, resources)).toBeUndefined();
});
