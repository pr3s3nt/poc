// Operator-only RUN_CLEANUP fixture. No UI/API permission or product lifecycle is changed.
import { execFileSync } from 'node:child_process';
import { existsSync, readFileSync, writeFileSync } from 'node:fs';

const sorted = (a) => [...a].sort();
const same = (a, b) => JSON.stringify(sorted(a)) === JSON.stringify(sorted(b));
export function recordProbeRun(input, preview, workload, descriptor) {
  if (!/^pending-[0-9a-f]{16}$/.test(preview?.runId)) throw new Error('negative Preview has no trusted Terraform run ID');
  if (!['probe-e01', 'probe'].includes(workload) || typeof descriptor !== 'string' || !descriptor) throw new Error('negative Preview identity is incomplete');
  const path = `${input.privateDir}/probe-cleanup-manifest.json`;
  const current = existsSync(path) ? JSON.parse(readFileSync(path, 'utf8')) : { runId: input.runId, target: 'k8s-4f', scenario: 'V04', generation: 0, namespace: input.namespace, namespaceIdentity: input.namespace, terraformRuns: [], probePreviews: [] };
  if (current.runId !== input.runId || current.generation !== 0 || !Array.isArray(current.probePreviews) || current.probePreviews.some((p) => p.workload === workload || p.descriptor === descriptor)) throw new Error('negative manifest identity or Preview differs');
  current.probePreviews.push({ workload, descriptor, runId: preview.runId });
  if (!current.terraformRuns.includes(preview.runId)) current.terraformRuns.push(preview.runId);
  writeFileSync(`${input.privateDir}/probe-cleanup-descriptors.json`, JSON.stringify(current.probePreviews.map((p) => p.descriptor)), { mode: 0o600 });
  writeFileSync(path, JSON.stringify(current), { mode: 0o600 });
}

export function validateProbeSelection({ before, probes, runs }) {
  if (!Array.isArray(before) || !Array.isArray(probes) || probes.length !== 2 || new Set(probes).size !== 2 || !Array.isArray(runs) || runs.length < 1 || runs.length > 2 || new Set(runs).size !== runs.length) throw new Error('probe cleanup selection is incomplete');
  if (before.some((r) => !r.id || !r.descriptor || r.generation !== 0) || new Set(before.map((r) => r.id)).size !== before.length) throw new Error('resource identity/generation is incomplete');
  const selected = before.filter((r) => probes.includes(r.descriptor));
  const protectedRows = before.filter((r) => !probes.includes(r.descriptor));
  if (selected.length !== 2 || selected.some((r) => r.status !== (r.descriptor === probes[0] ? 'PENDING' : 'FAILED')) || protectedRows.length !== 4 || protectedRows.some((r) => r.status !== 'READY')) throw new Error('negative/protected resources are not in their expected states');
  return { selected, protectedRows };
}

export function verifyProbeCleanupReport({ status, report, before, probes, runs }) {
  const { selected, protectedRows } = validateProbeSelection({ before, probes, runs });
  if (status !== 3 || report?.Outcome !== 'NEEDS_ATTENTION' || !Array.isArray(report.Runs) || !same(report.Runs.map((r) => r.RunID), runs) || report.Runs.some((r) => r.Terminal !== 'SUCCEEDED')) throw new Error('original engine did not complete exactly the negative runs');
  const destroyed = report.Runs.flatMap((r) => r.Instances ?? []);
  if (!same(destroyed.map((r) => r.InstanceID), selected.map((r) => r.id)) || destroyed.some((r) => r.Destroyed !== true || r.RemainingUnknown || (r.Remaining?.length ?? 0) !== 0)) throw new Error('negative instances are not all verified DESTROYED');
  if (!Array.isArray(report.Blocked) || !same(report.Blocked.map((r) => r.InstanceID), protectedRows.map((r) => r.id)) || report.Blocked.some((r) => {
    const original = protectedRows.find((x) => x.id === r.InstanceID);
    return r.Reason !== 'NOT_SELECTED' || r.Descriptor !== original?.descriptor || r.Status !== original?.status;
  })) throw new Error('unexpected cleanup block or protection change');
  if (!Array.isArray(report.Remaining) || !same(report.Remaining, protectedRows.map((r) => `${r.descriptor}=${r.status}`))) throw new Error('remaining set differs from the protected instances');
  return { selected, protectedRows };
}

export function cleanupFailedProbes(input, before, probes) {
  const manifest = JSON.parse(readFileSync(`${input.privateDir}/probe-cleanup-manifest.json`, 'utf8'));
  if (manifest.runId !== input.runId || manifest.generation !== 0 || manifest.namespace !== input.namespace || manifest.namespaceIdentity !== input.namespace) throw new Error('negative manifest identity differs');
  if (!Array.isArray(manifest.probePreviews) || !same(manifest.probePreviews.map((p) => p.workload), ['probe', 'probe-e01']) || !same(manifest.probePreviews.map((p) => p.descriptor), probes)) throw new Error('negative Preview selections differ');
  validateProbeSelection({ before, probes, runs: manifest.terraformRuns }); // reject incomplete selection BEFORE invoking any cleanup
  let stdout;
  let status = 0;
  try {
    stdout = execFileSync(`${input.privateDir}/bin/v2cleanup`, ['terraform', '-handles', `${input.privateDir}/probe-cleanup-handles.json`, '-manifest', `${input.privateDir}/probe-cleanup-manifest.json`, '-selected-descriptors', `${input.privateDir}/probe-cleanup-descriptors.json`], { encoding: 'utf8', timeout: 10 * 60_000, stdio: ['ignore', 'pipe', 'pipe'], env: { PATH: process.env.PATH, HOME: `${input.privateDir}/home`, TMPDIR: `${input.privateDir}/tmp`, LANG: 'C.UTF-8' } });
  } catch (error) {
    status = error.status;
    stdout = typeof error.stdout === 'string' ? error.stdout : '';
  }
  let report;
  try { report = JSON.parse(stdout); } catch { throw new Error('original-engine probe cleanup returned no valid report; recovery required'); }
  const verified = verifyProbeCleanupReport({ status, report, before, probes, runs: manifest.terraformRuns });
  // Persist only the product's safe report; never command stderr, handles or state.
  writeFileSync(`${input.privateDir}/probe-cleanup-report.json`, JSON.stringify(report), { mode: 0o600 });
  return verified;
}


export function verifyDeliveryCleanupReport(status, report, namespace, expectedRemoved = ['worker', 'frontend']) {
  const emptyList = (value) => value === null || (Array.isArray(value) && value.length === 0);
  if (status !== 0 || !Array.isArray(report?.removed) || !same(report.removed, expectedRemoved) || !emptyList(report.failed) || !emptyList(report.protected) || report.namespace !== namespace || !Array.isArray(report.secretObjects) || report.secretObjects.length === 0) throw new Error('delivery/VSO cleanup is not verified');
}

export function cleanupRemainingDelivery(input, expectedRemoved = ['worker', 'frontend']) {
  let stdout;
  try {
    stdout = execFileSync(`${input.privateDir}/bin/v2cleanup`, ['delivery', '-handles', `${input.privateDir}/probe-cleanup-handles.json`, '-manifest', `${input.privateDir}/probe-cleanup-manifest.json`], { encoding: 'utf8', timeout: 10 * 60_000, stdio: ['ignore', 'pipe', 'pipe'], env: { PATH: process.env.PATH, HOME: `${input.privateDir}/home`, TMPDIR: `${input.privateDir}/tmp`, LANG: 'C.UTF-8' } });
  } catch { throw new Error('original-target delivery cleanup failed; recovery required'); }
  let report;
  try { report = JSON.parse(stdout); } catch { throw new Error('delivery cleanup has no valid report'); }
  verifyDeliveryCleanupReport(0, report, input.namespace, expectedRemoved);
  writeFileSync(`${input.privateDir}/delivery-cleanup-report.json`, JSON.stringify(report), { mode: 0o600 });
  return report;
}

// Exact pre-cleanup object proof: the intentional failed provider is not a second
// healthy cache, and no original provider may be replaced or disappear.
export function expectedProbeObjects(before, after, failed) {
  if (!Array.isArray(before) || !Array.isArray(after) || before.length !== 2 || after.length !== 3 || !failed?.metadata?.uid) return false;
  const ids = (items) => items.map((item) => item.metadata?.uid);
  const original = ids(before); const current = ids(after);
  return original.every(Boolean) && new Set(original).size === 2 && new Set(current).size === 3 && !original.includes(failed.metadata.uid) && same(current, [...original, failed.metadata.uid]);
}
