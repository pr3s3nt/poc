// The real-runner scenarios cannot run in CI (they need k8s-4f, Terraform and a
// headed browser), so this test proves statically that every Console label they
// drive still exists in the Web Console source (or, for the acceptance app, in
// its own HTML template). A renamed label breaks here instead of in a live run.
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import { requireScenario } from './terraform-v2-acceptance.mjs';

const here = join(process.cwd(), 'test/e2e') + '/';
const read = (file) => readFileSync(join(here, file), 'utf8');

function walk(dir, out = []) {
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) walk(full, out);
    else if (/\.(ts|tsx)$/.test(entry) && !/\.test\./.test(entry)) out.push(full);
  }
  return out;
}

// Console text lives in the Vietnamese message catalog; source files hold only keys.
const corpus = walk(join(process.cwd(), 'src')).map((f) => readFileSync(f, 'utf8')).join('\n')
  + '\n' + readFileSync(join(process.cwd(), 'public/locales/vi.json'), 'utf8')
  + '\n' + readFileSync(join(process.cwd(), '../backend/examples/acceptance-app/frontend/main.go'), 'utf8');

const sources = ['terraform-v2-steps.mjs', 'terraform-v2-acceptance.mjs', 'terraform-v2-aws-definitions.mjs', 'full-app-flows.mjs', 'full-app-kit.mjs', 'v04-internal-full.mjs'].map(read).join('\n');
const patterns = [
  /getByRole\('[^']+',\s*\{[^}]*?\bname:\s*'([^']+)'/g, /getByLabel\('([^']+)'/g, /form\('([^']+)'\)/g, /form\(`([^`]+)`\)/g, /getByText\('([^']+)'/g,
  /toContainText\('([^']+)'/g, /getByRole\('(?:region|group)', \{ name: '([^']+)'/g, /getByLabel\(`([^`]+)`\)/g, /getByRole\('[^']+',\s*\{[^}]*?\bname:\s*`([^`]+)`/g,
];

function labels() {
  const found = new Set();
  for (const pattern of patterns) for (const match of sources.matchAll(pattern)) found.add(match[1]);
  return [...found];
}

// Labels the Console composes at runtime (a noun or a variable name is interpolated), checked by their constituents.
const COMPOSED = {
  '+ Thêm tham số đầu vào': ['+ Thêm', 'tham số đầu vào'], '+ Thêm tham số đầu ra': ['+ Thêm', 'tham số đầu ra'], '+ Thêm biến': ['+ Thêm', '"biến"'],
  'Biến của ứng dụng cho main': ['Biến của ứng dụng'], 'Bí mật của ứng dụng cho main': ['Bí mật của ứng dụng'], 'Điều kiện 1 Class': ['Điều kiện'], 'Triển khai thành công': ['Triển khai', 'thành công'], 'Triển khai thất bại': ['Triển khai', 'thất bại'],
};

describe('terraform v2 scenario labels', () => {
  it.each(['A01', 'E01', 'E02'])('rejects retired %s before runtime', (id) => {
    expect(() => requireScenario(id)).toThrow(/retired scenario/);
  });
  it.each(['V05', 'A02', 'A03'])('retains current %s entry', (id) => {
    expect(typeof requireScenario(id)).toBe('function');
  });
  it('extracts a meaningful set of labels', () => {
    expect(labels().length).toBeGreaterThan(60);
  });

  it('every label the scenarios drive exists in the Console (or acceptance app) source', () => {
    const missing = [];
    for (const label of labels()) {
      // ASCII-only labels are roles/technical names (button, dialog, ...), not Console text.
      if (![...label].some((ch) => ch.charCodeAt(0) > 127) && !/^(Submit job|Acceptance application)$/.test(label)) continue;
      const pieces = (COMPOSED[label] ?? label.split(/\$\{[^}]*\}|\d+/)).map((p) => p.trim()).filter((p) => p.length >= 3);
      for (const piece of pieces) if (!corpus.includes(piece)) missing.push(`${label} -> ${piece}`);
    }
    expect(missing).toEqual([]);
  });

  it('never uses a fake or interception seam', () => {
    for (const forbidden of [/page\.route\(/, /route\.fulfill/, /\bfake\b.*provider/i, /seed-progress/, /touch-environment/]) expect(sources).not.toMatch(forbidden);
  });

  it('the AWS scenarios take their catalog from the shared planner-tested file and bind no secret store', () => {
    const aws = read('terraform-v2-aws-definitions.mjs');
    expect(aws).toMatch(/catalogFile/);
    expect(aws).not.toMatch(/registerSecretStore|putKeys|kind: 'secret'/);
    expect(aws).toMatch(/\['backend connection', 'environment', 'database', 'cache'\]/);
    expect(aws).toMatch(/app\.secret-row-not-configured/);
  });

  it('never passes a password in a command line', () => {
    // Authentication reaches psql and redis-cli through stdin; the argv carries no secret.
    expect(sources).not.toMatch(/'--pass'|--pass\b|PGPASSWORD=|REDISCLI_AUTH=|--password/);
    expect(sources).toMatch(/IFS= read -r PGPASSWORD/);
    expect(sources).toMatch(/IFS= read -r REDISCLI_AUTH/);
  });

  it('asserts the secret row together with every other application row', () => {
    expect(sources).toMatch(/\['backend connection', 'environment', 'secret', 'database', 'cache'\]/);
    expect(sources).not.toMatch(/setEnvironment.*ACCEPTANCE_SECRET\b.*['"]\$\{/);
  });

  it('creates no job, Workload or resource through the API', () => {
    expect(sources).not.toMatch(/request\.post\(|request\.put\(|request\.delete\(|method: 'POST'|method: 'PUT'/);
    const browserRead = read('full-app-flows.mjs').match(/export async function readAppJSON\(page, path\) \{[\s\S]*?^}/m)?.[0];
    expect(browserRead).toContain("fetch(route, { method: 'GET'");
    expect(browserRead).toContain("cache: 'no-store'");
    const readSources = sources.replace(browserRead, '');
    // Read-only fetches of the app: job detail and checks only.
    for (const call of readSources.matchAll(/fetch\(`?([^`),]+)/g)) expect(call[1]).toMatch(/healthz|\/api\/jobs\/|\/api\/checks|auth\.docker|manifests/);
  });

});
