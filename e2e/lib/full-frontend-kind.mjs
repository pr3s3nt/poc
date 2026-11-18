import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdirSync, writeFileSync } from 'node:fs';
import { resolve, join } from 'node:path';

// Full frontend validation only. Backend evidence/source attribution is a
// separate check, never inferred from this runner's success.
const evidence = resolve(process.env.ORCH_EVIDENCE_DIR ?? '');
assert(process.env.ORCH_EVIDENCE_DIR && !evidence.startsWith(resolve('.') + '/'), 'Private evidence directory required');
assert(process.env.ORCH_CONTEXT === 'kind-k8s-4f' && process.env.ORCH_KUBECONFIG, 'Explicit authorized kind context required');
mkdirSync(evidence, { recursive: true, mode: 0o700 });
const runID = 'definition87-full-' + Date.now().toString(36);
const base = ['--kubeconfig', process.env.ORCH_KUBECONFIG, '--context', process.env.ORCH_CONTEXT, '--request-timeout=60s'];
const kube = (args, input) => execFileSync('kubectl', [...base, ...args], { input, encoding: 'utf8', maxBuffer: 128 * 1024 * 1024, stdio: ['pipe', 'pipe', 'pipe'] });
const json = (args, input) => JSON.parse(kube(args, input));
const wait = ms => new Promise(resolve => setTimeout(resolve, ms));
const source = execFileSync('git', ['rev-parse', 'HEAD'], { encoding: 'utf8' }).trim();
const receipt = { runID, source, frontendTree: execFileSync('git', ['rev-parse', 'HEAD:frontend'], { encoding: 'utf8' }).trim(), cleanup: false };
const save = () => writeFileSync(join(evidence, 'receipt.json'), JSON.stringify(receipt, null, 2), { mode: 0o600 });
save();
try {
  const ns = json(['create', '-f', '-', '-o', 'json'], JSON.stringify({ apiVersion: 'v1', kind: 'Namespace', metadata: { name: runID, labels: { 'orchestrator.task': runID } } }));
  receipt.namespaceUID = ns.metadata.uid; save();
  const pod = { apiVersion: 'v1', kind: 'Pod', metadata: { name: 'frontend', namespace: runID, labels: { 'orchestrator.task': runID } }, spec: {
    automountServiceAccountToken: false, restartPolicy: 'Never', volumes: [{ name: 'source', emptyDir: {} }], containers: [{ name: 'checks', image: 'mirror.gcr.io/library/node:22-bookworm', imagePullPolicy: 'IfNotPresent',
      command: ['sh', '-c', 'mkdir /results; while [ ! -f /source/.ready ]; do sleep 1; done; cd /source/frontend; node --version > /results/tools; npm ci > /results/install.log 2>&1; install=$?; if [ "$install" = 0 ]; then npm test -- --maxWorkers=1 > /results/tests.log 2>&1; test=$?; npm run typecheck > /results/typecheck.log 2>&1; types=$?; npm run lint > /results/lint.log 2>&1; lint=$?; npm run build > /results/build.log 2>&1; build=$?; else test=99; types=99; lint=99; build=99; fi; echo "$install $test $types $lint $build" > /results/status; touch /results/.done; sleep 7200'],
      resources: { requests: { cpu: '500m', memory: '512Mi' }, limits: { cpu: '4', memory: '5Gi' } }, volumeMounts: [{ name: 'source', mountPath: '/source' }],
    }],
  } };
  kube(['create', '-f', '-'], JSON.stringify(pod));
  const startDeadline = Date.now() + 300_000;
  for (;;) {
    const p = json(['-n', runID, 'get', 'pod', 'frontend', '-o', 'json']);
    if (p.status.containerStatuses?.[0]?.state?.running) break;
    assert(Date.now() < startDeadline, 'Frontend Pod did not start'); await wait(3000);
  }
  const archive = execFileSync('git', ['archive', source], { maxBuffer: 128 * 1024 * 1024 });
  kube(['-n', runID, 'exec', '-i', 'frontend', '--', 'tar', '-xf', '-', '-C', '/source'], archive);
  kube(['-n', runID, 'exec', 'frontend', '--', 'touch', '/source/.ready']);
  console.log('STARTED', source);
  const deadline = Date.now() + 45 * 60_000;
  for (;;) {
    try { kube(['-n', runID, 'exec', 'frontend', '--', 'test', '-f', '/results/.done']); break; } catch { assert(Date.now() < deadline, 'Frontend checks deadline exceeded'); await wait(15_000); }
  }
  const codes = kube(['-n', runID, 'exec', 'frontend', '--', 'cat', '/results/status']).trim().split(/\s+/).map(Number);
  assert(codes.length === 5 && codes.every(n => Number.isInteger(n) && n >= 0 && n <= 255));
  for (const file of ['tools', 'install.log', 'tests.log', 'typecheck.log', 'lint.log', 'build.log']) writeFileSync(join(evidence, file), kube(['-n', runID, 'exec', 'frontend', '--', 'cat', '/results/' + file]), { mode: 0o600 });
  receipt.results = Object.fromEntries(['install', 'test', 'typecheck', 'lint', 'build'].map((key, i) => [key, codes[i]])); save();
  console.log('COMPLETED', JSON.stringify(receipt.results));
} finally {
  if (receipt.namespaceUID) {
    const ns = json(['get', 'namespace', runID, '-o', 'json']);
    assert.equal(ns.metadata.uid, receipt.namespaceUID); assert.equal(ns.metadata.labels['orchestrator.task'], runID);
    kube(['delete', '--raw', '/api/v1/namespaces/' + runID, '-f', '-'], JSON.stringify({ apiVersion: 'v1', kind: 'DeleteOptions', preconditions: { uid: ns.metadata.uid, resourceVersion: ns.metadata.resourceVersion } }));
    const deadline = Date.now() + 180_000;
    while (kube(['get', 'namespace', runID, '--ignore-not-found', '-o', 'name']).trim()) { assert(Date.now() < deadline, 'Cleanup unconfirmed'); await wait(2000); }
    receipt.cleanup = true; save(); console.log('CLEANUP VERIFIED');
  }
}
assert(receipt.results && Object.values(receipt.results).every(n => n === 0), 'Frontend checks failed; inspect private logs');
