import { describe, expect, it } from 'vitest';
import { availability, chosen, initialSelection, tick, untick, validSelection, type Plan } from './selection';
import { candidate, depA, depB, depC, dependencyA, dependencyB, dependencyC, rootCandidate, rootId } from '../../test/resources';

const plan: Plan = { root: rootCandidate, candidates: [dependencyA, dependencyB, dependencyC] };
// A chain: root -> mid -> leaf, plus an independent branch.
const mid = candidate({ id: 'mid', descriptor: 'vpc.default#environments.shop.mid' });
const leaf = candidate({ id: 'leaf', descriptor: 'k8s-namespace.default#environments.shop.leaf', requiredBy: ['mid'] });
const twin = candidate({ id: 'twin', descriptor: 'k8s-secret.default#environments.shop.twin', requiredBy: [rootId, 'mid'] });
const chain: Plan = { root: rootCandidate, candidates: [mid, leaf, twin] };

describe('deletion selection rules', () => {
  it('ticks only the root at first and never a dependency on its own', () => {
    expect([...initialSelection(plan)]).toEqual([rootId]);
    expect([...initialSelection(chain)]).toEqual([rootId]);
  });

  it('offers a dependency only when no permanent blocker applies and every consumer is selected', () => {
    const selected = initialSelection(plan);
    expect(availability(dependencyA, selected)).toEqual({ available: true });
    expect(availability(dependencyB, selected)).toMatchObject({ available: false, reasons: ['HAS_CONSUMERS'] });
    expect(availability(leaf, initialSelection(chain))).toEqual({ available: false, reasons: ['HAS_CONSUMERS'], waitingFor: ['mid'] });
    expect(availability(leaf, new Set([rootId, 'mid']))).toEqual({ available: true });
    expect(availability(twin, new Set([rootId]))).toMatchObject({ available: false, waitingFor: ['mid'] });
  });

  it('refuses to tick a locked or waiting item and adds nothing else', () => {
    const start = initialSelection(plan);
    expect(tick(plan, start, depB).selected).toBe(start);
    expect(tick(chain, initialSelection(chain), 'leaf').selected.has('leaf')).toBe(false);
    const next = tick(plan, start, depA);
    expect([...next.selected].sort()).toEqual([rootId, depA].sort());
    expect(next.selected.has(depC)).toBe(false);
  });

  it('unticking cascades to every selection that became invalid, explains it and ticks nothing', () => {
    let selected: ReadonlySet<string> = initialSelection(chain);
    selected = tick(chain, selected, 'mid').selected;
    selected = tick(chain, selected, 'leaf').selected;
    selected = tick(chain, selected, 'twin').selected;
    expect([...selected].sort()).toEqual(['leaf', 'mid', 'twin', rootId].sort());
    const result = untick(chain, selected, 'mid');
    // leaf needs mid; twin needs both the root and mid: both become invalid and are unticked.
    expect([...result.selected]).toEqual([rootId]);
    expect(result.removed).toEqual(['leaf', 'twin']);
    expect(validSelection(chain, result.selected)).toBe(true);
    // Nothing is ticked again by itself: ticking mid back does not bring leaf or twin back.
    expect([...tick(chain, result.selected, 'mid').selected].sort()).toEqual(['mid', rootId].sort());
  });

  it('the root cannot be unticked and unticking an unselected item changes nothing', () => {
    const start = initialSelection(plan);
    expect(untick(plan, start, rootId).selected).toBe(start);
    expect(untick(plan, start, depA).selected).toBe(start);
  });

  it('validates the submitted set as a whole and lists exactly the chosen items in order', () => {
    expect(validSelection(plan, new Set([rootId]))).toBe(true);
    expect(validSelection(plan, new Set([depA]))).toBe(false);
    expect(validSelection(plan, new Set([rootId, depB]))).toBe(false);
    expect(validSelection(chain, new Set([rootId, 'leaf']))).toBe(false);
    expect(chosen(plan, new Set([depC, rootId, depA])).map((c) => c.id)).toEqual([rootId, depA, depC]);
    expect(validSelection({ root: { ...rootCandidate, selectable: false, reasons: ['NOT_UNREFERENCED'] }, candidates: [] }, new Set([rootId]))).toBe(false);
  });
});
