// node render-captions.mjs <in.srt> <out.ass> <canvas-width> <canvas-height>
// Converts recorded SRT captions to an ASS file with explicit resolution so a
// burn-in below the browser window is deterministic (see captions.mjs).
import { readFileSync, writeFileSync } from 'node:fs';
import { srtToAss } from './captions.mjs';

const [input, output, width, height] = process.argv.slice(2);
if (!input || !output || !width || !height) {
  console.error('usage: render-captions.mjs <in.srt> <out.ass> <width> <height>');
  process.exit(2);
}
writeFileSync(output, srtToAss(readFileSync(input, 'utf8'), { width: Number(width), height: Number(height) }));
