import assert from 'node:assert/strict';
import { parse as parseYAML } from 'yaml';
import { randomBytes } from 'node:crypto';
import { mkdirSync, readFileSync, writeFileSync, existsSync, realpathSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { resolve, join, sep } from 'node:path';
import { readConsoleConfig, readSecondAccount, openConsole, signInContext, closeConsole } from '../../lib/console.mjs';
const args = process.argv.slice(2);
const evidence = resolve(args[args.indexOf('--evidence') + 1] ?? '');
assert(args.includes('--evidence') && evidence !== process.cwd(), '--evidence outside Git is required');
const repository = realpathSync(fileURLToPath(new URL('../../../', import.meta.url)));
assert(evidence !== repository && !evidence.startsWith(repository + sep), 'Evidence must be outside Git');
mkdirSync(evidence, { recursive: true, mode: 0o700 });
const actualEvidence = realpathSync(evidence);
assert(actualEvidence !== repository && !actualEvidence.startsWith(repository + sep), 'Evidence symlink points into Git');
const receiptFile = join(evidence, 'receipt.json');
const receipt = existsSync(receiptFile) ? JSON.parse(readFileSync(receiptFile, 'utf8')) : { run: Date.now().toString(), stage: 'new' };
const record = () => writeFileSync(receiptFile, JSON.stringify(receipt, null, 2), { mode: 0o600 });
record();
const peConfig = readConsoleConfig();
const devConfig = readSecondAccount(peConfig, process.env, { usernameVar: 'ORCH_DEV_USERNAME', passwordFileVar: 'ORCH_DEV_PASSWORD_FILE', defaultUsername: 'developer' });
const pe = await openConsole(peConfig, { role: 'PLATFORM_ENGINEER' });
let dev;
async function json(context, path, method = 'GET', data) {
  const response = await context.request.fetch(`${peConfig.baseURL}/api/v1${path}`, { method, ...(data === undefined ? {} : { data }) });
  assert(response.ok(), `${method} ${path}: HTTP ${response.status()}`);
  return response.status() === 204 ? null : response.json();
}
try {
  dev = await signInContext(pe.browser, devConfig, { role: 'DEVELOPER' });
  const page = dev.page;
  const volumeSection = name => page.getByRole('group',{name:`Volume và file của ${name}`,exact:true});
  async function addVolume(kind, container='main') {
    await volumeSection(container).getByRole('button',{name:'Thêm volume hoặc file',exact:true}).click();
    const dialog=page.getByRole('dialog',{name:'Thêm volume hoặc file',exact:true});
    await dialog.getByLabel('Loại',{exact:true}).selectOption(kind);
    return dialog;
  }
  async function attachVolume() {
    const dialog=page.getByRole('dialog',{name:'Thêm volume hoặc file',exact:true});
    await dialog.getByRole('button',{name:'Thêm vào container',exact:true}).click();
    await dialog.waitFor({state:'hidden'});
  }

  if (!receipt.appKey) {
    const name = `chinhform-${receipt.run}`;
    assert(!(await json(dev.context, '/applications')).applications.some(a => a.name === name), 'Application outcome needs reconciliation');
    await page.goto(`${devConfig.baseURL}/ui/applications/new`);
    await page.getByLabel('Tên ứng dụng', { exact: true }).fill(name);
    const reply = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/applications');
    await page.getByRole('button', { name: 'Tạo ứng dụng', exact: true }).click();
    const response = await reply; assert.equal(response.status(), 201);
    receipt.appKey = (await response.json()).application.key; receipt.stage = 'application-created'; record();
  }
  const app = (await json(dev.context, `/applications/${receipt.appKey}`)).application;
  assert.equal(app.name, `chinhform-${receipt.run}`);
  const envBase = `/applications/${receipt.appKey}/environments/staging`;
  const defs = (await json(pe.context, '/resource-definitions')).resourceDefinitions;
  const definitionKey = `chinhform-${receipt.run}`;
  if (!receipt.definitionKey) {
    assert(!defs.some(d => d.key === definitionKey), 'Definition outcome needs reconciliation');
    const base = defs.find(d => d.resourceType === 'workload' && d.driverType === 'template');
    assert(base, 'A workload Definition is required for placement');
    let source;
    for (const summary of (await json(pe.context, '/template-sources')).templateSources) {
      const detail = (await json(pe.context, `/template-sources/${summary.id}`)).templateSource;
      if (detail.status === 'AVAILABLE' && detail.name === 'standard-workload') source = detail;
    }
    assert(source, 'A current prepared standard source is required');
    const driverInputs = structuredClone(base.driverInputs);
    driverInputs.values.variables.template_source = source.id;
    driverInputs.values.variables.template_inputs = {};
    await json(pe.context, '/resource-definitions', 'POST', { key: definitionKey, name: `Chinhform ${receipt.run}`, resourceType: 'workload', driverType: 'template', driverInputs, criteria: [{ app_id: receipt.appKey, env_type: 'staging', class: 'chinhform' }] });
    receipt.definitionKey = definitionKey; receipt.sourceId = source.id; record();
  }
  // A backend update changes the executable pin. Upgrade only this run's
  // scoped Definition and Environment, never unrelated installed Definitions.
  const previousSource = (await json(pe.context, `/template-sources/${receipt.sourceId}`)).templateSource;
  if (previousSource.status !== 'AVAILABLE') {
    let source;
    for (const item of (await json(pe.context, '/template-sources')).templateSources) {
      const candidate = (await json(pe.context, `/template-sources/${item.id}`)).templateSource;
      if (candidate.status === 'AVAILABLE' && candidate.name === 'standard-workload') source = candidate;
    }
    assert(source, 'Updated prepared standard source required');
    const current = (await json(pe.context, '/resource-definitions')).resourceDefinitions.find(d => d.key === receipt.definitionKey);
    assert(current && current.criteria.every(c => c.app_id === receipt.appKey), 'Definition ownership changed');
    const driverInputs = structuredClone(current.driverInputs); driverInputs.values.variables.template_source = source.id;
    await json(pe.context, `/resource-definitions/${current.key}`, 'PUT', { key: current.key, name: current.name, version: current.version, resourceType: current.resourceType, driverType: current.driverType, driverInputs, criteria: current.criteria });
    const history = (await json(pe.context, `/resource-definitions/${current.key}/versions`)).history;
    const revision = history.versions.at(-1).id;
    await json(pe.context, `/resource-definitions/${current.key}/versions/${revision}/promote`, 'POST', { expectedGeneration: history.generation });
    const selection = await json(dev.context, envBase + '/definition-versions');
    await json(dev.context, envBase + '/definition-versions', 'PUT', { definitionKey: current.key, revisionId: revision, expectedRevision: selection.revision, expectedEnvironmentVersion: selection.environmentVersion });
    receipt.sourceId = source.id; receipt.stage = 'source-upgraded'; record();
  }
  const workloadPath = `${envBase}/workloads/app`;
  const editorURL = `${devConfig.baseURL}/ui/applications/${receipt.appKey}/environments/staging/workloads/app`;
  let workloads = (await json(dev.context, envBase + '/workloads')).workloads;
  if (!workloads.some(w => w.id === 'app')) {
    await page.goto(`${devConfig.baseURL}/ui/applications/${receipt.appKey}/environments/staging/workloads/new`);
    await page.getByLabel('Tên Workload', { exact: true }).fill('app');
    await page.getByLabel('Ảnh Container', { exact: true }).fill('busybox:1.36');
    await page.getByRole('button', { name: 'Hiện nâng cao Workload', exact: true }).click();
    await page.getByRole('combobox', { name: 'Class Workload', exact: true }).selectOption('chinhform');
    await page.getByRole('button', { name: 'Thêm annotation', exact: true }).click();
    await page.getByLabel('Tên annotation', { exact: true }).fill('example.com/owner');
    await page.getByLabel('Giá trị annotation', { exact: true }).fill('chinhform');
    for (const role of ['init', 'sidecar']) {
      await page.getByRole('button', { name: '+ Thêm Container', exact: true }).click();
      const n = role === 'init' ? 2 : 3;
      await page.getByLabel('Ảnh Container', { exact: true }).nth(n - 1).fill('busybox:1.36');
      await page.getByRole('combobox', { name: `Vai trò container container-${n}`, exact: true }).selectOption(role);
    }
    await addVolume('storage');
    await page.getByRole('button',{name:'Tạo hoặc chọn lưu trữ',exact:true}).click();
    const storageDialog = page.getByRole('dialog', { name: 'Thêm tài nguyên lưu trữ', exact: true });
    await storageDialog.getByLabel('Loại tài nguyên', { exact: true }).selectOption('persistent-storage');
    const storageInput = storageDialog.getByLabel('Tham số storage', { exact: true });
    if (await storageInput.count()) await storageInput.fill('1Gi');
    await storageDialog.getByRole('button', { name: 'Thêm và chọn', exact: true }).click();
    await storageDialog.waitFor({ state: 'hidden' });
    let volumeDialog=await addVolume('emptyDir');
    await volumeDialog.getByLabel('Tên volume',{exact:true}).fill('shared');
    await volumeDialog.getByLabel('Nơi lưu volume tạm',{exact:true}).selectOption('Memory');
    await volumeDialog.getByLabel('Giới hạn dung lượng',{exact:true}).fill('64Mi');
    await attachVolume();
    for (const container of ['container-2','container-3']) {
      volumeDialog=await addVolume('emptyDir',container);
      await volumeDialog.getByLabel('Nguồn',{exact:true}).selectOption('shared');
      await attachVolume();
    }
    volumeDialog=await addVolume('configMap');
    await volumeDialog.getByLabel('Nội dung (không chứa bí mật)',{exact:true}).fill('enabled=true\n');
    await volumeDialog.getByLabel('Đường dẫn mount',{exact:true}).fill('/config/app.conf');
    await volumeDialog.getByLabel('Thư mục con / file (subPath)',{exact:true}).fill('app.conf');
    await attachVolume();
    await addVolume('secret');
    await page.getByRole('button',{name:'Tạo bí mật cho file',exact:true}).click();
    const dialog = page.getByRole('dialog', { name: 'Tạo bí mật', exact: true });
    await dialog.getByText('Đang tải kho bí mật…', { exact: true }).waitFor({ state: 'hidden' });
    const store = dialog.getByRole('combobox', { name: 'Kho bí mật', exact: true });
    if (await store.count()) {
      const choices = await store.locator('option').evaluateAll(items => items.map(o => o.value).filter(Boolean));
      assert(choices.length, 'No ready Secret Store'); await store.selectOption(choices[0]);
      await dialog.getByRole('button', { name: 'Lưu kho bí mật', exact: true }).click();
    }
    await dialog.getByLabel('Tên key', { exact: true }).fill('RUNTIME_FILE');
    const secret = randomBytes(32).toString('hex') + '\nline-two\n';
    const file = join(evidence, 'secret-input.txt'); writeFileSync(file, secret, { mode: 0o600 });
    await dialog.getByLabel('Đọc bí mật từ file', { exact: true }).setInputFiles(file);
    await dialog.getByRole('button', { name: 'Tạo và chọn', exact: true }).click();
    await dialog.waitFor({ state: 'hidden' });
    const reply = page.waitForResponse(r => r.request().method() === 'PUT' && new URL(r.url()).pathname === '/api/v1' + workloadPath);
    await page.getByRole('button', { name: 'Lưu draft Workload', exact: true }).click();
    const saved = await reply; assert.equal(saved.status(), 200);
    workloads = (await json(dev.context, envBase + '/workloads')).workloads;
    assert(!JSON.stringify(workloads).includes(secret.trim()), 'Secret bytes leaked into workload');
    receipt.stage = 'saved'; record();
  }
  if (!workloads.find(w => w.id === 'app').workload.workloadExecution.volumes.some(v => v.hostPath)) {
    await page.goto(editorURL);
    let volumeDialog=await addVolume('hostPath');
    await volumeDialog.getByLabel('Tên volume',{exact:true}).fill('node-data');
    await volumeDialog.getByLabel('Đường dẫn trên node',{exact:true}).fill('/var/local/chinhform');
    await volumeDialog.getByLabel('Loại đường dẫn node',{exact:true}).selectOption('Directory');
    await attachVolume();
    for (const container of ['container-2','container-3']) {
      volumeDialog=await addVolume('hostPath',container);
      await volumeDialog.getByLabel('Nguồn',{exact:true}).selectOption('node-data');
      await attachVolume();
    }
    const reply = page.waitForResponse(r => r.request().method() === 'PUT' && new URL(r.url()).pathname === '/api/v1' + workloadPath);
    await page.getByRole('button', { name: 'Lưu draft Workload', exact: true }).click();
    assert.equal((await reply).status(), 200);
    workloads = (await json(dev.context, envBase + '/workloads')).workloads;
    receipt.stage = 'hostpath-saved'; record();
  }
  const before = workloads.find(w => w.id === 'app').workload;
  assert.equal(before.containers['container-2'].role, 'init'); assert.equal(before.containers['container-3'].role, 'sidecar');
  assert.equal(before.workloadExecution.mounts.length, 1); assert.equal(before.workloadExecution.volumes.length, 3); assert.equal(before.workloadExecution.volumeMounts.length, 7); assert.equal(before.workloadExecution.secretFiles[0].ref, '${resources.env.RUNTIME_FILE}');
  await page.goto(editorURL);
  await page.getByRole('combobox', { name: 'Vai trò container container-2', exact: true }).waitFor();
  assert.equal(await page.getByRole('heading',{name:'Pod volume sources',exact:true}).count(),0);
  assert.equal(await page.getByRole('heading',{name:'Volume và file',exact:true}).count(),3);
  await page.screenshot({ path: join(evidence, 'editor.png'), fullPage: true });
  const resave = page.waitForResponse(r => r.request().method() === 'PUT' && new URL(r.url()).pathname === '/api/v1' + workloadPath);
  await page.getByRole('button', { name: 'Lưu draft Workload', exact: true }).click(); assert.equal((await resave).status(), 200);
  const after = (await json(dev.context, envBase + '/workloads')).workloads.find(w => w.id === 'app').workload;
  assert.deepEqual(after, before, 'Edit/save changed runtime intent');
  await page.goto(`${devConfig.baseURL}/ui/applications/${receipt.appKey}?environment=staging`);
  const previewReply = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1' + envBase + '/preview');
  await page.getByRole('button', { name: 'Xem trước thay đổi', exact: true }).click();
  const previewResponse = await previewReply; assert.equal(previewResponse.status(), 200);
  const preview = await previewResponse.json();
  writeFileSync(join(evidence, 'preview.json'), JSON.stringify(preview, null, 2), { mode: 0o600 });
  const text = JSON.stringify(preview);
  assert(text.includes('initContainers') && text.includes('emptyDir') && text.includes('ConfigMap'), 'Preview omitted runtime manifest intent');
  assert(!text.includes('RENDER_UNAVAILABLE'), 'Manifest rendering unavailable');
  const artifact = preview.review.artifacts.find(a => a.path === 'Deployment/app');
  const controller = parseYAML(artifact.after.text); const pod = controller.spec.template.spec;
  assert.equal(pod.containers.length, 1); assert.equal(pod.initContainers.length, 2);
  assert.equal(pod.initContainers[0].name, 'container-2');
  assert.equal(pod.initContainers[1].name, 'container-3'); assert.equal(pod.initContainers[1].restartPolicy, 'Always');
  assert.equal(pod.volumes.length, 5);
  assert(pod.volumes.some(v => v.hostPath?.path === '/var/local/chinhform' && v.hostPath.type === 'Directory'), 'HostPath source missing');
  for (const c of [...pod.containers, ...pod.initContainers]) assert(c.volumeMounts.some(m => m.mountPath === '/node-data' && m.readOnly === true), 'HostPath read-only mount missing');
  assert(pod.volumes.some(v => v.persistentVolumeClaim), 'PVC source missing');
  assert(pod.volumes.some(v => v.emptyDir?.medium === 'Memory'), 'Memory emptyDir missing');
  assert(pod.volumes.some(v => v.configMap?.name), 'ConfigMap reference missing');
  assert(pod.volumes.some(v => v.secret?.secretName && v.secret.defaultMode === 256), 'Approved file Secret/mode missing');
  assert.equal(pod.containers[0].volumeMounts.length, 5);
  for (const c of pod.initContainers) assert(c.volumeMounts.some(m => m.mountPath === '/shared'), 'Shared init/sidecar mount missing');
  const config = parseYAML(preview.review.artifacts.find(a => a.kind === 'kubernetes-manifest' && a.path.startsWith('ConfigMap/')).after.text);
  assert.equal(config.data['app.conf'], '<redacted>', 'Preview must keep data redacted');
  const privateInput = join(evidence, 'secret-input.txt');
  if (existsSync(privateInput)) assert(!text.includes(readFileSync(privateInput, 'utf8').split('\n')[0]), 'Preview leaked Secret bytes');

  await page.screenshot({ path: join(evidence, 'preview.png'), fullPage: true });
  delete receipt.lastError; receipt.stage = 'PASS'; receipt.editorURL = editorURL; receipt.checkedAt = new Date().toISOString(); record();
  console.log(JSON.stringify({ result: 'PASS', editorURL, appKey: receipt.appKey, scope: 'Save/reopen/Preview only; no Kubernetes/cloud mutation' }));
} catch (error) {
  receipt.lastError = error.message; record();
  if (dev) await dev.page.screenshot({ path: join(evidence, 'failure.png'), fullPage: true }).catch(() => {});
  throw error;
} finally {
  if (dev) await dev.context.close();
  await closeConsole(pe, peConfig);
}
