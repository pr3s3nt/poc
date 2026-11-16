import { t } from '../../shared/i18n';

// Platform routing policy of the built-in endpoint driver (EP-02). Developers
// choose only name/port/path; these literal values are never overridden.
export type EndpointDriverDraft = { gateway?: string; rootDomain: string; scheme: 'http' | 'https'; gatewayName: string; gatewayNamespace: string; gatewaySectionName: string; reservedNames: string };
export const emptyEndpointDriver = (): EndpointDriverDraft => ({ rootDomain: '', scheme: 'https', gatewayName: '', gatewayNamespace: '', gatewaySectionName: '', reservedNames: '' });
export const emptyEndpointReference = (): EndpointDriverDraft => ({ ...emptyEndpointDriver(), gateway: "${resources['k8s-gateway.default#environments.@app.@env'].outputs.gateway}" });

const label = /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/;
const domain = (value: string) => value.length <= 253 && value.includes('.') && value.split('.').every((part) => label.test(part));
const names = (value: string) => value.split(/[\s,]+/).map((name) => name.trim()).filter((name) => name !== '');

// Mirrors the server policy rules for early guidance; the server checks again.
export function endpointToVariables(draft: EndpointDriverDraft): { variables?: Record<string, unknown>; problem?: string } {
  if (draft.gateway !== undefined) {
    const gateway = draft.gateway.trim();
    if (!/^\$\{resources\['k8s-gateway\.[^'\]\s]+#[^'\]\s]+'\]\.outputs\.gateway\}$/.test(gateway)) return { problem: t('gatewayDriver.invalidReference') };
    return { variables: { gateway } };
  }
  const rootDomain = draft.rootDomain.trim();
  if (!domain(rootDomain)) return { problem: t('endpointDriver.invalidDomain') };
  const gatewayName = draft.gatewayName.trim(), gatewayNamespace = draft.gatewayNamespace.trim(), gatewaySectionName = draft.gatewaySectionName.trim();
  if (!label.test(gatewayName) || !label.test(gatewayNamespace) || (gatewaySectionName !== '' && !label.test(gatewaySectionName))) return { problem: t('endpointDriver.invalidGateway') };
  const reserved = names(draft.reservedNames);
  if (reserved.some((name) => !label.test(name))) return { problem: t('endpointDriver.invalidReserved') };
  return { variables: { rootDomain, scheme: draft.scheme, gatewayName, gatewayNamespace, ...(gatewaySectionName ? { gatewaySectionName } : {}), ...(reserved.length ? { reservedNames: reserved } : {}) } };
}

export function endpointFromVariables(variables: Record<string, unknown>): EndpointDriverDraft {
  const text = (value: unknown) => (typeof value === 'string' ? value : '');
  if (typeof variables.gateway === 'string') return { ...emptyEndpointDriver(), gateway: variables.gateway };
  return {
    rootDomain: text(variables.rootDomain), scheme: variables.scheme === 'http' ? 'http' : 'https',
    gatewayName: text(variables.gatewayName), gatewayNamespace: text(variables.gatewayNamespace), gatewaySectionName: text(variables.gatewaySectionName),
    reservedNames: Array.isArray(variables.reservedNames) ? variables.reservedNames.filter((name): name is string => typeof name === 'string').join(', ') : '',
  };
}

export function EndpointDriverFields({ value, onChange, disabled, title = t('endpointDriver.title') }: { value: EndpointDriverDraft; onChange: (value: EndpointDriverDraft) => void; disabled: boolean; title?: string }) {
  const patch = (change: Partial<EndpointDriverDraft>) => onChange({ ...value, ...change });
  if (value.gateway !== undefined) return <section className="definition-card" aria-label={title}>
    <h3>{title}</h3><label>{t('gatewayDriver.reference')}<input value={value.gateway} disabled={disabled} aria-required="true" onChange={(event) => patch({ gateway: event.target.value })} /></label>
  </section>;
  const sample = value.rootDomain.trim() ? `${value.scheme}://<name>.${value.rootDomain.trim()}/` : '';
  return <section className="definition-card" aria-label={title}>
    <div className="definition-card-title"><span className="definition-step">1</span><h3>{title}</h3></div>
    <div className="field-grid">
      <label>{t('endpointDriver.rootDomain')}<input value={value.rootDomain} disabled={disabled} aria-required="true" placeholder="apps.company.example" onChange={(event) => patch({ rootDomain: event.target.value })} /></label>
      <label>{t('endpointDriver.scheme')}<select value={value.scheme} disabled={disabled} onChange={(event) => patch({ scheme: event.target.value === 'http' ? 'http' : 'https' })}><option value="https">https</option><option value="http">http</option></select></label>
      <label>{t('endpointDriver.gatewayName')}<input value={value.gatewayName} disabled={disabled} aria-required="true" onChange={(event) => patch({ gatewayName: event.target.value })} /></label>
      <label>{t('endpointDriver.gatewayNamespace')}<input value={value.gatewayNamespace} disabled={disabled} aria-required="true" onChange={(event) => patch({ gatewayNamespace: event.target.value })} /></label>
      <label>{t('endpointDriver.listener')}<input value={value.gatewaySectionName} disabled={disabled} onChange={(event) => patch({ gatewaySectionName: event.target.value })} /></label>
      <label>{t('endpointDriver.reservedNames')}<input value={value.reservedNames} disabled={disabled} placeholder="www, api" onChange={(event) => patch({ reservedNames: event.target.value })} /></label>
    </div>
    {sample ? <p className="feature-note">{t('endpointDriver.address')} <code>{sample}</code></p> : null}
    <details className="definition-help"><summary>{t('endpointDriver.helpSummary')}</summary><p>{t('endpointDriver.help')}</p></details>
  </section>;
}
