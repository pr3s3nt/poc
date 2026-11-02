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
  if (raw.resourceDefinitions !== undefined) throw Error('Definitions phải nằm trong Git Catalog');
  return { account, resourceDefinitions: [] };
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

export function readSetupPacing(options = {}, env = process.env) {
  const parse = (value, label) => { const n = Number(value); check(String(value).trim() !== '' && Number.isInteger(n) && n >= 0 && n <= 10000, `${label} phải là số nguyên 0..10000`); return n; };
  const base = parse(options['step-delay-ms'] ?? env.ORCH_STEP_DELAY_MS ?? '800', 'step-delay-ms');
  return Object.fromEntries(['account', 'type', 'definition', 'bundle'].map(phase => [phase, parse(options[`${phase}-delay-ms`] ?? env[`ORCH_${phase.toUpperCase()}_DELAY_MS`] ?? base, `${phase}-delay-ms`)]));
}
