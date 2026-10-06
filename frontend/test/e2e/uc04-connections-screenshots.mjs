// UC-04 Connections layout screenshots with synthetic data only. Serves the
// built console (frontend/dist) from a private local HTTP server and answers
// /api/v1 with route stubs, so no backend, credential store or cluster is
// involved. Writes PNG files to ORCH_SCREENSHOT_DIR (outside the repository)
// and fails when the page overflows horizontally or a radio/checkbox control
// is stretched like a text field. Also captures keyboard focus on both
// kubeconfig source choices and fails when that focus ring is missing or
// clipped by the segmented group.
//
// Usage: npm run build && ORCH_SCREENSHOT_DIR=/tmp/<dir> node test/e2e/uc04-connections-screenshots.mjs
import { chromium, expect } from '@playwright/test';
import { createServer } from 'node:http';
import { existsSync, mkdirSync, readFileSync } from 'node:fs';
import { extname, join, normalize } from 'node:path';
import { fileURLToPath } from 'node:url';

const outDir = process.env.ORCH_SCREENSHOT_DIR;
if (!outDir) throw new Error('ORCH_SCREENSHOT_DIR is required');
const repoFrontend = fileURLToPath(new URL('../..', import.meta.url));
if (normalize(outDir).startsWith(normalize(join(repoFrontend, '..')))) throw new Error('ORCH_SCREENSHOT_DIR must be outside the repository');
mkdirSync(outDir, { recursive: true });
const dist = join(repoFrontend, 'dist');
if (!existsSync(join(dist, 'index.html'))) throw new Error('frontend/dist is missing; run npm run build');

const types = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.svg': 'image/svg+xml' };
const server = createServer((request, response) => {
  const path = decodeURIComponent(new URL(request.url, 'http://local').pathname);
  const relative = normalize(path.replace(/^\/ui\/?/, '')).replace(/^(\.\.[/\\])+/, '');
  const file = join(dist, relative);
  const target = relative && existsSync(file) && !file.endsWith('/') && extname(file) ? file : join(dist, 'index.html');
  response.writeHead(200, { 'content-type': types[extname(target)] ?? 'application/octet-stream' });
  response.end(readFileSync(target));
});
await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
const baseURL = `http://127.0.0.1:${server.address().port}`;

// Synthetic records: a named kubeconfig Connection with a long endpoint, an
// unnamed legacy entry and an AWS record without Kubernetes fields.
const connections = [
  { key: 'lab-eu-west', name: 'Lab EU West', kind: 'KUBERNETES', authenticationType: 'KUBECONFIG', status: 'READY', config: { cluster: 'lab-eu-west-cluster', kubeContext: 'lab-eu-west', endpoint: 'https://lab-eu-west-1.synthetic-cluster-endpoint.example.invalid:6443' } },
  { key: 'internal-k8s', name: 'internal-k8s', kind: 'KUBERNETES', status: 'READY', config: { cluster: 'kind-internal', kubeContext: 'kind-internal' } },
  { key: 'aws-sandbox', name: 'AWS sandbox', kind: 'AWS', status: 'READY', config: { region: 'eu-west-1', accountId: '000000000000' } },
];
const contexts = [
  { name: 'lab-eu-west', cluster: 'lab-eu-west-cluster', endpoint: 'https://lab-eu-west-1.synthetic-cluster-endpoint.example.invalid:6443' },
  { name: 'demo-unreachable', cluster: 'demo-unreachable', endpoint: 'https://127.0.0.1:9' },
];
const syntheticToken = 'synthetic-token-not-a-credential-0123456789';
const kubeconfig = `apiVersion: v1\nkind: Config\nclusters:\n- name: lab-eu-west-cluster\n  cluster:\n    server: ${contexts[0].endpoint}\nusers:\n- name: lab\n  user:\n    token: ${syntheticToken}\ncontexts:\n- name: lab-eu-west\n  context: {cluster: lab-eu-west-cluster, user: lab}\n`;

const viewports = [{ name: 'desktop', width: 1440, height: 900 }, { name: 'mobile', width: 390, height: 844 }];
const browser = await chromium.launch();
const shots = [];
try {
  for (const viewport of viewports) {
    const page = await browser.newPage({ viewport: { width: viewport.width, height: viewport.height } });
    await page.route('**/api/v1/**', async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (path === '/api/v1/auth/session') return route.fulfill({ json: { user: { Username: 'platform-engineer', Role: 'PLATFORM_ENGINEER' } } });
      if (path === '/api/v1/applications') return route.fulfill({ json: { applications: [] } });
      if (path === '/api/v1/connections') return route.fulfill({ json: { connections } });
      if (path === '/api/v1/connections/kubernetes/inspect') return route.fulfill({ json: { contexts } });
      return route.fulfill({ status: 404, json: { error: 'not stubbed' } });
    });
    const shot = async (name, locator) => {
      const width = await page.locator('html').evaluate((root) => ({ scroll: root.scrollWidth, client: root.clientWidth }));
      if (width.scroll > width.client) throw new Error(`${viewport.name}/${name}: horizontal overflow ${width.scroll} > ${width.client}`);
      for (const control of await page.locator('form input[type=radio], form input[type=checkbox]').all()) {
        const box = await control.boundingBox();
        if (box && box.width > 24) throw new Error(`${viewport.name}/${name}: radio/checkbox stretched to ${box.width}px`);
      }
      // The pasted document may live only inside its own masked field.
      const outsideField = await page.locator('html').evaluate((root) => {
        const copy = root.cloneNode(true);
        for (const field of copy.querySelectorAll('textarea.masked-text')) field.remove();
        return copy.outerHTML;
      });
      if (outsideField.includes(syntheticToken)) throw new Error(`${viewport.name}/${name}: kubeconfig token rendered outside the field`);
      const path = join(outDir, `uc04-${viewport.name}-${name}.png`);
      await (locator ?? page).screenshot({ path, fullPage: !locator });
      shots.push(path);
    };

    await page.goto(`${baseURL}/ui/platform/connections`);
    await expect(page.getByRole('heading', { name: 'Connections', level: 1 })).toBeVisible();
    const table = page.getByRole('table', { name: 'Registered connections' });
    await expect(table.locator('tbody tr')).toHaveCount(3);
    await shot('list', page.locator('.content-panel').first());

    // Upload: styled chooser with the selected filename beside it.
    await page.getByLabel('Connection name').fill('Lab EU West');
    await page.locator('input[type=file]').setInputFiles({ name: 'lab-eu-west.kubeconfig', mimeType: 'text/plain', buffer: Buffer.from(kubeconfig) });
    await expect(page.getByText('Selected file: lab-eu-west.kubeconfig.', { exact: false })).toBeVisible();
    await page.locator('input[type=file]').focus();
    await shot('upload', page.locator('.content-panel').nth(1));

    // Keyboard focus on each source choice: Tab from the name field reaches
    // Upload, ArrowRight moves focus (and native selection) to Paste, and
    // Tab/Shift+Tab returns to it. The ring must be fully inside the clipped
    // segmented group.
    const radios = page.locator('.source-choices input[type=radio]');
    const focusShot = async (name, index) => {
      await expect(radios.nth(index)).toBeFocused();
      const ring = await radios.nth(index).evaluate((input) => {
        if (!input.matches(':focus-visible')) return 'radio is not :focus-visible';
        const choice = input.closest('.source-choice');
        const style = input.ownerDocument.defaultView.getComputedStyle(choice);
        if (style.outlineStyle === 'none' || parseFloat(style.outlineWidth) === 0) return 'no outline on focused choice';
        const extent = parseFloat(style.outlineOffset) + parseFloat(style.outlineWidth);
        const item = choice.getBoundingClientRect();
        const group = choice.parentElement.getBoundingClientRect();
        const clip = 1; // group border
        if (item.left - extent < group.left + clip - 0.5 || item.right + extent > group.right - clip + 0.5 || item.top - extent < group.top + clip - 0.5 || item.bottom + extent > group.bottom - clip + 0.5) return `outline extends ${extent}px outside the clipped group`;
        return '';
      });
      if (ring) throw new Error(`${viewport.name}/${name}: ${ring}`);
      const box = await page.locator('.connection-source').boundingBox();
      const path = join(outDir, `uc04-${viewport.name}-${name}.png`);
      await page.screenshot({ path, clip: { x: Math.max(0, box.x - 12), y: Math.max(0, box.y - 12), width: box.width + 24, height: Math.min(box.height, 160) + 24 } });
      shots.push(path);
    };
    await page.getByLabel('Connection name').focus();
    await page.keyboard.press('Tab');
    await focusShot('focus-upload', 0);
    await page.keyboard.press('ArrowRight');
    await expect(radios.nth(1)).toBeChecked();
    await page.keyboard.press('Tab');
    await page.keyboard.press('Shift+Tab');
    await focusShot('focus-paste', 1);

    // Paste: masked content, Show content beside its checkbox, focus visible.
    await page.getByLabel('Paste kubeconfig').check();
    await page.getByLabel('Kubeconfig content').fill(kubeconfig);
    await expect(page.getByLabel('Kubeconfig content')).toHaveClass(/masked-text/);
    await page.getByLabel('Show content').focus();
    await page.keyboard.press('Shift+Tab');
    await page.keyboard.press('Tab');
    await shot('paste-masked', page.locator('.content-panel').nth(1));

    // Inspect, choose a context and review the grouped destination summary.
    await page.getByRole('button', { name: 'Inspect kubeconfig' }).click();
    await page.getByRole('combobox').selectOption('lab-eu-west');
    await expect(page.getByLabel('Selected destination')).toContainText(`Endpoint ${contexts[0].endpoint}`);
    await shot('destination', page.locator('.content-panel').nth(1));
    await shot('page');
    await page.close();
  }
  console.log(`PASS: uc04 connections screenshots ${shots.length}`);
  for (const path of shots) console.log(path);
} finally {
  await browser.close();
  server.close();
}
