// Human-paced variant of acceptance-kind.mjs for recorded UI review.
// It fills every Web Console form with a visible mouse cursor and keyboard
// typing instead of Score import or instant fill, so the video shows the same
// UC-00/01/12/16/05/06 flow a person would perform.
// Invoked by backend/test/integration/acceptance-playwright-kind.sh --human.
import { chromium, expect } from '@playwright/test';
import { createHash, randomBytes } from 'node:crypto';
import { spawn } from 'node:child_process';
import { createServer } from 'node:net';
import { chmodSync, renameSync, writeFileSync } from 'node:fs';
import { acceptanceApp, addWorkload, createApplication, createHuman, installCursor, previewAndDeploy, putKeys, reviewAcceptancePage, signIn } from './human.mjs';

const baseURL = process.env.ORCH_E2E_URL;
const runId = process.env.ORCH_E2E_RUN_ID;
const context = process.env.ORCH_E2E_KUBE_CONTEXT;
const evidenceDir = process.env.ORCH_E2E_EVIDENCE_DIR;
if (!baseURL || !/^[a-z0-9-]+$/.test(runId ?? '') || !context || !evidenceDir) throw new Error('ORCH_E2E_URL, ORCH_E2E_RUN_ID, ORCH_E2E_KUBE_CONTEXT and ORCH_E2E_EVIDENCE_DIR are required');

const browser = await chromium.launch({ headless: true });
const browserContext = await browser.newContext({ recordVideo: { dir: evidenceDir, size: { width: 1280, height: 800 } }, viewport: { width: 1280, height: 800 } });
await browserContext.addInitScript(installCursor);
const page = await browserContext.newPage();
const video = page.video();
const h = createHuman(page);
let forward;
try {
  page.setDefaultTimeout(30_000);
  await page.goto(`${baseURL}/ui/sign-in`);
  await signIn(h);
  const applicationName = `Acceptance ${runId}`;
  const appId = await createApplication(h, applicationName, runId);
  const namespace = `app-${appId}-staging`;
  if (process.env.ORCH_E2E_NAMESPACE_FILE) writeFileSync(process.env.ORCH_E2E_NAMESPACE_FILE, namespace, { mode: 0o600 });

  const secret = randomBytes(32).toString('hex');
  const app = acceptanceApp(runId, secret, createHash('sha256').update(secret).digest('hex'));
  await putKeys(h, applicationName, app.keys);
  for (const workload of app.workloads) await addWorkload(h, workload);
  await previewAndDeploy(h, app.workloads.map((workload) => workload.name));

  const port = await freePort();
  forward = spawn('kubectl', ['--context', context, '-n', namespace, 'port-forward', 'svc/frontend', `${port}:8080`], { stdio: ['ignore', 'pipe', 'pipe'] });
  let forwardLog = '';
  forward.stdout.on('data', (chunk) => { forwardLog += chunk.toString(); });
  forward.stderr.on('data', (chunk) => { forwardLog += chunk.toString(); });
  await expect.poll(() => forwardLog.includes(`Forwarding from 127.0.0.1:${port}`), { timeout: 30_000 }).toBe(true);
  await page.goto(`http://127.0.0.1:${port}/`);
  await reviewAcceptancePage(h, runId, secret);
  console.log(`PASS: application=${appId} namespace=${namespace} checks=backend,environment,secret,database,job-submit`);
} finally {
  if (forward && forward.exitCode === null && forward.signalCode === null) {
    forward.kill();
    await new Promise((resolve) => forward.once('exit', resolve));
  }
  try {
    await browserContext.close();
    if (video) {
      const videoPath = `${evidenceDir}/acceptance-full.webm`;
      renameSync(await video.path(), videoPath);
      chmodSync(videoPath, 0o600);
    }
  } finally {
    await browser.close();
  }
}

async function freePort() {
  const server = createServer();
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  const port = server.address().port;
  await new Promise((resolve) => server.close(resolve));
  return port;
}
