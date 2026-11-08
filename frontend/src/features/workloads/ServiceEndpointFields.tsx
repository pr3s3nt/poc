import { Button } from '../../shared/ui/Button';
import { t } from '../../shared/i18n';
import type { Dependency } from './DependencyFields';

export type EndpointPort = {name: string; port: string; protocol?: string};
export const endpointPorts = (ports: EndpointPort[]) => ports.filter(p => /^[a-z0-9]([a-z0-9-]{0,13}[a-z0-9])?$/.test(p.name) && /[a-z]/.test(p.name) && ports.filter(other => other.name === p.name).length === 1 && Number.isInteger(Number(p.port)) && Number(p.port) > 0 && Number(p.port) <= 65535 && (!p.protocol || p.protocol === 'TCP'));

export function ServiceEndpointFields({ dependencies, ports, available, serviceEnabled, workloadName, disabled, onChange }: {
 dependencies: Dependency[]; ports: EndpointPort[]; available: boolean; serviceEnabled: boolean;
 workloadName: string; disabled: boolean; onChange(dependencies: Dependency[]): void;
}) {
 const endpoint = dependencies.find(d => d.type === 'endpoint');
 const choices = endpointPorts(ports);
 const label = (key: string) => t(`workloads.endpoint.${key}`);
 function add(port: string) {
  let alias = 'public'; for (let i = 2; dependencies.some(d => d.alias === alias); i++) alias = `public-${i}`;
  onChange([...dependencies, {alias, type: 'endpoint', className: '', sharedId: '', params: {name: workloadName, port, path: '/'}}]);
 }
 function set(key: string, value: string) { onChange(dependencies.map(d => d === endpoint ? {...d, params: {...d.params, [key]: value}} : d)); }
 return <div className="editor-grid" role="group" aria-label={label('title')}>
  {endpoint ? <>
   <strong>{label('title')}</strong>
   <div className="field-grid">
    <label>{label('name')}<input disabled={disabled} value={endpoint.params.name ?? ''} onChange={e => set('name', e.target.value)} /></label>
    <label>{label('port')}<select aria-label={label('port')} disabled={disabled} value={endpoint.params.port ?? ''} onChange={e => set('port', e.target.value)}><option value="">{label('choosePort')}</option>{endpoint.params.port && !choices.some(p => p.name === endpoint.params.port) ? <option value={endpoint.params.port} disabled>{endpoint.params.port}</option> : null}{choices.map(p => <option key={p.name} value={p.name}>{p.name} · {p.port}</option>)}</select></label>
    <label>{label('path')}<input disabled={disabled} value={endpoint.params.path ?? '/'} onChange={e => set('path', e.target.value)} /></label>
   </div>
   <Button disabled={disabled} tone="quiet" onClick={() => onChange(dependencies.filter(d => d !== endpoint))}>{label('remove')}</Button>
  </> : choices.map(p => <Button key={p.name} disabled={disabled || !available || !serviceEnabled} onClick={() => add(p.name)}>{t('workloads.endpoint.create', {port: p.name})}</Button>)}
 </div>;
}
