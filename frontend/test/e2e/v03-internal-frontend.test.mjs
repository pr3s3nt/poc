import assert from 'node:assert/strict';
import { test } from 'vitest';
import { REQUIRED_ASSERTIONS, REQUIRED_PHASES, SCENARIOS, catalogIds, parseArgs, requireScenario, verifyEvidence } from './v03-internal-frontend.mjs';

const complete = () => ({
  assertions: REQUIRED_ASSERTIONS.map((id) => ({ id, pass: true, source: id.startsWith('ready.workload') ? 'private-read' : 'ui' })),
  phases: REQUIRED_PHASES.map((phase) => ({ phase })),
  assertionsText: 'PASS ready\n',
});

test('a complete evidence set has no problems', () => {
  assert.deepEqual(verifyEvidence({ ...complete() }), []);
});

test('a missing, failed or FAIL-line assertion, a missing phase or no UI observation is reported', () => {
  const missing = complete();
  missing.assertions = missing.assertions.filter((a) => a.id !== 'update.rollout');
  assert.match(verifyEvidence(missing).join(), /missing assertion update.rollout/);
  const failed = complete();
  failed.assertions.find((a) => a.id === 'cleanup.namespaces-absent').pass = false;
  assert.match(verifyEvidence(failed).join(), /failed assertion cleanup.namespaces-absent/);
  const line = { ...complete(), assertionsText: 'PASS a\nFAIL b\n' };
  assert.match(verifyEvidence(line).join(), /FAIL line/);
  const phases = { ...complete(), phases: [{ phase: 'PREPARE' }] };
  assert.match(verifyEvidence(phases).join(), /missing phase CLEANUP/);
  const noUi = complete();
  noUi.assertions.forEach((a) => { a.source = 'private-read'; });
  assert.match(verifyEvidence(noUi).join(), /no assertion was observed through the UI/);
});

test('the restricted-credential negative and the structured negatives are mandatory', () => {
  for (const id of ['negative.restricted-credential', 'negative.no-cluster-match.matcher', 'negative.tied-cluster-definitions.matcher', 'negative.prerequisites-valid']) {
    const without = complete();
    without.assertions = without.assertions.filter((a) => a.id !== id);
    assert.match(verifyEvidence(without).join(), new RegExp(`missing assertion ${id}`));
  }
  // verifyEvidence takes no flag that could make the restricted-credential negative optional.
  assert.equal(verifyEvidence.length, 1);
  assert.ok(REQUIRED_ASSERTIONS.includes('negative.restricted-credential'));
});

test('the V03 scenario is registered, strict on arguments and its catalog IDs are valid Definition IDs', () => {
  assert.equal(typeof requireScenario('V03'), 'function');
  assert.deepEqual(Object.keys(SCENARIOS), ['V03']);
  assert.throws(() => requireScenario('V04'));
  assert.throws(() => parseArgs(['--bogus']));
  assert.deepEqual(parseArgs(['--scenario', 'V03', '--headed', '--captions', 'vi', '--evidence', '/x']), { scenario: 'V03', headed: true, captions: 'vi', evidence: '/x' });
  for (const id of Object.values(catalogIds({ runId: 'v03-1018123456-ab12' }))) assert.match(id, /^[a-z0-9-]+$/);
});

test('the scenario never fakes the deferred backend: it names V04 and records only FAIL rows as expected', async () => {
  const { readFileSync } = await import('node:fs');
  const source = readFileSync(`${process.cwd()}/test/e2e/v03-internal-frontend.mjs`, 'utf8');
  assert.match(source, /DEFERRED V04/);
  assert.match(source, /everyFail/);
  assert.doesNotMatch(source, /route\(|fulfill\(|page\.route/);
});

test('a timed-out wait keeps the last safe verdict instead of discarding it', async () => {
  const { waitFor } = await import('./terraform-v2-lib.mjs');
  let verdict = '';
  await assert.rejects(
    waitFor(async () => { verdict = 'generation=2 observed=2 pod-image-ids=[sha256:aa] env-mismatch-names=[]'; return false; }, { timeoutMs: 30, intervalMs: 10, what: 'the rollout', describe: () => verdict }),
    /timed out waiting for the rollout \(last verdict: generation=2 observed=2 pod-image-ids=\[sha256:aa\]/,
  );
});

test('the port-forward is re-established after a rollout: the dead one is gone first, the new one must answer, and every forward is stopped at the end', async () => {
  const { createForwardManager } = await import('./terraform-v2-lib.mjs');
  const events = [];
  let seq = 0;
  const start = () => {
    const id = ++seq;
    let resolveExit;
    const exited = new Promise((resolve) => { resolveExit = resolve; });
    events.push(`start#${id}`);
    return { id, url: `http://127.0.0.1:1${id}`, exited, stop() { events.push(`stop#${id}`); resolveExit(); }, kill() { events.push(`kill#${id}`); resolveExit(); } };
  };
  const forwarders = [];
  let answers = 0;
  // The replacement answers only on its second probe (the new Pod is not Ready yet at first).
  const healthy = async (url) => { events.push(`probe ${url}`); return url.endsWith('2') ? ++answers >= 2 : true; };
  const manager = createForwardManager({}, { forwarders, start, healthy, timeoutMs: 5000, stopWaitMs: 50 });
  await manager.connect();
  assert.equal(manager.url, 'http://127.0.0.1:11');
  await manager.connect('after rollout');
  assert.equal(manager.url, 'http://127.0.0.1:12');
  assert.ok(events.indexOf('stop#1') < events.indexOf('start#2'), 'the old forward must be gone before the new one binds the port');
  assert.ok(answers >= 2, 'the new forward must really answer before it is used');
  await manager.stop();
  assert.equal(manager.url, undefined);
  for (const f of forwarders) f.stop(); // the scenario-level cleanup stops old and replacement forwards (idempotent)
  assert.equal(forwarders.length, 2);
  assert.equal(events.filter((e) => e === 'stop#2').length >= 1, true);
});

test('a forward that will not die is killed before the replacement starts', async () => {
  const { createForwardManager } = await import('./terraform-v2-lib.mjs');
  const events = [];
  let seq = 0;
  const start = () => {
    const id = ++seq;
    let resolveExit;
    const exited = new Promise((resolve) => { resolveExit = resolve; });
    return { url: `http://x/${id}`, exited, stop() { events.push(`stop#${id}`); }, kill() { events.push(`kill#${id}`); resolveExit(); } };
  };
  const manager = createForwardManager({}, { forwarders: [], start, healthy: async () => true, timeoutMs: 1000, stopWaitMs: 30 });
  await manager.connect();
  await manager.connect();
  assert.deepEqual(events, ['stop#1', 'kill#1']);
});

test('the resource entry locator matches the exact descriptor, not its companion that extends it', async () => {
  const { exactTextPattern } = await import('./v03-internal-frontend.mjs');
  const main = 'k8s-namespace.default#environments.acc-v03.staging';
  const aux = `${main}-aux`;
  const re = (text) => new RegExp(exactTextPattern(text));
  assert.equal(re(main).test(main), true);
  assert.equal(re(main).test(aux), false, 'a substring match would select both and break Playwright strict mode');
  assert.equal(re(aux).test(aux), true);
  assert.equal(re(aux).test(main), false);
  assert.equal(re('a.b').test('axb'), false, 'regex metacharacters in a descriptor are literal');
  // Against the real list component: the pattern selects exactly one <strong> of the prefix pair.
  const { render } = await import('@testing-library/react');
  const { createElement } = await import('react');
  const { EnvironmentResources } = await import('../../src/features/resources/EnvironmentResources');
  const { vi } = await import('vitest');
  const item = (id, descriptor) => ({ id, descriptor, type: 'k8s-namespace', class: 'default', resourceId: 'x', status: 'UNREFERENCED', scope: 'ENVIRONMENT', generation: 0, module: 'k8s-namespace', updatedAt: '2026-10-18T00:00:00Z' });
  vi.stubGlobal('fetch', async () => Response.json({ resources: [item('1', main), item('2', aux)] }));
  const { container, findAllByText } = render(createElement(EnvironmentResources, { applicationId: 'a', environment: 'staging' }));
  await findAllByText(main);
  const strongs = [...container.querySelectorAll('button.deployment-entry strong')];
  assert.equal(strongs.length, 2);
  assert.equal(strongs.filter((el) => re(main).test(el.textContent)).length, 1);
  assert.equal(strongs.filter((el) => el.textContent.includes(main)).length, 2, 'the plain substring filter is ambiguous');
  vi.unstubAllGlobals();
});
