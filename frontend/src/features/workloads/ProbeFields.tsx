import { t } from '../../shared/i18n';
export type Probe = { type?: 'http' | 'tcp' | 'exec'; path?: string; port?: number; scheme?: string; command?: string[]; initialDelaySeconds?: number; timeoutSeconds?: number; periodSeconds?: number; successThreshold?: number; failureThreshold?: number };
export function ProbeFields({ label, value, onChange, disabled }: { label: string; value?: Probe; onChange(value?: Probe): void; disabled?: boolean }) {
  const text = (key: string) => t(`workloads.probes.${key}`);
  const type = value?.type ?? 'http';
  return <fieldset className="editor-grid form-fieldset" disabled={disabled}>
    <legend>{label}</legend>
    <label className="checkbox-row"><input aria-label={label} type="checkbox" checked={!!value} onChange={e => onChange(e.target.checked ? { path: '', port: 8080 } : undefined)} />{text('enabled')}</label>
    {value ? <>
      <label>{text('type')}<select aria-label={text('type')} value={type} onChange={e => { const next = e.target.value as Probe['type']; const { path: _path, port: _port, command: _command, scheme: _scheme, ...timing } = value; void _path; void _port; void _command; void _scheme; onChange({ ...timing, ...(next === 'exec' ? { type: next, command: [] } : next === 'tcp' ? { type: next, port: 8080 } : { type: next, path: '', port: 8080 }) }); }}><option value="http">HTTP</option><option value="tcp">TCP</option><option value="exec">Exec</option></select></label>
      {type === 'exec' ? <label>{text('command')}<textarea value={(value.command ?? []).join('\n')} onChange={e => onChange({ ...value, command: e.target.value === '' ? [] : e.target.value.split('\n') })} /></label> : <label>{text('port')}<input type="number" min="1" max="65535" value={value.port ?? ''} onChange={e => onChange({ ...value, port: Number(e.target.value) })} /></label>}
      {type === 'http' ? <><label>{text('path')}<input value={value.path ?? ''} placeholder="/health" onChange={e => onChange({ ...value, path: e.target.value })} /></label><label>{text('scheme')}<select aria-label={text('scheme')} value={value.scheme ?? ''} onChange={e => { const next = { ...value }; if (e.target.value) next.scheme = e.target.value; else delete next.scheme; onChange(next); }}><option value="">HTTP</option><option value="HTTPS">HTTPS</option></select></label></> : null}
      <details><summary>{text('timing')}</summary><div className="field-grid">
        {(['initialDelaySeconds', 'timeoutSeconds', 'periodSeconds', 'successThreshold', 'failureThreshold'] as const).map(field => <label key={field}>{text(field)}<input type="number" min={field === 'initialDelaySeconds' ? 0 : 1} value={value[field] ?? ''} onChange={e => { const next = { ...value }; if (e.target.value === '') delete next[field]; else next[field] = Number(e.target.value); onChange(next); }} /></label>)}
      </div></details>
    </> : null}
  </fieldset>;
}
