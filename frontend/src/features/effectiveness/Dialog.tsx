import { useEffect, useId, useRef, type KeyboardEvent, type PropsWithChildren, type ReactNode } from 'react';
import { Button } from '../../shared/ui/Button';

/** Product dialog shell (shared .dialog styles): Escape, × and the backdrop close it. */
export function Dialog({ title, subtitle, onClose, busy = false, children }: PropsWithChildren<{ title: string; subtitle?: ReactNode; onClose(): void; busy?: boolean }>) {
  const heading = useId();
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => { ref.current?.focus(); }, []);
  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key === 'Escape' && !busy) { event.stopPropagation(); onClose(); }
  }
  return <div className="dialog-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget && !busy) onClose(); }}>
    <div className="dialog eff-dialog" role="dialog" aria-modal="true" aria-labelledby={heading} tabIndex={-1} ref={ref} onKeyDown={onKeyDown}>
      <div className="section-header">
        <div><h2 id={heading}>{title}</h2>{subtitle ? <p>{subtitle}</p> : null}</div>
        <Button tone="quiet" aria-label="Đóng" onClick={onClose} disabled={busy}>×</Button>
      </div>
      {children}
    </div>
  </div>;
}
