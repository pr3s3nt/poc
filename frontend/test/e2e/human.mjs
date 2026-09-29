/* global window, document -- page.evaluate and addInitScript callbacks run in the browser. */
// Human-paced Playwright helpers shared by the recorded kind flows. Every
// action moves a visible cursor to its target and every form value is typed
// key by key, so a recording shows what a person would do.
import { expect } from '@playwright/test';

export const TYPE_DELAY = 85;
export const LONG_TYPE_DELAY = 30;

export function createHuman(page) {
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

  return { page, pause, moveTo, click, type, paste, choose, showCursor };
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

// Returns the Application id parsed from the detail page URL.
export async function createApplication(h, name, subdomain) {
  const { page } = h;
  await h.click(page.getByRole('button', { name: /create application|new application/i }).first());
  await h.type(page.getByLabel('Application name'), name);
  await h.type(page.getByLabel('Subdomain'), subdomain);
  await h.click(page.getByRole('button', { name: 'Create application' }));
  await expect(page.getByRole('heading', { name })).toBeVisible();
  await h.pause(1500);
  const appId = new URL(page.url()).pathname.match(/\/applications\/([^/]+)$/)?.[1];
  if (!appId || !/^[a-z0-9-]+$/.test(appId)) throw new Error(`invalid application id: ${appId}`);
  return appId;
}

// Opens Variables & Secrets, saves each entry and returns to the Application.
// Entries: { kind: 'variable' | 'secret', name, value, paste? }.
export async function putKeys(h, applicationName, entries) {
  const { page } = h;
  await h.click(page.getByRole('button', { name: 'Variables & Secrets' }));
  await h.pause(1000);
  for (const entry of entries) {
    const section = page.locator(`section[aria-label="${entry.kind === 'secret' ? 'Secrets' : 'Environment variables'}"]`);
    await h.click(section.getByRole('button', { name: `+ Add ${entry.kind}` }));
    await h.type(page.getByLabel('Key name'), entry.name);
    const field = page.getByLabel(entry.kind === 'secret' ? 'New secret value' : 'Value');
    if (entry.paste) await h.paste(field, entry.value);
    else await h.type(field, entry.value, { delay: entry.value.length > 32 ? LONG_TYPE_DELAY : TYPE_DELAY });
    await h.click(page.getByRole('button', { name: 'Save pending change' }));
    await expect(section).toContainText(entry.name);
    if (entry.kind === 'secret') await expect(section).not.toContainText(entry.value.slice(0, 16));
    await h.pause(1200);
  }
  await h.click(page.getByRole('button', { name: `← ${applicationName}` }));
  await h.pause(1200);
}

// Enters one workload through the workload form. Spec: name, image,
// resource?, bindings[], port { name, port, targetPort }, publicPath?.
export async function addWorkload(h, spec) {
  const { page } = h;
  await h.click(page.getByRole('button', { name: '+ Add workload' }));
  await expect(page.getByRole('button', { name: 'Enter on form' })).toBeVisible();
  await h.pause(1000);
  await h.type(page.getByLabel('Workload name', { exact: true }), spec.name);
  await h.type(page.getByLabel('Image', { exact: true }), spec.image);

  if (spec.resource) {
    await h.click(page.getByRole('button', { name: '+ Add resource' }));
    await h.type(page.getByLabel('Resource alias', { exact: true }), spec.resource.alias);
    await h.choose(page.getByLabel('Resource type', { exact: true }), spec.resource.type);
    await h.type(page.getByLabel('Resource class', { exact: true }), spec.resource.className);
    for (const [name, value] of Object.entries(spec.resource.params)) await h.type(page.getByLabel(`Resource ${name}`, { exact: true }), value);
  }

  const bindingRows = page.locator('.binding-row').filter({ has: page.getByLabel('Container variable name', { exact: true }) });
  for (const [index, binding] of spec.bindings.entries()) {
    await h.click(page.getByRole('button', { name: '+ Add binding' }));
    const row = bindingRows.nth(index);
    await h.type(row.getByLabel('Container variable name', { exact: true }), binding.name);
    await h.choose(row.getByLabel('Reference source', { exact: true }), binding.source);
    if (binding.key) await h.choose(row.getByLabel('Application key', { exact: true }), binding.key);
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
  await h.click(page.getByRole('button', { name: 'Save pending workload' }));
  await expect(page.getByRole('heading', { name: 'Workloads' })).toBeVisible();
  await h.pause(1500);
}

export async function previewAndDeploy(h, workloadNames) {
  const { page } = h;
  await h.click(page.getByRole('button', { name: 'Preview changes' }));
  const preview = page.getByLabel('Deployment preview');
  await expect(preview).toContainText(`${workloadNames.length} workload(s) affected`, { timeout: 120_000 });
  await h.moveTo(preview);
  await h.pause(4000);
  await h.click(page.getByRole('button', { name: 'Deploy these changes' }));
  const result = page.getByLabel('Deployment result');
  await expect(result).toContainText('Deploy succeeded', { timeout: 600_000 });
  for (const name of workloadNames) await expect(result).toContainText(`${name}: succeeded`);
  await h.moveTo(result);
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
  expect(JSON.stringify(body)).not.toContain(secret);

  const payload = `hello from ${runId}`;
  await h.type(page.locator('input[name="payload"]'), payload, { replace: true });
  await h.click(page.getByRole('button', { name: 'Submit job' }));
  const job = page.locator('#jobs tr.job').filter({ hasText: payload });
  await expect(job).toBeVisible();
  await h.showCursor();
  await h.moveTo(job);
  await h.pause(6000);
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
