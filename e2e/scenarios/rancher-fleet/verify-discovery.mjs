import assert from 'node:assert/strict';
import { readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { parse } from 'yaml';
import { loadBootstrapConfig } from '../../../scripts/bootstrap/lib/bootstrap-config.mjs';
import { kubectl } from '../../../scripts/bootstrap/lib/verification.mjs';
import { openConsole, closeConsole } from '../../lib/console.mjs';

const directory = process.env.ORCH_SIMPLIFIED_CD_RUN_DIR;
assert(directory, 'Private run directory required');
const config = loadBootstrapConfig(process.argv[2]);
assert.equal(config.environment, 'dev');
const setup = JSON.parse(readFileSync(join(directory, 'setup.json')));
assert(setup.setupComplete && setup.origin === config.console.baseURL);
const session = await openConsole(config.console, { role: 'PLATFORM_ENGINEER' });
try {
 const { page } = session;
 await page.goto(config.console.baseURL + '/ui/platform/cd-providers');
 await page.getByRole('button', { name: 'Thêm cấu hình CD', exact: true }).click();
 const reply = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/delivery-providers/fleet/inspect');
 await page.getByLabel('Tài khoản quản lý CD', { exact: true }).selectOption(setup.account.key);
 const response = await reply;
 assert(response.ok());
 const discovery = await response.json();
 const options = discovery.options.targetCluster.filter(o => !o.disabled);
 assert(options.length > 0);
 const details = await page.request.get(config.console.baseURL + `/api/v1/delivery-bindings/${setup.binding.id}?version=${setup.binding.version}`);
 assert(details.ok());
 const binding = (await details.json()).config;
 const option = options.find(o => o.value === binding.targetCluster && o.values.workspace === binding.workspace);
 assert(option);
 for (const key of ['controllerVersion', 'controllerKind', 'agentKind', 'agentName', 'agentNamespace', 'targetKubeSystemUID']) {
  assert(option.values[key]); assert.equal(option.values[key], binding[key]);
 }
 const registration = kubectl(config, '-n', binding.workspace, 'get', 'clusters.fleet.cattle.io', binding.targetCluster);
 assert.equal(registration.metadata.uid, binding.targetClusterUID);
 const cm = kubectl(config, '-n', binding.agentNamespace, 'get', 'configmap', 'fleet-agent');
 const clientID = parse(cm.data.config).clientID;
 assert(clientID && clientID === registration.spec.clientID);
 const physical = kubectl(config, 'get', 'namespace', 'kube-system').metadata.uid;
 assert.equal(physical, binding.targetKubeSystemUID);
 await page.screenshot({ path: join(directory, 'discovery-physical-target.png'), fullPage: true });
 writeFileSync(join(directory, 'topology-proof.json'), JSON.stringify({ controllerVersion: binding.controllerVersion, controllerKind: binding.controllerKind, agentKind: binding.agentKind, registrationUID: registration.metadata.uid, physicalUID: physical, opaqueClientIDMatched: clientID !== physical }, null, 2), { mode: 0o600 });
 console.log('PASS: real kind discovery/binding share controller version, agent identity, registration UID and physical target UID; nonsecret opaque agent clientID matches registration.');
} finally { await closeConsole(session, config.console); }
