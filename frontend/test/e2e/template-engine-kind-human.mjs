// Human-paced recording: Definition-selected score-k8s rendering ("template
// engine") on the existing kind cluster. Invoked by
// backend/test/integration/template-engine-playwright-kind.sh.
//
// Flow, all product mutations through the browser UI:
//   Developer creates the Application and Variables & Secrets (including
//   literal values with $ / ${...} / {{...}}), Platform Engineer registers a
//   score-k8s workload renderer Definition scoped to this Application and
//   staging, Developer enters both workloads, Preview shows renderer
//   provenance, Deploy applies through the real Kubernetes adapter. Read-only
//   kubectl/API calls then compare live objects to the renderer's contract.
/* global document -- page.evaluate callbacks run in the browser. */
import { expect } from '@playwright/test';
import { createHash, randomBytes } from 'node:crypto';
import { execFileSync, spawn } from 'node:child_process';
import { createServer } from 'node:net';
import { readFileSync, writeFileSync } from 'node:fs';
import { acceptanceApp, addWorkload, createApplication, createHuman, installCursor, putKeys, reviewAcceptancePage, TYPE_DELAY } from './human.mjs';
import { launchHeaded, nativeSelect, screenSize, startRecording, stopRecording, x11Input } from './video.mjs';

const env = process.env;
for (const name of ['ORCH_E2E_URL', 'ORCH_E2E_RUN_ID', 'ORCH_E2E_KUBE_CONTEXT', 'ORCH_E2E_EVIDENCE_DIR', 'ORCH_E2E_NAMESPACE_FILE', 'ORCH_E2E_XDOTOOL', 'ORCH_E2E_SCORE_K8S_LOG', 'DISPLAY']) {
  if (!env[name]) throw new Error(`${name} is required`);
}
const baseURL = env.ORCH_E2E_URL;
const runId = env.ORCH_E2E_RUN_ID;
const kubeContext = env.ORCH_E2E_KUBE_CONTEXT;
const evidenceDir = env.ORCH_E2E_EVIDENCE_DIR;
if (!/^[a-z0-9-]+$/.test(runId)) throw new Error('ORCH_E2E_RUN_ID must be a DNS label');
const { width, height } = screenSize(env);
const videoPath = `${evidenceDir}/template-engine-review.mp4`;
const KEY_SCREEN = 8000;

// Literal Application variables: resolved values must reach the Deployment
// byte for byte (no substitution, no template evaluation). An empty value
// cannot be entered in the UI (Save needs a value); covered by CLI tests.
const LITERALS = {
  LIT_DOLLAR: '$HOME',
  LIT_BRACE: '${HOME}',
  LIT_ESCAPED: '$${HOME}',
  LIT_MIXED: 'price=$5 x$${y} $$',
  LIT_TEMPLATE: '{{ .Values.name }}',
};

const kubectl = (...args) => execFileSync('kubectl', ['--context', kubeContext, ...args], { encoding: 'utf8' });
const kubectlJSON = (...args) => JSON.parse(kubectl(...args, '-o', 'json'));

const browser = await launchHeaded({ width, height });
const context = await browser.newContext({ viewport: null });
context.setDefaultTimeout(30_000);
await context.addInitScript(installCursor);
const marks = [];
const observations = {};
let recorder;
let startedAt;
let forward;
try {
  const page = await context.newPage();
  const x11 = x11Input(env.ORCH_E2E_XDOTOOL);
  const h = createHuman(page, { selectNative: nativeSelect(x11) });
  await page.goto(`${baseURL}/ui/sign-in`);
  await expect(page.getByLabel('Username')).toBeVisible();
  await page.waitForTimeout(1500);
  x11.focusBrowser();
  recorder = await startRecording({ display: env.DISPLAY, width, height, videoPath });
  startedAt = Date.now();
  await page.waitForTimeout(1500);
  const mark = (label) => marks.push({ label, seconds: Number(((Date.now() - startedAt) / 1000).toFixed(1)) });

  const secret = randomBytes(32).toString('hex');
  const app = acceptanceApp(runId, secret, createHash('sha256').update(secret).digest('hex'));
  const noSecretOnScreen = async () => expect(await page.evaluate((value) => document.body.innerText.includes(value), secret), 'secret value on screen').toBe(false);
  const signInAs = async (username) => {
    await h.showCursor();
    await h.pause(1500);
    await h.type(page.getByLabel('Username'), username);
    await h.type(page.getByLabel('Password'), 'test-password');
    await h.click(page.getByRole('button', { name: 'Sign in' }));
    await expect(page.getByRole('heading', { name: /applications|resource types|resource definitions/i }).first()).toBeVisible();
    await h.pause(1500);
  };
  const signOut = async () => {
    await h.click(page.getByRole('button', { name: 'Sign out' }));
    await expect(page.getByLabel('Username')).toBeVisible();
    await h.pause(1500);
  };

  // 1. Developer: Application + Variables & Secrets (config, secret, literals).
  mark('developer-sign-in');
  await signInAs('developer');
  const applicationName = `Template engine ${runId}`;
  const appId = await createApplication(h, applicationName, runId);
  const namespace = `app-${appId}-staging`;
  writeFileSync(env.ORCH_E2E_NAMESPACE_FILE, namespace, { mode: 0o600 });
  mark('application-created');
  const literalKeys = Object.entries(LITERALS).map(([name, value]) => ({ kind: 'variable', name, value, delay: 110 }));
  await putKeys(h, applicationName, [...app.keys.map((key) => ({ ...key, delay: TYPE_DELAY })), ...literalKeys]);
  await noSecretOnScreen();
  mark('settings-saved');
  await signOut();

  // 2. Platform Engineer: register the renderer Definition for this app only.
  mark('platform-sign-in');
  await signInAs('platform-engineer');
  await h.click(page.getByRole('link', { name: /Mẫu dựng ứng dụng/ }));
  await expect(page.getByRole('heading', { name: 'Mẫu dựng ứng dụng', level: 1 })).toBeVisible();
  await h.pause(2000);
  const definitionKey = `render-${runId}`.slice(0, 63);
  await h.type(page.getByLabel('ID mẫu', { exact: true }), definitionKey);
  // The bundle ID is typed explicitly (T03: no installed-bundle list until T18).
  await h.type(page.getByLabel('ID bundle dựng ứng dụng', { exact: true }), 'score-k8s-internal-v1');
  await h.pause(3000);
  const criteria = page.locator('.criteria-editor');
  await h.click(criteria.getByRole('radio', { name: /Tùy chỉnh nâng cao/ }));
  await h.type(page.getByLabel('Điều kiện 1 ID ứng dụng'), appId);
  await h.type(page.getByLabel('Điều kiện 1 ID môi trường'), 'staging');
  await h.pause(3000);
  await h.click(page.getByRole('button', { name: 'Đăng ký mẫu dựng ứng dụng' }));
  await expect(page.getByRole('status').filter({ hasText: `Đã đăng ký mẫu dựng ứng dụng ${definitionKey}` })).toBeVisible();
  await expect(page.locator('.catalog-entry').filter({ hasText: definitionKey })).toContainText('Bundle: score-k8s-internal-v1 · 1 điều kiện áp dụng');
  await h.moveTo(page.locator('.catalog-entry').filter({ hasText: definitionKey }));
  mark('definition-registered');
  await h.pause(KEY_SCREEN);
  // Read-only: the stored Definition is scoped to this Application/staging.
  const listed = await page.request.get(`${baseURL}/api/v1/resource-definitions`);
  expect(listed.ok()).toBe(true);
  const stored = (await listed.json()).resourceDefinitions?.find((item) => item.key === definitionKey);
  expect(stored?.driverType).toBe('score-k8s');
  expect(stored?.criteria).toEqual([{ app_id: appId, env_id: 'staging' }]);
  await signOut();

  // 3. Developer: workloads (backend with DB + literals + resources).
  mark('developer-sign-in-2');
  await signInAs('developer');
  await h.click(page.getByRole('button', { name: new RegExp(applicationName) }));
  await expect(page.getByRole('heading', { name: applicationName })).toBeVisible();
  await h.pause(1500);
  const [backend, frontend] = app.workloads;
  backend.resources = { cpuRequest: '50m', memoryRequest: '64Mi', cpuLimit: '200m', memoryLimit: '128Mi' };
  for (const key of Object.keys(LITERALS)) backend.bindings.push({ name: key, source: 'Application variable', key });
  frontend.resources = { cpuLimit: '100m', memoryLimit: '96Mi' };
  const backendScore = await addWorkload(h, backend);
  expect(JSON.stringify(backendScore).includes(secret), 'secret value in Score').toBe(false);
  writeFileSync(`${evidenceDir}/backend-score.json`, JSON.stringify(backendScore, null, 2));
  mark('backend-saved');
  await addWorkload(h, frontend);
  mark('frontend-saved');

  // 4. Preview: renderer provenance, then Deploy.
  await h.click(page.getByRole('button', { name: 'Preview changes' }));
  const preview = page.getByLabel('Deployment preview');
  await expect(preview).toContainText('2 workload(s) affected', { timeout: 120_000 });
  for (const name of ['backend', 'frontend']) {
    await expect(preview.locator('li').filter({ hasText: new RegExp(`^${name} `) })).toContainText(`score-k8s 0.15.0 (${definitionKey})`);
    await expect(preview.locator('li').filter({ hasText: new RegExp(`^${name} `) })).not.toContainText('built-in Kubernetes');
  }
  observations.previewProvenance = await preview.locator('li').allInnerTexts();
  await h.moveTo(preview);
  mark('preview-provenance');
  await h.pause(KEY_SCREEN);
  await h.click(page.getByRole('button', { name: 'Deploy these changes' }));
  const result = page.getByLabel('Deployment result');
  await expect(result).toContainText('Deploy succeeded', { timeout: 600_000 });
  for (const name of ['backend', 'frontend']) await expect(result.locator('li').filter({ hasText: new RegExp(`^${name} · [a-z]+: succeeded`) })).toHaveCount(1);
  await h.moveTo(result);
  mark('deploy-succeeded');
  await h.pause(KEY_SCREEN);
  await noSecretOnScreen();

  // 5. Live cluster vs renderer contract (read-only kubectl).
  // Independent witness: the wrapper at -score-k8s logged every CLI call.
  const calls = readFileSync(env.ORCH_E2E_SCORE_K8S_LOG, 'utf8').split('\n').filter(Boolean);
  const generated = calls.filter((line) => / generate /.test(` ${line.split('\t')[1]} `) && line.includes(`--namespace ${namespace}`));
  expect(generated.length, 'score-k8s generate calls for this namespace').toBeGreaterThanOrEqual(2);
  expect(calls.some((line) => line.split('\t')[1].startsWith('init ')), 'score-k8s init call').toBe(true);
  observations.scoreK8sCalls = calls.map((line) => line.split('\t')[1]);
  const live = verifyLive(namespace, appId, secret, backend, frontend);
  writeFileSync(`${evidenceDir}/live-manifests.json`, JSON.stringify(live.summary, null, 2));
  observations.live = live.summary;
  observations.runtimeLiterals = runtimeLiterals(namespace);
  mark('live-verified');

  // 6. Open the deployed app, review the diagnostic PASS page.
  const port = await freePort();
  forward = spawn('kubectl', ['--context', kubeContext, '-n', namespace, 'port-forward', 'svc/frontend', `${port}:8080`], { stdio: ['ignore', 'pipe', 'pipe'] });
  let forwardLog = '';
  forward.stdout.on('data', (chunk) => { forwardLog += chunk.toString(); });
  forward.stderr.on('data', (chunk) => { forwardLog += chunk.toString(); });
  await expect.poll(() => forwardLog.includes(`Forwarding from 127.0.0.1:${port}`), { timeout: 30_000 }).toBe(true);
  const appURL = `http://127.0.0.1:${port}/`;
  x11.keys(['ctrl+l']);
  await h.pause(900);
  x11.type(appURL);
  await h.pause(2000);
  x11.keys(['Return']);
  await expect(page).toHaveURL(appURL);
  mark('app-opened');
  await reviewAcceptancePage(h, runId, secret);
  await noSecretOnScreen();
  mark('app-reviewed');
  await h.pause(KEY_SCREEN);

  writeFileSync(`${evidenceDir}/run.json`, JSON.stringify({ applicationId: appId, namespace, definitionKey, observations }, null, 2));
  console.log(`PASS: application=${appId} namespace=${namespace} definition=${definitionKey} marks=${marks.length}`);
} finally {
  if (forward && forward.exitCode === null && forward.signalCode === null) {
    forward.kill();
    await new Promise((resolve) => forward.once('exit', resolve));
  }
  try {
    if (recorder) await stopRecording(recorder);
  } finally {
    writeFileSync(`${evidenceDir}/marks.json`, JSON.stringify(marks, null, 2));
    await browser.close();
  }
}

// Compares live Deployments/Services with the renderer contract. Env values
// that are Secret references are reported by name/key only.
function verifyLive(ns, appId, secret, backend, frontend) {
  const summary = {};
  for (const [spec, expected] of [[backend, { cpu: '50m', memory: '64Mi', limits: { cpu: '200m', memory: '128Mi' } }], [frontend, { cpu: '100m', memory: '96Mi', limits: { cpu: '100m', memory: '96Mi' } }]]) {
    const name = spec.name;
    const deployment = kubectlJSON('-n', ns, 'get', 'deployment', name);
    const service = kubectlJSON('-n', ns, 'get', 'service', name);
    const raw = JSON.stringify(deployment);
    expect(raw.includes(secret), `${name}: secret bytes in Deployment`).toBe(false);
    // The Definition-selected renderer patches metadata back to the platform's
    // protected identity, so score-k8s invocation is proven by the wrapper log.
    expect(deployment.metadata.labels['app.kubernetes.io/managed-by'], `${name} managed-by`).toBe('orchestrator');
    expect(deployment.metadata.labels['app.kubernetes.io/name']).toBe(name);
    expect(service.metadata.labels['app.kubernetes.io/name']).toBe(name);
    // Protected identity.
    expect(deployment.metadata.name).toBe(name);
    expect(deployment.metadata.namespace).toBe(ns);
    expect(service.metadata.namespace).toBe(ns);
    for (const [key, value] of Object.entries(deployment.spec.selector.matchLabels)) expect(deployment.spec.template.metadata.labels[key]).toBe(value);
    expect(service.spec.selector).toEqual(deployment.spec.selector.matchLabels);
    const container = deployment.spec.template.spec.containers.find((item) => item.name === 'main');
    const envByName = Object.fromEntries((container.env ?? []).map((item) => [item.name, item]));
    summary[name] = {
      labels: deployment.metadata.labels,
      annotations: deployment.metadata.annotations,
      replicas: deployment.spec.replicas,
      readyReplicas: deployment.status.readyReplicas,
      resources: container.resources,
      probes: { readiness: container.readinessProbe ?? null, liveness: container.livenessProbe ?? null },
      ports: container.ports,
      service: { selector: service.spec.selector, ports: service.spec.ports },
      env: Object.fromEntries(Object.entries(envByName).map(([key, item]) => [key, item.valueFrom ? { secretKeyRef: { name: item.valueFrom.secretKeyRef?.name, key: item.valueFrom.secretKeyRef?.key } } : { value: item.value }])),
    };
    writeFileSync(`${evidenceDir}/live-manifests.json`, JSON.stringify(summary, null, 2));
    expect(container.image).toBe(spec.image);
    expect(container.ports.map((p) => p.containerPort)).toEqual([8080]);
    expect(service.spec.ports.map((p) => [p.port, String(p.targetPort)])).toEqual([[8080, '8080']]);
    expect(container.resources.requests).toEqual({ cpu: expected.cpu, memory: expected.memory });
    expect(container.resources.limits).toEqual(expected.limits);
    expect(deployment.status.readyReplicas).toBe(deployment.spec.replicas);
    if (name === 'backend') {
      // With VSO delivery every Application variable (also non-secret ones) is a
      // synced Kubernetes Secret key; the delivered value is checked at runtime.
      for (const key of Object.keys(LITERALS)) expect(envByName[key]?.valueFrom?.secretKeyRef?.key, `literal ${key} secretKeyRef`).toBe(`main_${key}`);
      expect(envByName.ACCEPTANCE_SECRET.valueFrom.secretKeyRef.key).toBeTruthy();
      expect(envByName.PGPASSWORD.valueFrom.secretKeyRef.name).toBeTruthy();
      expect(envByName.PGPASSWORD.value).toBeUndefined();
      expect(envByName.PGHOST.value).toMatch(/\./);
      expect(envByName.PGPORT.value).toBe('5432');
    } else {
      expect(envByName.BACKEND_URL.value).toMatch(/^http:\/\/backend(\.|:)/);
    }
  }
  const kinds = kubectlJSON('-n', ns, 'get', 'statefulset,pvc').items.map((item) => `${item.kind}/${item.metadata.name}`);
  summary.databaseObjects = kinds;
  expect(kinds.length, 'shared PostgreSQL StatefulSet/PVC present, not rendered by score-k8s').toBeGreaterThan(0);
  return { summary, appId };
}

// Reads the running process environment (LIT_ keys only) through an
// ephemeral debug container in the run-owned namespace.
function runtimeLiterals(ns) {
  const pods = kubectlJSON('-n', ns, 'get', 'pods', '-l', 'app.kubernetes.io/name=backend').items;
  const pod = pods.find((item) => item.status.phase === 'Running');
  expect(pod, 'running backend pod').toBeTruthy();
  // LIT_ lines only; the process list is evidence that the target namespace is shared.
  const script = "echo procs=$(ls -d /proc/[0-9]* | wc -l); for p in /proc/[0-9]*; do tr '\\0' '\\n' < $p/environ 2>/dev/null | grep '^LIT_'; done | sort -u";
  let seen = {};
  const attempts = [];
  for (const profile of ['general', 'sysadmin']) {
    const container = `lit-${profile}`;
    let out = kubectl('-n', ns, 'debug', `pod/${pod.metadata.name}`, '-c', container, '--image=busybox:1.37', '--target=main', `--profile=${profile}`, '--quiet', '--attach', '--', 'sh', '-c', script);
    if (!out.trim()) {
      // The ephemeral container may finish before attach; read its log instead.
      for (let attempt = 0; attempt < 15 && !out.trim(); attempt += 1) {
        try { out = kubectl('-n', ns, 'logs', pod.metadata.name, '-c', container); } catch { /* not started yet */ }
        if (!out.trim()) execFileSync('sleep', ['1']);
      }
    }
    const lines = out.split('\n').filter(Boolean);
    seen = Object.fromEntries(lines.filter((line) => line.startsWith('LIT_')).map((line) => [line.slice(0, line.indexOf('=')), line.slice(line.indexOf('=') + 1)]));
    attempts.push({ profile, procs: lines.find((line) => line.startsWith('procs=')), literalKeys: Object.keys(seen) });
    if (Object.keys(seen).length) break;
  }
  writeFileSync(`${evidenceDir}/runtime-debug.json`, JSON.stringify(attempts, null, 2));
  expect(seen).toEqual(LITERALS);
  return Object.keys(seen);
}

async function freePort() {
  const server = createServer();
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  const port = server.address().port;
  await new Promise((resolve) => server.close(resolve));
  return port;
}
