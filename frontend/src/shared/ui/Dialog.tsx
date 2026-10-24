import { useEffect, useRef, type KeyboardEvent, type ReactNode } from 'react';

const FOCUSABLE = 'button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [href], [tabindex]:not([tabindex="-1"])';

// Modal dialog on the existing `.dialog` styles: focus moves in on open, Tab stays
// inside, Escape cancels unless `busy`, and focus returns to the opener on close.
// It never nests another dialog and never closes in the middle of a mutation.
export function Dialog({ label, busy = false, onClose, children }: { label: string; busy?: boolean; onClose(): void; children: ReactNode }) {
  const dialog = useRef<HTMLDivElement>(null);
  const opener = useRef<Element | null>(null);
  useEffect(() => {
    opener.current = document.activeElement;
    const first = dialog.current?.querySelector<HTMLElement>('input:not(:disabled), select:not(:disabled), button:not(:disabled)');
    (first ?? dialog.current)?.focus();
    return () => { if (opener.current instanceof HTMLElement && opener.current.isConnected) opener.current.focus(); };
  }, []);

  // A disabled control drops focus to the page while a mutation runs; Escape and Tab must still reach
  // the dialog, and focus returns inside it when the mutation ends.
  const latest = useRef({ busy, onClose });
  latest.current = { busy, onClose };
  useEffect(() => {
    const onDocumentKey = (event: globalThis.KeyboardEvent) => {
      if (event.key === 'Tab' && !event.defaultPrevented && dialog.current && !dialog.current.contains(document.activeElement)) {
        event.preventDefault();
        (dialog.current.querySelector<HTMLElement>(FOCUSABLE) ?? dialog.current).focus();
      }
      if (event.key === 'Escape' && !event.defaultPrevented && !latest.current.busy) { event.preventDefault(); latest.current.onClose(); }
    };
    document.addEventListener('keydown', onDocumentKey);
    return () => document.removeEventListener('keydown', onDocumentKey);
  }, []);
  useEffect(() => {
    if (!dialog.current) return;
    if (busy) { dialog.current.focus(); return; }
    if (dialog.current.contains(document.activeElement) && document.activeElement !== dialog.current) return;
    const first = dialog.current.querySelector<HTMLElement>('input:not(:disabled), select:not(:disabled), button:not(:disabled)');
    (first ?? dialog.current).focus();
  }, [busy]);

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key === 'Escape') { event.preventDefault(); if (!busy) onClose(); return; }
    if (event.key !== 'Tab' || !dialog.current) return;
    const focusable = [...dialog.current.querySelectorAll<HTMLElement>(FOCUSABLE)];
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    if (!first || !last) { event.preventDefault(); dialog.current.focus(); return; }
    if (event.shiftKey && (document.activeElement === first || document.activeElement === dialog.current)) { event.preventDefault(); last.focus(); }
    else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
  }

  return <div className="dialog-backdrop">
    <div className="dialog env-dialog" role="dialog" aria-modal="true" aria-label={label} aria-busy={busy || undefined} tabIndex={-1} ref={dialog} onKeyDown={onKeyDown}>{children}</div>
  </div>;
}
