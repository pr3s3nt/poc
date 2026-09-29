// Recorded self-hosting flow on kind: a host Orchestrator deploys the
// Orchestrator images (API + Web Console) through its own Web Console, then
// the in-cluster Orchestrator deploys the diagnostic acceptance app.
// A headed Chromium runs on an Xvfb display and ffmpeg records the whole
// window, including tabs and the address bar.
// Invoked by backend/test/integration/self-host-playwright-kind.sh.
import { chromium, expect } from '@playwright/test';
import { createHash, randomBytes } from 'node:crypto';
import { spawn } from 'node:child_process';
import { request } from 'node:http';
import { readFileSync, writeFileSync } from 'node:fs';
import { acceptanceApp, addWorkload, createApplication, createHuman, installCursor, previewAndDeploy, putKeys, reviewAcceptancePage, signIn } from './human.mjs';

const env = process.env;
const hostURL = env.ORCH_E2E_URL;
const runId = env.ORCH_E2E_RUN_ID;
const evidenceDir = env.ORCH_E2E_EVIDENCE_DIR;
const traefikPort = Number(env.ORCH_E2E_TRAEFIK_PORT);
const subdomain = env.ORCH_E2E_SELF_SUBDOMAIN;
const [width, height] = (env.ORCH_E2E_SCREEN ?? '1280x900').split('x').map(Number);
for (const name of ['ORCH_E2E_URL', 'ORCH_E2E_EVIDENCE_DIR', 'ORCH_E2E_TRAEFIK_PORT', 'ORCH_E2E_SELF_SUBDOMAIN', 'ORCH_E2E_KUBE_CONTEXT', 'ORCH_E2E_CLUSTER', 'ORCH_E2E_VAULT_ADDR', 'ORCH_E2E_KUBECONFIG_B64_FILE', 'ORCH_E2E_VAULT_TOKEN_FILE', 'ORCH_E2E_SELF_NAMESPACE_FILE', 'ORCH_E2E_NAMESPACE_FILE', 'DISPLAY']) {
  if (!env[name]) throw new Error(`${name} is required`);
}
if (!/^[a-z0-9-]+$/.test(runId ?? '') || !/^[a-z0-9-]+$/.test(subdomain)) throw new Error('ORCH_E2E_RUN_ID and ORCH_E2E_SELF_SUBDOMAIN must be DNS labels');

const selfHost = `staging.${subdomain}.example.com`;
const appHost = `staging.${runId}.example.com`;
const videoPath = `${evidenceDir}/self-host.mp4`;

const browser = await chromium.launch({
  headless: false,
  args: ['--window-position=0,0', `--window-size=${width},${height}`, `--host-resolver-rules=MAP *.example.com 127.0.0.1:${traefikPort}`, '--test-type'],
  ignoreDefaultArgs: ['--enable-automation'],
});
const browserContext = await browser.newContext({ viewport: null });
browserContext.setDefaultTimeout(30_000);
await browserContext.addInitScript(installCursor);
let recorder;
try {
  const hostTab = await browserContext.newPage();
  await hostTab.goto(`${hostURL}/ui/sign-in`);
  await hostTab.waitForTimeout(1000);
  recorder = startRecording();
  await hostTab.waitForTimeout(1500);

  // Part 1: the host Orchestrator deploys the Orchestrator itself.
  const host = createHuman(hostTab);
  await signIn(host);
  const selfName = 'Orchestrator';
  const selfAppId = await createApplication(host, selfName, subdomain);
  writeFileSync(env.ORCH_E2E_SELF_NAMESPACE_FILE, `app-${selfAppId}-staging`, { mode: 0o600 });
  const kubeconfigB64 = readFileSync(env.ORCH_E2E_KUBECONFIG_B64_FILE, 'utf8').trim();
  const vaultToken = readFileSync(env.ORCH_E2E_VAULT_TOKEN_FILE, 'utf8').trim();
  const selfVariables = {
    ORCHESTRATOR_KUBE_CONTEXT: env.ORCH_E2E_KUBE_CONTEXT,
    ORCHESTRATOR_CLUSTER: env.ORCH_E2E_CLUSTER,
    ORCHESTRATOR_VAULT_ADDR: env.ORCH_E2E_VAULT_ADDR,
    ORCHESTRATOR_VAULT_AGENT_ADDR: env.ORCH_E2E_VAULT_ADDR,
    ORCHESTRATOR_VAULT_DELIVERY: 'vso',
  };
  const selfSecrets = { ORCHESTRATOR_KUBECONFIG_B64: kubeconfigB64, ORCHESTRATOR_VAULT_TOKEN: vaultToken };
  await putKeys(host, selfName, [
    ...Object.entries(selfVariables).map(([name, value]) => ({ kind: 'variable', name, value })),
    ...Object.entries(selfSecrets).map(([name, value]) => ({ kind: 'secret', name, value, paste: true })),
  ]);
  await addWorkload(host, {
    name: 'backend',
    image: `orchestrator-backend:${runId}`,
    bindings: [
      ...Object.keys(selfVariables).map((name) => ({ name, source: 'Application variable', key: name })),
      ...Object.keys(selfSecrets).map((name) => ({ name, source: 'Application secret', key: name })),
    ],
    port: { name: 'http', port: '8080', targetPort: '8080' },
  });
  await addWorkload(host, {
    name: 'frontend',
    image: `orchestrator-frontend:${runId}`,
    bindings: [{ name: 'BACKEND_URL', source: 'Workload Service', workload: 'backend', port: 'http' }],
    port: { name: 'http', port: '8080', targetPort: '8080' },
    publicPath: '/',
  });
  await previewAndDeploy(host, ['backend', 'frontend']);

  // Part 2: the in-cluster Orchestrator, reached through Traefik by host name.
  await waitForRoute(selfHost, '/api/v1/healthz');
  const selfTab = await browserContext.newPage();
  await selfTab.goto(`http://${selfHost}/`);
  await expect(selfTab.getByLabel('Username')).toBeVisible();
  const inner = createHuman(selfTab);
  await signIn(inner);
  const applicationName = `Acceptance ${runId}`;
  const appId = await createApplication(inner, applicationName, runId);
  writeFileSync(env.ORCH_E2E_NAMESPACE_FILE, `app-${appId}-staging`, { mode: 0o600 });
  const secret = randomBytes(32).toString('hex');
  const app = acceptanceApp(runId, secret, createHash('sha256').update(secret).digest('hex'));
  await putKeys(inner, applicationName, app.keys);
  for (const workload of app.workloads) await addWorkload(inner, workload);
  await previewAndDeploy(inner, app.workloads.map((workload) => workload.name));

  // Part 3: the deployed acceptance app on its public route.
  await waitForRoute(appHost, '/healthz');
  const appTab = await browserContext.newPage();
  await appTab.goto(`http://${appHost}/`);
  await reviewAcceptancePage(createHuman(appTab), runId, secret);
  console.log(`PASS: self-host application=${selfAppId} url=http://${selfHost}/ acceptance application=${appId} checks=backend,environment,secret,database,job-submit`);
} finally {
  if (recorder) await stopRecording(recorder);
  await browser.close();
}

// Polls Traefik with the route's Host header until the backend answers 200,
// so the browser does not record a pre-readiness error page.
async function waitForRoute(host, path) {
  await expect.poll(() => new Promise((resolve) => {
    const req = request({ host: '127.0.0.1', port: traefikPort, path, headers: { Host: host }, timeout: 5000 }, (res) => { res.resume(); resolve(res.statusCode); });
    req.on('error', () => resolve(0));
    req.on('timeout', () => { req.destroy(); resolve(0); });
    req.end();
  }), { timeout: 180_000, intervals: [2000] }).toBe(200);
}

function startRecording() {
  const ffmpeg = spawn('ffmpeg', ['-hide_banner', '-loglevel', 'error', '-y', '-f', 'x11grab', '-draw_mouse', '0', '-framerate', '15', '-video_size', `${width}x${height}`, '-i', env.DISPLAY,
    '-c:v', 'libx264', '-preset', 'veryfast', '-crf', '28', '-pix_fmt', 'yuv420p', '-movflags', '+faststart', videoPath], { stdio: ['pipe', 'ignore', 'inherit'] });
  return ffmpeg;
}

// Sends ffmpeg's interactive quit key so it finalizes the MP4 index.
async function stopRecording(ffmpeg) {
  if (ffmpeg.exitCode !== null) return;
  const exited = new Promise((resolve) => ffmpeg.once('exit', resolve));
  ffmpeg.stdin.end('q');
  const timer = setTimeout(() => ffmpeg.kill('SIGINT'), 30_000);
  await exited;
  clearTimeout(timer);
}
