import { useId, useState, type ReactNode } from 'react';
import { t } from '../../shared/i18n';
import { CatalogDocumentImport } from './CatalogDocumentImport';
import type { DocumentKind } from './catalogDocument';

// Keep both editors mounted so switching methods preserves independent drafts.
export function CatalogEntryTabs({ children, kind, onRegistered, disabled = false, importAvailable = true }: {
  children: ReactNode; kind: DocumentKind; onRegistered(): Promise<unknown>; disabled?: boolean; importAvailable?: boolean;
}) {
  const id = useId();
  const [selected, setSelected] = useState('form');
  const [importBusy, setImportBusy] = useState(false);
  const active = importAvailable ? selected : 'form';
  const locked = disabled || importBusy;
  const tabs = importAvailable ? ['form', 'import'] : ['form'];
  return <>
    <div className="tabs" role="tablist" aria-label={t('catalogDocument.method')}>
      {tabs.map((tab, index) => <button type="button" role="tab" key={tab} id={`${id}-${tab}-tab`} aria-controls={`${id}-${tab}-panel`} aria-selected={active === tab} tabIndex={active === tab ? 0 : -1} disabled={locked} className={`tab ${active === tab ? 'tab-active' : ''}`} onClick={() => setSelected(tab)} onKeyDown={event => {
        if (locked || !['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
        event.preventDefault();
        const next = event.key === 'Home' ? tabs[0]! : event.key === 'End' ? tabs[tabs.length - 1]! : tabs[(index + 1) % tabs.length]!;
        setSelected(next); document.getElementById(`${id}-${next}-tab`)?.focus();
      }}>{t(tab === 'form' ? 'catalogDocument.form' : 'catalogDocument.title')}</button>)}
    </div>
    <div role="tabpanel" id={`${id}-form-panel`} aria-labelledby={`${id}-form-tab`} hidden={active !== 'form'}>{children}</div>
    <div role="tabpanel" id={`${id}-import-panel`} aria-labelledby={`${id}-import-tab`} hidden={active !== 'import' || !importAvailable}>
      <CatalogDocumentImport kind={kind} onRegistered={onRegistered} onBusyChange={setImportBusy} />
    </div>
  </>;
}
