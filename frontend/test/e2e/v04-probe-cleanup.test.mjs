import { describe, expect, it } from 'vitest';
import { mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { recordProbeRun, verifyProbeCleanupReport, verifyDeliveryCleanupReport, expectedProbeObjects } from './v04-probe-cleanup.mjs';
function fixture() {
  const before = ['pg-probe', 'redis-probe', 'pg', 'redis', 'namespace', 'aux'].map((descriptor, i) => ({ id: `id${i}`, descriptor, status: i === 0 ? 'PENDING' : i === 1 ? 'FAILED' : 'READY', generation: 0 }));
  const runs = ['pending-0123456789abcdef', 'pending-fedcba9876543210'];
  return { status: 3, before, probes: ['pg-probe', 'redis-probe'], runs, report: {
    Outcome: 'NEEDS_ATTENTION', Runs: runs.map((RunID, i) => ({ RunID, Terminal: 'SUCCEEDED', Instances: [{ InstanceID: before[i].id, Destroyed: true, Remaining: null, RemainingUnknown: false }] })),
    Blocked: before.slice(2).map((r) => ({ InstanceID: r.id, Descriptor: r.descriptor, Status: r.status, Reason: 'NOT_SELECTED' })),
    Remaining: before.slice(2).map((r) => `${r.descriptor}=${r.status}`),
  } };
}
describe('operator-only original-engine negative subset proof (synthetic report unit contract)', () => {
  it('accepts only both probes destroyed with all four live providers protected, never calling the whole run clean', () => {
    const f = fixture();
    expect(verifyProbeCleanupReport(f).protectedRows).toHaveLength(4);
    expect(f.report.Outcome).toBe('NEEDS_ATTENTION');
  });
  it('accepts stable Environment run ID shared by both probes, narrowing by exact instance IDs', () => {
    const f = fixture(); f.runs = [f.runs[0]];
    f.report.Runs[0].Instances.push(f.report.Runs[1].Instances[0]); f.report.Runs.pop();
    expect(verifyProbeCleanupReport(f).selected).toHaveLength(2);
  });
  it.each([
    ['whole-run CLEAN', (f) => { f.status = 0; f.report.Outcome = 'CLEAN'; }],
    ['missing negative', (f) => { f.report.Runs.pop(); }],
    ['unrecorded run', (f) => { f.report.Runs[0].RunID = 'pending-aaaaaaaaaaaaaaaa'; }],
    ['unknown result', (f) => { f.report.Runs[0].Instances[0].RemainingUnknown = true; }],
    ['probe retained', (f) => { f.report.Runs[0].Instances[0].Destroyed = false; }],
    ['protected resource destroyed', (f) => { f.report.Runs[0].Instances.push({ InstanceID: 'id2', Destroyed: true }); }],
    ['wrong blocking reason', (f) => { f.report.Blocked[0].Reason = 'NOT_DESTROYABLE_STATUS'; }],
    ['changed protected state', (f) => { f.report.Blocked[0].Status = 'FAILED'; }],
    ['unproven remaining', (f) => { f.report.Remaining.pop(); }],
    ['pre-apply probe not PENDING', (f) => { f.before[0].status = 'FAILED'; }],
    ['readiness probe not FAILED', (f) => { f.before[1].status = 'PENDING'; }],
    ['different generation', (f) => { f.before[0].generation = 1; }],
  ])('refuses %s', (_name, change) => {
    const f = fixture(); change(f);
    expect(() => verifyProbeCleanupReport(f)).toThrow();
  });
  it('records two distinct negative Previews under the actual stable Environment run ID', () => {
    const privateDir = mkdtempSync(join(tmpdir(), 'v04-stable-preview-'));
    const input = { runId: 'v04-test', namespace: 'orch-v2-test-v04-test', privateDir };
    try {
      const preview = { runId: fixture().runs[0] };
      recordProbeRun(input, preview, 'probe-e01', 'pg-probe');
      recordProbeRun(input, preview, 'probe', 'redis-probe');
      const manifest = JSON.parse(readFileSync(join(privateDir, 'probe-cleanup-manifest.json')));
      expect(manifest.terraformRuns).toEqual([preview.runId]);
      expect(manifest.probePreviews.map((p) => p.workload)).toEqual(['probe-e01', 'probe']);
      expect(JSON.parse(readFileSync(join(privateDir, 'probe-cleanup-descriptors.json')))).toEqual(['pg-probe', 'redis-probe']);
    } finally { rmSync(privateDir, { recursive: true }); }
  });
  it('records valid Preview IDs before deploy, rejects absent IDs and duplicate/foreign manifests', () => {
    const privateDir = mkdtempSync(join(tmpdir(), 'v04-probe-contract-'));
    const input = { runId: 'v04-test', namespace: 'orch-v2-test-v04-test', privateDir };
    try {
      expect(() => recordProbeRun(input, {})).toThrow();
      for (const [i, runId] of fixture().runs.entries()) recordProbeRun(input, { runId }, i ? 'probe' : 'probe-e01', i ? 'redis-probe' : 'pg-probe');
      expect(JSON.parse(readFileSync(join(privateDir, 'probe-cleanup-manifest.json'))).terraformRuns).toEqual(fixture().runs);
      expect(() => recordProbeRun(input, { runId: fixture().runs[0] }, 'probe-e01', 'pg-probe')).toThrow();
      expect(() => recordProbeRun({ ...input, runId: 'foreign' }, { runId: 'pending-aaaaaaaaaaaaaaaa' }, 'probe-e01', 'pg-probe')).toThrow();
    } finally { rmSync(privateDir, { recursive: true }); }
  });
});

// Synthetic CLI wire contract; live absence is independently checked by the runner.
describe('remaining delivery cleanup report', () => {
  const fixture = () => ({ removed: ['worker', 'frontend'], failed: null, protected: null, namespace: 'owned', secretObjects: ['VaultStaticSecret/worker'] });
  it('accepts Go null and empty-array lists only after successful cleanup', () => {
    expect(() => verifyDeliveryCleanupReport(0, fixture(), 'owned')).not.toThrow();
    // Main flow removed frontend/backend together first; only worker remains delivered.
    expect(() => verifyDeliveryCleanupReport(0, { ...fixture(), removed: ['worker'] }, 'owned', ['worker'])).not.toThrow();
    expect(() => verifyDeliveryCleanupReport(0, { ...fixture(), failed: [], protected: [] }, 'owned')).not.toThrow();
  });
  it.each([
    ['missing failures', (r) => { delete r.failed; }],
    ['malformed failures', (r) => { r.failed = {}; }],
    ['failed removal', (r) => { r.failed = ['worker']; }],
    ['missing protection', (r) => { delete r.protected; }],
    ['malformed protection', (r) => { r.protected = ''; }],
    ['protected workload', (r) => { r.protected = ['worker']; }],
    ['wrong namespace', (r) => { r.namespace = 'foreign'; }],
    ['missing workload', (r) => { r.removed.pop(); }],
    ['unexpected workload', (r) => { r.removed.push('backend'); }],
    ['no VSO cleanup', (r) => { r.secretObjects = []; }],
  ])('rejects %s', (_name, change) => {
    const report = fixture(); change(report);
    expect(() => verifyDeliveryCleanupReport(0, report, 'owned')).toThrow();
  });
  it('rejects nonzero exit and missing report', () => {
    expect(() => verifyDeliveryCleanupReport(1, fixture(), 'owned')).toThrow();
    expect(() => verifyDeliveryCleanupReport(0, null, 'owned')).toThrow();
  });
});

describe('exact E02 pre-cleanup provider set (synthetic object contract)', () => {
  const item = (uid) => ({ metadata: { uid } });
  it('accepts exactly the original two identities plus the intentional failed provider', () => {
    expect(expectedProbeObjects([item('pg'), item('redis')], [item('bad'), item('redis'), item('pg')], item('bad'))).toBe(true);
  });
  it.each([
    ['replaced original', ['pg', 'replacement', 'bad'], 'bad'],
    ['extra provider', ['pg', 'redis', 'bad', 'extra'], 'bad'],
    ['missing failed', ['pg', 'redis'], 'bad'],
    ['wrong failed UID', ['pg', 'redis', 'foreign'], 'bad'],
    ['duplicate UID', ['pg', 'pg', 'bad'], 'bad'],
    ['failed aliases original', ['pg', 'redis', 'bad'], 'redis'],
  ])('rejects %s', (_name, after, failed) => {
    expect(expectedProbeObjects([item('pg'), item('redis')], after.map(item), item(failed))).toBe(false);
  });
});
