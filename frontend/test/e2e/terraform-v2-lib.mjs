// Operator helpers of the real Terraform v2 acceptance scenarios (M04 I04-10).
// Everything here is read-only technical access to the REAL resources through
// the host kubeconfig handle (private, never a product credential) and
// evidence recording. Nothing here creates a job, a Workload or a resource,
// and no helper ever prints an argument list, a credential or a value read
// from a Secret: errors are reduced to a fixed first line without arguments.
import { execFile, spawn } from 'node:child_process';
import { createHash } from 'node:crypto';
import { existsSync, readFileSync, writeFileSync } from 'node:fs';

export const PHASES = ['PREPARE', 'FORM', 'PREVIEW', 'TERRAFORM', 'READY', 'JOB', 'CACHE', 'CLEANUP'];
export const SOURCES = ['ui', 'private-read'];

/** Reduces an error to one safe line: never the command line or stderr tail. */
export function safeError(error) {
  const text = String(error?.safe ?? error?.message ?? error).split('\n')[0];
  return text.replace(/--(?:pass|password|token|kubeconfig)[= ]\S+/gi, '--$1 …').slice(0, 200);
}

/** Records assertions and phase marks; the files are rewritten on every call so a failed run keeps them. */
export function createRecorder({ dir, now = () => Date.now(), startedAt = () => 0 }) {
  const assertions = [];
  const phases = [];
  const flush = () => {
    writeFileSync(`${dir}/assertions.json`, JSON.stringify(assertions, null, 2));
    writeFileSync(`${dir}/phases.json`, JSON.stringify(phases, null, 2));
  };
  return {
    assertions, phases,
    /** One assertion. source is 'ui' (observed in the Console / app UI) or 'private-read' (read-only query of the real resource). */
    record(id, phase, source, pass, detail = '') {
      if (!PHASES.includes(phase)) throw new Error(`unknown phase ${phase}`);
      if (!SOURCES.includes(source)) throw new Error(`assertion source ${source} cannot count as live proof`);
      if (assertions.some((a) => a.id === id)) throw new Error(`duplicate assertion ${id}`);
      assertions.push({ id, phase, pass: Boolean(pass), source, detail: String(detail).slice(0, 300) });
      flush();
      if (!pass) throw new Error(`assertion ${id} failed: ${String(detail).slice(0, 200)}`);
    },
    /** Phase mark with its Vietnamese caption, in video seconds. */
    phase(name, caption) {
      if (!PHASES.includes(name)) throw new Error(`unknown phase ${name}`);
      phases.push({ phase: name, atSec: Number(((now() - startedAt()) / 1000).toFixed(1)), caption });
      flush();
    },
  };
}

/** kubectl through the private host kubeconfig handle. Resolves {stdout, stderr, code}; never rejects on a non-zero exit. */
export function kubectlRunner({ kubectl, kubeconfig, context, env: extraEnv = {} }) {
  return (args, { timeoutMs = 60_000, input } = {}) => new Promise((resolve) => {
    const child = execFile(kubectl, ['--kubeconfig', kubeconfig, '--context', context, ...args], { timeout: timeoutMs, maxBuffer: 16 << 20, env: { PATH: process.env.PATH, HOME: process.env.HOME ?? '/tmp', ...extraEnv } }, (error, stdout, stderr) => {
      resolve({ stdout, stderr, code: error ? (typeof error.code === 'number' ? error.code : 1) : 0 });
    });
    if (input !== undefined) child.stdin.end(input);
  });
}

/** Polls fn until it returns a truthy value; throws a safe timeout error naming only `what`. */
export async function waitFor(fn, { timeoutMs, intervalMs = 1000, what }) {
  const deadline = Date.now() + timeoutMs;
  let last;
  for (;;) {
    try {
      const value = await fn();
      if (value) return value;
    } catch (error) {
      last = safeError(error);
    }
    if (Date.now() >= deadline) throw new Error(`timed out waiting for ${what}${last ? ` (${last})` : ''}`);
    await new Promise((resolve) => setTimeout(resolve, intervalMs));
  }
}

/** Reads the latest runner-owned input only after its supervisor gate is released. */
export async function readScenarioInputAfterGate(inputPath, gatePath, { timeoutMs = 180_000, intervalMs = 200 } = {}) {
  if (gatePath) await waitFor(() => existsSync(gatePath), { timeoutMs, intervalMs, what: 'the cleanup supervisor to be armed' });
  let raw;
  try {
    raw = readFileSync(inputPath, 'utf8');
  } catch {
    throw new Error('scenario input is unavailable after the supervisor gate');
  }
  try {
    return JSON.parse(raw);
  } catch {
    throw new Error('scenario input is invalid after the supervisor gate');
  }
}

/** Parses name[:tag][@sha256:digest] with an optional registry host. */
export function parseImageRef(ref) {
  const m = /^(?:(?<host>[a-z0-9.-]+(?::\d+)?)\/)?(?<repo>[a-z0-9][a-z0-9._/-]*?)(?::(?<tag>[A-Za-z0-9._-]{1,128}))?(?:@(?<digest>sha256:[0-9a-f]{64}))?$/.exec(ref);
  if (!m) return undefined;
  let { host, repo } = m.groups;
  // "redis/x" without a dot or colon in the first segment is a Docker Hub namespace.
  if (host && !/[.:]/.test(host) && host !== 'localhost') { repo = `${host}/${repo}`; host = undefined; }
  if (!host) host = 'registry-1.docker.io';
  if (host === 'registry-1.docker.io' && !repo.includes('/')) repo = `library/${repo}`;
  return { host, repo, tag: m.groups.tag, digest: m.groups.digest };
}

/** A syntactically valid digest that no registry holds: derived from the run, not random, so it is reproducible. */
export function nonexistentDigest(seed) {
  return `sha256:${createHash('sha256').update(`i0410-nonexistent-digest:${seed}`).digest('hex')}`;
}

/**
 * Asks the registry whether a manifest digest exists (anonymous pull token for
 * Docker Hub). 'missing' only on a definitive 404; anything else is 'unknown',
 * so a network failure can never prove the fixture.
 */
export async function registryDigestStatus(ref, fetchImpl = fetch) {
  const parsed = parseImageRef(ref);
  if (!parsed?.digest) return 'unknown';
  try {
    const headers = { accept: 'application/vnd.oci.image.index.v1+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json, application/vnd.docker.distribution.manifest.list.v2+json' };
    if (parsed.host === 'registry-1.docker.io') {
      const token = await fetchImpl(`https://auth.docker.io/token?service=registry.docker.io&scope=repository:${parsed.repo}:pull`);
      if (!token.ok) return 'unknown';
      headers.authorization = `Bearer ${(await token.json()).token}`;
    }
    const res = await fetchImpl(`https://${parsed.host}/v2/${parsed.repo}/manifests/${parsed.digest}`, { method: 'HEAD', headers });
    if (res.status === 404) return 'missing';
    if (res.status === 200) return 'present';
    return 'unknown';
  } catch {
    return 'unknown';
  }
}

/** kubectl port-forward as a child in its own process group; stop() ends the group. */
export function startPortForward({ kubectl, kubeconfig, context, namespace, service, remotePort, localPort, env: extraEnv = {} }) {
  const child = spawn(kubectl, ['--kubeconfig', kubeconfig, '--context', context, '-n', namespace, 'port-forward', `service/${service}`, `${localPort}:${remotePort}`, '--address', '127.0.0.1'], { stdio: 'ignore', detached: true, env: { PATH: process.env.PATH, HOME: process.env.HOME ?? '/tmp', ...extraEnv } });
  const exited = new Promise((resolve) => child.once('exit', resolve));
  return {
    url: `http://127.0.0.1:${localPort}`,
    exited,
    stop() { try { process.kill(-child.pid, 'SIGTERM'); } catch { /* already gone */ } },
  };
}

/** Counts the Kubernetes objects of one kind by container image substring (read-only). */
export function pickByImage(list, substring) {
  return (list.items ?? []).filter((item) => (item.spec?.template?.spec?.containers ?? []).some((c) => String(c.image).includes(substring)));
}
