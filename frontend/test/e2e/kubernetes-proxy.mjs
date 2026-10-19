// Authenticated loopback transport used only by owned technical-Pod cleanup.
// Business scenario API reads stay in the scenario; UID/label admission remains in its caller.
import { spawn } from 'node:child_process';
import { createServer } from 'node:net';
import { statSync } from 'node:fs';

export function validateKubectlTarget(kube) {
  if (!kube || typeof kube.kubectl !== 'string' || !kube.kubectl.startsWith('/')) throw new Error('resolved Kubernetes target has no absolute kubectl path');
  if (typeof kube.kubeconfig !== 'string' || !kube.kubeconfig.startsWith('/')) throw new Error('resolved Kubernetes target has no private kubeconfig');
  try { if (!statSync(kube.kubeconfig).isFile()) throw new Error(); } catch { throw new Error('resolved Kubernetes target has no private kubeconfig'); }
  if (typeof kube.context !== 'string' || kube.context.trim() === '') throw new Error('resolved Kubernetes target has no explicit context');
  if (!kube.env || typeof kube.env !== 'object' || Object.keys(kube.env).length === 0) throw new Error('resolved Kubernetes target has no scoped execution environment');
  if (Object.hasOwn(kube.env, 'KUBECONFIG')) throw new Error('resolved Kubernetes target must not use host KUBECONFIG fallback');
}

async function boundedFetch(url, options = {}, timeoutMs = 10000) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const response = await fetch(url, { ...options, signal: controller.signal });
    const text = await response.text(); // deadline includes reading the body
    let body = {};
    try { body = JSON.parse(text); } catch { /* status still distinguishes API errors */ }
    return { status: response.status, ok: response.ok, body };
  } finally { clearTimeout(timer); }
}

export async function stopOwnedProcess(child, label = 'owned Kubernetes API proxy') {
  if (!child || !child.pid) throw new Error(`${label} did not start`);
  const stopped = () => child.exitCode !== null || child.signalCode !== null;
  const waitForExit = (ms) => stopped() ? Promise.resolve() : new Promise((resolve) => {
    let timer;
    const done = () => { clearTimeout(timer); child.removeListener('exit', done); child.removeListener('error', done); resolve(); };
    child.once('exit', done);
    child.once('error', done);
    timer = setTimeout(done, ms);
  });
  if (!stopped()) {
    const graceful = waitForExit(5000); // listeners are installed before the signal
    try { child.kill('SIGTERM'); } catch (error) { if (!stopped()) throw error; }
    await graceful;
  }
  if (!stopped()) {
    const forced = waitForExit(2000);
    try { child.kill('SIGKILL'); } catch (error) { if (!stopped()) throw error; }
    await forced;
  }
  if (!stopped()) throw new Error(`${label} process could not be stopped`);
}

export async function startKubectlProxy(kube) {
  validateKubectlTarget(kube);
  const reservation = createServer();
  await new Promise((resolve, reject) => reservation.once('error', reject).listen(0, '127.0.0.1', resolve));
  const port = reservation.address().port;
  await new Promise((resolve) => reservation.close(resolve));
  const child = spawn(kube.kubectl, ['--kubeconfig', kube.kubeconfig, '--context', kube.context, 'proxy', '--address=127.0.0.1', `--port=${port}`], {
    stdio: 'ignore', env: { PATH: process.env.PATH, HOME: process.env.HOME ?? '/tmp', ...kube.env },
  });
  const base = `http://127.0.0.1:${port}`;
  let spawnError;
  child.once('error', (error) => { spawnError = error; });
  const stop = () => !child.pid && spawnError ? Promise.resolve() : stopOwnedProcess(child);
  let ready = false;
  try {
    const readyDeadline = Date.now() + 10000;
    while (Date.now() < readyDeadline && !spawnError && child.exitCode === null && child.signalCode === null) {
      try { const response = await boundedFetch(`${base}/version`, {}, Math.min(2000, readyDeadline - Date.now())); if (response.ok) { ready = true; break; } } catch { /* Retry within the fixed readiness deadline; no credentials or raw transport error are exposed. */ }
      await new Promise((resolve) => setTimeout(resolve, 100));
    }
    if (!ready) throw new Error('authenticated Kubernetes API proxy did not become ready');
  } catch (error) {
    await stop();
    throw error;
  }
  return {
    request: async (method, path, body) => {
      const response = await boundedFetch(`${base}${path}`, { method, headers: body ? { 'content-type': 'application/json' } : undefined, body: body ? JSON.stringify(body) : undefined }, 10000);
      return { status: response.status, body: response.body };
    },
    stop,
  };
}
