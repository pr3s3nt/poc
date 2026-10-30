const object = value => value !== null && typeof value === 'object' && !Array.isArray(value);
const check = (ok, message) => { if (!ok) throw new Error(message); };
const only = (value, keys, label) => check(object(value) && Object.keys(value).every(key => keys.includes(key)), `${label}: trường không hỗ trợ`);
const text = value => typeof value === 'string' && value.trim() === value && value.length > 0;
const id = value => text(value) && /^[a-z0-9-]+$/.test(value);
const criteriaFields = ['env_type', 'app_id', 'env_id', 'res_id', 'class'];

export function parseInfrastructure(raw) {
  let account;
  if (raw.account !== undefined) {
    only(raw.account, ['name', 'endpoint', 'context', 'kubeconfigFile', 'discover'], 'account');
    check(text(raw.account.name), 'account cần name');
    check(raw.account.discover === undefined || raw.account.discover === true, 'discover phải là true');
    if (raw.account.discover) {
      check(raw.account.context === undefined && raw.account.endpoint === undefined, 'discover không đi kèm context/endpoint cố định');
    } else {
      check(text(raw.account.context), 'account cần context');
      let url; try { url = new URL(raw.account.endpoint); } catch { throw new Error('account.endpoint không hợp lệ'); }
      check(url.protocol === 'https:' && !url.username && !url.password && !url.search && !url.hash, 'account.endpoint phải HTTPS không credential');
    }
    check(raw.account.kubeconfigFile === undefined || text(raw.account.kubeconfigFile), 'kubeconfigFile phải là đường dẫn');
    account = { ...raw.account };
  }
  const definitions = raw.resourceDefinitions ?? [];
  check(Array.isArray(definitions), 'resourceDefinitions phải là danh sách');
  const keys = new Set();
  const resourceDefinitions = definitions.map(item => {
    only(item, ['key', 'name', 'resourceType', 'driverType', 'accountName', 'module', 'source', 'clusterClass', 'inputMapping', 'outputMapping', 'variables', 'criteria'], 'resourceDefinition');
    check(id(item.key) && !keys.has(item.key) && text(item.name) && item.name.length <= 253 && id(item.resourceType), 'Definition key/name/resourceType không hợp lệ hoặc trùng');
    keys.add(item.key);
    check(['terraform', 'existing-cluster'].includes(item.driverType), 'Chỉ hỗ trợ terraform/existing-cluster');
    const criteria = item.criteria ?? [{}];
    check(Array.isArray(criteria) && criteria.length, 'Definition cần criteria không rỗng');
    for (const row of criteria) { only(row, criteriaFields, 'criteria'); check(Object.values(row).every(text), 'criteria cần chuỗi không rỗng'); }
    if (item.driverType === 'existing-cluster') {
      check(item.resourceType === 'k8s-cluster' && text(item.accountName), 'existing-cluster cần accountName và Type k8s-cluster');
      check(['module', 'source', 'clusterClass', 'inputMapping', 'outputMapping', 'variables'].every(key => item[key] === undefined), 'existing-cluster không nhận Terraform inputs');
      return { ...item, criteria };
    }
    check(item.accountName === undefined && text(item.clusterClass), 'Kubernetes Terraform cần clusterClass; không chọn account trực tiếp');
    check(Boolean(item.source) !== Boolean(item.module), 'Chọn đúng một source Git hoặc installed module');
    if (item.source) {
      only(item.source, ['url', 'path', 'ref'], 'module source');
      let url; try { url = new URL(item.source.url); } catch { throw new Error('module source.url không hợp lệ'); }
      check(url.protocol === 'https:' && !url.username && !url.password && !url.search && !url.hash, 'module Git phải HTTPS không credential');
      check(text(item.source.path) && !item.source.path.startsWith('/') && !item.source.path.split('/').includes('..') && /^[0-9a-f]{40}$/.test(item.source.ref), 'module Git cần relative path và full commit');
    } else check(text(item.module), 'Thiếu installed module');
    for (const key of ['inputMapping', 'outputMapping']) check(object(item[key]) && Object.values(item[key]).every(text), `${key} cần object ánh xạ tên trường`);
    const variables = item.variables ?? {};
    check(object(variables), `${item.key}.variables: cần object nguồn giá trị, ví dụ {"image":{"literal":"mysql:8"}}`);
    for (const [name, source] of Object.entries(variables)) {
      const path = `${item.key}.variables[${JSON.stringify(name)}]`;
      check(text(name), `${path}: tên biến không được rỗng hoặc có khoảng trắng thừa`);
      const kind = value => value === null ? 'null' : Array.isArray(value) ? 'array' : typeof value;
      check(object(source) && Object.keys(source).length === 1, `${path}: nhận ${kind(source)}; cần đúng một nguồn, ví dụ {"literal":"mysql:8"}, {"context":"env.namespace"} hoặc {"resource":{"type":"k8s-namespace","class":"default","scope":"environments.@app.@env","output":"name"}}`);
      if (Object.hasOwn(source, 'literal')) check(['string', 'number', 'boolean'].includes(typeof source.literal) && (typeof source.literal !== 'number' || Number.isFinite(source.literal)), `${path}.literal: nhận ${kind(source.literal)}; cần string/number/boolean, ví dụ {"literal":"mysql:8"}; không nhận object/array/null`);
      else if (Object.hasOwn(source, 'context')) check(['env.namespace', 'infra.resourceName'].includes(source.context), `${path}.context: chỉ nhận "env.namespace" hoặc "infra.resourceName"`);
      else {
        only(source, ['resource'], path);
        only(source.resource, ['type', 'class', 'scope', 'output'], `${path}.resource`);
        for (const field of ['type', 'class', 'output']) check(text(source.resource[field]), `${path}.resource.${field}: cần chuỗi không rỗng`);
        check(['environments.@app.@env', '@infra'].includes(source.resource.scope), `${path}.resource.scope: chỉ nhận "environments.@app.@env" hoặc "@infra"`);
      }
    }
    return { ...item, variables, criteria };
  });
  return { account, resourceDefinitions };
}

// Validate authored literals against inspected module metadata before UI typing
// can coerce their types. No submitted value is included in the error.
export function validateVariableLiterals(item, capability) {
  const types = { string: 'string', number: 'number', bool: 'boolean' };
  for (const [name, source] of Object.entries(item.variables ?? {})) {
    if (!Object.hasOwn(source, 'literal')) continue;
    const variable = capability.variables?.find(variable => variable.name === name);
    const expected = types[variable?.type];
    if (expected && typeof source.literal !== expected) throw Error(`${item.key}.variables[${JSON.stringify(name)}].literal: module yêu cầu ${expected}, seed khai báo ${typeof source.literal}; sửa kiểu giá trị trước khi chạy lại`);
  }
}

// Discovery is limited to a single inspected context; credentials never leave UI.
export function discoverAccountTarget(name, contexts) {
  check(Array.isArray(contexts) && contexts.length === 1, 'Preset cần kubeconfig chỉ có một context để tránh chọn nhầm cluster');
  const chosen = contexts[0];
  check(text(chosen?.name), 'Kubeconfig thiếu context');
  let endpoint; try { endpoint = new URL(chosen.endpoint); } catch { throw new Error('Kubeconfig endpoint không hợp lệ'); }
  check(endpoint.protocol === 'https:' && !endpoint.username && !endpoint.password && !endpoint.search && !endpoint.hash, 'Kubeconfig endpoint phải là HTTPS không credential');
  return { name, context: chosen.name, endpoint: chosen.endpoint };
}

export function assertAccountTarget(account, expected) {
  check(account.name === expected.name && account.type === 'INTERNAL' && account.authenticationType === 'KUBECONFIG' && account.status === 'READY' && account.config?.endpoint === expected.endpoint && account.config?.kubeContext === expected.context,
    `${expected.name}: account đã có nhưng chưa READY hoặc khác endpoint/context; không thay credential/target`);
}

export function desiredInfrastructureDefinition(item, capability, account) {
  const base = { key: item.key, name: item.name, resourceType: item.resourceType, driverType: item.driverType, provision: {}, criteria: item.criteria };
  if (item.driverType === 'existing-cluster') return { ...base, driverAccount: account.key, driverInputs: { values: { variables: {} } } };
  const pin = capability.source;
  check(pin && capability.resourceType === item.resourceType && capability.driverType === 'terraform', 'Module capability không hợp lệ');
  const variables = Object.fromEntries(Object.entries(item.variables).map(([name, source]) => [name,
    Object.hasOwn(source, 'literal') ? source.literal : source.context ? '${context.' + source.context + '}' : "${resources['" + source.resource.type + '.' + source.resource.class + '#' + source.resource.scope + "'].outputs." + source.resource.output + '}',
  ]));
  const bindings = Object.fromEntries(capability.providerRequirements.filter(provider => provider.binding === 'kubernetes').map(provider => [provider.name, { target: 'resource', resource_ref: `k8s-cluster.${item.clusterClass}#environments.@app.@env` }]));
  return { ...base, driverInputs: { values: { source: { source_id: pin.sourceId, module_id: pin.moduleId, revision: pin.revision, subdirectory: pin.subdirectory, content_sha256: pin.contentSha256 }, variables,
    input_mapping: Object.fromEntries(Object.entries(item.inputMapping).map(([param, variable]) => [variable, { param }])),
    output_mapping: Object.fromEntries(Object.entries(item.outputMapping).map(([output, moduleOutput]) => [moduleOutput, output])), provider_bindings: bindings } } };
}

export function readSetupPacing(options = {}, env = process.env) {
  const parse = (value, label) => { const n = Number(value); check(String(value).trim() !== '' && Number.isInteger(n) && n >= 0 && n <= 10000, `${label} phải là số nguyên 0..10000`); return n; };
  const base = parse(options['step-delay-ms'] ?? env.ORCH_STEP_DELAY_MS ?? '800', 'step-delay-ms');
  return Object.fromEntries(['account', 'type', 'definition', 'bundle'].map(phase => [phase, parse(options[`${phase}-delay-ms`] ?? env[`ORCH_${phase.toUpperCase()}_DELAY_MS`] ?? base, `${phase}-delay-ms`)]));
}
