import { mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterEach, describe, expect, it } from 'vitest';
import { readScenarioInputAfterGate } from './terraform-v2-lib.mjs';

const temporaryDirs = new Set();
function fixture() {
  const dir = mkdtempSync(join(tmpdir(), 'v2-input-gate-'));
  temporaryDirs.add(dir);
  return { dir, input: join(dir, 'scenario-input.json'), gate: join(dir, 'STARTED') };
}

afterEach(() => {
  for (const dir of temporaryDirs) rmSync(dir, { recursive: true, force: true });
  temporaryDirs.clear();
});

describe('runner input supervisor gate', () => {
  it('reads published ECR repositories and image refs written before gate release', async () => {
    const f = fixture();
    writeFileSync(f.input, JSON.stringify({ images: { backend: 'old-image' } }));

    const pending = readScenarioInputAfterGate(f.input, f.gate, { timeoutMs: 1000, intervalMs: 5 });
    await new Promise((resolve) => setTimeout(resolve, 25));
    const published = {
      images: {
        backend: '123456789012.dkr.ecr.us-east-1.amazonaws.com/run/backend@sha256:abc',
        frontend: '123456789012.dkr.ecr.us-east-1.amazonaws.com/run/frontend@sha256:def',
        worker: '123456789012.dkr.ecr.us-east-1.amazonaws.com/run/worker@sha256:ghi',
      },
      aws: { ecrPreparation: { repositories: [
        'arn:aws:ecr:us-east-1:123456789012:repository/run/backend',
        'arn:aws:ecr:us-east-1:123456789012:repository/run/frontend',
        'arn:aws:ecr:us-east-1:123456789012:repository/run/worker',
      ] } },
    };
    writeFileSync(f.input, JSON.stringify(published));
    writeFileSync(f.gate, 'supervisor armed\n');

    await expect(pending).resolves.toEqual(published);
  });

  it('fails closed when the released input is missing or malformed', async () => {
    const missing = fixture();
    writeFileSync(missing.gate, 'supervisor armed\n');
    await expect(readScenarioInputAfterGate(missing.input, missing.gate)).rejects.toThrow('scenario input is unavailable');

    const malformed = fixture();
    writeFileSync(malformed.input, '{');
    writeFileSync(malformed.gate, 'supervisor armed\n');
    await expect(readScenarioInputAfterGate(malformed.input, malformed.gate)).rejects.toThrow('scenario input is invalid');
  });

  it('does not read input or proceed when the supervisor gate never opens', async () => {
    const f = fixture();
    writeFileSync(f.input, '{');
    await expect(readScenarioInputAfterGate(f.input, f.gate, { timeoutMs: 10, intervalMs: 1 }))
      .rejects.toThrow('timed out waiting for the cleanup supervisor to be armed');
  });
});
