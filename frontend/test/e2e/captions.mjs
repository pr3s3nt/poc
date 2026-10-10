// Recording captions: the runner announces each step with caption(text); the
// cues become an SRT (and are burned below the browser window by the runner
// script). Captions exist only in the recording, never in the product UI, and
// must not contain tokens, passwords or kubeconfig content.
import { writeFileSync } from 'node:fs';

const MAX_CUE_SECONDS = 14;
const MIN_CUE_SECONDS = 1.5;

// Refactor runners caption only in Vietnamese; any other locale is rejected.
export function requireCaptionLocale(locale) {
  if (locale !== 'vi') throw new Error(`unsupported caption locale "${locale}"; only vi exists`);
  return locale;
}

export function createCaptions(secondsSinceStart) {
  const cues = [];
  return {
    caption(text) {
      cues.push({ start: secondsSinceStart(), text });
    },
    writeSrt(path, endSeconds) {
      const stamp = (seconds) => {
        const ms = Math.round(seconds * 1000);
        const part = (value, size) => String(value).padStart(size, '0');
        return `${part(Math.floor(ms / 3_600_000), 2)}:${part(Math.floor(ms / 60_000) % 60, 2)}:${part(Math.floor(ms / 1000) % 60, 2)},${part(ms % 1000, 3)}`;
      };
      // A cue replaced within MIN_CUE_SECONDS is unreadable: it is dropped and
      // the previous cue keeps the screen.
      const readable = cues.filter((cue, index) => (cues[index + 1]?.start ?? endSeconds) - cue.start >= MIN_CUE_SECONDS || index === cues.length - 1);
      const body = readable.map((cue, index) => {
        const next = readable[index + 1]?.start ?? endSeconds;
        const end = Math.min(next, cue.start + MAX_CUE_SECONDS, endSeconds);
        return `${index + 1}\n${stamp(cue.start)} --> ${stamp(Math.max(end, cue.start + 0.5))}\n${cue.text}\n`;
      }).join('\n');
      writeFileSync(path, body);
      return readable.length;
    },
  };
}

// Deterministic caption rendering. libass interprets SRT font sizes in its
// default 384x288 play resolution, so SRT captions render far too large. The
// ASS file below fixes PlayRes to the real canvas, splits each caption into at
// most two lines and bottom-aligns it inside the caption strip.
const MAX_LINE_CHARS = 100;

export function wrapCaption(text, maxChars = MAX_LINE_CHARS) {
  const words = text.replace(/\s+/g, ' ').trim().split(' ');
  const lines = [''];
  for (const word of words) {
    const last = lines[lines.length - 1];
    if (last && `${last} ${word}`.length > maxChars) lines.push(word);
    else lines[lines.length - 1] = last ? `${last} ${word}` : word;
  }
  if (lines.length > 2) throw new Error(`caption needs ${lines.length} lines (max 2): ${text.slice(0, 40)}…`);
  return lines.join('\\N');
}

export function srtToAss(srt, { width, height, fontSize = 22, marginV = 12 }) {
  const seconds = (stamp) => {
    const [h, m, rest] = stamp.split(':');
    const [s, ms] = rest.split(',');
    return Number(h) * 3600 + Number(m) * 60 + Number(s) + Number(ms) / 1000;
  };
  const clock = (value) => {
    const cs = Math.round(value * 100);
    const part = (v, n) => String(v).padStart(n, '0');
    return `${Math.floor(cs / 360000)}:${part(Math.floor(cs / 6000) % 60, 2)}:${part(Math.floor(cs / 100) % 60, 2)}.${part(cs % 100, 2)}`;
  };
  const cues = srt.trim().split(/\n\s*\n/).map((block) => {
    const [, time, ...text] = block.split('\n');
    const [start, end] = time.split(' --> ');
    return { start: seconds(start), end: seconds(end), text: text.join(' ') };
  });
  // A caption stays until the next one starts (no uncaptioned gaps).
  const events = cues.map((cue, index) => {
    const end = cues[index + 1] ? Math.max(cue.end, cues[index + 1].start) : cue.end;
    return `Dialogue: 0,${clock(cue.start)},${clock(end)},Caption,,0,0,0,,${wrapCaption(cue.text)}`;
  });
  return [
    '[Script Info]', 'ScriptType: v4.00+', `PlayResX: ${width}`, `PlayResY: ${height}`, 'WrapStyle: 2', 'ScaledBorderAndShadow: yes', '',
    '[V4+ Styles]',
    'Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding',
    `Style: Caption,DejaVu Sans,${fontSize},&H00FFFFFF,&H00FFFFFF,&H00000000,&H00000000,0,0,0,0,100,100,0,0,1,0,0,2,30,30,${marginV},1`, '',
    '[Events]', 'Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text', ...events, '',
  ].join('\n');
}
