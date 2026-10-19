// Shared headed-scenario harness (V04-V06): argument parsing, the gated start, the video recorder, the
// marks/phases/assertions files and the failure evidence. It is the harness of v03-internal-frontend.mjs
// generalised for several scenarios; a scenario function receives the same context object.
import { appendFileSync, readFileSync, writeFileSync } from 'node:fs';
import { requireCaptionLocale, createCaptions } from './captions.mjs';
import { createHuman, installCursor } from './human.mjs';
import { createRecorder, readScenarioInputAfterGate, safeError } from './terraform-v2-lib.mjs';
import { launchHeaded, nativeSelect, screenSize, startRecording, stopRecording, x11Input } from './video.mjs';

export function parseArgs(argv) {
  const args = {};
  for (let i = 0; i < argv.length; i += 1) {
    if (argv[i] === '--scenario') args.scenario = argv[++i] ?? '';
    else if (argv[i] === '--captions') args.captions = argv[++i] ?? '';
    else if (argv[i] === '--evidence') args.evidence = argv[++i] ?? '';
    else if (argv[i] === '--headed') args.headed = true;
    else if (argv[i] === '--check') args.check = argv[++i] ?? '';
    else if (argv[i] === '--verify-evidence') args.verify = argv[++i] ?? '';
    else throw new Error(`unknown argument ${argv[i]}`);
  }
  return args;
}

export function requireScenarioIn(SCENARIOS, id) {
  if (!Object.hasOwn(SCENARIOS, id)) throw new Error(`unsupported scenario "${id}"; supported: ${Object.keys(SCENARIOS).join(', ')}`);
  return SCENARIOS[id];
}

export async function runMain({ SCENARIOS, verifyEvidence }) {
  const requireScenario = (id) => requireScenarioIn(SCENARIOS, id);
  const args = parseArgs(process.argv.slice(2));
  if (args.check !== undefined) { requireScenario(args.check); return; }
  if (args.verify !== undefined) {
    const dir = args.verify;
    const problems = verifyEvidence({
      assertions: JSON.parse(readFileSync(`${dir}/assertions.json`, 'utf8')), phases: JSON.parse(readFileSync(`${dir}/phases.json`, 'utf8')),
      assertionsText: readFileSync(`${dir}/assertions.txt`, 'utf8'),
    });
    if (problems.length) { console.error(problems.join('\n')); process.exit(1); }
    console.log('evidence complete');
    return;
  }
  const scenario = requireScenario(args.scenario);
  if (!args.headed) throw new Error('--headed is required: recordings use a headed browser');
  requireCaptionLocale(args.captions);
  const env = process.env;
  for (const name of ['ORCH_E2E_URL', 'ORCH_E2E_EVIDENCE_DIR', 'ORCH_E2E_INPUT', 'ORCH_E2E_XDOTOOL', 'ORCH_E2E_RESTART_CMD', 'DISPLAY']) if (!env[name]) throw new Error(`${name} is required`);
  const baseURL = env.ORCH_E2E_URL;
  const evidenceDir = env.ORCH_E2E_EVIDENCE_DIR;
  const { width, height } = screenSize(env);
  const videoPath = `${evidenceDir}/${args.scenario}-raw.mp4`;
  const assertionsPath = `${evidenceDir}/assertions.txt`;
  const browser = await launchHeaded({ width, height });
  const context = await browser.newContext({ viewport: null });
  context.setDefaultTimeout(30_000);
  await context.addInitScript(installCursor);
  const marks = [];
  const forwarders = [];
  let input;
  let recorder;
  let startedAt;
  let currentPage;
  let result = 'FAIL';
  const secondsNow = () => (startedAt ? Number(((Date.now() - startedAt) / 1000).toFixed(1)) : 0);
  const captions = createCaptions(secondsNow);
  // V03 labels its few API-driven negative checks 'api' (never counted as UI observation; at least one assertion must still be 'ui').
  const rec = createRecorder({ dir: evidenceDir, now: Date.now, startedAt: () => startedAt ?? Date.now(), extraSources: ['api'] });
  const origRecord = rec.record;
  rec.record = (id, phaseName, source, pass, detail) => {
    appendFileSync(assertionsPath, `${pass ? 'PASS' : 'FAIL'} ${id} [${source}] ${String(detail).slice(0, 220)}\n`);
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
      if (startedAt) captions.writeSrt(`${evidenceDir}/captions.srt`, total);
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
    // The runner arms its safety net first and only then releases the gate, so the first create can never precede it.
    input = await readScenarioInputAfterGate(env.ORCH_E2E_INPUT, env.ORCH_E2E_START_GATE);
    const page = await context.newPage();
    currentPage = page;
    const x11 = x11Input(env.ORCH_E2E_XDOTOOL);
    page.on('dialog', () => {}); // keep native confirmations open until an X11 key answers them
    const h = createHuman(page, { selectNative: nativeSelect(x11) });
    await page.goto(`${baseURL}/ui/sign-in`);
    await page.waitForLoadState('domcontentloaded');
    await page.waitForTimeout(1500);
    x11.focusBrowser();
    recorder = await startRecording({ display: env.DISPLAY, width, height, videoPath });
    startedAt = recorder.spawnedAtMs;
    await page.waitForTimeout(1500);
    const mark = (label) => marks.push({ label, seconds: secondsNow() });
    const phase = (name, text) => { mark(name.toLowerCase()); captions.caption(text); rec.phase(name, text); };
    await scenario({ page, h, context, x11, selectNative: nativeSelect(x11), caption: captions.caption, phase, mark, rec, baseURL, input, forwarders, evidenceDir });
    const recorded = readFileSync(assertionsPath, 'utf8');
    if (/^FAIL /m.test(recorded)) throw new Error('FAIL assertions were recorded during the run');
    result = 'PASS';
    console.log(`PASS: scenario ${args.scenario}`);
  } catch (error) {
    const text = safeError(error);
    appendFileSync(assertionsPath, `FAIL ${text}\n`);
    console.error(String(error?.stack ?? error).split('\n').slice(0, 6).join('\n'));
    try { if (currentPage) await currentPage.screenshot({ path: `${evidenceDir}/failure.png`, timeout: 10_000 }); } catch { /* the page may be closed */ }
    process.exitCode = 1;
  } finally {
    await finalize();
  }
}
