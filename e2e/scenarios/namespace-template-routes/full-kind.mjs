import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdirSync, readFileSync, writeFileSync, copyFileSync } from 'node:fs';
import { resolve, join, dirname } from 'node:path';

const evidence = resolve(process.env.ORCH_EVIDENCE_DIR ?? '');
assert(process.env.ORCH_EVIDENCE_DIR && !evidence.startsWith(resolve('.') + '/'), 'Evidence must be outside Git');
mkdirSync(evidence, { recursive: true, mode: 0o700 });
const kubeconfig = process.env.ORCH_KUBECONFIG;
assert(kubeconfig && process.env.ORCH_CONTEXT === 'kind-k8s-4f', 'Explicit authorized kind context required');
const runID = 'httproute84-full-' + Date.now().toString(36);
const namespace = runID;
const base = ['--kubeconfig', kubeconfig, '--context', process.env.ORCH_CONTEXT, '--request-timeout=60s'];
const kubectl = (args, input) => execFileSync('kubectl', [...base, ...args], { input, encoding: 'utf8', maxBuffer: 128 * 1024 * 1024, stdio: ['pipe', 'pipe', 'pipe'] });
const json = args => JSON.parse(kubectl(args));
const delay = ms => new Promise(resolve => setTimeout(resolve, ms));
const candidate = execFileSync('git', ['rev-parse', 'HEAD'], { encoding: 'utf8' }).trim();
const baseline = process.env.ORCH_MAIN_REF ?? 'origin/main';
const baselineSHA = execFileSync('git', ['rev-parse', baseline], { encoding: 'utf8' }).trim();
const receipt = { runID, namespace, candidate, baseline: baselineSHA, results: {}, cleanup: false };
const parts = (process.env.ORCH_FULL_CHECK_PARTS ?? 'candidate,main,frontend').split(',');
assert(parts.length > 0 && new Set(parts).size === parts.length && parts.every(p => ['candidate','main','frontend'].includes(p)), 'Invalid full-check parts');
if (parts.length !== 3) {
  const previousPath = process.env.ORCH_REUSE_FULL_RECEIPT;
  assert(previousPath, 'Partial reruns require a completed full-check receipt');
  const previous = JSON.parse(readFileSync(previousPath,'utf8'));
  assert(previous.cleanup && previous.baseline === baselineSHA, 'Baseline/cleanup mismatch');
  for (const name of ['candidate','main','frontend']) {
    if (parts.includes(name)) continue;
    assert(previous.results[name], 'Missing earlier '+name+' result');
    if (name === 'candidate') {
      const tree = sha => execFileSync('git',['rev-parse',sha+':backend'],{encoding:'utf8'}).trim();
      assert.equal(tree(previous.candidate),tree(candidate),'Backend source changed; full candidate suite required');
    }
    if (name === 'frontend') {
      assert(Object.values(previous.results.frontend).every(code => code === 0), 'Failed frontend checks cannot be reused');
      const tree = sha => execFileSync('git',['rev-parse',sha+':frontend'],{encoding:'utf8'}).trim();
      assert.equal(tree(previous.candidate),tree(candidate),'Frontend source changed');
    }
    const files = name === 'frontend' ? ['tools','install.log','tests.log','typecheck.log','lint.log','build.log'] : ['tools','tests.jsonl','tests.stderr','build.log'];
    for (const file of files) copyFileSync(join(dirname(previousPath),name+'-'+file),join(evidence,name+'-'+file));
    receipt.results[name]=previous.results[name];
  }
  receipt.reusedFrom=previous.runID;
}

const save = () => writeFileSync(join(evidence, 'full-kind-receipt.json'), JSON.stringify(receipt, null, 2), { mode: 0o600 });
save();
const pod = (name, frontend) => ({ apiVersion: 'v1', kind: 'Pod', metadata: { name, namespace, labels: { 'orchestrator.task': runID } }, spec: {
  restartPolicy: 'Never', automountServiceAccountToken: false,
  volumes: [{ name: 'source', emptyDir: {} }, { name: 'results', emptyDir: {} }, { name: 'tools', emptyDir: {} }],
  ...(!frontend ? { initContainers: [{ name: 'tools', image: 'hashicorp/terraform:1.9.8', command: ['sh', '-ec', 'cp /bin/terraform /tools/terraform'], volumeMounts: [{ name: 'tools', mountPath: '/tools' }] }] } : {}),
  // Cleanup-gate unit tests invoke bash; Alpine's base Node image lacks it.
  containers: [{ name: 'checks', image: frontend ? 'mirror.gcr.io/library/node:22-bookworm' : 'mirror.gcr.io/library/golang:1.25', imagePullPolicy: 'IfNotPresent',
    command: ['sh', '-c', frontend ?
      'while [ ! -f /source/.ready ]; do sleep 1; done; cd /source/frontend; node --version > /results/tools; npm ci > /results/install.log 2>&1; install=$?; if [ "$install" = 0 ]; then npm test -- --maxWorkers=2 > /results/tests.log 2>&1; test=$?; npm run typecheck > /results/typecheck.log 2>&1; types=$?; npm run lint > /results/lint.log 2>&1; lint=$?; npm run build > /results/build.log 2>&1; build=$?; else test=99; types=99; lint=99; build=99; fi; echo "$install $test $types $lint $build" > /results/status; touch /results/.done; sleep 7200' :
      'while [ ! -f /source/.ready ]; do sleep 1; done; cd /source/backend; go version > /results/tools; terraform version >> /results/tools; go test -json -p 2 -parallel 4 -count=1 -timeout 30m ./... > /results/tests.jsonl 2>/results/tests.stderr; test=$?; go build ./... > /results/build.log 2>&1; build=$?; echo "$test $build" > /results/status; touch /results/.done; sleep 7200'],
    env: frontend ? [] : [{ name: 'PATH', value: '/tools:/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin' }, { name: 'GOMAXPROCS', value: '4' }, { name: 'REQUIRE_POSTGRES', value: '1' }, { name: 'REQUIRE_TERRAFORM', value: '1' }, { name: 'ORCHESTRATOR_POSTGRES_TEST_URL', value: 'postgres://postgres:fixture-only@127.0.0.1:5432/postgres?sslmode=disable' }],
    resources: { requests: { cpu: '500m', memory: '512Mi' }, limits: { cpu: '4', memory: '5Gi' } },
    volumeMounts: [{ name: 'source', mountPath: '/source' }, { name: 'results', mountPath: '/results' }, { name: 'tools', mountPath: '/tools' }],
  }, ...(!frontend ? [{ name: 'postgres', image: 'postgres:16-alpine', args: ['postgres', '-c', 'max_locks_per_transaction=1024', '-c', 'max_connections=512'], env: [{ name: 'POSTGRES_PASSWORD', value: 'fixture-only' }], resources: { requests: { cpu: '100m', memory: '256Mi' }, limits: { cpu: '2', memory: '2Gi' } } }] : [])],
} });
try {
  const created = JSON.parse(kubectl(['create', '-f', '-', '-o', 'json'], JSON.stringify({ apiVersion: 'v1', kind: 'Namespace', metadata: { name: namespace, labels: { 'orchestrator.task': runID } } })));
  receipt.namespaceUID = created.metadata.uid; save();
  for (const [name, frontend, sha] of [['candidate', false, candidate], ['main', false, baselineSHA], ['frontend', true, candidate]].filter(([name])=>parts.includes(name))) {
    kubectl(['create', '-f', '-'], JSON.stringify(pod(name, frontend)));
    const deadline = Date.now() + 300_000;
    for (;;) {
      const status = json(['-n', namespace, 'get', 'pod', name, '-o', 'json']);
      if (status.status.containerStatuses?.find(x => x.name === 'checks')?.state?.running) break;
      assert(Date.now() < deadline, name + ' did not start');
      await delay(3000);
    }
    const archive = execFileSync('git', ['archive', sha], { maxBuffer: 128 * 1024 * 1024 });
    kubectl(['-n', namespace, 'exec', '-i', name, '-c', 'checks', '--', 'tar', '-xf', '-', '-C', '/source'], archive);
    if (!frontend) {
      const deadline = Date.now() + 120_000;
      for (;;) {
        try { kubectl(['-n', namespace, 'exec', name, '-c', 'postgres', '--', 'pg_isready', '-U', 'postgres']); break; } catch { assert(Date.now() < deadline, 'PostgreSQL did not start'); await delay(1000); }
      }
    }
    kubectl(['-n', namespace, 'exec', name, '-c', 'checks', '--', 'touch', '/source/.ready']);
    console.log('STARTED', name, sha);
  }
  const deadline = Date.now() + 45 * 60_000;
  const pending = new Set(parts);
  while (pending.size) {
    assert(Date.now() < deadline, 'Full checks deadline exceeded');
    for (const name of pending) {
      try { kubectl(['-n', namespace, 'exec', name, '-c', 'checks', '--', 'test', '-f', '/results/.done']); } catch { continue; }
      const exitCodes = kubectl(['-n', namespace, 'exec', name, '-c', 'checks', '--', 'cat', '/results/status']).trim().split(/\s+/).map(Number);
      const keys = name === 'frontend' ? ['install','test','typecheck','lint','build'] : ['test','build'];
      assert(exitCodes.length === keys.length && exitCodes.every(n => Number.isInteger(n) && n >= 0 && n <= 255), 'Invalid check exit status');
      const result = Object.fromEntries(keys.map((key,i)=>[key,exitCodes[i]]));
      const files = name === 'frontend' ? ['tools', 'install.log', 'tests.log', 'typecheck.log', 'lint.log', 'build.log'] : ['tools', 'tests.jsonl', 'tests.stderr', 'build.log'];
      for (const file of files) writeFileSync(join(evidence, name + '-' + file), kubectl(['-n', namespace, 'exec', name, '-c', 'checks', '--', 'cat', '/results/' + file]), { mode: 0o600 });
      receipt.results[name] = result; save(); pending.delete(name); console.log('COMPLETED', name, JSON.stringify(result));
    }
    if (pending.size) await delay(15_000);
  }
} finally {
  if (receipt.namespaceUID) {
    const current = json(['get', 'namespace', namespace, '-o', 'json']);
    assert.equal(current.metadata.uid, receipt.namespaceUID, 'Cleanup namespace UID mismatch');
    assert.equal(current.metadata.labels['orchestrator.task'], runID, 'Cleanup owner mismatch');
    kubectl(['delete', '--raw', '/api/v1/namespaces/' + namespace, '-f', '-'], JSON.stringify({ apiVersion: 'v1', kind: 'DeleteOptions', preconditions: { uid: current.metadata.uid, resourceVersion: current.metadata.resourceVersion } }));
    const deadline = Date.now() + 180_000;
    while (kubectl(['get', 'namespace', namespace, '--ignore-not-found', '-o', 'name']).trim()) { assert(Date.now() < deadline, 'Namespace cleanup unconfirmed'); await delay(2000); }
    receipt.cleanup = true; save(); console.log('CLEANUP VERIFIED');
  }
}
