import { useEffect, useRef, useState } from 'react';
import { api } from '../../shared/api/client';
import { t } from '../../shared/i18n';
import { DocumentProblem } from './catalogDocument';
import { applyFormChanges, editDocument, parseEditDocument, sameDocument, type DefinitionDocument } from './definitionEditDraft';

export type PreparedEdit = { definition: DefinitionDocument; source?: { url: string; path: string; revision: string } };
type Context = { key: string; resourceType: string; version: number };
export function useDefinitionCodeEditor({ form, context, hydrate, save, describeFailure }: {
  form: DefinitionDocument | null;
  context: Context | null;
  hydrate: (doc: DefinitionDocument, version: number) => void;
  save: (doc: DefinitionDocument) => Promise<boolean>;
  describeFailure: (reason: unknown) => string;
}) {
  const [mode, setMode] = useState<'form' | 'code'>('form');
  const [text, setText] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [prepared, setPrepared] = useState<PreparedEdit>();
  const [capture, setCapture] = useState(false);
  const [pending, setPending] = useState<{ prepared: PreparedEdit; epoch: number; version: number }>();
  const original = useRef<DefinitionDocument | undefined>(undefined);
  const baseline = useRef<DefinitionDocument | undefined>(undefined);
  const epoch = useRef(0);
  const inFlight = useRef(false);
  const hydration = useRef<typeof pending>(undefined);
  const latest = useRef({ context, hydrate, save, describeFailure });
  latest.current = { context, hydrate, save, describeFailure };
  useEffect(() => { const tracker = epoch; return () => { tracker.current++; }; }, []);

  function open(doc: object) {
    epoch.current++; inFlight.current = false; setBusy(false); setPending(undefined);
    original.current = editDocument(doc); baseline.current = undefined;
    setMode('form'); setText(JSON.stringify(original.current, null, 2));
    setError(''); setPrepared(undefined); setCapture(true);
  }
  function reset() {
    epoch.current++; inFlight.current = false; setBusy(false); setPending(undefined);
    original.current = undefined; baseline.current = undefined; setCapture(false);
    setMode('form'); setText(''); setError(''); setPrepared(undefined);
  }
  const document = form && original.current && baseline.current
    ? applyFormChanges(original.current, baseline.current, editDocument(form)) as DefinitionDocument
    : form ? editDocument(form) : null;

  useEffect(() => {
    if (!capture) return;
    baseline.current = form ? editDocument(form) : undefined;
    setCapture(false);
  }, [capture, form]);

  function fail(reason: unknown) {
    setError(reason instanceof DocumentProblem ? t(`catalogDocument.${reason.code}`)
      : reason instanceof Error && reason.message === 'identity' ? t('definitionCode.identity')
      : latest.current.describeFailure(reason));
  }
  async function prepare(): Promise<PreparedEdit> {
    const selected = latest.current.context;
    if (!selected) throw new Error('identity');
    const input = parseEditDocument(text, selected.key, selected.resourceType);
    if (prepared) return prepared;
    const result = await api<PreparedEdit>('/resource-definitions/import/prepare', { method: 'POST', body: JSON.stringify(input) });
    if (!result.definition || result.definition.key !== selected.key || result.definition.resourceType !== selected.resourceType) throw new Error('identity');
    return result;
  }
  async function run(action: 'check' | 'form' | 'save') {
    if (inFlight.current || !latest.current.context) return;
    const current = epoch.current;
    const version = latest.current.context.version;
    inFlight.current = true; setBusy(true); setError('');
    let hydrating = false;
    try {
      const result = await prepare();
      if (current !== epoch.current) return;
      setPrepared(result);
      if (action === 'form') {
        hydrating = true;
        latest.current.hydrate(result.definition, version);
        setPending({ prepared: result, epoch: current, version });
      } else if (action === 'save') {
        await latest.current.save(result.definition);
      }
    } catch (reason) { if (current === epoch.current) fail(reason); }
    finally { if (current === epoch.current && !hydrating) { inFlight.current = false; setBusy(false); } }
  }

  // Hydrate first, then validate the actual form serializer. A code-only field
  // must never disappear merely because a user clicks the other tab.
  useEffect(() => {
    if (!pending || hydration.current === pending) return;
    hydration.current = pending;
    const candidate = form ? editDocument(form) : null;
    void (async () => {
      try {
        if (!candidate) throw new Error('projection');
        const projected = await api<PreparedEdit>('/resource-definitions/import/prepare', { method: 'POST', body: JSON.stringify(candidate) });
        if (epoch.current !== pending.epoch) return;
        if (!sameDocument(editDocument(projected.definition), editDocument(pending.prepared.definition))) throw new Error('projection');
        original.current = editDocument(pending.prepared.definition); baseline.current = candidate;
        setMode('form'); setError('');
      } catch (reason) {
        if (epoch.current === pending.epoch) {
          if (reason instanceof Error && reason.message === 'projection') setError(t('definitionCode.projection'));
          else fail(reason);
        }
      }
      finally { if (epoch.current === pending.epoch) { setPending(undefined); inFlight.current = false; setBusy(false); } }
    })();
  }, [pending, form]);

  function show(next: 'form' | 'code') {
    if (inFlight.current || next === mode) return;
    if (next === 'form') { void run('form'); return; }
    // An initially unrepresentable stored Definition can still be repaired in
    // code. An invalid edited form cannot silently revert to its old contents.
    const candidate = document ?? (baseline.current === undefined ? original.current : undefined);
    if (!candidate) { setError(t('definitionCode.formInvalid')); return; }
    setText(JSON.stringify(candidate, null, 2)); setPrepared(undefined); setError(''); setMode('code');
  }
  function change(value: string) { setText(value); setPrepared(undefined); setError(''); }
  function saved(doc: object) {
    original.current = editDocument(doc); setText(JSON.stringify(original.current, null, 2));
    setPrepared(undefined); setCapture(true);
  }
  return { mode, text, busy, error, prepared, document, open, reset, show, change, run, saved };
}
