/* global window, document -- page.evaluate and addInitScript callbacks run in the browser. */
// Human-paced Playwright helpers shared by the recorded kind flows. Every
// action moves a visible cursor to its target and every form value is typed
// key by key, so a recording shows what a person would do.
import { expect } from '@playwright/test';

export const TYPE_DELAY = 85;
export const LONG_TYPE_DELAY = 30;

// Options: selectNative(h, select, label) operates a native <select> popup
// in headed recordings (see video.mjs); without it choose() uses selectOption.
export function createHuman(page, { selectNative } = {}) {
  const cursor = { x: 640, y: 400 };

  async function pause(milliseconds) {
    await page.waitForTimeout(milliseconds);
  }

  // Moves the real Playwright mouse in small steps so the injected cursor
  // glides to the target like a hand-held pointer.
  async function moveTo(locator) {
    await revealComfortably(locator);
    let box = await locator.boundingBox();
    if (!box) throw new Error('target has no bounding box');
    const hit = (point) => locator.evaluate((element, { px, py }) => element.contains(document.elementFromPoint(px, py)), point);
    if (!(await hit({ px: box.x + box.width / 2, py: box.y + box.height / 2 }))) {
      await locator.evaluate((element) => element.scrollIntoView({ block: 'center' }));
      await pause(300);
      box = await locator.boundingBox();
      if (!box) throw new Error('target has no bounding box');
    }
    const x = box.x + Math.min(box.width / 2, 40 + Math.random() * 20);
    const y = box.y + box.height / 2;
    const steps = Math.max(10, Math.round(Math.hypot(x - cursor.x, y - cursor.y) / 10));
    await page.mouse.move(x, y, { steps });
    cursor.x = x;
    cursor.y = y;
    await pause(250);
  }

  // Sticky headers and the editor's sticky action bar can cover an element
  // that is merely "in view", so targets near an edge are smooth-scrolled to
  // the middle and the helper waits until the layout stops moving.
  async function revealComfortably(locator) {
    await locator.scrollIntoViewIfNeeded();
    const moved = await locator.evaluate((element) => {
      const rect = element.getBoundingClientRect();
      if (rect.top >= 120 && rect.bottom <= window.innerHeight - 140) return false;
      element.scrollIntoView({ block: 'center', behavior: 'smooth' });
      return true;
    });
    if (!moved) return;
    let previous;
    for (let attempt = 0; attempt < 20; attempt += 1) {
      await pause(120);
      const box = await locator.boundingBox();
      if (previous && box && Math.abs(box.y - previous.y) < 1) break;
      previous = box;
    }
  }

  async function click(locator) {
    await expect(locator).toBeVisible();
    await expect(locator).toBeEnabled();
    await moveTo(locator);
    await page.mouse.down();
    await pause(90);
    await page.mouse.up();
    await pause(600);
  }

  async function type(locator, text, { delay = TYPE_DELAY, replace = false } = {}) {
    await click(locator);
    if (replace) {
      await page.keyboard.press('ControlOrMeta+A');
      await pause(300);
      await page.keyboard.press('Backspace');
      await pause(300);
    }
    await locator.pressSequentially(text, { delay });
    await pause(500);
  }

  // For long credential blobs a person pastes instead of typing; the value
  // arrives as one insertText event, like a clipboard paste.
  async function paste(locator, text) {
    await click(locator);
    await pause(400);
    await page.keyboard.insertText(text);
    await pause(700);
  }

  // Headless Chromium does not paint native <select> popups, so the cursor
  // clicks the control and the option is chosen by its visible label.
  async function choose(locator, label) {
    if (selectNative) return selectNative(human, locator, label);
    await expect(locator).toBeVisible();
    await moveTo(locator);
    await page.evaluate(() => window.__pwCursorPulse?.());
    await pause(400);
    await locator.selectOption({ label });
    await pause(700);
  }

  async function showCursor() {
    await page.mouse.move(cursor.x + 1, cursor.y + 1);
    await page.mouse.move(cursor.x, cursor.y);
  }

  // Reading pause scaled to the caption: about 60 ms per character, 2-6 s.
  async function read(text = '') {
    await pause(Math.min(6000, Math.max(2000, text.length * 60)));
  }

  const human = { page, pause, moveTo, click, type, paste, choose, showCursor, read };
  return human;
}

export async function signIn(h) {
  await h.showCursor();
  await h.pause(1500);
  await h.type(h.page.getByLabel('Username'), 'developer');
  await h.type(h.page.getByLabel('Password'), 'test-password');
  await h.click(h.page.getByRole('button', { name: 'Sign in' }));
  await expect(h.page.getByRole('heading', { name: /applications/i })).toBeVisible();
  await h.pause(1500);
}

// Sets the execution Connection of each listed Environment through
// Environment Settings (UC-01 ES-03..06) and returns to the Application.
// targets: { staging?: connectionKey, production?: connectionKey }. The option
// is found by its "(key)" label, the choice is typed by the real select popup,
// and the PUT reply plus the saved "Generation" state are awaited.
export async function setEnvironmentConnections(h, applicationName, targets) {
  const { page } = h;
  await h.click(page.getByRole('button', { name: 'Environment settings' }));
  await expect(page.getByRole('heading', { name: 'Environment settings', level: 1 })).toBeVisible();
  for (const [environment, key] of Object.entries(targets)) {
    const title = environment === 'staging' ? 'Staging' : 'Production';
    await h.click(page.getByRole('tab', { name: title }));
    const select = page.getByLabel(`Connection for ${title}`);
    await expect(select).toHaveValue('');
    const label = (await select.locator('option').allTextContents()).find((text) => text.includes(`(${key})`));
    if (!label) throw new Error(`connection ${key} is not offered for ${environment}`);
    await h.choose(select, label);
    await expect(select).toHaveValue(key);
    const saved = page.waitForResponse((response) => response.request().method() === 'PUT'
      && new URL(response.url()).pathname.endsWith(`/environments/${environment}/connection`));
    await h.click(page.getByRole('button', { name: 'Save connection' }));
    const response = await saved;
    if (!response.ok()) throw new Error(`setting ${environment} connection returned HTTP ${response.status()}`);
    await expect(page.getByText('Generation 0', { exact: true })).toBeVisible();
    await h.pause(1200);
  }
  await h.click(page.getByRole('button', { name: `← ${applicationName}` }));
  await h.pause(1200);
}

// Returns the Application id parsed from the detail page URL. By default both
// Environments are set to the seeded cluster Connection afterwards, which keeps
// the older flows that deploy to the default cluster unchanged; pass
// { connections: null } to leave both UNCONFIGURED, or an explicit map.
export async function createApplication(h, name, subdomain, { connections = { staging: 'internal-cluster', production: 'internal-cluster' } } = {}) {
  const { page } = h;
  await h.click(page.getByRole('button', { name: /create application|new application/i }).first());
  await h.type(page.getByLabel('Application name'), name);
  await h.type(page.getByLabel('Subdomain'), subdomain);
  await h.click(page.getByRole('button', { name: 'Create application' }));
  await expect(page.getByRole('heading', { name })).toBeVisible();
  await h.pause(1500);
  const appId = new URL(page.url()).pathname.match(/\/applications\/([^/]+)$/)?.[1];
  if (!appId || !/^[a-z0-9-]+$/.test(appId)) throw new Error(`invalid application id: ${appId}`);
  if (connections) await setEnvironmentConnections(h, name, connections);
  return appId;
}

// The Secret Store that new-Environment secret writes must use. There is no
// product fallback: the store registered for the run is named by the operator,
// by key or by name, in ORCH_E2E_SECRET_STORE.
export function configuredSecretStore(env = process.env) {
  return (env.ORCH_E2E_SECRET_STORE ?? '').trim();
}

// Selects a READY Secret Store for ONE Environment through Settings (UC-12 /
// ADR-012). Must be called on the Environment settings page. An Environment that
// already uses the wanted store is left alone; otherwise the select is chosen
// from its popup and the PUT reply is awaited.
export async function selectSecretStore(h, environment, store) {
  const { page } = h;
  if (!store) throw new Error('writing a secret needs a selected Secret Store: set ORCH_E2E_SECRET_STORE to the key or name of a READY store registered for this run');
  const title = environment === 'staging' ? 'Staging' : 'Production';
  await h.click(page.getByRole('tab', { name: title }));
  const select = page.getByLabel(`Secret store for ${title}`);
  await expect(select).toBeVisible();
  const label = (await select.locator('option').allTextContents()).find((text) => text.includes(`(${store})`) || text.startsWith(`${store} (`));
  if (!label) throw new Error(`secret store ${store} is not offered for ${environment}`);
  const selected = await select.inputValue();
  const wanted = await select.locator('option').evaluateAll((options, text) => options.find((option) => option.textContent === text)?.value, label);
  if (selected === wanted) return;
  await h.choose(select, label);
  await expect(select).toHaveValue(wanted);
  const saved = page.waitForResponse((response) => response.request().method() === 'PUT'
    && new URL(response.url()).pathname.endsWith(`/environments/${environment}/secret-store`));
  await h.click(page.getByRole('button', { name: 'Save secret store' }));
  const response = await saved;
  if (!response.ok()) throw new Error(`selecting the ${environment} secret store returned HTTP ${response.status()}`);
  await expect(page.getByRole('button', { name: 'Save secret store' })).toBeDisabled();
  await h.pause(1200);
}

// Opens Environment settings (staging), saves each entry and returns to the Application.
// Entries: { kind: 'variable' | 'secret', name, value, paste?, delay? }. Any
// secret entry first selects the configured Secret Store (ORCH_E2E_SECRET_STORE
// or options.secretStore); there is no implicit store.
// Each save waits for the PUT reply and the closed editor before the saved
// row is matched, so a typed-but-unsaved name never satisfies the check.
export async function putKeys(h, applicationName, entries, { secretStore = configuredSecretStore() } = {}) {
  const { page } = h;
  await h.click(page.getByRole('button', { name: 'Environment settings' }));
  await h.pause(1000);
  // A new Environment has no secret store: choose the run's configured one first.
  if (entries.some((entry) => entry.kind === 'secret')) await selectSecretStore(h, 'staging', secretStore);
  for (const entry of entries) {
    const section = page.locator(`section[aria-label="${entry.kind === 'secret' ? 'Secrets' : 'Environment variables'}"]`);
    await h.click(section.getByRole('button', { name: `+ Add ${entry.kind}` }));
    await h.type(page.getByLabel('Key name'), entry.name);
    const field = page.getByLabel(entry.kind === 'secret' ? 'New secret value' : 'Value');
    if (entry.paste) await h.paste(field, entry.value);
    else await h.type(field, entry.value, { delay: entry.delay ?? (entry.value.length > 32 ? LONG_TYPE_DELAY : TYPE_DELAY) });
    const saved = page.waitForResponse((response) => response.request().method() === 'PUT'
      && new URL(response.url()).pathname.endsWith(`/configuration/keys/${encodeURIComponent(entry.name)}`));
    await h.click(page.getByRole('button', { name: 'Save pending change' }));
    const response = await saved;
    if (!response.ok()) throw new Error(`saving ${entry.kind} ${entry.name} returned HTTP ${response.status()}`);
    await expect(page.getByLabel('Key name')).toHaveCount(0);
    const row = section.locator('.settings-table-row').filter({ has: page.locator('strong').getByText(entry.name, { exact: true }) });
    await expect(row).toHaveCount(1);
    if (entry.kind === 'secret') {
      await expect(row).toContainText('Configured');
      // Boolean form: a failure message must not echo the secret.
      expect(await section.evaluate((element, value) => element.textContent.includes(value), entry.value), 'secret value rendered').toBe(false);
    } else {
      await expect(row).toContainText(entry.value);
    }
    await h.pause(1200);
  }
  await h.click(page.getByRole('button', { name: `← ${applicationName}` }));
  await h.pause(1200);
}

// Enters one workload through the workload form. Spec: name, image,
// container? (default main), resource?, bindings[], port { name, port,
// targetPort }, publicPath?. Bindings with source 'Application variable' or
// 'Application secret' tick the existing key in the container's checklist
// (name !== key reveals the alias field); every other binding is one
// "Other sources" row, indexed among those rows only. Returns the Score sent
// by the successful Save.
export async function addWorkload(h, spec) {
  const { page } = h;
  const container = spec.container ?? 'main';
  await h.click(page.getByRole('button', { name: '+ Add workload' }));
  await expect(page.getByRole('button', { name: 'Enter on form' })).toBeVisible();
  await h.pause(1000);
  await h.type(page.getByLabel('Workload name', { exact: true }), spec.name);
  await h.type(page.getByLabel('Image', { exact: true }), spec.image);
  // Optional container resources: { cpuRequest, memoryRequest, cpuLimit, memoryLimit }.
  const quantities = [['CPU request', 'cpuRequest'], ['Memory request', 'memoryRequest'], ['CPU limit', 'cpuLimit'], ['Memory limit', 'memoryLimit']];
  for (const [label, field] of quantities) if (spec.resources?.[field]) await h.type(page.getByLabel(label, { exact: true }), spec.resources[field]);

  if (spec.resource) {
    await h.click(page.getByRole('button', { name: '+ Add resource' }));
    await h.type(page.getByLabel('Resource alias', { exact: true }), spec.resource.alias);
    await h.choose(page.getByLabel('Resource type', { exact: true }), spec.resource.type);
    await h.type(page.getByLabel('Resource class', { exact: true }), spec.resource.className);
    for (const [name, value] of Object.entries(spec.resource.params)) await h.type(page.getByLabel(`Resource ${name}`, { exact: true }), value);
  }

  const isKey = (binding) => binding.source === 'Application variable' || binding.source === 'Application secret';
  for (const binding of spec.bindings.filter(isKey)) {
    const group = page.getByRole('group', { name: `${binding.source === 'Application secret' ? 'Application secrets' : 'Application variables'} for ${container}`, exact: true });
    const box = group.getByRole('checkbox', { name: binding.key, exact: true });
    await h.click(box);
    await expect(box).toBeChecked();
    if (binding.name !== binding.key) {
      await h.click(group.getByRole('checkbox', { name: `Use a different container name for ${binding.key}`, exact: true }));
      await h.type(group.getByLabel(`Container name for ${binding.key}`, { exact: true }), binding.name, { replace: true });
    }
  }

  const otherRows = page.locator('.binding-row').filter({ has: page.getByLabel('Container variable name', { exact: true }) });
  for (const [index, binding] of spec.bindings.filter((binding) => !isKey(binding)).entries()) {
    await h.click(page.getByRole('button', { name: '+ Add other source' }));
    const row = otherRows.nth(index);
    await h.type(row.getByLabel('Container variable name', { exact: true }), binding.name);
    // A new row starts as Resource output; only another source is chosen.
    if (binding.source !== 'Resource output') await h.choose(row.getByLabel('Reference source', { exact: true }), binding.source);
    if (binding.alias) {
      await h.choose(row.getByLabel('Resource dependency', { exact: true }), binding.alias);
      await h.choose(row.getByLabel('Resource output', { exact: true }), binding.output);
    }
    if (binding.workload) {
      await h.choose(row.getByLabel('Service workload', { exact: true }), binding.workload);
      await h.choose(row.getByLabel('Service port', { exact: true }), binding.port);
    }
  }

  await h.click(page.getByRole('button', { name: '+ Add port' }));
  await h.type(page.getByLabel('Service port name', { exact: true }), spec.port.name);
  await h.type(page.locator('input[aria-label="Service port"]'), spec.port.port);
  await h.type(page.getByLabel('Container target port', { exact: true }), spec.port.targetPort);
  if (spec.publicPath) {
    await h.click(page.getByRole('button', { name: '+ Add public path' }));
    await h.type(page.getByLabel('Public path', { exact: true }), spec.publicPath);
    await h.choose(page.getByLabel('Public Service port', { exact: true }), `${spec.port.name} · ${spec.port.port}`);
  }
  await h.pause(1500);
  const saved = page.waitForResponse((response) => response.request().method() === 'PUT'
    && new URL(response.url()).pathname.endsWith(`/workloads/${encodeURIComponent(spec.name)}`));
  await h.click(page.getByRole('button', { name: 'Save pending workload' }));
  const response = await saved;
  if (!response.ok()) throw new Error(`saving workload ${spec.name} returned HTTP ${response.status()}`);
  await expect(page.getByRole('heading', { name: 'Workloads' })).toBeVisible();
  await h.pause(1500);
  return response.request().postDataJSON().score;
}

// mark(label), when given, records the 'preview' and 'deploy-succeeded' phases.
export async function previewAndDeploy(h, workloadNames, { mark } = {}) {
  const { page } = h;
  await h.click(page.getByRole('button', { name: 'Preview changes' }));
  const preview = page.getByLabel('Deployment preview');
  await expect(preview).toContainText(`${workloadNames.length} workload(s) affected`, { timeout: 120_000 });
  await h.moveTo(preview);
  mark?.('preview');
  await h.pause(4000);
  await h.click(page.getByRole('button', { name: 'Deploy these changes' }));
  const result = page.getByLabel('Deployment result');
  await expect(result).toContainText('Deploy succeeded', { timeout: 600_000 });
  // Result rows read "<workload> · <action>: <status>".
  for (const name of workloadNames) await expect(result.locator('li').filter({ hasText: new RegExp(`^${name} · [a-z]+: succeeded`) })).toHaveCount(1);
  await h.moveTo(result);
  mark?.('deploy-succeeded');
  await h.pause(5000);
}

// Configuration entries and workloads of the diagnostic acceptance app.
export function acceptanceApp(runId, secret, secretHash) {
  return {
    keys: [
      { kind: 'variable', name: 'ACCEPTANCE_CONFIG', value: 'acceptance-config-ok' },
      { kind: 'variable', name: 'ACCEPTANCE_SECRET_SHA256', value: secretHash },
      { kind: 'secret', name: 'ACCEPTANCE_SECRET', value: secret },
    ],
    workloads: [
      {
        name: 'backend',
        image: `acceptance-backend:${runId}`,
        resource: { alias: 'db', type: 'postgres', className: 'default', params: { database: 'acceptance', username: 'acceptance' } },
        bindings: [
          { name: 'PGHOST', source: 'Resource output', alias: 'db', output: 'host' },
          { name: 'PGPORT', source: 'Resource output', alias: 'db', output: 'port' },
          { name: 'PGDATABASE', source: 'Resource output', alias: 'db', output: 'database' },
          { name: 'PGUSER', source: 'Resource output', alias: 'db', output: 'username' },
          { name: 'PGPASSWORD', source: 'Resource output', alias: 'db', output: 'password · secret' },
          { name: 'ACCEPTANCE_CONFIG', source: 'Application variable', key: 'ACCEPTANCE_CONFIG' },
          { name: 'ACCEPTANCE_SECRET_SHA256', source: 'Application variable', key: 'ACCEPTANCE_SECRET_SHA256' },
          { name: 'ACCEPTANCE_SECRET', source: 'Application secret', key: 'ACCEPTANCE_SECRET' },
        ],
        port: { name: 'http', port: '8080', targetPort: '8080' },
      },
      {
        name: 'frontend',
        image: `acceptance-frontend:${runId}`,
        bindings: [{ name: 'BACKEND_URL', source: 'Workload Service', workload: 'backend', port: 'http' }],
        port: { name: 'http', port: '8080', targetPort: '8080' },
        publicPath: '/',
      },
    ],
  };
}

// Reviews the deployed acceptance page and submits one job. No worker is
// deployed, so the job stays pending; the row still proves the
// frontend -> backend -> PostgreSQL write and read.
export async function reviewAcceptancePage(h, runId, secret) {
  const { page } = h;
  await expect(page.getByRole('heading', { name: 'Acceptance application' })).toBeVisible();
  await h.showCursor();
  await h.pause(1500);
  for (const name of ['backend connection', 'environment', 'secret', 'database']) {
    const row = page.locator(`[data-check="${name}"]`);
    await expect(row).toContainText('PASS');
    await h.moveTo(row);
    await h.pause(900);
  }
  const body = await page.evaluate(async () => (await fetch('/api/checks')).json());
  expect(body.checks).toEqual({ environment: true, secret: true, database: true });
  expect(JSON.stringify(body).includes(secret), 'secret value in /api/checks').toBe(false);

  const payload = `hello from ${runId}`;
  await h.type(page.locator('input[name="payload"]'), payload, { replace: true });
  await h.click(page.getByRole('button', { name: 'Submit job' }));
  const job = page.locator('#jobs tr.job').filter({ hasText: payload });
  await expect(job).toBeVisible();
  await h.showCursor();
  await h.moveTo(job);
  await h.pause(6000);
}

// Asserts the injected pointer exists and moves with the real mouse, so a
// recording always shows where the human-paced click lands.
export async function expectVisibleCursor(page) {
  await expect.poll(() => page.evaluate(() => typeof window.__pwCursorPulse === 'function' && Boolean(document.querySelector('svg path[fill="#111"]')))).toBe(true);
}

// Runs in every page before its scripts. Draws an arrow that follows real
// mouse events and a ripple on each mouse press; both ignore pointer events.
export function installCursor() {
  if (window.top !== window) return;
  let x = -100;
  let y = -100;
  let arrow;
  const ripple = () => {
    const ring = document.createElement('div');
    Object.assign(ring.style, { position: 'fixed', left: `${x - 14}px`, top: `${y - 14}px`, width: '28px', height: '28px', borderRadius: '50%', border: '3px solid #e5534b', pointerEvents: 'none', zIndex: '2147483646' });
    document.documentElement.appendChild(ring);
    ring.animate([{ transform: 'scale(0.4)', opacity: 1 }, { transform: 'scale(1.6)', opacity: 0 }], { duration: 450, easing: 'ease-out' }).onfinish = () => ring.remove();
  };
  const install = () => {
    if (arrow) return;
    arrow = document.createElement('div');
    arrow.innerHTML = '<svg width="24" height="24" viewBox="0 0 24 24"><path d="M3 2 L3 20 L8 15.5 L11.5 22.5 L14.5 21 L11 14 L18 14 Z" fill="#111" stroke="#fff" stroke-width="1.5" stroke-linejoin="round"/></svg>';
    Object.assign(arrow.style, { position: 'fixed', left: '0', top: '0', width: '24px', height: '24px', pointerEvents: 'none', zIndex: '2147483647', transform: `translate(${x - 3}px, ${y - 2}px)` });
    document.documentElement.appendChild(arrow);
  };
  document.addEventListener('mousemove', (event) => {
    x = event.clientX;
    y = event.clientY;
    install();
    arrow.style.transform = `translate(${x - 3}px, ${y - 2}px)`;
  }, true);
  document.addEventListener('mousedown', ripple, true);
  window.__pwCursorPulse = ripple;
}
