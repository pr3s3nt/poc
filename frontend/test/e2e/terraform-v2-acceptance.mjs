// Current headed AWS acceptance entry: V05, A02 and A03 use Definitions and
// resource references. Historical M04 A01/E01/E02 handlers were retired after
// V04 replacement coverage; their evidence remains labelled in verification/Git.
// node terraform-v2-acceptance.mjs --check V05
import { appendFileSync, writeFileSync } from 'node:fs';
import { requireCaptionLocale, createCaptions } from './captions.mjs';
import { createHuman, installCursor } from './human.mjs';
import { createRecorder, readScenarioInputAfterGate, safeError } from './terraform-v2-lib.mjs';
import { v2ctl } from './full-app-kit.mjs';
import { awsDefinitionScenario } from './terraform-v2-aws-definitions.mjs';
import { launchHeaded, nativeSelect, screenSize, startRecording, stopRecording, x11Input } from './video.mjs';

const FORBIDDEN_IN_PAGE = [/AKIA/, /vault:\/\//i, /tfstate/i, /kubeconfig/i, /secretRef/i, /\/var\/lib/, /\.terraform/, /recoveryPath/i, /BEGIN [A-Z ]*PRIVATE KEY/];
const SCENARIOS = {
  async V05(ctx) { return awsDefinitionScenario(ctx, 'V05', { forbidden: FORBIDDEN_IN_PAGE }); },
  async A02(ctx) { return awsDefinitionScenario(ctx, 'A02', { forbidden: FORBIDDEN_IN_PAGE }); },
  async A03(ctx) { return awsDefinitionScenario(ctx, 'A03', { forbidden: FORBIDDEN_IN_PAGE }); },
};

export function parseArgs(argv) {
  const args = { scenario: '', captions: '', evidence: '', headed: false, devHeadless: false, browserOnly: false };
  for (let i = 0; i < argv.length; i += 1) {
    if (argv[i] === '--scenario') args.scenario = argv[++i] ?? '';
    else if (argv[i] === '--captions') args.captions = argv[++i] ?? '';
    else if (argv[i] === '--evidence') args.evidence = argv[++i] ?? '';
    else if (argv[i] === '--headed') args.headed = true;
    else if (argv[i] === '--browser-only') args.browserOnly = true;
    else if (argv[i] === '--dev-headless') args.devHeadless = true;
    else if (argv[i] === '--check') args.check = argv[++i] ?? '';
    else throw new Error(`unknown argument ${argv[i]}`);
  }
  return args;
}

export function requireScenario(id) {
  if (['A01', 'E01', 'E02'].includes(id)) {
    throw new Error(`retired scenario "${id}": its Environment Connection/builtin-cluster path is historical; use V04 for the existing-cluster full application or V05/A02/A03 for Definition-driven AWS scenarios`);
  }
  if (!Object.hasOwn(SCENARIOS, id)) throw new Error(`unsupported scenario "${id}"; supported: ${Object.keys(SCENARIOS).join(', ')}`);
  return SCENARIOS[id];
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  if (args.check !== undefined) { requireScenario(args.check); return; }
  const scenario = requireScenario(args.scenario);
  if (!args.browserOnly && !args.headed && !(args.devHeadless && process.env.ORCH_E2E_DEV_HEADLESS_OK === '1')) throw new Error('--headed is required: recordings use a headed browser');
  if (!args.browserOnly) requireCaptionLocale(args.captions);
  const env = process.env;
  for (const name of ['ORCH_E2E_URL', 'ORCH_E2E_EVIDENCE_DIR', 'ORCH_E2E_INPUT']) if (!env[name]) throw new Error(`${name} is required`);
  if (args.headed) for (const name of ['ORCH_E2E_XDOTOOL', 'DISPLAY']) if (!env[name]) throw new Error(`${name} is required`);
  const baseURL = env.ORCH_E2E_URL;
  const evidenceDir = env.ORCH_E2E_EVIDENCE_DIR;
  let input;
  const { width, height } = screenSize(env);
  const videoPath = `${evidenceDir}/${args.scenario}-raw.mp4`;
  const assertionsPath = `${evidenceDir}/assertions.txt`;
  writeFileSync(`${evidenceDir}/browser-mode.json`, JSON.stringify({ browserOnly: args.browserOnly, headed: args.headed, recording: args.headed && !args.browserOnly, inputPacing: args.browserOnly ? 'Playwright actionability; no presentation pauses' : 'historical human-paced' }, null, 2));

  const browser = args.headed ? await launchHeaded({ width, height }) : await (await import('@playwright/test')).chromium.launch(env.ORCH_E2E_BROWSER_EXECUTABLE ? { executablePath: env.ORCH_E2E_BROWSER_EXECUTABLE } : {});
  const context = await browser.newContext({ locale: 'vi-VN', ...(args.headed ? { viewport: null } : { viewport: { width, height } }) });
  context.setDefaultTimeout(30_000);
  if (!args.browserOnly) await context.addInitScript(installCursor);
  const marks = [];
  const forwarders = [];
  // Every Preview names the Terraform run ID its Deploy will use. It is recorded in the private manifest BEFORE the Deploy click:
  // that record is the only authority the independent safety net has over Terraform instances.
  const previewRuns = [];
  context.on('response', (response) => {
    if (response.request().method() !== 'POST' || !new URL(response.url()).pathname.endsWith('/preview')) return;
    previewRuns.push((async () => {
      const body = await response.json();
      if (!body?.runId) throw new Error('the Preview carries no run id');
      v2ctl(input, 'manifest-run', '-manifest', input.manifestFile, '-id', body.runId);
    })());
  });
  let recorder;
  let startedAt;
  let currentPage;
  let result = 'FAIL';
  const secondsNow = () => (startedAt ? Number(((Date.now() - startedAt) / 1000).toFixed(1)) : 0);
  const captions = createCaptions(secondsNow);
  const rec = createRecorder({ dir: evidenceDir, now: Date.now, startedAt: () => startedAt ?? Date.now() });
  const origRecord = rec.record;
  rec.record = (id, phaseName, source, pass, detail) => {
    appendFileSync(assertionsPath, `${pass ? 'PASS' : 'FAIL'} ${id} [${source}] ${String(detail).slice(0, 200)}\n`);
    return origRecord(id, phaseName, source, pass, detail);
  };
  let finalized = false;
  const finalize = async () => {
    if (finalized) return;
    finalized = true;
    for (const f of forwarders) f.stop();
    try {
      if (recorder) await stopRecording(recorder);
    } finally {
      const total = startedAt ? (Date.now() - startedAt) / 1000 : 0;
      writeFileSync(`${evidenceDir}/marks.json`, JSON.stringify(marks, null, 2));
      if (startedAt && !args.browserOnly) captions.writeSrt(`${evidenceDir}/captions.srt`, total);
      writeFileSync(`${evidenceDir}/result.txt`, `${result}\n`);
      await browser.close();
    }
  };
  for (const [signal, code] of [['SIGHUP', 129], ['SIGINT', 130], ['SIGTERM', 143]]) {
    process.once(signal, () => {
      appendFileSync(assertionsPath, `FAIL interrupted by ${signal}\n`);
      finalize().catch((error) => console.error(safeError(error))).finally(() => process.exit(code));
    });
  }
  try {
    // The runner arms the independent cleanup supervisor (it watches this process) and
    // only then releases the gate, so the first create can never precede the supervisor.
    input = await readScenarioInputAfterGate(env.ORCH_E2E_INPUT, env.ORCH_E2E_START_GATE);
    const page = await context.newPage();
    currentPage = page;
    const x11 = args.headed ? x11Input(env.ORCH_E2E_XDOTOOL) : undefined;
    if (x11) page.on('dialog', () => {}); // keep native confirmations open until an X11 key answers them
    const selectNative = x11 ? nativeSelect(x11) : undefined;
    const h = createHuman(page, { selectNative, fast: args.browserOnly });
    await page.goto(`${baseURL}/ui/sign-in`);
    await page.waitForLoadState('domcontentloaded');
    if (!args.browserOnly) await page.waitForTimeout(1500);
    if (x11 && !args.browserOnly) {
      x11.focusBrowser();
      recorder = await startRecording({ display: env.DISPLAY, width, height, videoPath });
      startedAt = recorder.spawnedAtMs;
    } else {
      startedAt = Date.now();
    }
    if (!args.browserOnly) await page.waitForTimeout(1500);
    const stopPhase = args.devHeadless ? env.ORCH_E2E_DEV_STOP_PHASE : undefined; // development harness only: never set by the runner
    const phase = (name, text) => {
      if (stopPhase && name === stopPhase) throw new Error(`development stop at ${name}`);
      marks.push({ label: name.toLowerCase(), seconds: secondsNow() }); captions.caption(text); rec.phase(name, text);
    };
    const deploy = async () => {
      await Promise.all(previewRuns); // the run id of the Preview is in the manifest before anything is created
      if (previewRuns.length === 0) throw new Error('Deploy without a recorded Preview run id');
      await h.click(page.getByRole('button', { name: 'Triển khai các thay đổi này', exact: true }));
    };
    await scenario({ browserOnly: args.browserOnly, page, h, context, x11, selectNative, caption: captions.caption, phase, rec, baseURL, input, forwarders, evidenceDir, deploy });
    result = 'PASS';
    console.log(`PASS: scenario ${args.scenario}`);
  } catch (error) {
    const text = safeError(error);
    appendFileSync(assertionsPath, `FAIL ${text}\n`);
    console.error(String(error?.stack ?? error).split('\n').slice(0, 6).join('\n'));
    // The Console and the acceptance app carry no secret text (scanned like every public file); the image documents where it failed.
    try { if (currentPage) await currentPage.screenshot({ path: `${evidenceDir}/failure.png`, timeout: 10_000 }); } catch { /* the page may be closed */ }
    process.exitCode = 1;
  } finally {
    await finalize();
  }
}

if (import.meta.url === `file://${process.argv[1]}`) {
  main().catch((error) => { console.error(safeError(error)); process.exit(2); });
}
