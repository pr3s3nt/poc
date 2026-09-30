// Human-paced UC-02/03/04 registration recording. Headed Chromium on Xvfb,
// ffmpeg records the whole window including the real address bar. Invoked by
// backend/test/integration/uc02-04-video-local.sh.
//
// No API fixtures: the fake-adapter backend has only its seeded accounts,
// catalog and connections. Every shown step is a UI action: the cursor moves
// to each target, text is typed key by key, native <select> popups are
// answered with X11 keys. Cluster verification is NOT faked: the recording
// shows the seeded Connection and a validation rejection only.
import { chromium, expect } from '@playwright/test';
import { execFileSync, spawn } from 'node:child_process';
import { writeFileSync } from 'node:fs';
import { createApplication, createHuman, installCursor, LONG_TYPE_DELAY } from './human.mjs';

const env = process.env;
for (const name of ['ORCH_E2E_URL', 'ORCH_E2E_RUN_ID', 'ORCH_E2E_EVIDENCE_DIR', 'ORCH_E2E_XDOTOOL', 'DISPLAY']) {
  if (!env[name]) throw new Error(`${name} is required`);
}
const baseURL = env.ORCH_E2E_URL;
const runId = env.ORCH_E2E_RUN_ID;
const evidenceDir = env.ORCH_E2E_EVIDENCE_DIR;
if (!/^[a-z0-9-]+$/.test(runId)) throw new Error('ORCH_E2E_RUN_ID must be a DNS label');
const [width, height] = (env.ORCH_E2E_SCREEN ?? '1440x900').split('x').map(Number);
const videoPath = `${evidenceDir}/uc02-04-review.mp4`;

const READ = 3000;
const LONG_READ = 4500;
const SHORT_READ = 2000;

const driverVariables = '{"image":"postgres:17-alpine","storage":"2Gi","namespace":"${resources[\'k8s-namespace.default#environments.@app.@env\'].outputs.name}"}';
const score = [
  'apiVersion: score.dev/v1b1',
  'metadata:',
  '  name: api',
  'containers:',
  '  main:',
  '    image: example.invalid/api:v1',
  'resources:',
  '  db:',
  '    type: postgres',
  '    class: fast',
  '    params:',
  '      database: shop',
  '      username: shop',
].join('\n');

const browser = await chromium.launch({
  headless: false,
  args: ['--window-position=0,0', `--window-size=${width},${height}`, '--test-type', '--no-first-run', '--password-store=basic'],
  ignoreDefaultArgs: ['--enable-automation'],
});
const context = await browser.newContext({ viewport: null });
context.setDefaultTimeout(30_000);
await context.addInitScript(installCursor);
const marks = [];
let recorder;
let startedAt;
try {
  const page = await context.newPage();
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
  async function chooseNative(select, text, value) {
    await h.click(select);
    await h.pause(700);
    x11.type(text);
    await h.pause(500);
    x11.keys(['Return']);
    await h.pause(700);
    await expect(select).toHaveValue(value);
  }
  async function signIn(user) {
    await h.showCursor();
    await h.type(page.getByLabel('Username'), user);
    await h.type(page.getByLabel('Password'), 'test-password');
    await h.click(page.getByRole('button', { name: 'Sign in' }));
    await expect(page.getByRole('heading', { name: 'Your applications' })).toBeVisible();
  }
  async function signOut() {
    await h.click(page.getByRole('button', { name: 'Sign out' }));
    await expect(page.getByLabel('Username')).toBeVisible();
  }
  const formError = () => page.locator('form .form-error[role="alert"]');

  // 1. Platform Engineer signs in.
  mark('sign-in');
  await signIn('platform-engineer');
  mark('platform-engineer-home');
  await h.pause(SHORT_READ);

  // 2. Resource Type: valid registration.
  await h.click(page.getByRole('link', { name: /Resource types/ }));
  await expect(page.getByRole('heading', { name: 'Resource types', level: 1 })).toBeVisible();
  await expect(page).toHaveURL(`${baseURL}/ui/platform/resource-types`);
  await h.type(page.getByLabel('Resource type ID'), 'cache');
  await h.click(page.getByRole('button', { name: '+ Add input' }));
  await h.type(page.getByLabel('Inputs 1 name'), 'size');
  await chooseNative(page.getByLabel('Inputs 1 type'), 'number', 'number');
  await h.click(page.locator('.catalog-field-row').first().getByLabel('Required'));
  await h.click(page.getByRole('button', { name: '+ Add output' }));
  await h.type(page.getByLabel('Outputs 1 name'), 'host');
  await h.pause(1200);
  await h.click(page.getByRole('button', { name: 'Register resource type' }));
  await expect(page.getByRole('status')).toHaveText('Registered resource type cache.');
  const typeList = page.locator('.catalog-entry').filter({ has: page.getByText('cache', { exact: true }) });
  await expect(typeList).toContainText('1 inputs · 1 outputs');
  await h.moveTo(typeList);
  mark('type-registered');
  await h.pause(READ);

  // 3. Duplicate: the same ID is rejected and the form is kept.
  await h.type(page.getByLabel('Resource type ID'), 'cache');
  await h.click(page.getByRole('button', { name: '+ Add output' }));
  await h.type(page.getByLabel('Outputs 1 name'), 'url');
  await h.click(page.getByRole('button', { name: 'Register resource type' }));
  await expect(formError()).toContainText('duplicate id');
  await expect(page.getByLabel('Resource type ID')).toHaveValue('cache');
  await h.moveTo(formError());
  mark('type-duplicate');
  await h.pause(READ);

  // 4. Invalid: two inputs with the same name.
  await h.type(page.getByLabel('Resource type ID'), 'broken', { replace: true });
  await h.click(page.getByRole('button', { name: '+ Add input' }));
  await h.type(page.getByLabel('Inputs 1 name'), 'size');
  await h.click(page.getByRole('button', { name: '+ Add input' }));
  await h.type(page.getByLabel('Inputs 2 name'), 'size');
  await h.click(page.getByRole('button', { name: 'Register resource type' }));
  await expect(formError()).toContainText('duplicate input "size"');
  await expect(page.getByLabel('Inputs 2 name')).toHaveValue('size');
  await h.moveTo(formError());
  mark('type-invalid');
  await h.pause(READ);

  // 5. Supported PostgreSQL Definition for class "fast".
  await h.click(page.getByRole('link', { name: /Resource definitions/ }));
  await expect(page.getByRole('heading', { name: 'Resource definitions', level: 1 })).toBeVisible();
  await expect(page).toHaveURL(`${baseURL}/ui/platform/resource-definitions`);
  await h.type(page.getByLabel('Definition ID'), 'postgres-fast');
  await chooseNative(page.getByLabel('Resource Type'), 'postgres', 'postgres');
  await h.type(page.getByLabel('Driver variables (JSON object)'), driverVariables, { replace: true, delay: LONG_TYPE_DELAY });
  await h.type(page.getByLabel('Criterion 1 Class'), 'fast');
  mark('definition-filled');
  await h.pause(SHORT_READ);
  await h.click(page.getByRole('button', { name: 'Register resource definition' }));
  await expect(page.getByRole('status')).toHaveText('Registered resource definition postgres-fast.');
  const definitionEntry = page.locator('.catalog-entry').filter({ has: page.getByText('postgres-fast', { exact: true }) });
  await expect(definitionEntry).toContainText('postgres · internal-k8s · kubernetes · 1 criteria');
  await h.moveTo(definitionEntry);
  mark('definition-registered');
  await h.pause(READ);

  // 6. Connections: seeded READY cluster with context; invalid ID rejected
  // before any verification (no cluster is contacted in this recording).
  await h.click(page.getByRole('link', { name: /Connections/ }));
  await expect(page.getByRole('heading', { name: 'Connections', level: 1 })).toBeVisible();
  const seeded = page.locator('.catalog-entry').first();
  await expect(seeded).toContainText('context');
  await expect(seeded).toContainText('READY');
  await h.moveTo(seeded);
  mark('connections-list');
  await h.pause(READ);
  await h.type(page.getByLabel('Connection ID'), 'Bad ID');
  await h.type(page.getByLabel('Cluster ID'), 'demo');
  await h.type(page.getByLabel('Kube context'), 'no-such-context');
  await h.click(page.getByRole('button', { name: 'Register cluster' }));
  await expect(formError()).toContainText('valid connection ID');
  await expect(page.getByLabel('Connection ID')).toHaveValue('Bad ID');
  await h.moveTo(formError());
  mark('connection-invalid');
  await h.pause(READ);
  await signOut();
  mark('platform-engineer-signed-out');

  // 7. Developer: new Application, Score Preview matches the new Definition.
  await signIn('developer');
  const applicationId = await createApplication(h, 'Catalog Consumer', `catalog-${runId.slice(-6)}`);
  mark('developer-application');
  await h.click(page.getByRole('button', { name: 'Preview Score' }));
  await expect(page.getByRole('heading', { name: 'Preview Score', level: 1 })).toBeVisible();
  await h.type(page.getByLabel('Workload ID'), 'api');
  await h.type(page.getByLabel('Run ID'), 'catalog-demo');
  await h.type(page.getByLabel('Score after (YAML or JSON)'), score, { delay: LONG_TYPE_DELAY });
  await h.click(page.getByRole('button', { name: 'Preview', exact: true }));
  const matches = page.locator('section.content-panel').filter({ has: page.getByRole('heading', { name: 'Matched Resource Definitions' }) });
  await expect(matches).toContainText('postgres-fast');
  await h.moveTo(matches.getByText('postgres-fast'));
  mark('preview-matches-new-definition');
  await h.pause(LONG_READ);

  // 8. Sign out.
  await signOut();
  mark('signed-out');
  await h.pause(READ);

  writeFileSync(`${evidenceDir}/run.json`, JSON.stringify({ applicationId }, null, 2));
  console.log(`PASS: uc02-04 video application=${applicationId} marks=${marks.length}`);
} finally {
  try {
    if (recorder) await stopRecording(recorder);
  } finally {
    writeFileSync(`${evidenceDir}/marks.json`, JSON.stringify(marks, null, 2));
    await browser.close();
  }
}

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
