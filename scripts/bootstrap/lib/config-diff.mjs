import { isDeepStrictEqual } from 'node:util';

// Values belong to non-secret Definition configuration. Still redact sensitive
// field names and credential-bearing URLs before including a value in errors.
function display(value, path, missing = false) {
  if (missing) return '<không khai báo>';
  if (/password|token|credential|secret|private.?key/i.test(path)) return '<ẩn>';
  const clean = (v, field) => {
    if (/password|token|credential|secret|private.?key/i.test(field)) return '<ẩn>';
    if (v && typeof v === 'object') return Array.isArray(v) ? v.map(item => clean(item, field)) : Object.fromEntries(Object.entries(v).map(([key, item]) => [key, clean(item, key)]));
    return v;
  };
  const encoded = JSON.stringify(clean(value, path)) ?? String(value);
  if (/https?:\/\/[^/\s"\\]*@/i.test(encoded)) return '<ẩn URL có credential>';
  return encoded.length > 180 ? encoded.slice(0, 177) + '...' : encoded;
}
export function differences(actual, desired, path = '', limit = 12) {
  const result = [];
  function walk(a, b, field, hasA = true, hasB = true) {
    if (result.length >= limit || hasA && hasB && isDeepStrictEqual(a, b)) return;
    const objects = hasA && hasB && a !== null && b !== null && typeof a === 'object' && typeof b === 'object' && !Array.isArray(a) && !Array.isArray(b);
    if (objects) {
      for (const key of [...new Set([...Object.keys(a), ...Object.keys(b)])].sort()) walk(a[key], b[key], field ? `${field}[${JSON.stringify(key)}]` : key, Object.hasOwn(a, key), Object.hasOwn(b, key));
    } else result.push(`${field}: Console=${display(a, field, !hasA)}; seed=${display(b, field, !hasB)}`);
  }
  walk(actual, desired, path);
  return result.join('; ');
}

// Preserve only Console-owned variables. Seed-owned entries remain strict and
// are compared by assertSameDefinition, so explicit conflicting values fail.
export function preserveConsoleVariables(existing, desired, enabled, warn = () => {}) {
  if (!enabled || !existing || !['terraform', 'score-k8s'].includes(desired.driverType)) return desired;
  const current = existing.driverInputs?.values?.variables ?? {};
  const incoming = desired.driverInputs?.values?.variables ?? {};
  for (const [label, value] of [['Console', current], ['seed', incoming]]) {
    if (!value || typeof value !== 'object' || Array.isArray(value)) throw Error(`${desired.key}: driverInputs.values.variables của ${label} phải là object`);
  }
  const extras = Object.keys(current).filter(name => !Object.hasOwn(incoming, name));
  if (!extras.length) return desired;
  const result = structuredClone(desired);
  result.driverInputs.values.variables = { ...structuredClone(current), ...structuredClone(incoming) };
  for (const name of extras) warn(`WARN: ${desired.key}: giữ nguyên biến Console driverInputs.values.variables[${JSON.stringify(name)}]; seed không khai báo`);
  return result;
}
