// Human-paced UC-05 Score Preview recording. A headed Chromium runs on an
// Xvfb display and ffmpeg records the whole window, including the real
// address bar. Invoked by backend/test/integration/uc05-video-local.sh.
//
// There is no API fixture setup: the backend starts with only its seeded test
// accounts and catalog. Every recorded step is a real UI action. The Playwright
// mouse moves the injected cursor to each target, values are typed key by key,
// and the native Action <select> is driven through X11 keys (xdotool).
// The UC-16 workload that makes the no-change preview possible is created and
// deployed through the Web Console on camera (fake adapters, no runtime).
import { chromium, expect } from '@playwright/test';
import { execFileSync, spawn } from 'node:child_process';
import { writeFileSync } from 'node:fs';
import { addWorkload, createApplication, createHuman, installCursor, LONG_TYPE_DELAY, previewAndDeploy } from './human.mjs';

const env = process.env;
for (const name of ['ORCH_E2E_URL', 'ORCH_E2E_RUN_ID', 'ORCH_E2E_EVIDENCE_DIR', 'ORCH_E2E_XDOTOOL', 'DISPLAY']) {
  if (!env[name]) throw new Error(`${name} is required`);
}
const baseURL = env.ORCH_E2E_URL;
const runId = env.ORCH_E2E_RUN_ID;
const evidenceDir = env.ORCH_E2E_EVIDENCE_DIR;
if (!/^[a-z0-9-]+$/.test(runId)) throw new Error('ORCH_E2E_RUN_ID must be a DNS label');
const [width, height] = (env.ORCH_E2E_SCREEN ?? '1440x900').split('x').map(Number);
const videoPath = `${evidenceDir}/uc05-review.mp4`;
const applicationName = 'Score Preview Demo';
const subdomain = `preview-${runId.slice(-6)}`;
const previewRunId = 'demo-run-1';

const READ = 3000;
const LONG_READ = 4500;
const SHORT_READ = 2000;

const apiScore = [
  'apiVersion: score.dev/v1b1',
  'metadata:',
  '  name: api',
  'containers:',
  '  main:',
  '    image: example.invalid/api:v1',
  '    variables:',
  '      PGHOST: ${resources.db.host}',
  '    resources:',
  '      requests:',
  '        cpu: 50m',
  'service:',
  '  ports:',
  '    http:',
  '      port: 8080',
  'resources:',
  '  db:',
  '    type: postgres',
  '    params:',
  '      database: shop',
  '      username: shop',
].join('\n');

// Exactly the Score the UC-16 form saves for the "web" workload below.
const webScore = [
  'apiVersion: score.dev/v1b1',
  'metadata:',
  '  name: web',
  'containers:',
  '  main:',
  '    image: example.invalid/web:v1',
  'service:',
  '  ports:',
  '    http:',
  '      port: 8080',
  '      targetPort: 8080',
].join('\n');

const browser = await chromium.launch({
  headless: false,
  args: ['--window-position=0,0', `--window-size=${width},${height}`, '--test-type', '--no-first-run', '--password-store=basic'],
  ignoreDefaultArgs: ['--enable-automation'],
});
const context = await browser.newContext({ viewport: null });
context.setDefaultTimeout(20_000);
await context.addInitScript(installCursor);
const marks = [];
const previewRequests = [];
let recorder;
let startedAt;
try {
  const page = await context.newPage();
  page.on('request', (item) => { if (item.url().endsWith('/score-preview')) previewRequests.push(item.url()); });
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

  // 1. Sign in with the seeded local test account.
  mark('sign-in-page');
  await h.showCursor();
  await h.pause(SHORT_READ);
  await h.type(page.getByLabel('Username'), 'developer');
  await h.type(page.getByLabel('Password'), 'test-password');
  await h.click(page.getByRole('button', { name: 'Sign in' }));
  await expect(page.getByRole('heading', { name: 'Your applications' })).toBeVisible();
  mark('applications');
  await h.pause(SHORT_READ);

  // 2. Create the Application in the UI.
  const applicationId = await createApplication(h, applicationName, subdomain);
  mark('application-created');

  // 3. Add and deploy one workload through the normal UC-16 flow, so that a
  // later Score Preview can show an update with no change.
  await addWorkload(h, { name: 'web', image: 'example.invalid/web:v1', bindings: [], port: { name: 'http', port: '8080', targetPort: '8080' } });
  mark('web-draft-saved');
  await previewAndDeploy(h, ['web']);
  mark('web-deployed');

  // 4. Open Preview Score for staging.
  await h.click(page.getByRole('button', { name: 'Preview Score' }));
  await expect(page.getByRole('heading', { name: 'Preview Score', level: 1 })).toBeVisible();
  await expect(page).toHaveURL(`${baseURL}/ui/applications/${applicationId}/environments/staging/preview`);
  await h.moveTo(page.getByRole('note'));
  mark('preview-page');
  await h.pause(LONG_READ);

  // 5. Invalid input: submit empty, then a Workload ID that does not match
  // the Score metadata name. Input stays in place after each error.
  const previewButton = page.getByRole('button', { name: 'Preview', exact: true });
  await h.click(previewButton);
  const alert = page.getByRole('alert');
  await expect(alert).toContainText('Workload ID is required.');
  await expect(alert).toContainText('Score after is required.');
  expect(previewRequests).toHaveLength(0);
  await h.moveTo(alert);
  mark('local-validation');
  await h.pause(READ);

  await h.type(page.getByLabel('Workload ID'), 'backend');
  await h.type(page.getByLabel('Run ID'), previewRunId);
  const after = page.getByLabel('Score after (YAML or JSON)');
  await h.type(after, apiScore, { delay: LONG_TYPE_DELAY });
  await h.click(previewButton);
  await expect(alert).toContainText('scoreAfter metadata.name "api" does not match workloadId "backend"');
  await expect(after).toHaveValue(apiScore);
  await h.moveTo(alert);
  mark('server-validation');
  await h.pause(LONG_READ);

  // 6. Fix the Workload ID and preview again.
  await h.type(page.getByLabel('Workload ID'), 'api', { replace: true });
  await h.click(previewButton);
  const result = page.getByRole('region', { name: 'Score preview result' });
  await expect(result).toBeVisible();
  await expect(alert).toHaveCount(0);
  await expect(result).toContainText('Preview · api · deploy');
  await expect(result).toContainText(previewRunId);
  await h.moveTo(result.getByRole('heading', { name: 'Preview · api · deploy' }));
  mark('preview-success');
  await h.pause(LONG_READ);

  // 7. Inspect every artifact.
  const panel = (name) => result.locator('section.content-panel').filter({ has: page.getByRole('heading', { name, exact: true }) });
  await expect(panel('Delta')).toContainText('add module api');
  await h.moveTo(panel('Delta').getByText('add module api'));
  mark('delta');
  await h.pause(READ);
  await expect(panel('Resource changes')).toContainText('postgres');
  await h.moveTo(panel('Resource changes').getByRole('heading', { name: /^New/ }));
  mark('resource-changes');
  await h.pause(READ);
  await expect(panel('Provision order').locator('ol > li').first()).toContainText('Batch 1');
  await h.moveTo(panel('Provision order').locator('ol > li').first());
  mark('provision-order');
  await h.pause(READ);
  await expect(panel('Matched Resource Definitions')).toContainText('postgres-internal-statefulset');
  await h.moveTo(panel('Matched Resource Definitions').getByText('postgres-internal-statefulset'));
  mark('matches');
  await h.pause(READ);
  await expect(panel('Resource Graph')).toContainText('→');
  await h.moveTo(panel('Resource Graph').getByRole('heading', { name: 'Dependencies (consumer → provider)' }));
  mark('graph');
  await h.pause(READ);
  await h.click(page.getByText('Delta document'));
  await expect(panel('Documents (sanitized view)').locator('pre').first()).toBeVisible();
  mark('delta-document');
  await h.pause(READ);
  await h.click(page.getByText('Candidate Deployment Set — sanitized view, not executable'));
  await expect(panel('Documents (sanitized view)').locator('pre').nth(1)).toContainText('"web"');
  await h.moveTo(panel('Documents (sanitized view)').locator('pre').nth(1));
  mark('candidate-set');
  await h.pause(LONG_READ);
  await expect(page.getByRole('button', { name: /deploy|save/i })).toHaveCount(0);

  // 8. Changing scope clears the old result.
  await h.click(page.getByRole('tab', { name: 'Production' }));
  await expect(page).toHaveURL(`${baseURL}/ui/applications/${applicationId}/environments/production/preview`);
  await expect(result).toHaveCount(0);
  await h.moveTo(page.getByRole('heading', { name: 'Preview Score', level: 1 }));
  mark('scope-cleared');
  await h.pause(READ);
  await h.click(page.getByRole('tab', { name: 'Staging' }));
  await expect(page).toHaveURL(`${baseURL}/ui/applications/${applicationId}/environments/staging/preview`);
  await h.pause(SHORT_READ);

  // 9. No change: update the deployed web workload with its current Score.
  const action = page.getByLabel('Action');
  await h.click(action);
  await h.pause(900);
  x11.keys(['Down']);
  await h.pause(700);
  x11.keys(['Return']);
  await expect(action).toHaveValue('update');
  await h.type(page.getByLabel('Workload ID'), 'web', { replace: true });
  await h.type(page.getByLabel('Score before (YAML or JSON)'), webScore, { delay: LONG_TYPE_DELAY });
  await h.type(after, webScore, { delay: LONG_TYPE_DELAY, replace: true });
  await h.click(previewButton);
  await expect(result).toContainText('Preview · web · update');
  const noChange = result.getByText(/No workload change/);
  await expect(noChange).toBeVisible();
  await h.moveTo(noChange);
  mark('no-change');
  await h.pause(LONG_READ);
  await h.moveTo(panel('Resource Graph').getByRole('heading', { name: 'Nodes' }));
  await h.pause(READ);

  // 10. Back to the Application: the preview saved no draft.
  await h.click(page.getByRole('button', { name: `← ${applicationName}` }));
  await expect(page.getByRole('heading', { name: applicationName, level: 1 })).toBeVisible();
  await expect(page.locator('.table-row')).toHaveCount(1);
  await expect(page.locator('.table-row')).toContainText('web');
  await expect(page.locator('.table-row')).toContainText('Ready');
  await h.moveTo(page.locator('.table-row'));
  mark('no-draft-saved');
  await h.pause(READ);

  // 11. Sign out.
  await h.click(page.getByRole('button', { name: 'Sign out' }));
  await expect(page.getByLabel('Username')).toBeVisible();
  mark('signed-out');
  await h.pause(READ);

  writeFileSync(`${evidenceDir}/run.json`, JSON.stringify({ applicationId, previewRequests: previewRequests.length }, null, 2));
  console.log(`PASS: uc05 video application=${applicationId} marks=${marks.length} previews=${previewRequests.length}`);
} finally {
  try {
    if (recorder) await stopRecording(recorder);
  } finally {
    writeFileSync(`${evidenceDir}/marks.json`, JSON.stringify(marks, null, 2));
    await browser.close();
  }
}

// Native X11 input for the native <select> popup, which Playwright's
// page-level keyboard cannot reach in a headed window.
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
