import { describe, expect, it } from 'vitest';
import { emptyBinding, type RowContext } from './bindings';
import { mergeEnvfile, parseEnvfile, serializeEnvfile } from './envfile';
const context: RowContext = { catalog: { status: 'ready', version: 1, keys: [{ name: 'REGION', kind: 'VARIABLE', configured: true, usedBy: [] }, { name: 'TOKEN', kind: 'SECRET', configured: true, usedBy: [] }] }, dependencies: [{ alias: 'db', type: 'postgres' }], outputs: alias => alias === 'db' ? ['host', 'password'] : undefined, services: [{ id: 'backend', ports: ['http'] }] };
describe('envfile', () => {
  it('round trips exact literals without interpolation or quote-dependent source changes', () => {
    const values = ['', ' a ', 'a=b', 'a # b', '@db.host', '@@x', '\\n', 'a\nb\r\nc\t"\\', '${TOKEN}', '$VAR', 'Tiếng Việt 🐈'];
    const rows = values.map((value, i) => ({ ...emptyBinding(), name: `KEY_${i}`, value }));
    const parsed = parseEnvfile(serializeEnvfile(rows, context), context);
    expect(parsed.problems).toEqual([]);
    expect(parsed.bindings.map(row => row.value)).toEqual(values);
    expect(parseEnvfile('X="@db.host"', context).bindings[0]?.source).toBe('RESOURCE');
  });
  it('keeps all source types and Service alias identity', () => {
    const previous = [{ ...emptyBinding('SERVICE'), name: 'URL', alias: 'old_service', workload: 'backend', port: 'http' }];
    const parsed = parseEnvfile('REGION=@variable.REGION\nTOKEN=@secret.TOKEN\nHOST=@db.host\nURL=@service.backend.http', context, previous);
    expect(parsed.problems).toEqual([]);
    expect(parsed.bindings.map(row => row.source)).toEqual(['VARIABLE', 'SECRET', 'RESOURCE', 'SERVICE']);
    expect(parsed.bindings[3]?.alias).toBe('old_service');
    expect(parseEnvfile(serializeEnvfile(parsed.bindings, context), context).problems).toEqual([]);
  });
  it('accepts comments, CRLF, quotes, multiline, equals and escaped @', () => {
    const result = parseEnvfile("# comment\r\nA = a=b # c\r\nB='a\r\nb'\r\nC=@@team.example\r\nD=\n", context);
    expect(result.problems).toEqual([]);
    expect(result.bindings.map(row => row.value)).toEqual(['a=b', 'a\r\nb', '@team.example', '']);
  });
  it('scopes a marker to exactly the following declaration', () => {
    const rows = parseEnvfile('# @secret\nA=one\nB=two', context);
    expect(rows.problems).toEqual([]); expect(rows.bindings.map(row => !!row.secretDraft)).toEqual([true, false]);
    for (const text of ['# @secret', '# @secret\n\nA=x', '# @secret\n# comment\nA=x', '# @secret\n# @secret\nA=x', '# @secret\nA=@secret.TOKEN']) expect(parseEnvfile(text, context).problems.some(p => p.code === 'marker')).toBe(true);
  });
  it('rejects duplicates, malformed references, invalid targets, escapes and quotes with positions', () => {
    expect(parseEnvfile('A=1\nA=2', context).problems).toContainEqual({ line: 2, code: 'duplicate', first: 1 });
    for (const text of ['A=@missing.host', 'A=@db.missing', 'A=@secret.REGION', 'A=@service.backend.missing', 'A=@db.host/suffix', 'A="bad\\q"', 'A="missing', 'A="ok" junk', 'bad line', 'A=\u0000']) expect(parseEnvfile(text, context).problems.length).toBeGreaterThan(0);
  });
  it('merges with explicit replacement and never removes absent rows', () => {
    const current = parseEnvfile('A=old\nB=keep', context).bindings;
    const incoming = parseEnvfile('A=new\nC=append', context).bindings;
    expect(mergeEnvfile(current, incoming, new Set()).map(row => row.value)).toEqual(['old', 'keep', 'append']);
    const result = mergeEnvfile(current, incoming, new Set(['A']));
    expect(result.map(row => row.value)).toEqual(['new', 'keep', 'append']); expect(result[0]?.id).toBe(current[0]?.id);
  });
});
