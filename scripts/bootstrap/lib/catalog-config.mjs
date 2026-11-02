import { isDeepStrictEqual } from 'node:util';
const object = value => value !== null && typeof value === 'object' && !Array.isArray(value);
const requireValue = (ok, message) => { if (!ok) throw new Error(message); };
const only = (value, keys, path) => requireValue(object(value) && Object.keys(value).every(key => keys.includes(key)), `${path}: trường không được hỗ trợ`);
import { gitURL } from './git-access-config.mjs';
export function parseCatalogConfig(raw, options = {}) {
 if (raw.resourceDefinitions !== undefined || raw.bundles !== undefined) throw Error('Definitions và templates phải nằm trong Git Catalog');
 const catalog = raw.catalog;
 if (!catalog || typeof catalog !== 'object' || Array.isArray(catalog)) throw Error('Cần catalog source; Definitions được quản lý trong Git');
 only(catalog, ['id','repository','branch','tagPattern','manifestPath','includePrereleases','accountBindings'], 'catalog');
 const id = catalog.id;
 if (!/^[a-z0-9]+(-[a-z0-9]+)*$/.test(id ?? '')) throw Error('catalog.id không hợp lệ');
 const repository = gitURL(options['git-url'] ?? catalog.repository);
 if (new URL(repository).hostname === 'git.example.com') throw Error('Đặt repository Git thật bằng --git-url hoặc catalog.repository');
 if (options['git-ref'] !== undefined) throw Error('Dùng catalog.branch và tag SemVer thay cho --git-ref');
 const branch = catalog.branch ?? 'main';
 if (!/^[A-Za-z0-9][A-Za-z0-9._/-]{0,199}$/.test(branch) || /\.\.|\/\/|[/.]$/.test(branch)) throw Error('catalog.branch không hợp lệ');
 const path = catalog.manifestPath ?? 'catalog.yaml';
 if (typeof path !== 'string' || !path || path.startsWith('/') || path.includes('\\') || path.split('/').some(part => !part || ['..','.','.git'].includes(part))) throw Error('catalog.manifestPath phải là đường dẫn tương đối');
 if (catalog.accountBindings !== undefined && (!object(catalog.accountBindings) || Object.values(catalog.accountBindings).some(value => typeof value !== 'string' || !value))) throw Error('catalog.accountBindings không hợp lệ');
 return { id, repository, branch, tagPattern: catalog.tagPattern ?? 'v*', manifestPath: catalog.manifestPath ?? 'catalog.yaml', includePrereleases: catalog.includePrereleases ?? false, accountBindings: catalog.accountBindings ?? {} };
}

export function parseResourceTypes(items) {
  requireValue(Array.isArray(items), 'resourceTypes phải là danh sách');
  const ids = new Set();
  return items.map(item => {
    only(item, ['key', 'inputs', 'outputs'], 'resourceType');
    requireValue(typeof item.key === 'string' && /^[a-z0-9-]+$/.test(item.key) && !['environment', 'service'].includes(item.key) && !ids.has(item.key), 'Resource Type key không hợp lệ, reserved hoặc trùng');
    ids.add(item.key);
    const schema = (kind, secret) => {
      const values = item[kind] ?? [];
      requireValue(Array.isArray(values), `${item.key}.${kind} phải là danh sách`);
      const names = new Set();
      return values.map(field => {
        only(field, ['name', 'type', 'required', ...(secret ? ['secret'] : [])], `${item.key}.${kind}`);
        requireValue(typeof field.name === 'string' && field.name.trim() === field.name && field.name && !names.has(field.name), `${item.key}.${kind}: tên field rỗng/trùng/có khoảng trắng thừa`);
        names.add(field.name);
        requireValue(['string', 'number', 'bool', 'any'].includes(field.type), `${item.key}.${kind}: kiểu field không hỗ trợ`);
        for (const flag of ['required', ...(secret ? ['secret'] : [])]) requireValue(field[flag] === undefined || typeof field[flag] === 'boolean', `${flag} phải là boolean`);
        return { name: field.name, type: field.type, required: field.required ?? false, ...(secret ? { secret: field.secret ?? false } : {}) };
      });
    };
    return { key: item.key, inputs: schema('inputs', false), outputs: schema('outputs', true) };
  });
}

export function assertSameResourceType(actual, desired) {
  // Missing false flags and null/omitted empty schemas are normal API projections.
  // Field order has no semantic effect. Ignore server-owned classes metadata.
  const normalize = item => ({ key: item.key, ...Object.fromEntries(['inputs', 'outputs'].map(kind => [kind,
    (item[kind] ?? []).map(field => ({ name: field.name, type: field.type, required: field.required === true, ...(kind === 'outputs' ? { secret: field.secret === true } : {}) })).sort((a, b) => a.name.localeCompare(b.name)),
  ])) });
  requireValue(isDeepStrictEqual(normalize(actual), normalize(desired)), `${desired.key}: schema Resource Type đã có khác cấu hình; script không ghi đè. `);
}
