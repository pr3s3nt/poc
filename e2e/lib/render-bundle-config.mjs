import { isDeepStrictEqual } from 'node:util';

const object = value => value !== null && typeof value === 'object' && !Array.isArray(value);
const requireValue = (ok, message) => { if (!ok) throw new Error(message); };
const fields = ['env_type', 'app_id', 'env_id', 'res_id', 'class'];
const only = (value, keys, path) => requireValue(object(value) && Object.keys(value).every(key => keys.includes(key)), `${path}: trường không được hỗ trợ`);

export function parseBundleConfig(raw) {
  only(raw, ['bundles', 'verify'], 'config');
  requireValue(Array.isArray(raw.bundles) && raw.bundles.length > 0, 'bundles phải là danh sách không rỗng');
  const ids = new Set();
  const bundles = raw.bundles.map(item => {
    only(item, ['key', 'name', 'source', 'placement', 'renderInputs', 'provision', 'criteria', 'expectedTemplates'], 'bundle');
    requireValue(typeof item.key === 'string' && /^[a-z0-9-]+$/.test(item.key) && !ids.has(item.key), 'key không hợp lệ hoặc trùng');
    ids.add(item.key);
    requireValue(typeof item.name === 'string' && item.name.trim() && item.name.length <= 253, `${item.key}: thiếu name hợp lệ`);
    only(item.source, ['url', 'path', 'ref'], 'source');
    let url;
    try { url = new URL(item.source.url); } catch { throw new Error(`${item.key}: source.url không hợp lệ`); }
    requireValue(url.protocol === 'https:' && !url.username && !url.password && !url.search && !url.hash, 'Git URL phải là HTTPS, không chứa credential/query/hash');
    requireValue(typeof item.source.path === 'string' && item.source.path && !item.source.path.startsWith('/') && !item.source.path.split('/').includes('..'), 'source.path phải tương đối trong repo');
    requireValue(typeof item.source.ref === 'string' && item.source.ref.trim(), 'Thiếu source.ref (tag/commit/branch)');
    only(item.placement, ['namespace', 'cluster'], 'placement');
    for (const field of ['namespace', 'cluster']) requireValue(typeof item.placement[field] === 'string' && item.placement[field].trim(), `Thiếu placement.${field}`);
    for (const field of ['renderInputs', 'provision']) requireValue(item[field] === undefined || object(item[field]), `${field} phải là object`);
    const criteria = item.criteria ?? [{}];
    requireValue(Array.isArray(criteria) && criteria.length > 0, 'criteria phải có ít nhất một điều kiện; [{}] là wildcard');
    for (const row of criteria) {
      only(row, fields, 'criteria');
      requireValue(Object.values(row).every(value => typeof value === 'string' && value.trim()), 'criteria chỉ nhận chuỗi không rỗng; bỏ field không dùng');
    }
    const expectedTemplates = item.expectedTemplates ?? [];
    requireValue(Array.isArray(expectedTemplates) && expectedTemplates.every(pin => object(pin) && Object.keys(pin).every(key => ['id', 'version'].includes(key)) && typeof pin.id === 'string' && pin.id && typeof pin.version === 'string' && pin.version), 'expectedTemplates phải chứa {id, version}');
    return { ...item, renderInputs: item.renderInputs ?? {}, provision: item.provision ?? {}, criteria, expectedTemplates };
  });
  const verify = raw.verify ?? {};
  only(verify, ['applicationKey', 'environment'], 'verify');
  requireValue(verify.applicationKey === undefined || typeof verify.applicationKey === 'string' && verify.applicationKey, 'verify.applicationKey không hợp lệ');
  requireValue(verify.environment === undefined || ['staging', 'production'].includes(verify.environment), 'verify.environment phải là staging/production');
  return { bundles, verify: { ...verify, environment: verify.environment ?? 'staging' } };
}

export function desiredDefinition(item, bundleId) {
  return { key: item.key, name: item.name, resourceType: 'workload', driverType: 'score-k8s',
    driverInputs: { values: { variables: { render_bundle: bundleId, namespace: item.placement.namespace, cluster: item.placement.cluster, render_inputs: item.renderInputs } } },
    provision: item.provision, criteria: item.criteria };
}

export function assertSameDefinition(actual, desired) {
  const criteria = rows => rows.map(row => Object.fromEntries(fields.filter(key => row[key] !== undefined).map(key => [key, row[key]]))).sort((a, b) => JSON.stringify(a).localeCompare(JSON.stringify(b)));
  for (const field of ['key', 'name', 'resourceType', 'driverType', 'driverInputs', 'provision', 'criteria']) {
    let left = actual[field], right = desired[field];
    if (field === 'provision') { left ??= {}; right ??= {}; }
    if (field === 'criteria') { left = criteria(left ?? []); right = criteria(right ?? []); }
    requireValue(isDeepStrictEqual(left, right), `${desired.key}: cấu hình đã có khác ${field}; script không ghi đè. Kiểm tra trên Console hoặc dùng ID mới.`);
  }
}
