import assert from 'node:assert/strict';
import { readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { loadBootstrapConfig } from '../../../scripts/bootstrap/lib/bootstrap-config.mjs';
import { openConsole, closeConsole, signInContext, signOutContext } from '../../lib/console.mjs';
import { createRuntime, makeUI } from '../../../scripts/bootstrap/lib/runtime.mjs';
import { ensureWorkloads } from '../../../scripts/bootstrap/lib/workloads.mjs';
import { ensureDeployment } from '../../../scripts/bootstrap/lib/deployment.mjs';
import { ensureAccess, verifyAcceptance } from '../../../scripts/bootstrap/lib/verification.mjs';

const directory = process.env.ORCH_SIMPLIFIED_CD_RUN_DIR;
assert(directory, 'Private run directory required');
const config = loadBootstrapConfig(process.argv[2]);
assert.equal(config.environment, 'dev');
assert.equal(config.application.environment, 'staging');
const file = join(directory, 'setup.json');
const state = JSON.parse(readFileSync(file));
assert(state.setupComplete);
assert.equal(state.origin, config.console.baseURL);
const rt = createRuntime(config);
assert(rt.state.appKey, 'Confirmed Application receipt required');
let session, developer;
try {
 session = await openConsole(config.console, { role: 'PLATFORM_ENGINEER' });
 const ui = makeUI(session.page, config.console.baseURL, config.runtime.timeoutMs);
 const base = `/applications/${rt.state.appKey}/environments/staging`;
 const before = await ui.get(base + '/delivery');
 assert.equal(before.applied.bindingId, state.binding.id);
 assert.equal(before.applied.bindingVersion, state.binding.version);
 const versions = (await ui.get('/delivery-bindings')).bindings.filter(b => b.id === state.binding.id && b.version === state.binding.version + 1);
 assert(versions.length <= 1);
 let next = versions[0];
 await session.page.goto(config.console.baseURL + '/ui/platform/cd-providers');
 const row = version => session.page.locator('tbody tr').filter({ has: session.page.getByRole('cell', { name: new RegExp('^' + state.name + '(?:\\s*Mặc định)?$') }) }).filter({ has: session.page.locator('td:nth-child(3)').filter({ hasText: new RegExp('^' + version + '$') }) });
 if (!next) {
  await row(state.binding.version).getByRole('button', { name: 'Sửa thành phiên bản mới', exact: true }).click();
  const form = session.page.locator('form.secret-store-form');
  assert.equal(await form.getByLabel('Kết nối Git', { exact: true }).inputValue(), state.gitKey);
  assert.equal(await form.getByLabel('Token hoặc mật khẩu Git', { exact: true }).count(), 0);
  next = await ui.submit(`/delivery-bindings/${state.binding.id}/versions`, 'POST', form.getByRole('button', { name: 'Lưu cấu hình', exact: true }));
 }
 if (next.status !== 'READY') next = await ui.submit(`/delivery-bindings/${next.id}/verify`, 'POST', row(next.version).getByRole('button', { name: 'Xác minh', exact: true }));
 assert.equal(next.status, 'READY');
 const current = await ui.get('/delivery-default');
 if (current.bindingId !== next.id || current.bindingVersion !== next.version) await ui.submit('/delivery-default', 'PUT', row(next.version).getByRole('button', { name: 'Dùng cho ứng dụng mới', exact: true }));
 const after = await ui.get(base + '/delivery');
 assert.deepEqual(after.desired, before.desired); assert.deepEqual(after.applied, before.applied);
 state.nextBinding = next;
 writeFileSync(file, JSON.stringify(state, null, 2), { mode: 0o600 });
 developer = await signInContext(session.browser, config.developer, { role: 'DEVELOPER' });
 const dev = makeUI(developer.page, config.developer.baseURL, config.runtime.timeoutMs);
 const app = (await dev.get('/applications/' + rt.state.appKey)).application;
 assert.equal(app.name, config.application.name);
 // The previous approved image set is a normal desired-state update. Data and
 // Definition/Catalog pins stay intact; this does not roll back database data.
 const reversed = { ...config, application: { ...config.application, images: config.upgrade.images } };
 await ensureWorkloads(dev, reversed, app, config.application, rt, { upgrade: true });
 await ensureDeployment(dev, config, app, rt);
 await ensureAccess(dev, config, app, rt);
 await verifyAcceptance(dev, config, rt, { submitJob: false });
 await ensureDeployment(dev, config, app, rt);
 const final = await dev.get(base + '/delivery');
 assert.deepEqual(final.applied, before.applied);
 state.rollbackPassed = true;
 writeFileSync(file, JSON.stringify(state, null, 2), { mode: 0o600 });
 console.log('PASS: reuse Git connection in new binding version, preserve applied binding across default change, rollback images with retained data/job, unchanged Preview reuse.');
 rt.close('SUCCEEDED');
} catch (error) { rt.close('FAILED'); throw error; }
finally {
 if (developer) { await signOutContext(developer.context, config.developer); await developer.context.close(); }
 if (session) await closeConsole(session, config.console);
}
