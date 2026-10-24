import type { JsonObject } from './typedVariables';

// Typed render_inputs of a Git render bundle (ADR-021, contract RB-C03). The
// bundle's closed JSON Schema subset drives the form; the same rules as the
// server apply: defaults only for absent keys (false, 0 and "" are explicit
// values), no coercion, no unknown keys and no `${` references. The server
// remains the authority; this mirror only gives early, field-level feedback.

export type InputType = 'string' | 'boolean' | 'integer' | 'number' | 'object' | 'array';
export type InputField = {
  name: string; type: InputType; title: string; description?: string; required: boolean;
  hasDefault: boolean; defaultValue?: unknown; enumValues?: unknown[]; schema: JsonObject;
};
export type InputProblem = 'type' | 'range' | 'length' | 'pattern' | 'enum' | 'required' | 'reference' | 'unknown';

const isObject = (value: unknown): value is JsonObject => typeof value === 'object' && value !== null && !Array.isArray(value);
const types: readonly string[] = ['string', 'boolean', 'integer', 'number', 'object', 'array'];

/** Top-level input fields of a bundle schema, in schema order. */
export function inputFields(schema: JsonObject | undefined): InputField[] {
  const properties = isObject(schema?.properties) ? schema.properties : {};
  const required = Array.isArray(schema?.required) ? schema.required.filter((item): item is string => typeof item === 'string') : [];
  const out: InputField[] = [];
  for (const [name, raw] of Object.entries(properties)) {
    if (!isObject(raw) || typeof raw.type !== 'string' || !types.includes(raw.type)) continue;
    out.push({
      name, type: raw.type as InputType, title: typeof raw.title === 'string' && raw.title !== '' ? raw.title : name,
      ...(typeof raw.description === 'string' ? { description: raw.description } : {}),
      required: required.includes(name), hasDefault: Object.prototype.hasOwnProperty.call(raw, 'default'),
      ...(Object.prototype.hasOwnProperty.call(raw, 'default') ? { defaultValue: raw.default } : {}),
      ...(Array.isArray(raw.enum) ? { enumValues: raw.enum } : {}), schema: raw,
    });
  }
  return out;
}

/** Keys of supplied inputs the schema does not declare (shown, never silently dropped). */
export function unknownInputs(schema: JsonObject | undefined, inputs: JsonObject): string[] {
  const properties = isObject(schema?.properties) ? schema.properties : {};
  return Object.keys(inputs).filter((key) => !Object.prototype.hasOwnProperty.call(properties, key)).sort();
}

const same = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b);

/** First problem of one present value against its schema, or undefined. */
export function checkValue(schema: JsonObject, value: unknown): InputProblem | undefined {
  switch (schema.type) {
    case 'string': {
      if (typeof value !== 'string') return 'type';
      const length = [...value].length;
      if (typeof schema.minLength === 'number' && length < schema.minLength) return 'length';
      if (typeof schema.maxLength === 'number' && length > schema.maxLength) return 'length';
      if (typeof schema.pattern === 'string') {
        try { if (!new RegExp(schema.pattern, 'u').test(value)) return 'pattern'; } catch { return 'pattern'; }
      }
      if (value.includes('${')) return 'reference';
      break;
    }
    case 'boolean': if (typeof value !== 'boolean') return 'type'; break;
    case 'integer':
    case 'number': {
      if (typeof value !== 'number' || !Number.isFinite(value) || (schema.type === 'integer' && !Number.isInteger(value))) return 'type';
      if (typeof schema.minimum === 'number' && value < schema.minimum) return 'range';
      if (typeof schema.maximum === 'number' && value > schema.maximum) return 'range';
      break;
    }
    case 'array': {
      if (!Array.isArray(value)) return 'type';
      if (typeof schema.minItems === 'number' && value.length < schema.minItems) return 'length';
      if (typeof schema.maxItems === 'number' && value.length > schema.maxItems) return 'length';
      const items = isObject(schema.items) ? schema.items : {};
      for (const item of value) { const problem = checkValue(items, item); if (problem) return problem; }
      break;
    }
    case 'object': {
      if (!isObject(value)) return 'type';
      const properties = isObject(schema.properties) ? schema.properties : {};
      for (const key of Object.keys(value)) if (!Object.prototype.hasOwnProperty.call(properties, key)) return 'unknown';
      for (const [key, child] of Object.entries(properties)) {
        if (!isObject(child)) continue;
        if (Object.prototype.hasOwnProperty.call(value, key)) { const problem = checkValue(child, value[key]); if (problem) return problem; }
      }
      const required = Array.isArray(schema.required) ? schema.required : [];
      for (const key of required) {
        const child = typeof key === 'string' && isObject(properties[key]) ? properties[key] : undefined;
        if (typeof key === 'string' && !Object.prototype.hasOwnProperty.call(value, key) && !(child && Object.prototype.hasOwnProperty.call(child, 'default'))) return 'required';
      }
      break;
    }
    default: return 'type';
  }
  if (Array.isArray(schema.enum) && !schema.enum.some((candidate) => same(candidate, value))) return 'enum';
  return undefined;
}

/** Field problems of supplied inputs; an absent required field without a default is `required`. */
export function inputProblems(schema: JsonObject | undefined, inputs: JsonObject): Record<string, InputProblem> {
  const out: Record<string, InputProblem> = {};
  for (const field of inputFields(schema)) {
    if (Object.prototype.hasOwnProperty.call(inputs, field.name)) {
      const problem = checkValue(field.schema, inputs[field.name]);
      if (problem) out[field.name] = problem;
    } else if (field.required && !field.hasDefault) out[field.name] = 'required';
  }
  for (const key of unknownInputs(schema, inputs)) out[key] = 'unknown';
  return out;
}

/** Display text of a default or enum value. */
export function displayValue(value: unknown): string {
  return typeof value === 'string' ? value : JSON.stringify(value);
}
