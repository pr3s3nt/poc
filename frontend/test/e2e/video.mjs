// Full-window recording helpers for headed Chromium on a private Xvfb display.
// ffmpeg grabs the whole X screen (tabs and address bar included); xdotool
// sends real X11 keys to browser chrome and native <select> popups, which
// Playwright's page-level keyboard cannot reach. Used by the recorded kind
// flows; the older local video scripts keep their own copies.
import { chromium, expect } from '@playwright/test';
import { execFileSync, spawn } from 'node:child_process';

export function screenSize(env) {
  const [width, height] = (env.ORCH_E2E_SCREEN ?? '1440x900').split('x').map(Number);
  return { width, height };
}

export async function launchHeaded({ width, height }) {
  return chromium.launch({
    headless: false,
    args: ['--window-position=0,0', `--window-size=${width},${height}`, '--test-type', '--no-first-run', '--password-store=basic', ...(process.env.ORCH_E2E_BROWSER_ARGS ? process.env.ORCH_E2E_BROWSER_ARGS.split('|') : [])],
    ignoreDefaultArgs: ['--enable-automation'],
  });
}

export function x11Input(xdotool) {
  const run = (args) => execFileSync(xdotool, args, { encoding: 'utf8' }).trim();
  let window;
  return {
    focusBrowser() {
      const ids = run(['search', '--onlyvisible', '--class', 'chrom']).split('\n').filter(Boolean);
      if (!ids.length) throw new Error('no visible Chromium window on the X display');
      window = ids[ids.length - 1];
      run(['windowfocus', '--sync', window]);
    },
    keys(sequence) {
      run(['windowfocus', '--sync', window]);
      run(['key', '--clearmodifiers', '--delay', '250', ...sequence]);
    },
    type(text) {
      run(['windowfocus', '--sync', window]);
      run(['type', '--clearmodifiers', '--delay', '90', text]);
    },
  };
}

// Opt-in native <select> handling for createHuman: the click opens the real
// popup, typed text moves the type-ahead to the option and Return picks it.
// Only the first word is typed ("password · secret" -> "password",
// "Workload Service" -> "Workload"): a space could toggle the popup.
export function nativeSelect(x11) {
  return async (h, select, label) => {
    await h.click(select);
    await h.pause(700);
    x11.type(label.split(' ')[0]);
    await h.pause(500);
    x11.keys(['Return']);
    await h.pause(700);
    await expect.poll(() => select.evaluate((element) => element.selectedOptions[0]?.textContent ?? '')).toBe(label);
  };
}

// Starts ffmpeg and fails fast when it cannot open the display or exits early.
export async function startRecording({ display, width, height, videoPath }) {
  const ffmpeg = spawn('ffmpeg', ['-hide_banner', '-loglevel', 'error', '-n', '-f', 'x11grab', '-draw_mouse', '0', '-framerate', '15', '-video_size', `${width}x${height}`, '-i', display,
    '-c:v', 'libx264', '-preset', 'veryfast', '-crf', '26', '-pix_fmt', 'yuv420p', '-movflags', '+faststart', videoPath], { stdio: ['pipe', 'ignore', 'inherit'] });
  ffmpeg.exitPromise = new Promise((resolve) => {
    ffmpeg.once('error', (error) => resolve({ error }));
    ffmpeg.once('exit', (code, signal) => resolve({ code, signal }));
  });
  const early = await Promise.race([ffmpeg.exitPromise, new Promise((resolve) => setTimeout(() => resolve(undefined), 1500))]);
  if (early) throw new Error(`ffmpeg failed to start: ${early.error?.message ?? `code ${early.code} signal ${early.signal}`}`);
  return ffmpeg;
}

// Sends ffmpeg's interactive quit key so it finalizes the MP4 index. A
// recorder that died mid-run or needed a forced kill fails the run.
export async function stopRecording(ffmpeg) {
  let forced = false;
  if (ffmpeg.exitCode === null && ffmpeg.signalCode === null) ffmpeg.stdin.end('q');
  const timer = setTimeout(() => { forced = true; ffmpeg.kill('SIGKILL'); }, 30_000);
  const result = await ffmpeg.exitPromise;
  clearTimeout(timer);
  if (forced || result.error || result.code !== 0) {
    throw new Error(`ffmpeg did not finish cleanly: ${result.error?.message ?? `code ${result.code} signal ${result.signal}`}${forced ? ' (forced)' : ''}`);
  }
}
