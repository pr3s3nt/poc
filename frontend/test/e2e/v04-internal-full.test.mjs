import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'vitest';
import { REQUIRED_ASSERTIONS, REQUIRED_PHASES, SCENARIOS, catalogIds, verifyEvidence } from './v04-internal-full.mjs';
import { wrapCaption } from './captions.mjs';
import { parseArgs, requireScenarioIn } from './scenario-main.mjs';

const complete = () => ({
  assertions: REQUIRED_ASSERTIONS.map((id) => ({ id, pass: true, source: id.startsWith('ready.') ? 'private-read' : 'ui' })),
  phases: REQUIRED_PHASES.map((phase) => ({ phase })),
  assertionsText: 'PASS ready\n',
});

test('a complete V04 evidence set has no problems', () => {
  assert.deepEqual(verifyEvidence(complete()), []);
});

test('every full-application coverage row is mandatory: a missing or failed one is reported', () => {
  for (const id of ['job.completed-ui', 'cache.ttl-expiry-miss', 'cache.refill', 'update.config-applied', 'restart.identity-reused', 'cleanup.volumes-absent', 'ready.single-postgres-redis']) {
    const without = complete();
    without.assertions = without.assertions.filter((a) => a.id !== id);
    assert.match(verifyEvidence(without).join(), new RegExp(`missing assertion ${id}`));
    const failed = complete();
    failed.assertions.find((a) => a.id === id).pass = false;
    assert.match(verifyEvidence(failed).join(), new RegExp(`failed assertion ${id}`));
  }
  assert.match(verifyEvidence({ ...complete(), assertionsText: 'FAIL x\n' }).join(), /FAIL line/);
  assert.match(verifyEvidence({ ...complete(), phases: [{ phase: 'PREPARE' }] }).join(), /missing phase CLEANUP/);
});

test('the V04 scenario is registered, strict on arguments and its catalog IDs are valid Definition IDs', () => {
  assert.deepEqual(Object.keys(SCENARIOS), ['V04']);
  assert.equal(typeof requireScenarioIn(SCENARIOS, 'V04'), 'function');
  assert.throws(() => requireScenarioIn(SCENARIOS, 'V03'));
  assert.throws(() => parseArgs(['--bogus']));
  for (const id of Object.values(catalogIds({ runId: 'v04-1018123456-ab12' }))) assert.match(id, /^[a-z0-9-]+$/);
});

test('every asserted ID is recorded by the scenario, flows or kit sources (no required ID can be a typo)', () => {
  const sources = ['v04-internal-full.mjs', 'full-app-flows.mjs', 'full-app-kit.mjs', 'terraform-v2-steps.mjs', 'v04-probe-cleanup.mjs'].map((f) => readFileSync(`${process.cwd()}/test/e2e/${f}`, 'utf8')).join('\n');
  for (const id of REQUIRED_ASSERTIONS) assert.ok(sources.includes(`'${id}'`) || sources.includes(`\`${id}\``), `no source records ${id}`);
});

test('the scenario never fakes or intercepts anything', () => {
  for (const f of ['v04-internal-full.mjs', 'full-app-flows.mjs', 'full-app-kit.mjs', 'scenario-main.mjs']) {
    const source = readFileSync(`${process.cwd()}/test/e2e/${f}`, 'utf8');
    assert.doesNotMatch(source, /page\.route\(|\.fulfill\(|route\.continue/, f);
  }
});

// Validate the actual authored cues before spending a live run on recording.
test('all authored V04 phase and narration cues fit the two-line caption strip', () => {
  const source = readFileSync(`${process.cwd()}/test/e2e/v04-internal-full.mjs`, 'utf8');
  const cues = [...source.matchAll(/(?:phase\('[^']+',\s*|caption\()'([^'\n]*)'/g)].map((match) => match[1]);
  assert.ok(cues.length >= REQUIRED_PHASES.length);
  for (const cue of cues) assert.doesNotThrow(() => wrapCaption(cue), cue);
});
