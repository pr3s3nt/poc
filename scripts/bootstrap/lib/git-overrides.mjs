// Rewrite source location only; keep module layout, contracts and selection rules.
const exportedModules = new Set(['k8s-namespace', 'postgres-k8s', 'redis-k8s', 'persistent-storage', 'mysql', 'vpc', 'eks', 'aurora', 'elasticache']);
export function applyGitOverrides(raw, options) {
  const result = structuredClone(raw);
  if (options['git-url'] !== undefined) {
    let url;
    try { url = new URL(options['git-url']); } catch { throw Error('--git-url phải là HTTPS Git URL'); }
    if (url.protocol !== 'https:' || url.username || url.password || url.search || url.hash) throw Error('--git-url phải HTTPS không chứa credential/query/hash');
    for (const definition of result.resourceDefinitions ?? []) {
      if (definition.driverType !== 'terraform') continue;
      if (definition.module) {
        if (!exportedModules.has(definition.module)) throw Error(`${definition.key}: module chưa có trong platform-catalog; không tự suy đường dẫn`);
        if (!options['git-ref']) throw Error('--git-url cần --git-ref khi chuyển installed modules sang Git công ty');
        definition.source = { url: options['git-url'], path: `backend/terraform-modules/${definition.module}`, ref: options['git-ref'] };
        delete definition.module;
      } else if (definition.source) definition.source.url = options['git-url'];
    }
    for (const bundle of result.bundles ?? []) bundle.source.url = options['git-url'];
  }
  if (options['git-ref'] !== undefined) {
    // Commit pins avoid following a moving company branch across seed steps.
    if (!/^[0-9a-f]{40}$/.test(options['git-ref'])) throw Error('--git-ref cần full commit SHA 40 ký tự của repo Git');
    for (const definition of result.resourceDefinitions ?? []) if (definition.source) definition.source.ref = options['git-ref'];
    for (const bundle of result.bundles ?? []) bundle.source.ref = options['git-ref'];
  }
  return result;
}
