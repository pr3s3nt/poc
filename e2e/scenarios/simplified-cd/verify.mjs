import assert from 'node:assert/strict';
import { readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { execFileSync } from 'node:child_process';
import { parse } from 'yaml';
import { loadBootstrapConfig } from '../../../scripts/bootstrap/lib/bootstrap-config.mjs';
import { openConsole, closeConsole, readPrivateFile } from '../../lib/console.mjs';
import { kubectl } from '../../../scripts/bootstrap/lib/verification.mjs';

const directory = process.env.ORCH_SIMPLIFIED_CD_RUN_DIR;
assert(directory, 'Private run directory required');
const config = loadBootstrapConfig(process.argv[2]);
assert.equal(config.environment, 'dev');
assert.equal(config.application.environment, 'staging');
const setup = JSON.parse(readFileSync(join(directory, 'setup.json')));
assert.equal(setup.origin, config.console.baseURL);
const state = JSON.parse(readFileSync(join(config.runtime.stateDir, 'identity.json')));
assert(setup.setupComplete && state.appKey, 'Confirmed setup and application receipts required');
assert(/^[0-9a-f-]{36}$/.test(state.appKey));
const namespace = `app-${state.appKey}-staging`;
assert.equal(kubectl(config, 'get', 'namespace', namespace).metadata.uid, state.namespaceUid);
const sql = `SELECT row_to_json(r) FROM application_delivery_repositories r WHERE application_id='${state.appKey}'`;
const allocation = JSON.parse(execFileSync('docker', ['exec', 'deploy-main-postgres-1', 'psql', '-U', 'orchestrator', '-d', 'orchestrator', '-At', '-c', sql], { encoding: 'utf8' }));
assert.equal(allocation.status, 'READY');
assert.equal(allocation.binding_id, setup.binding.id);
assert.equal(allocation.binding_version, setup.binding.version);
const remote = new URL(allocation.clone_url);
assert.equal(remote.hostname, 'github.com');
const repository = remote.pathname.replace(/^\//, '').replace(/\.git$/, '');
const token = readPrivateFile(config.platform.gitAccess.tokenFile, 'Git credential');
async function github(path) {
 const response = await fetch('https://api.github.com' + path, { headers: { Authorization: `Bearer ${token}`, Accept: 'application/vnd.github+json', 'X-GitHub-Api-Version': '2022-11-28' }, redirect: 'error' });
 assert(response.ok, `GitHub read HTTP ${response.status}`);
 return response.json();
}
const info = await github(`/repos/${repository}`);
assert.equal(info.private, true);
assert.equal(String(info.id), allocation.remote_id);
const controllers = kubectl(config, '-n', setup.managedReader.workspace, 'get', 'gitrepos.fleet.cattle.io').items.filter(c => c.spec.repo === allocation.clone_url);
assert(controllers.length > 0, 'No application GitRepos');
const bundles = kubectl(config, '-n', setup.managedReader.workspace, 'get', 'bundles.fleet.cattle.io').items;
const seen = new Map();
let files = 0;
for (const controller of controllers) {
 assert.match(controller.spec.revision, /^[0-9a-f]{40}$/);
 assert.equal(controller.status.commit, controller.spec.revision, 'Fleet has not consumed the pinned commit');
 const owned = bundles.filter(b => b.metadata.labels?.['fleet.cattle.io/repo-name'] === controller.metadata.name);
 assert.equal(owned.length, 1, 'Expected one rendered Bundle per unit');
 assert.equal(owned[0].metadata.labels['fleet.cattle.io/commit'], controller.spec.revision, 'Bundle still carries a different commit');
 assert.equal(owned[0].status.summary?.desiredReady, 1);
 assert.equal(owned[0].status.summary?.ready, 1);
 assert.equal(controller.spec.clientSecretName, setup.managedReader.name);
 assert.equal(controller.spec.targets.length, 1);
 assert.equal(controller.spec.paths.length, 1);
 const path = controller.spec.paths[0];
 assert.match(path, /^environments\/staging\/manifests\/orch-[0-9a-f]+$/);
 let tree = seen.get(controller.spec.revision);
 if (!tree) { tree = await github(`/repos/${repository}/git/trees/${controller.spec.revision}?recursive=1`); assert.equal(tree.truncated, false); seen.set(controller.spec.revision, tree); }
 const entries = tree.tree.filter(entry => entry.type === 'blob' && entry.path.startsWith(path + '/'));
 assert(entries.some(entry => entry.path === path + '/fleet.yaml'));
 assert(entries.some(entry => entry.path !== path + '/fleet.yaml'));
 for (const entry of entries) {
  assert(entry.path.endsWith('.yaml'), 'Generated manifest is not YAML');
  const blob = await github(`/repos/${repository}/git/blobs/${entry.sha}`);
  const text = Buffer.from(blob.content, 'base64').toString('utf8');
  assert(!text.includes(token), 'Credential entered Git');
  const document = parse(text);
  assert(document && typeof document === 'object');
  if (entry.path.endsWith('/fleet.yaml')) { assert.equal(document.helm.force, false); assert.equal(document.helm.takeOwnership, false); assert.equal(document.deleteNamespace, false); }
  else { assert.notEqual(document.kind, 'Secret'); assert(document.apiVersion && document.kind && document.metadata?.name); }
  files++;
 }
}
const metadata = execFileSync('kubectl', ['--kubeconfig', config.runtime.clusterKubeconfigFile, '--context', config.platform.account.context, '-n', setup.managedReader.workspace, 'get', 'secret', setup.managedReader.name, '-o', 'go-template={{.metadata.uid}}|{{index .metadata.labels "orchestrator.io/delivery-binding"}}|{{index .metadata.labels "orchestrator.io/binding-version"}}'], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim().split('|');
assert(metadata[0]); assert.equal(metadata[1], setup.binding.id); assert.equal(metadata[2], String(setup.binding.version));
const session = await openConsole(config.console);
try {
 const response = await session.page.request.get(config.console.baseURL + `/api/v1/applications/${state.appKey}/environments/staging/delivery`);
 assert(response.ok());
 const view = await response.json();
 assert.equal(view.applied.bindingId, setup.binding.id);
 assert.equal(view.applied.bindingVersion, setup.binding.version);
 await session.page.goto(config.console.baseURL + `/ui/applications/${state.appKey}?environment=staging`);
 await session.page.getByRole('button', { name: 'Xem trước thay đổi', exact: true }).waitFor();
 await session.page.screenshot({ path: join(directory, `application-delivered-${Date.now()}.png`), fullPage: true });
} finally { await closeConsole(session, config.console); }
writeFileSync(join(directory, `git-fleet-proof-${Date.now()}.json`), JSON.stringify({ application: state.appKey, repositoryId: allocation.remote_id, gitRepoUIDs: controllers.map(c => c.metadata.uid), revisions: [...seen.keys()], yamlFiles: files, readerUID: metadata[0], namespaceUID: state.namespaceUid }, null, 2), { mode: 0o600 });
console.log(`PASS: private application repository, ${controllers.length} SHA-pinned GitRepos, ${files} YAML files, owned private reader Secret and applied binding.`);
