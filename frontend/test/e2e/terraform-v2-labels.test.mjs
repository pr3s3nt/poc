// Historical mixed Score/Render Bundle runners are retained for provenance,
// not current Console acceptance (see e2e/README.md). Keep runner safety and
// registration checks; current UI behavior is covered by component/browser tests.
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import { requireScenario } from './terraform-v2-acceptance.mjs';

const here = join(process.cwd(), 'test/e2e') + '/';
const read = (file) => readFileSync(join(here, file), 'utf8');

const sources = ['terraform-v2-steps.mjs', 'terraform-v2-acceptance.mjs', 'terraform-v2-aws-definitions.mjs', 'full-app-flows.mjs', 'full-app-kit.mjs', 'v04-internal-full.mjs'].map(read).join('\n');
describe('historical terraform v2 runner safety', () => {
  it.each(['A01', 'E01', 'E02'])('rejects retired %s before runtime', (id) => {
    expect(() => requireScenario(id)).toThrow(/retired scenario/);
  });
  it.each(['V05', 'A02', 'A03'])('retains current %s entry', (id) => {
    expect(typeof requireScenario(id)).toBe('function');
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
