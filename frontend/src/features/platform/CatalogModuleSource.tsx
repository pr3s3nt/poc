import { useEffect, useRef, useState } from 'react';
import { api } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { t } from '../../shared/i18n';
import { parseCapabilities, type Capability } from './capabilities';

export type CatalogModuleSelection = { sourceId: string; tag: string; componentId: string };
type Module = { componentId: string; capability: Capability };
type Props = { resourceType: string; disabled: boolean; initialSelection?: CatalogModuleSelection; onResult(capability: Capability | null, selection?: CatalogModuleSelection): void };
const segment = encodeURIComponent;

export function CatalogModuleSource({ resourceType, disabled, initialSelection, onResult }: Props) {
  const [sources, setSources] = useState<{ id: string }[]>([]);
  const [releases, setReleases] = useState<{ tag: string }[]>([]);
  const [modules, setModules] = useState<Module[]>([]);
  const [source, setSource] = useState(initialSelection?.sourceId ?? '');
  const [tag, setTag] = useState(initialSelection?.tag ?? '');
  const [component, setComponent] = useState(initialSelection?.componentId ?? '');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(false);
  const [retry, setRetry] = useState(0);
  const callback = useRef(onResult);
  callback.current = onResult;

  useEffect(() => {
    let current = true;
    setLoading(true); setError(false); callback.current(null);
    async function load() {
      try {
        const sourceList = (await api<{ sources: { id: string }[] }>('/platform/catalog/sources')).sources;
        if (!current) return;
        setSources(sourceList);
        if (!source || !sourceList.some(s => s.id === source)) { setReleases([]); setModules([]); return; }
        const base = `/platform/catalog/sources/${segment(source)}/releases`;
        const versions = (await api<{ releases: { tag: string }[] }>(base)).releases;
        if (!current) return;
        setReleases(versions);
        if (!tag || !versions.some(r => r.tag === tag)) { setModules([]); return; }
        const body = await api<{ modules: { componentId: string; capability: unknown }[] }>(`${base}/${segment(tag)}/modules?resourceType=${segment(resourceType)}`);
        const choices = body.modules.map(m => {
          const capability = parseCapabilities({ capabilities: [m.capability] })[0];
          if (!capability || capability.resourceType !== resourceType || capability.driverType !== 'terraform' || !capability.source) throw new Error('invalid metadata');
          return { componentId: m.componentId, capability };
        });
        if (!current) return;
        setModules(choices);
        const selected = choices.find(m => m.componentId === component);
        if (selected) callback.current(selected.capability, { sourceId: source, tag, componentId: component });
      } catch { if (current) { setModules([]); setError(true); } }
      finally { if (current) setLoading(false); }
    }
    void load();
    return () => { current = false; };
  }, [resourceType, source, tag, component, retry]);

  function changeSource(value: string) {
    callback.current(null); setSource(value); setTag(''); setComponent(''); setReleases([]); setModules([]);
  }
  function changeRelease(value: string) {
    callback.current(null); setTag(value); setComponent(''); setModules([]);
  }
  function changeModule(value: string) { callback.current(null); setComponent(value); }
  return <section className="catalog-fields" aria-label={t('platform.catalogModule.title')} aria-busy={loading}>
    <div className="field-grid">
      <label>{t('platform.catalogModule.source')}<select value={source} disabled={disabled || loading} onChange={e => changeSource(e.target.value)}>
        <option value="">{t('platform.catalogModule.chooseSource')}</option>
        {sources.map(s => <option key={s.id} value={s.id}>{s.id}</option>)}
      </select></label>
      <label>{t('platform.catalogModule.release')}<select value={tag} disabled={disabled || loading || !source || releases.length === 0} onChange={e => changeRelease(e.target.value)}>
        <option value="">{t('platform.catalogModule.chooseRelease')}</option>
        {releases.map(r => <option key={r.tag} value={r.tag}>{r.tag}</option>)}
      </select></label>
      <label>{t('platform.catalogModule.module')}<select value={component} disabled={disabled || loading || !tag || modules.length === 0} onChange={e => changeModule(e.target.value)}>
        <option value="">{t('platform.catalogModule.chooseModule')}</option>
        {modules.map(m => <option key={m.componentId} value={m.componentId}>{m.componentId}</option>)}
      </select></label>
    </div>
    {loading ? <p role="status">{t('platform.catalogModule.loading')}</p> : error ? <div className="form-error" role="alert">{t('platform.catalogModule.error')} <Button type="button" disabled={disabled} onClick={() => setRetry(n => n + 1)}>{t('platform.catalogModule.retry')}</Button></div> : sources.length === 0 ? <p role="status">{t('platform.catalogModule.noSources')}</p> : source && releases.length === 0 ? <p role="status">{t('platform.catalogModule.noReleases')}</p> : tag && modules.length === 0 ? <p role="status">{t('platform.catalogModule.noModules')}</p> : null}
  </section>;
}
