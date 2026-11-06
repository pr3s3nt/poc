import { useEffect, useState } from 'react';
import { api, describeError } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { t } from '../../shared/i18n';
import type { EnvironmentKey } from '../../shared/types/application';
type Choice = { id: string; name: string; verifiedAt?: string };
export function MonitoringBinding({ applicationId, environment, busy }: { applicationId: string; environment: EnvironmentKey; busy: boolean }) {
  const [choices, setChoices] = useState<Choice[]>([]), [selected, setSelected] = useState(''), [revision, setRevision] = useState(0), [pending, setPending] = useState(true), [error, setError] = useState(''), [notice, setNotice] = useState('');
  const base = `/applications/${encodeURIComponent(applicationId)}/environments/${environment}/monitoring/binding`;
  useEffect(() => { let active = true; setPending(true); setNotice(''); setError('');
    Promise.all([api<{ connections: Choice[] }>('/monitoring/connections'), api<{ binding: { connectionId: string; revision: number } | null }>(base)]).then(([c, b]) => { if (active) { setChoices(c.connections.filter(x => x.verifiedAt)); setSelected(b.binding?.connectionId ?? ''); setRevision(b.binding?.revision ?? 0); } }).catch(e => { if (active) setError(describeError(e, t('monitoring.failed'))); }).finally(() => { if (active) setPending(false); }); return () => { active = false; };
  }, [base]);
  async function save() { setPending(true); setError(''); try { const b = await api<{ revision: number }>(base, { method: 'PUT', body: JSON.stringify({ connectionId: selected, expectedRevision: revision }) }); setRevision(b.revision); setNotice(t('monitoring.bindingSaved')); } catch (e) { setError(describeError(e, t('monitoring.failed'))); } finally { setPending(false); } }
  return <section className="content-panel"><h2>{t('monitoring.binding')}</h2><label>{t('monitoring.connections')} <select disabled={busy || pending} value={selected} onChange={e => setSelected(e.target.value)}><option value="">{t('monitoring.select')}</option>{choices.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}</select></label> <Button disabled={busy || pending || !selected} onClick={() => void save()}>{t('monitoring.bind')}</Button>{error ? <p className="form-error" role="alert">{error}</p> : null}{notice ? <p role="status">{notice}</p> : null}</section>;
}
