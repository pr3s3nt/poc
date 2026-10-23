import { expect, it, vi } from 'vitest';
import { newReviewRequestId } from './reviewRequestId';

it('generates fresh v4 UUIDs on HTTP when randomUUID is unavailable', () => {
  let draw = 0;
  const getRandomValues = vi.fn((bytes: Uint8Array) => bytes.fill(++draw));
  vi.stubGlobal('crypto', { getRandomValues });
  try {
    const first = newReviewRequestId();
    const second = newReviewRequestId();
    expect(first).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/);
    expect(second).not.toBe(first);
    expect(getRandomValues).toHaveBeenCalledTimes(2);
  } finally { vi.unstubAllGlobals(); }
});
