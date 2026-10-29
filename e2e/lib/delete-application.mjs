import { expect } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { readFileSync, writeFileSync } from 'node:fs';

// UC-18 main flow: only the confirmation button mutates application state.
// Successful native reads and retained SQL provide independent absence proof.
export async function deleteApplicationViaUI({ page, baseURL, appId, name, namespace, kube, evidence, check }) {
  expect(appId).toMatch(/^[0-9a-f-]{36}$/);
  const kubeRead = args => execFileSync('kubectl', ['--context', kube, ...args], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] });
  const clusterIdentity = kubeRead(['get', 'namespace', 'kube-system', '-o', 'jsonpath={.metadata.uid}']);
  const api = `${baseURL}/api/v1/applications/${appId}`;
  const get = async path => {
    const r = await page.request.get(`${api}${path}`);
    expect(r.ok()).toBe(true);
    return r.json();
  };
  const sql = query => execFileSync(process.env.WM_DB_QUERY, [query], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
  const countOwned = `SELECT count(*) FROM terraform_resource_instances i JOIN applications a ON a.id=i.application_id WHERE a.application_key='${appId}' AND i.status<>'DESTROYED' AND i.descriptor NOT LIKE 'k8s-cluster.%'`;
  const before = Number(sql(countOwned));
  expect(before).toBe(3);
  // Retain only paths, never secret values. The exact config value and every
  // delivery bundle are checked for Vault 404 after product deletion.
  const refs = JSON.parse(sql(`SELECT COALESCE(json_agg(DISTINCT ce.value_ref),'[]') FROM configuration_revision_entries ce JOIN configuration_revisions cr ON cr.id=ce.revision_id JOIN environments e ON e.id=cr.environment_id JOIN applications a ON a.id=e.application_id WHERE a.application_key='${appId}' AND ce.kind='SECRET'`));
  expect(refs.length).toBeGreaterThan(0);
  const token = readFileSync(process.env.WM_WORKLOAD_VAULT_TOKEN_FILE, 'utf8').trim();
  const vaultContainer = new URL(process.env.WM_VAULT_ADDR).hostname;
  const vaultIP = execFileSync('docker', ['inspect', '-f', '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}', vaultContainer], { encoding: 'utf8' }).trim();
  const vaultStatus = async ref => {
    const u = new URL(ref);
    expect(u.protocol).toBe('kv2:');
    expect(u.pathname).toMatch(new RegExp(`^/orchestrator/apps/${appId}/envs/staging/values/`));
    const response = await fetch(`http://${vaultIP}:8200/v1/${u.hostname}/data${u.pathname}`, {
      headers: { 'X-Vault-Token': token }, signal: AbortSignal.timeout(15_000),
    });
    await response.body?.cancel();
    return response.status;
  };
  const deployments = JSON.parse(sql(`SELECT COALESCE(json_agg(d.id::text),'[]') FROM deployments d JOIN environments e ON e.id=d.environment_id JOIN applications a ON a.id=e.application_id WHERE a.application_key='${appId}' AND e.environment_key='staging'`));
  const bundles = deployments.map(id => `kv2://kv/orchestrator/apps/${appId}/envs/staging/values/${id}`);
  for (const ref of refs) expect(await vaultStatus(ref)).toBe(200);
  const liveBundles = [];
  for (const ref of bundles) {
    const status = await vaultStatus(ref);
    expect([200, 404]).toContain(status);
    if (status === 200) liveBundles.push(ref);
  }
  expect(liveBundles.length).toBeGreaterThan(0);
  await page.goto(`${baseURL}/ui/applications/${appId}/settings`);
  await page.getByRole('region', { name: 'Xóa ứng dụng', exact: true }).getByRole('button', { name: 'Xóa ứng dụng', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Xóa ứng dụng', exact: true });
  await expect(dialog.getByRole('region', { name: 'staging', exact: true })).toContainText('Workload sẽ bị gỡ: 3');
  await expect(dialog.getByRole('region', { name: 'staging', exact: true })).toContainText(`Tài nguyên sẽ bị xóa: ${before}`);
  await expect(dialog.getByRole('region', { name: 'production', exact: true })).toContainText('Tài nguyên sẽ bị xóa: 0');
  await dialog.getByRole('textbox').fill(name);
  await page.screenshot({ path: `${evidence}/delete-confirmation.png`, fullPage: true });
  const reply = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === `/api/v1/applications/${appId}/deletions`);
  await dialog.getByRole('button', { name: 'Xóa vĩnh viễn ứng dụng', exact: true }).click();
  const response = await reply;
  expect(response.status()).toBe(202);
  const operationId = (await response.json()).deletion.operationId;
  await expect(page.getByRole('region', { name: 'Tiến trình xóa ứng dụng', exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: '+ Thêm biến', exact: true })).toBeDisabled();
  let progress;
  await expect.poll(async () => {
    progress = (await get(`/deletions/${operationId}`)).deletion;
    return progress.running ? 'RUNNING' : progress.status;
  }, { timeout: 20 * 60_000, intervals: [3000] }).not.toBe('RUNNING');
  expect(progress.status, progress.failureCode).toBe('SUCCEEDED');
  expect(progress.applicationStatus).toBe('DELETED');
  expect(progress.environments.map(env => env.status)).toEqual(['SUCCEEDED', 'SUCCEEDED']);
  await expect(page.getByRole('heading', { name: 'Ứng dụng của bạn', exact: true })).toBeVisible({ timeout: 60_000 });
  await expect(page.getByRole('status').filter({ hasText: `Đã xóa ứng dụng ${name}.` })).toBeVisible();
  expect((await page.request.get(api)).status()).toBe(404);
  expect(Number(sql(countOwned))).toBe(0);
  expect(sql(`SELECT status FROM applications WHERE application_key='${appId}'`)).toBe('DELETED');
  expect(Number(sql(`SELECT count(*) FROM deployments d JOIN environments e ON e.id=d.environment_id JOIN applications a ON a.id=e.application_id WHERE a.application_key='${appId}'`))).toBeGreaterThan(0);
  const absent = kubeRead(['get', 'namespace', namespace, '--ignore-not-found', '-o', 'name']);
  expect(absent.trim()).toBe('');
  await expect.poll(() => JSON.parse(kubeRead(['get', 'persistentvolumes', '-o', 'json'])).items.filter(pv => pv.spec.claimRef?.namespace === namespace).length,
    { timeout: 120_000, intervals: [2000] }).toBe(0);
  expect(kubeRead(['get', 'nodes', '-o', 'name']).trim()).not.toBe('');
  expect(kubeRead(['get', 'namespace', 'kube-system', '-o', 'jsonpath={.metadata.uid}'])).toBe(clusterIdentity);
  for (const ref of [...refs, ...liveBundles]) expect(await vaultStatus(ref)).toBe(404);
  check('Vault confirms exact Application Secret and delivery bundle paths are absent with HTTP 404');
  writeFileSync(`${evidence}/deletion.json`, JSON.stringify(progress, null, 2));
  check('Whole Application deleted through UI; three owned instances DESTROYED, namespace absent, tombstone retained, platform cluster serving');
  await page.screenshot({ path: `${evidence}/deleted.png`, fullPage: true });
  await page.getByRole('button', { name: '+ Tạo ứng dụng', exact: true }).click();
  await page.getByLabel('Tên ứng dụng', { exact: true }).fill(name);
  await page.getByLabel('Subdomain', { exact: true }).fill(name);
  await page.getByRole('button', { name: 'Tạo ứng dụng', exact: true }).click();
  await expect(page.getByRole('heading', { name, exact: true })).toBeVisible();
  const reusedId = new URL(page.url()).pathname.match(/\/applications\/([^/]+)$/)?.[1];
  expect(reusedId).toMatch(/^[0-9a-f-]{36}$/);
  expect(reusedId).not.toBe(appId);
  await page.goto(`${baseURL}/ui/applications/${reusedId}/settings`);
  await page.getByRole('region', { name: 'Xóa ứng dụng', exact: true }).getByRole('button', { name: 'Xóa ứng dụng', exact: true }).click();
  const emptyDialog = page.getByRole('dialog', { name: 'Xóa ứng dụng', exact: true });
  for (const env of ['staging', 'production']) {
    await expect(emptyDialog.getByRole('region', { name: env, exact: true })).toContainText('Workload sẽ bị gỡ: 0');
    await expect(emptyDialog.getByRole('region', { name: env, exact: true })).toContainText('Tài nguyên sẽ bị xóa: 0');
  }
  await emptyDialog.getByRole('textbox').fill(name);
  await emptyDialog.getByRole('button', { name: 'Xóa vĩnh viễn ứng dụng', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Ứng dụng của bạn', exact: true })).toBeVisible({ timeout: 60_000 });
  expect((await page.request.get(`${baseURL}/api/v1/applications/${reusedId}`)).status()).toBe(404);
  writeFileSync(`${evidence}/name-reuse.json`, JSON.stringify({ originalId: appId, reusedId, reusedApplicationDeleted: true }, null, 2));
  check('Same name/subdomain accepted with a fresh ID; empty replacement also deleted through UI');
}
