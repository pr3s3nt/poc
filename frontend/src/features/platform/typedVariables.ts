import type { CapabilityVariable } from './capabilities';

// Typed authoring of Driver Input variables (UC-03 BR-24/BR-25). The typed form
// and the advanced JSON serialize to the same driverInputs.values.variables
// object. Omitted variables keep their default/param; a value is only sent when
// the author opts in. Whole placeholders stay strings for deferred checking.

export type VariableEntry = { on: boolean; raw: string; placeholder?: boolean };
export type VariableDraft = Record<string, VariableEntry>;
export type JsonObject = Record<string, unknown>;

const wholePlaceholder = /^\$\{[^{}]*\}$/;
// Exactly one whole placeholder like the backend: no surrounding whitespace is ignored.
export const isWholePlaceholder = (text: string) => wholePlaceholder.test(text);

export const baseKind = (type: string): 'string' | 'number' | 'bool' | 'list' | 'map' | 'any' => {
  if (type === 'string' || type === 'number' || type === 'bool') return type;
  if (type.startsWith('list(') || type.startsWith('set(')) return 'list';
  if (type.startsWith('map(')) return 'map';
  return 'any';
};

export const emptyVariableDraft = (): VariableDraft => ({});

export function parseObjectText(text: string, label: string): { value?: JsonObject; error?: string } {
  let parsed: unknown;
  try { parsed = JSON.parse(text); } catch { return { error: `${label} phải là JSON hợp lệ.` }; }
  if (parsed === null || Array.isArray(parsed) || typeof parsed !== 'object') return { error: `${label} phải là một JSON object.` };
  return { value: parsed as JsonObject };
}

// Value -> control text; undefined when the control cannot show the value without loss.
function entryFor(variable: CapabilityVariable, value: unknown): VariableEntry | undefined {
  const kind = baseKind(variable.type);
  if (value === null || value === undefined) return undefined;
  if (typeof value === 'string' && isWholePlaceholder(value)) {
    return kind === 'bool' ? { on: true, raw: value, placeholder: true } : { on: true, raw: value };
  }
  switch (kind) {
    case 'string': return typeof value === 'string' ? { on: true, raw: value } : undefined;
    case 'number': return typeof value === 'number' ? { on: true, raw: String(value) } : undefined;
    case 'bool': return typeof value === 'boolean' ? { on: true, raw: String(value) } : undefined;
    case 'list': return Array.isArray(value) ? { on: true, raw: JSON.stringify(value, null, 2) } : undefined;
    case 'map': return typeof value === 'object' && !Array.isArray(value) ? { on: true, raw: JSON.stringify(value, null, 2) } : undefined;
    // any is always edited as JSON, so every valid value (including plain strings) round-trips.
    default: return { on: true, raw: JSON.stringify(value, null, 2) };
  }
}

// Advanced JSON -> typed draft. Returns a reason in Vietnamese when typed controls
// cannot represent the object safely; the JSON then stays authoritative.
export function variablesToDraft(object: JsonObject, variables: CapabilityVariable[]): { draft?: VariableDraft; reason?: string } {
  const byName = new Map(variables.map((variable) => [variable.name, variable]));
  const draft: VariableDraft = {};
  for (const [name, value] of Object.entries(object)) {
    const variable = byName.get(name);
    if (!variable) return { reason: `Tham số "${name}" không có control typed (không được hỗ trợ, do hệ thống sở hữu hoặc là credential). Giữ trong JSON nâng cao.` };
    const entry = entryFor(variable, value);
    if (!entry) return { reason: `Giá trị của "${name}" không biểu diễn an toàn bằng control ${variable.type}. Giữ trong JSON nâng cao.` };
    draft[name] = entry;
  }
  return { draft };
}

export function entryValue(variable: CapabilityVariable, entry: VariableEntry): { value?: unknown; error?: string } {
  const kind = baseKind(variable.type);
  const text = entry.raw;
  if (isWholePlaceholder(text)) return { value: text };
  switch (kind) {
    case 'string': return { value: text };
    case 'number': {
      const trimmed = text.trim();
      const number = trimmed === '' ? Number.NaN : Number(trimmed);
      return Number.isFinite(number) ? { value: number } : { error: `"${variable.name}" phải là số hợp lệ hoặc một placeholder nguyên vẹn dạng \${...}.` };
    }
    case 'bool':
      if (text === 'true') return { value: true };
      if (text === 'false') return { value: false };
      return { error: `"${variable.name}" phải là true, false hoặc một placeholder nguyên vẹn dạng \${...}.` };
    default: {
      let parsed: unknown;
      try { parsed = JSON.parse(text); } catch { return { error: `"${variable.name}" phải là JSON hợp lệ (${variable.type}) hoặc một placeholder nguyên vẹn dạng \${...}.` }; }
      if (parsed === null) return { error: `"${variable.name}" không được là null.` };
      if (kind === 'list' && !Array.isArray(parsed)) return { error: `"${variable.name}" phải là danh sách JSON (${variable.type}).` };
      if (kind === 'map' && (typeof parsed !== 'object' || Array.isArray(parsed))) return { error: `"${variable.name}" phải là JSON object (${variable.type}).` };
      return { value: parsed };
    }
  }
}

// Typed draft -> variables object. Only opted-in entries are sent.
export function draftToVariables(draft: VariableDraft, variables: CapabilityVariable[]): { value?: JsonObject; problem?: string } {
  const out: JsonObject = {};
  // Never drop an opted-in value silently if the metadata no longer lists it.
  const known = new Set(variables.map((variable) => variable.name));
  const orphan = Object.entries(draft).find(([name, entry]) => entry.on && !known.has(name));
  if (orphan) return { problem: `Tham số "${orphan[0]}" không còn trong metadata của cách tạo này. Tải lại metadata hoặc bỏ ghi đè tham số này; giá trị không bị bỏ âm thầm.` };
  for (const variable of variables) {
    const entry = draft[variable.name];
    if (!entry?.on) continue;
    const result = entryValue(variable, entry);
    if (result.error) return { problem: result.error };
    out[variable.name] = result.value;
  }
  return { value: out };
}

// A variable that needs a value from the author: required, no default and no
// Resource Type param supplies it. Guidance only; the server stays authoritative.
export function missingRequired(draft: VariableDraft, variables: CapabilityVariable[]): CapabilityVariable[] {
  return variables.filter((variable) => variable.required && !variable.hasDefault && !variable.resourceParam && !draft[variable.name]?.on);
}
