import type { Candidate } from './api';

// Selection rules of the deletion dialog (UC-07 DEL-BR-01). The backend re-checks the whole set and
// rejects it as a whole; these rules only keep the dialog from offering a set it knows is invalid.
// Nothing here ever ticks an item the person did not tick.

export type Plan = { root: Candidate; candidates: readonly Candidate[] };

/** Only the root is ticked at first; dependencies are left for the person to choose. */
export function initialSelection(plan: Plan): ReadonlySet<string> {
  return new Set([plan.root.id]);
}

export type Availability = { available: true } | { available: false; reasons: string[]; waitingFor: string[] };

/**
 * A dependency may be ticked only when nothing blocks it for good and every consumer that still
 * holds it is inside the selected set. The root is always selected and never offered as a choice.
 */
export function availability(candidate: Candidate, selected: ReadonlySet<string>): Availability {
  if (!candidate.selectable) return { available: false, reasons: candidate.reasons, waitingFor: [] };
  const waitingFor = candidate.requiredBy.filter((id) => !selected.has(id));
  return waitingFor.length ? { available: false, reasons: ['HAS_CONSUMERS'], waitingFor } : { available: true };
}

export type Change = { selected: ReadonlySet<string>; removed: readonly string[] };

/** Ticks one dependency. Refused (unchanged) when it is not available; never ticks anything else. */
export function tick(plan: Plan, selected: ReadonlySet<string>, id: string): Change {
  const candidate = plan.candidates.find((c) => c.id === id);
  if (!candidate || selected.has(id) || !availability(candidate, selected).available) return { selected, removed: [] };
  return { selected: new Set([...selected, id]), removed: [] };
}

/**
 * Unticks one dependency and then unticks, repeatedly, every ticked dependency that has become
 * invalid because one of its consumers is no longer selected. The removed IDs are returned so the
 * dialog can explain the change. The root cannot be unticked.
 */
export function untick(plan: Plan, selected: ReadonlySet<string>, id: string): Change {
  if (id === plan.root.id || !selected.has(id)) return { selected, removed: [] };
  const next = new Set(selected);
  next.delete(id);
  const removed: string[] = [];
  for (let changed = true; changed;) {
    changed = false;
    for (const candidate of plan.candidates) {
      if (!next.has(candidate.id)) continue;
      if (candidate.requiredBy.some((consumer) => !next.has(consumer))) {
        next.delete(candidate.id);
        removed.push(candidate.id);
        changed = true;
      }
    }
  }
  return { selected: next, removed };
}

/** The chosen IDs in display order (root first, then the preview order). */
export function chosen(plan: Plan, selected: ReadonlySet<string>): Candidate[] {
  return [plan.root, ...plan.candidates].filter((c) => selected.has(c.id));
}

/** True when the set is exactly what the dialog may submit: the root plus available dependencies. */
export function validSelection(plan: Plan, selected: ReadonlySet<string>): boolean {
  if (!selected.has(plan.root.id) || !plan.root.selectable) return false;
  return plan.candidates.every((c) => !selected.has(c.id) || (c.selectable && c.requiredBy.every((consumer) => selected.has(consumer))));
}
