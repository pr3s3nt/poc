// Human-paced UC-07 update/remove recording. A headed Chromium runs on an
// Xvfb display and ffmpeg records the whole window, including the real
// address bar. Invoked by backend/test/integration/uc07-video-local.sh.
//
// No API fixtures: the backend starts with only its seeded test accounts and
// catalog. Every recorded step is a UI action: mouse moves with a visible
// cursor, values typed key by key, native <select> popups and the native
// delete confirmation are answered with X11 keys (xdotool). No fill,
// selectOption or dialog.accept/dismiss is used.
import { chromium, expect } from '@playwright/test';
import { execFileSync, spawn } from 'node:child_process';
import { writeFileSync } from 'node:fs';
import { createApplication, createHuman, installCursor } from './human.mjs';

const env = process.env;
for (const name of ['ORCH_E2E_URL', 'ORCH_E2E_RUN_ID', 'ORCH_E2E_EVIDENCE_DIR', 'ORCH_E2E_XDOTOOL', 'DISPLAY']) {
  if (!env[name]) throw new Error(`${name} is required`);
}
const baseURL = env.ORCH_E2E_URL;
const runId = env.ORCH_E2E_RUN_ID;
const evidenceDir = env.ORCH_E2E_EVIDENCE_DIR;
if (!/^[a-z0-9-]+$/.test(runId)) throw new Error('ORCH_E2E_RUN_ID must be a DNS label');
const [width, height] = (env.ORCH_E2E_SCREEN ?? '1440x900').split('x').map(Number);
const videoPath = `${evidenceDir}/uc07-review.mp4`;
const applicationName = 'Update Remove Demo';
const subdomain = `uc07-${runId.slice(-6)}`;

const READ = 3000;
const LONG_READ = 4500;
const SHORT_READ = 2000;

const browser = await chromium.launch({
  headless: false,
  args: ['--window-position=0,0', `--window-size=${width},${height}`, '--test-type', '--no-first-run', '--password-store=basic'],
  ignoreDefaultArgs: ['--enable-automation'],
});
const context = await browser.newContext({ viewport: null });
context.setDefaultTimeout(30_000);
await context.addInitScript(installCursor);
const marks = [];
const dialogs = [];
let recorder;
let startedAt;
try {
  const page = await context.newPage();
  // A listener keeps Playwright from auto-dismissing dialogs; the native
  // dialog stays on screen until an X11 key answers it.
  page.on('dialog', (dialog) => dialogs.push(dialog.message()));
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
  const row = (name) => page.locator('.table-row').filter({ has: page.locator('.workload-name', { hasText: new RegExp(`${name}$`) }) });

  // Native <select>: click opens the popup, typed text picks the option.
  async function chooseNative(select, text) {
    await h.click(select);
    await h.pause(700);
    x11.type(text);
    await h.pause(500);
    x11.keys(['Return']);
    await h.pause(700);
  }
  // Native confirm: the click opens the dialog, a real key answers it.
  async function answerConfirm(button, key) {
    const opened = new Promise((resolve) => page.once('dialog', resolve));
    const clicking = h.click(button);
    await opened;
    await h.pause(2200);
    x11.keys([key]);
    await clicking;
  }
  async function addWorkload(spec) {
    await h.click(page.getByRole('button', { name: '+ Add workload' }));
    await expect(page.getByRole('button', { name: 'Enter on form' })).toBeVisible();
    await h.type(page.getByLabel('Workload name', { exact: true }), spec.name);
    await h.type(page.getByLabel('Image', { exact: true }), spec.image);
    if (spec.database) {
      await h.click(page.getByRole('button', { name: '+ Add resource' }));
      await h.type(page.getByLabel('Resource alias', { exact: true }), 'db');
      await chooseNative(page.getByLabel('Resource type', { exact: true }), 'postgres');
      await expect(page.getByLabel('Resource type', { exact: true })).toHaveValue('postgres');
      await h.type(page.getByLabel('Resource class', { exact: true }), 'default');
      await h.type(page.getByLabel('Resource database', { exact: true }), spec.database);
      await h.type(page.getByLabel('Resource username', { exact: true }), spec.database);
    }
    await h.click(page.getByRole('button', { name: '+ Add port' }));
    await h.type(page.getByLabel('Service port name', { exact: true }), 'http');
    await h.type(page.locator('input[aria-label="Service port"]'), '8080');
    await h.type(page.getByLabel('Container target port', { exact: true }), '8080');
    await h.pause(1200);
    await h.click(page.getByRole('button', { name: 'Save pending workload' }));
    await expect(page.getByRole('heading', { name: 'Workloads' })).toBeVisible();
    await expect(row(spec.name)).toContainText('Pending change');
    await h.pause(1200);
  }
  async function previewAndDeploy(expectLine, expectResult) {
    await h.click(page.getByRole('button', { name: 'Preview changes' }));
    const panel = page.getByLabel('Deployment preview');
    for (const line of expectLine) await expect(panel).toContainText(line);
    await h.moveTo(panel);
    await h.pause(LONG_READ);
    await h.click(page.getByRole('button', { name: 'Deploy these changes' }));
    const result = page.getByLabel('Deployment result');
    await expect(result).toContainText('Deploy succeeded', { timeout: 120_000 });
    for (const line of expectResult) await expect(result).toContainText(line);
    await h.moveTo(result);
    await h.pause(READ);
    return result;
  }

  // 1. Sign in and create the Application in the UI.
  mark('sign-in-page');
  await h.showCursor();
  await h.pause(SHORT_READ);
  await h.type(page.getByLabel('Username'), 'developer');
  await h.type(page.getByLabel('Password'), 'test-password');
  await h.click(page.getByRole('button', { name: 'Sign in' }));
  await expect(page.getByRole('heading', { name: 'Your applications' })).toBeVisible();
  const applicationId = await createApplication(h, applicationName, subdomain);
  mark('application-created');

  // 2. Two workloads through the form; api owns a PostgreSQL dependency.
  await addWorkload({ name: 'api', image: 'example.invalid/api:v1', database: 'shop' });
  await addWorkload({ name: 'web', image: 'example.invalid/web:v1' });
  mark('drafts-saved');
  await previewAndDeploy(['2 workload(s) affected', 'api · deploy', 'web · deploy'], ['api · deploy: succeeded', 'web · deploy: succeeded']);
  await expect(row('api')).toContainText('Ready');
  await expect(row('web')).toContainText('Ready');
  mark('initial-deploy');

  // 3. Update web: new image and a CPU request. api stays untouched.
  await h.click(row('web').getByRole('button', { name: 'Edit' }));
  await expect(page.getByRole('heading', { name: 'Edit web' })).toBeVisible();
  await h.type(page.getByLabel('Image', { exact: true }), 'example.invalid/web:v2', { replace: true });
  await h.type(page.getByLabel('CPU request', { exact: true }), '100m');
  mark('web-edited');
  await h.pause(1500);
  await h.click(page.getByRole('button', { name: 'Save pending workload' }));
  await expect(row('web')).toContainText('Pending change');
  await expect(row('api')).toContainText('Ready');
  await h.moveTo(row('api'));
  mark('api-unchanged-before-deploy');
  await h.pause(READ);
  const updated = await previewAndDeploy(['1 workload(s) affected', 'web · update'], ['web · update: succeeded']);
  await expect(page.getByLabel('Deployment preview')).toHaveCount(0);
  expect(await updated.textContent()).not.toContain('api');
  await expect(row('web')).toContainText('Ready');
  await expect(row('api')).toContainText('Ready');
  await h.moveTo(row('api'));
  mark('web-updated-api-unchanged');
  await h.pause(READ);

  // 4. Delete web: cancel with Escape, confirm with Return, then Undo.
  await answerConfirm(row('web').getByRole('button', { name: 'Delete' }), 'Escape');
  await expect(row('web')).toContainText('Ready');
  mark('delete-cancelled');
  await h.pause(READ);
  await answerConfirm(row('web').getByRole('button', { name: 'Delete' }), 'Return');
  await expect(row('web')).toContainText('Pending deletion');
  await h.moveTo(row('web'));
  mark('delete-pending');
  await h.pause(READ);
  await h.click(row('web').getByRole('button', { name: 'Undo' }));
  await expect(row('web')).toContainText('Ready');
  await h.moveTo(row('web'));
  mark('delete-undone');
  await h.pause(READ);

  // 5. Remove api: its database becomes unreferenced, not destroyed.
  await answerConfirm(row('api').getByRole('button', { name: 'Delete' }), 'Return');
  await expect(row('api')).toContainText('Pending deletion');
  const removed = await previewAndDeploy(['api · remove', 'will become unreferenced (not destroyed)'], ['api · remove: succeeded']);
  mark('api-removed');
  await expect(row('api')).toHaveCount(0);
  await expect(row('web')).toContainText('Ready');
  await h.moveTo(row('web'));
  mark('web-remains');
  await h.pause(READ);

  // 6. History and the remove Deployment detail.
  await h.click(removed.getByRole('link', { name: 'view deployment' }));
  await expect(page.getByRole('heading', { name: 'api · remove' })).toBeVisible();
  await expect(page.locator('.status').first()).toHaveText('SUCCEEDED');
  await h.moveTo(page.getByRole('heading', { name: 'api · remove' }));
  mark('remove-detail');
  await h.pause(LONG_READ);
  await h.click(page.getByRole('button', { name: '← staging deployment history' }));
  await expect(page.getByRole('heading', { name: 'Deployments', level: 1 })).toBeVisible();
  const entries = page.locator('button.deployment-entry');
  await expect(entries).toHaveCount(4);
  await expect(entries.nth(0)).toContainText('api');
  await expect(entries.nth(0)).toContainText('remove');
  await expect(entries.nth(1)).toContainText('update');
  mark('history');
  for (let index = 0; index < 4; index += 1) {
    await h.moveTo(entries.nth(index));
    await h.pause(900);
  }
  await h.pause(SHORT_READ);

  // 7. Sign out.
  await h.click(page.getByRole('button', { name: 'Sign out' }));
  await expect(page.getByLabel('Username')).toBeVisible();
  mark('signed-out');
  await h.pause(READ);

  const expected = ['Mark web for deletion in staging?', 'Mark web for deletion in staging?', 'Mark api for deletion in staging?'];
  if (dialogs.length !== 3 || dialogs.some((text, i) => !text.startsWith(expected[i]))) throw new Error(`unexpected dialogs ${JSON.stringify(dialogs)}`);
  writeFileSync(`${evidenceDir}/run.json`, JSON.stringify({ applicationId, dialogs }, null, 2));
  console.log(`PASS: uc07 video application=${applicationId} marks=${marks.length}`);
} finally {
  try {
    if (recorder) await stopRecording(recorder);
  } finally {
    writeFileSync(`${evidenceDir}/marks.json`, JSON.stringify(marks, null, 2));
    await browser.close();
  }
}

// Native X11 input for select popups and the confirm dialog, which
// Playwright's page-level keyboard cannot reach in a headed window.
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
      run(['type', '--clearmodifiers', '--delay', '90', text]);
    },
  };
}

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

async function stopRecording(ffmpeg) {
  let forced = false;
  if (ffmpeg.exitCode === null && ffmpeg.signalCode === null) ffmpeg.stdin.end('q');
  const timer = setTimeout(() => { forced = true; ffmpeg.kill('SIGKILL'); }, 30_000);
  const result = await ffmpeg.exitPromise;
  clearTimeout(timer);
  if (forced || result.error || result.code !== 0) {
    throw new Error(`ffmpeg did not finish cleanly: ${result.error?.message ?? `code ${result.code} signal ${result.signal}`}${forced ? ' (forced)' : ''}`);
  }
}
