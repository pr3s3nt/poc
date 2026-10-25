// UC-08 BR-10 additions to the real full-application kind scenario. These
// helpers only author Scores or read live objects/state; all product writes
// are performed through the Console by workload-manifests-kind.mjs.
import { expect } from '@playwright/test';
import { registerK8sModuleDefinition } from './full-app-flows.mjs';

export const completionType = {
  key: 'app-config', inputs: [['revision', 'string', true]],
  outputs: [['config', 'string', true], ['revision', 'string', true], ['name', 'string', true]],
};

export async function registerCompletionModule(kit, ref) {
  const stored = await registerK8sModuleDefinition(kit, {
    id: 'runtime-config', type: 'app-config', presetNamespace: true, clusterClass: 'internal',
    git: { url: 'https://github.com/pr3s3nt/poc.git', path: 'backend/test/acceptance/terraform-completion', ref },
  });
  expect(stored.driverInputs.values.source.revision).toBe(ref);
  return stored;
}

export function bindCompletion(doc, shared, revision = 'v1') {
  doc.resources.runtime = {
    type: 'app-config', params: { revision }, ...(shared ? { id: 'runtime-config' } : {}),
  };
  const vars = doc.containers.main.variables ??= {};
  vars.RESOURCE_REVISION = '${resources.runtime.revision}';
  vars.RESOURCE_CONFIG_NAME = '${resources.runtime.name}';
  if (shared) vars.ACCEPTANCE_CONFIG = '${resources.runtime.config}';
}

export async function completionSnapshot({ kit, tech, sql, check, label, previous, revision }) {
  const ns = kit.input.namespace;
  const app = kit.input.applicationKey;
  const resources = (await kit.get(`${kit.appPath}/resources`)).resources;
  const configs = resources.filter((r) => r.type === 'app-config');
  check(`TC-${label}.two-config-instances`, configs.length === 2 && configs.every((r) => r.status === 'READY'),
    'one shared backend/worker resource and one private frontend resource are READY');
  const deployments = (await tech.deployments()).items;
  const names = {};
  const identities = {};
  for (const workload of ['backend', 'worker', 'frontend']) {
    const deployment = deployments.find((d) => d.metadata.name === workload);
    const name = await tech.resolvedEnv(deployment, 'RESOURCE_CONFIG_NAME');
    names[workload] = name;
    const expectedRevision = workload === 'frontend' ? revision : 'v1';
    const cm = await tech.json(['-n', ns, 'get', 'configmap', name, '-o', 'json']);
    const sa = await tech.json(['-n', ns, 'get', 'serviceaccount', name, '-o', 'json']);
    const manifest = await tech.json(['-n', ns, 'get', 'configmap', `${name}-manifest`, '-o', 'json']);
    const boundRevision = await tech.resolvedEnv(deployment, 'RESOURCE_REVISION');
    check(`TC-${label}.${workload}-outputs`, cm.data.config === 'acceptance-config-ok'
      && cm.data.revision === expectedRevision && manifest.data.revision === expectedRevision && boundRevision === expectedRevision,
    `${workload} consumes live Terraform resource output, revision ${expectedRevision}`);
    if (workload !== 'frontend') {
      check(`TC-${label}.${workload}-environment`, await tech.resolvedEnv(deployment, 'ACCEPTANCE_CONFIG') === cm.data.config,
        'the application environment check uses the Terraform ConfigMap output');
    }
    identities[name] = { configMap: cm.metadata.uid, serviceAccount: sa.metadata.uid, manifest: manifest.metadata.uid };
  }
  check(`TC-${label}.scope`, names.backend === names.worker && names.frontend !== names.backend,
    'two consumers share one resource; the private consumer has a distinct resource');
  // Read only identifiers, statuses and serials, never encrypted state contents.
  const instances = JSON.parse(sql(`SELECT COALESCE(json_agg(json_build_object('id',i.id,'descriptor',i.descriptor,'status',i.status,'stateKey',i.state_key,'serial',s.serial) ORDER BY i.descriptor),'[]')::text FROM terraform_resource_instances i JOIN environments e ON e.id=i.environment_id JOIN applications a ON a.id=e.application_id LEFT JOIN terraform_states s ON s.resource_instance_id=i.id WHERE a.application_key='${app}' AND e.environment_key='staging'`));
  check(`TC-${label}.acknowledged-state`, instances.length >= 5 && instances.every((i) => i.status === 'READY' && Number.isInteger(i.serial)),
    'all resource instances have acknowledged durable state and reached READY');
  const statefulSets = (await tech.statefulsets()).items.map((s) => ({ name: s.metadata.name, uid: s.metadata.uid })).sort((a, b) => a.name.localeCompare(b.name));
  if (previous) {
    check(`TC-${label}.reuse`, JSON.stringify(identities) === JSON.stringify(previous.identities)
      && instances.length === previous.instances.length
      && instances.every((i) => previous.instances.some((p) => p.id === i.id && p.stateKey === i.stateKey))
      && JSON.stringify(statefulSets) === JSON.stringify(previous.statefulSets),
    'UPDATE preserves ConfigMap/ServiceAccount UIDs, instance/state identities and database/cache StatefulSets');
  }
  const postgres = instances.find((i) => i.descriptor.startsWith('postgres.'));
  const mixed = Number(sql(`SELECT count(*) FROM terraform_resource_attempts WHERE resource_instance_id='${postgres.id}'::uuid AND status='SUCCEEDED' AND EXISTS (SELECT 1 FROM jsonb_array_elements_text(observed_physical_ids) x WHERE x LIKE 'kubernetes_stateful_set_v1.%') AND EXISTS (SELECT 1 FROM jsonb_array_elements_text(observed_physical_ids) x WHERE x LIKE 'kubernetes_service_v1.%')`));
  check(`TC-${label}.mixed-module`, mixed > 0,
    'the PostgreSQL module combines supplemental StatefulSet readiness with provider-completed Service/Secret/PVC resources');
  return { names, identities, instances, statefulSets };
}
