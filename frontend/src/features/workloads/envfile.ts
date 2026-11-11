import { emptyBinding, rowProblem, sourceOf, type Binding, type RowContext } from './bindings';

export type EnvfileProblem = { line: number; code: string; first?: number };
export type EnvfileResult = { bindings: Binding[]; lines: number[]; problems: EnvfileProblem[] };
export const ENVFILE_NAMESPACES = ['secret', 'variable', 'service'];

// This grammar is deliberately independent of dotenv/shell interpolation. Errors
// contain positions/codes only: submitted values never enter error messages.
export function parseEnvfile(text: string, context: RowContext, previous: Binding[] = []): EnvfileResult {
  const input = text.replace(/^\uFEFF/, '');
  const bindings: Binding[] = [], lines: number[] = [], problems: EnvfileProblem[] = [];
  const seen = new Map<string, number>();
  let pos = 0, line = 1, marker: number | undefined;
  const fail = (at: number, code: string, first?: number) => problems.push({ line: at, code, first });
  const advance = () => { if (input[pos] === '\n') line++; pos++; };
  const skipLine = () => { while (pos < input.length && input[pos] !== '\n') advance(); if (pos < input.length) advance(); };
  while (pos < input.length) {
    const start = line;
    const end = input.indexOf('\n', pos);
    const rawLine = input.slice(pos, end < 0 ? input.length : end).replace(/\r$/, '');
    if (rawLine.trim() === '# @secret') {
      if (marker !== undefined) fail(marker, 'marker');
      marker = start; skipLine(); continue;
    }
    if (!rawLine.trim() || rawLine.trimStart().startsWith('#')) {
      if (marker !== undefined) { fail(marker, 'marker'); marker = undefined; }
      skipLine(); continue;
    }
    const declaration = rawLine.match(/^[ \t]*([A-Za-z_][A-Za-z0-9_]*)[ \t]*=[ \t]*/);
    if (!declaration) { fail(start, 'syntax'); if (marker !== undefined) fail(marker, 'marker'); marker = undefined; skipLine(); continue; }
    const name = declaration[1]!;
    pos += declaration[0].length;
    let value = '', bad = false;
    const quote = input[pos];
    if (quote === '"' || quote === "'") {
      advance(); let closed = false;
      while (pos < input.length) {
        const ch = input[pos]!;
        if (ch === quote) { advance(); closed = true; break; }
        if (quote === '"' && ch === '\\') {
          advance(); const escaped = input[pos];
          const replacements: Record<string, string> = { n: '\n', r: '\r', t: '\t', '\\': '\\', '"': '"' };
          if (escaped === undefined || !(escaped in replacements)) { bad = true; fail(line, 'escape'); }
          else value += replacements[escaped];
          if (pos < input.length) advance();
        } else { value += ch; advance(); }
      }
      if (!closed) { fail(start, 'quote'); bad = true; }
      const tailEnd = input.indexOf('\n', pos);
      const tail = input.slice(pos, tailEnd < 0 ? input.length : tailEnd).trim();
      if (tail && !tail.startsWith('#')) { fail(line, 'syntax'); bad = true; }
      skipLine();
    } else {
      const valueEnd = input.indexOf('\n', pos);
      value = input.slice(pos, valueEnd < 0 ? input.length : valueEnd).replace(/\r$/, '').replace(/(^|[ \t])#.*$/, '').trim();
      skipLine();
    }
    const first = seen.get(name);
    if (first !== undefined) { fail(start, 'duplicate', first); bad = true; }
    seen.set(name, start);
    let binding: Binding = { ...emptyBinding(), name, value };
    if (value.startsWith('@@')) binding.value = value.slice(1);
    else if (value.startsWith('@')) {
      let match: RegExpMatchArray | null;
      if ((match = value.match(/^@(variable|secret)\.([A-Za-z_][A-Za-z0-9_]*)$/))) {
        binding = { ...binding, source: match[1] === 'secret' ? 'SECRET' : 'VARIABLE', key: match[2]!, value: '' };
      } else if ((match = value.match(/^@service\.([A-Za-z0-9_-]+)\.([A-Za-z0-9_-]+)$/))) {
        const old = previous.find(row => row.name === name && row.source === 'SERVICE' && row.workload === match![1] && row.port === match![2]);
        binding = { ...binding, source: 'SERVICE', workload: match[1]!, port: match[2]!, alias: old?.alias ?? '', value: '' };
      } else if ((match = value.match(/^@([A-Za-z0-9_-]+)\.([A-Za-z0-9_-]+)$/)) && !ENVFILE_NAMESPACES.includes(match[1]!)) {
        binding = { ...binding, source: 'RESOURCE', alias: match[1]!, output: match[2]!, value: '' };
      } else { fail(start, 'reference'); bad = true; }
    }
    if (marker !== undefined) {
      if (binding.source !== 'LITERAL' || bad) fail(marker, 'marker');
      else binding.secretDraft = true;
      marker = undefined;
    }
    const old = previous.find(row => row.name === name);
    if (old) { binding.id = old.id; if (binding.secretDraft && old.secretUncertain && binding.value === old.value) binding.secretUncertain = true; }
    if (!bad) { bindings.push(binding); lines.push(start); }
  }
  if (marker !== undefined) fail(marker, 'marker');
  bindings.forEach((binding, index) => {
    const issue = rowProblem(binding, index + 1, '', bindings, context);
    if (issue) fail(lines[index]!, 'binding');
    if (binding.source === 'RESOURCE' && (!context.outputs(binding.alias) || ENVFILE_NAMESPACES.includes(binding.alias))) fail(lines[index]!, 'reference');
    if ((binding.source === 'SECRET' || binding.source === 'VARIABLE') && context.catalog.status !== 'ready') fail(lines[index]!, 'metadata');
  });
  return { bindings, lines, problems };
}

function quoteValue(value: string): string {
  return '"' + value.replace(/\\/g, '\\\\').replace(/"/g, '\\"').replace(/\n/g, '\\n').replace(/\r/g, '\\r').replace(/\t/g, '\\t') + '"';
}

export function serializeEnvfile(bindings: Binding[], context: RowContext): string {
  return bindings.map(binding => {
    const source = sourceOf(binding, context.catalog);
    if (binding.source === 'KEY' && context.catalog.status !== 'ready') throw new Error('metadata');
    let value: string;
    if (source === 'LITERAL') value = binding.value.startsWith('@') ? `@${binding.value}` : binding.value;
    else if (source === 'SECRET' || source === 'VARIABLE') value = `@${source === 'SECRET' ? 'secret' : 'variable'}.${binding.key}`;
    else if (source === 'SERVICE') value = `@service.${binding.workload}.${binding.port}`;
    else {
      if (ENVFILE_NAMESPACES.includes(binding.alias)) throw new Error('reserved');
      value = `@${binding.alias}.${binding.output}`;
    }
    return `${source === 'LITERAL' && binding.secretDraft ? '# @secret\n' : ''}${binding.name}=${quoteValue(value)}`;
  }).join('\n');
}

// Merge never deletes absent keys. Replacing preserves position and row identity.
export function mergeEnvfile(current: Binding[], incoming: Binding[], replace: Set<string>): Binding[] {
  const result = current.map(row => {
    const next = incoming.find(item => item.name === row.name);
    return next && replace.has(row.name) ? { ...next, id: row.id } : row;
  });
  return [...result, ...incoming.filter(row => !current.some(item => item.name === row.name))];
}
