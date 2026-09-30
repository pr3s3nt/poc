// Human-paced UC-09 review recording. A headed Chromium runs on an Xvfb
// display and ffmpeg records the whole window, including tabs and the real
// address bar. Invoked by backend/test/integration/uc09-video-local.sh.
//
// Fixture setup happens OFFSCREEN before recording, through the HTTP API with
// a separate request context: one successful deploy, one planning failure and
// one redeploy of the same workload with a changed manifest, all against the
// fake-adapter backend. The setup is not UI evidence and proves no runtime
// deployment; the recording only demonstrates UC-09 observation in the UI.
//
// Every recorded step is a real UI action: the Playwright mouse moves the
// injected cursor to each target, form values are typed key by key, and the
// address bar and the native status <select> are driven through X11 input
// (xdotool), because Playwright keyboard events do not reach browser chrome.
import { chromium, expect, request } from '@playwright/test';
import { execFileSync, spawn } from 'node:child_process';
import { writeFileSync } from 'node:fs';
import { createHuman, installCursor } from './human.mjs';

const env = process.env;
for (const name of ['ORCH_E2E_URL', 'ORCH_E2E_RUN_ID', 'ORCH_E2E_EVIDENCE_DIR', 'ORCH_E2E_XDOTOOL', 'DISPLAY']) {
  if (!env[name]) throw new Error(`${name} is required`);
}
const baseURL = env.ORCH_E2E_URL;
const runId = env.ORCH_E2E_RUN_ID;
const evidenceDir = env.ORCH_E2E_EVIDENCE_DIR;
if (!/^[a-z0-9-]+$/.test(runId)) throw new Error('ORCH_E2E_RUN_ID must be a DNS label');
const [width, height] = (env.ORCH_E2E_SCREEN ?? '1280x900').split('x').map(Number);
const videoPath = `${evidenceDir}/uc09-review.mp4`;
const applicationName = 'Deployment Review';

// Reading pauses: 2-4 s on every screen a reviewer must read.
const READ = 3000;
const LONG_READ = 4000;
const SHORT_READ = 2000;

const fixtures = await prepareFixtures();
writeFileSync(`${evidenceDir}/fixtures.json`, JSON.stringify(fixtures, null, 2), { mode: 0o600 });
console.log(`fixtures (offscreen API setup, not UI evidence): ${JSON.stringify(fixtures)}`);

const browser = await chromium.launch({
  headless: false,
  args: ['--window-position=0,0', `--window-size=${width},${height}`, '--test-type', '--no-first-run', '--password-store=basic'],
  ignoreDefaultArgs: ['--enable-automation'],
});
const context = await browser.newContext({ viewport: null });
context.setDefaultTimeout(20_000);
await context.addInitScript(installCursor);
const marks = [];
const requestedURLs = [];
let recorder;
let startedAt;
try {
  const page = await context.newPage();
  page.on('request', (item) => requestedURLs.push(item.url()));
  const h = createHuman(page);
  const x11 = x11Input();
  await page.goto(`${baseURL}/ui/sign-in`);
  await expect(page.getByLabel('Username')).toBeVisible();
  await page.waitForTimeout(1500);
  x11.focusBrowser();
  recorder = await startRecording();
  startedAt = Date.now();
  await page.waitForTimeout(1500);
  const mark = (label) => marks.push({ label, seconds: Number(((Date.now() - startedAt) / 1000).toFixed(1)) });

  // 1. Sign in with the seeded test account; the password field stays masked.
  mark('sign-in-page');
  await h.showCursor();
  await h.pause(SHORT_READ);
  await h.type(page.getByLabel('Username'), 'developer');
  const password = page.getByLabel('Password');
  await expect(password).toHaveAttribute('type', 'password');
  await h.type(password, 'test-password');
  mark('password-masked');
  await h.pause(SHORT_READ);
  await h.click(page.getByRole('button', { name: 'Sign in' }));
  await expect(page.getByRole('heading', { name: 'Your applications' })).toBeVisible();
  mark('applications');
  await h.pause(READ);

  // 2. Application -> selected Environment -> recent deployments.
  await h.click(page.locator('button.application-card').filter({ hasText: applicationName }));
  await expect(page.getByRole('heading', { name: applicationName, level: 1 })).toBeVisible();
  const recent = page.locator('section.content-panel').filter({ has: page.getByRole('heading', { name: 'Recent deployments' }) });
  await expect(recent.locator('button.deployment-entry')).toHaveCount(3);
  await expect(recent).toContainText('Latest results for staging.');
  await h.moveTo(recent.getByRole('heading', { name: 'Recent deployments' }));
  mark('staging-recent-deployments');
  await h.pause(LONG_READ);

  // 3. Switch Environment: production has no deployments.
  await h.click(page.getByRole('button', { name: /^Production/ }));
  await expect(recent).toContainText('No deployments yet in production.');
  await expect(recent.locator('button.deployment-entry')).toHaveCount(0);
  await h.moveTo(recent.getByText('No deployments yet in production.'));
  mark('production-empty-recent');
  await h.pause(READ);
  await h.click(page.getByRole('button', { name: 'View all' }));
  await expect(page.getByRole('heading', { name: 'Deployments', level: 1 })).toBeVisible();
  await expect(page.getByText('No deployments in this Environment yet.')).toBeVisible();
  await h.moveTo(page.getByText('No deployments in this Environment yet.'));
  mark('production-empty-history');
  await h.pause(READ);
  await h.click(page.getByRole('button', { name: `← ${applicationName}` }));
  await h.click(page.getByRole('button', { name: /^Staging/ }));
  await expect(recent.locator('button.deployment-entry')).toHaveCount(3);
  await h.pause(SHORT_READ);

  // 4. Staging history, newest first.
  await h.click(page.getByRole('button', { name: 'View all' }));
  await expect(page.getByRole('heading', { name: 'Deployments', level: 1 })).toBeVisible();
  const entries = page.locator('button.deployment-entry');
  await expect(entries).toHaveCount(3);
  await expect(entries.nth(0)).toContainText('backend');
  await expect(entries.nth(0).locator('.status')).toHaveText('SUCCEEDED');
  await expect(entries.nth(1)).toContainText('broken');
  await expect(entries.nth(1).locator('.status')).toHaveText('FAILED');
  await expect(entries.nth(2)).toContainText('backend');
  await expect(entries.nth(2).locator('.status')).toHaveText('SUCCEEDED');
  mark('staging-history');
  for (let index = 0; index < 3; index += 1) {
    await h.moveTo(entries.nth(index));
    await h.pause(1200);
  }
  await h.pause(SHORT_READ);

  // 5. Status filter: click the native select, choose with the keyboard.
  const filter = page.getByLabel('Filter deployment status');
  await h.click(filter);
  await h.pause(900);
  x11.keys(['Down', 'Down']);
  await h.pause(700);
  x11.keys(['Return']);
  await expect(filter).toHaveValue('FAILED');
  await expect.poll(() => requestedURLs.some((url) => url.endsWith('/deployments?status=FAILED'))).toBe(true);
  await expect(entries).toHaveCount(1);
  await expect(entries.first()).toContainText('broken');
  await h.moveTo(entries.first());
  mark('history-filter-failed');
  await h.pause(READ);
  await h.click(filter);
  await h.pause(900);
  x11.keys(['Up']);
  await h.pause(700);
  x11.keys(['Return']);
  await expect(filter).toHaveValue('SUCCEEDED');
  await expect.poll(() => requestedURLs.some((url) => url.endsWith('/deployments?status=SUCCEEDED'))).toBe(true);
  await expect(entries).toHaveCount(2);
  mark('history-filter-succeeded');
  await h.pause(READ);
  await h.click(filter);
  await h.pause(900);
  x11.keys(['Up']);
  await h.pause(700);
  x11.keys(['Return']);
  await expect(filter).toHaveValue('ALL');
  await expect(entries).toHaveCount(3);
  await h.pause(SHORT_READ);

  // 6. Failed detail: persisted reason, no invented plan or workload rows.
  await h.click(entries.nth(1));
  await expect(page).toHaveURL(new RegExp(`/deployments/${fixtures.failedDeploymentId}$`));
  await expect(page.getByRole('heading', { name: 'broken · deploy' })).toBeVisible();
  const reason = page.locator('.form-error[role="alert"]');
  await expect(reason).toContainText(fixtures.failureReason);
  await h.moveTo(reason);
  mark('failed-detail-reason');
  await h.pause(LONG_READ);
  await expect(page.getByText('No workload status was recorded.')).toBeVisible();
  await expect(page.getByText('No provision plan was saved for this deployment.')).toBeVisible();
  await h.moveTo(page.getByText('No provision plan was saved for this deployment.'));
  mark('failed-detail-no-plan');
  await h.pause(READ);

  // 7. Newest successful detail (the redeploy): workload snapshot, redacted
  // outputs, graph and provision batches.
  await h.click(page.getByRole('button', { name: '← staging deployment history' }));
  await expect(entries).toHaveCount(3);
  await h.click(entries.nth(0));
  await expect(page).toHaveURL(new RegExp(`/deployments/${fixtures.secondDeploymentId}$`));
  await expect(page.getByRole('heading', { name: /^backend · (deploy|update)$/ })).toBeVisible();
  await reviewSuccessfulDetail(h, fixtures.secondDigest, 'redeploy');

  // 8. Older successful detail keeps its own workload snapshot.
  await h.click(page.getByRole('button', { name: '← staging deployment history' }));
  await expect(entries).toHaveCount(3);
  await h.click(entries.nth(2));
  await expect(page).toHaveURL(new RegExp(`/deployments/${fixtures.firstDeploymentId}$`));
  await expect(page.getByText(fixtures.firstDigest, { exact: true })).toBeVisible();
  await expect(page.getByText(fixtures.secondDigest, { exact: true })).toHaveCount(0);
  await h.moveTo(page.getByText(fixtures.firstDigest, { exact: true }));
  mark('first-detail-own-digest');
  await h.pause(LONG_READ);

  // 9. Typed address: the same Deployment under production is out of scope.
  const outOfScope = `${baseURL}/ui/applications/${fixtures.applicationId}/environments/production/deployments/${fixtures.firstDeploymentId}`;
  x11.keys(['ctrl+l']);
  await h.pause(900);
  x11.type(outOfScope);
  mark('address-typed');
  await h.pause(SHORT_READ);
  x11.keys(['Return']);
  await expect(page).toHaveURL(outOfScope);
  await expect(page.getByRole('heading', { name: 'Deployment not found' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Retry' })).toHaveCount(0);
  await h.showCursor();
  await h.moveTo(page.getByRole('heading', { name: 'Deployment not found' }));
  mark('scoped-not-found');
  await h.pause(LONG_READ);

  // 10. Sign out.
  await h.click(page.getByRole('button', { name: 'Sign out' }));
  await expect(page.getByLabel('Username')).toBeVisible();
  mark('signed-out');
  await h.pause(READ);

  console.log(`PASS: uc09 video application=${fixtures.applicationId} marks=${marks.length}`);
} finally {
  // Always finalize the MP4 and close the browser, even after a UI failure; a
  // recorder failure still fails the run.
  try {
    if (recorder) await stopRecording(recorder);
  } finally {
    writeFileSync(`${evidenceDir}/marks.json`, JSON.stringify(marks, null, 2));
    await browser.close();
  }
}

async function reviewSuccessfulDetail(h, digest, label) {
  const { page } = h;
  const workloads = page.locator('section.content-panel').filter({ has: page.getByRole('heading', { name: 'Workloads' }) });
  await expect(workloads).toContainText('READY');
  await expect(workloads.getByText(digest, { exact: true })).toBeVisible();
  await h.moveTo(workloads.getByText(digest, { exact: true }));
  marks.push({ label: `${label}-workload-digest`, seconds: Number(((Date.now() - startedAt) / 1000).toFixed(1)) });
  await h.pause(READ);

  const outputs = page.getByText(/password=\*\*\*redacted\*\*\*/);
  await expect(outputs).toBeVisible();
  await expect(page.locator('body')).not.toContainText('resolvedInputs');
  await h.moveTo(outputs);
  marks.push({ label: `${label}-redacted-outputs`, seconds: Number(((Date.now() - startedAt) / 1000).toFixed(1)) });
  await h.pause(LONG_READ);

  const plan = page.locator('section.content-panel').filter({ has: page.getByRole('heading', { name: 'Provision plan' }) });
  await expect(plan).toContainText(/\d+ graph nodes/);
  await expect(plan.getByRole('heading', { name: 'Nodes' })).toBeVisible();
  await h.moveTo(plan.getByRole('heading', { name: 'Nodes' }));
  marks.push({ label: `${label}-graph`, seconds: Number(((Date.now() - startedAt) / 1000).toFixed(1)) });
  await h.pause(READ);
  const batches = plan.getByRole('heading', { name: 'Provision batches' });
  await expect(batches).toBeVisible();
  await expect(plan.locator('ol > li').first()).toBeVisible();
  await h.moveTo(plan.locator('ol > li').first());
  marks.push({ label: `${label}-batches`, seconds: Number(((Date.now() - startedAt) / 1000).toFixed(1)) });
  await h.pause(READ);
}

// Offscreen fixture setup through the API; see the file header.
async function prepareFixtures() {
  const api = await request.newContext({ baseURL });
  try {
    const call = async (method, path, data) => {
      const response = await api.fetch(`/api/v1${path}`, { method, data });
      const body = await response.json().catch(() => ({}));
      return { status: response.status(), body };
    };
    const signIn = await call('POST', '/auth/sign-in', { username: 'developer', password: 'test-password' });
    if (signIn.status !== 200) throw new Error(`fixture sign-in ${signIn.status}`);
    const created = await call('POST', '/applications', { name: applicationName, subdomain: runId });
    if (created.status !== 201) throw new Error(`fixture application ${created.status} ${JSON.stringify(created.body)}`);
    const applicationId = created.body.application.key;
    const samples = await call('GET', '/score-samples');
    const backend = samples.body.samples.backend;
    const deploy = async (workloadId, score, scoreBefore) => call('POST', '/deployments', {
      applicationKey: applicationId, environmentKey: 'staging', workloadId, actor: 'uc09-video-fixture', score,
      ...(scoreBefore ? { scoreBefore } : {}),
    });
    const first = await deploy('backend', backend);
    if (first.status !== 201 || first.body.status !== 'SUCCEEDED') throw new Error(`fixture first deploy ${first.status}`);
    await new Promise((resolve) => setTimeout(resolve, 1100));
    const failed = await deploy('broken', {
      apiVersion: 'score.dev/v1b1', metadata: { name: 'broken' },
      containers: { main: { image: 'example.invalid/broken:test' } },
      resources: { unsupported: { type: 'missing-resource-type' } },
    });
    if (failed.status === 201) throw new Error('fixture planning failure unexpectedly succeeded');
    await new Promise((resolve) => setTimeout(resolve, 1100));
    const changed = structuredClone(backend);
    changed.containers.main.variables = { ...changed.containers.main.variables, UC09_REVISION: runId };
    const second = await deploy('backend', changed, backend);
    if (second.status !== 201 || second.body.status !== 'SUCCEEDED') throw new Error(`fixture redeploy ${second.status}`);

    const scope = `/applications/${applicationId}/environments/staging/deployments`;
    const history = await call('GET', scope);
    const failedRecord = history.body.deployments.find((item) => item.workloadId === 'broken');
    const digest = async (id) => (await call('GET', `${scope}/${id}`)).body.workloads[0].manifestDigest;
    const fixture = {
      applicationId,
      firstDeploymentId: first.body.deploymentId,
      secondDeploymentId: second.body.deploymentId,
      failedDeploymentId: failedRecord.id,
      failureReason: failedRecord.failureReason,
      firstDigest: await digest(first.body.deploymentId),
      secondDigest: await digest(second.body.deploymentId),
    };
    if (!fixture.failureReason || fixture.firstDigest === fixture.secondDigest) throw new Error(`invalid fixtures ${JSON.stringify(fixture)}`);
    return fixture;
  } finally {
    await api.dispose();
  }
}

// Native X11 input for browser chrome (address bar) and the native <select>
// popup, which Playwright's page-level keyboard cannot reach.
function x11Input() {
  const run = (args) => execFileSync(env.ORCH_E2E_XDOTOOL, args, { encoding: 'utf8' }).trim();
  let window;
  return {
    focusBrowser() {
      const ids = run(['search', '--onlyvisible', '--class', 'chrom']).split('\n').filter(Boolean);
      if (!ids.length) throw new Error('no visible Chromium window on the X display');
      window = ids[ids.length - 1];
      run(['windowfocus', '--sync', window]);
    },
    keys(sequence) {
      run(['windowfocus', '--sync', window]);
      run(['key', '--clearmodifiers', '--delay', '250', ...sequence]);
    },
    type(text) {
      run(['windowfocus', '--sync', window]);
      run(['type', '--clearmodifiers', '--delay', '70', text]);
    },
  };
}

// Starts ffmpeg and fails fast when it cannot open the display or exits early.
async function startRecording() {
  const ffmpeg = spawn('ffmpeg', ['-hide_banner', '-loglevel', 'error', '-n', '-f', 'x11grab', '-draw_mouse', '0', '-framerate', '15', '-video_size', `${width}x${height}`, '-i', env.DISPLAY,
    '-c:v', 'libx264', '-preset', 'veryfast', '-crf', '26', '-pix_fmt', 'yuv420p', '-movflags', '+faststart', videoPath], { stdio: ['pipe', 'ignore', 'inherit'] });
  ffmpeg.exitPromise = new Promise((resolve) => {
    ffmpeg.once('error', (error) => resolve({ error }));
    ffmpeg.once('exit', (code, signal) => resolve({ code, signal }));
  });
  const early = await Promise.race([ffmpeg.exitPromise, new Promise((resolve) => setTimeout(() => resolve(undefined), 1500))]);
  if (early) throw new Error(`ffmpeg failed to start: ${early.error?.message ?? `code ${early.code} signal ${early.signal}`}`);
  return ffmpeg;
}

// Sends ffmpeg's interactive quit key so it finalizes the MP4 index. A
// recorder that died mid-run or needed a forced kill fails the run.
async function stopRecording(ffmpeg) {
  let forced = false;
  if (ffmpeg.exitCode === null && ffmpeg.signalCode === null) {
    ffmpeg.stdin.end('q');
  }
  const timer = setTimeout(() => { forced = true; ffmpeg.kill('SIGKILL'); }, 30_000);
  const result = await ffmpeg.exitPromise;
  clearTimeout(timer);
  if (forced || result.error || result.code !== 0) {
    throw new Error(`ffmpeg did not finish cleanly: ${result.error?.message ?? `code ${result.code} signal ${result.signal}`}${forced ? ' (forced)' : ''}`);
  }
}
