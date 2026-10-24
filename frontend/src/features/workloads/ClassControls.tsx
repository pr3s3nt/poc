import { Button } from '../../shared/ui/Button';
import type { ResourceType } from './api';
import { t } from '../../shared/i18n';

// Display names of the classes the catalog serves (UC-16 screens, "Class và metadata
// dependency"). Any other class is a custom/imported one and is shown as written.
const classLabels: Record<string, string> = { 'in-cluster': 'Trong cluster (in-cluster)', 'cloud-managed': t('workloads.classControls.aWSManagedServiceCloudManaged') };
export const classLabel = (name: string) => (Object.hasOwn(classLabels, name) ? classLabels[name] ?? name : t('workloads.classControls.customName', { name }));

type Props = {
  index: number;
  alias: string;
  type: ResourceType | undefined;
  className: string;
  sharedId: string;
  metadata: 'ready' | 'loading' | 'error';
  disabled?: boolean;
  onClass(value: string): void;
  onSharedId(value: string): void;
  onReload(): void;
};

// "Cách cung cấp / Class" and the shared ID of one dependency. The choices are the
// classes GET /resource-types publishes for the Type; the Developer never picks a
// Terraform module, provider or credential. `default` (empty) keeps its Score
// meaning: it is never mapped to a class, and an imported/custom class is kept.
export function ClassControls({ index, alias, type, className, sharedId, metadata, disabled = false, onClass, onSharedId, onReload }: Props) {
  const name = alias || type?.key || t('workloads.classControls.resourceValue', { value: index + 1 });
  const classes = type?.classes ?? [];
  const known = className === '' || classes.includes(className);
  const locked = disabled || metadata !== 'ready';
  return <div className="class-controls" data-dependency={name}>
    <div className="field-grid">
      <label>{t('workloads.classControls.provisioningClass')}
        <select aria-label={t('workloads.classControls.provisioningClassOfName', { name })} value={className} disabled={locked || !type} onChange={(event) => onClass(event.target.value)}>
          <option value="">{t('workloads.classControls.defaultDefault')}</option>
          {classes.map((item) => <option key={item} value={item}>{classLabel(item)}</option>)}
          {!known ? <option value={className}>{t('workloads.classControls.classNameEnteredKept', { className })}</option> : null}
        </select></label>
      <label>{t('workloads.classControls.sharedIDOptional')}
        <input aria-label={t('workloads.classControls.sharedIDOfName', { name })} value={sharedId} disabled={disabled} spellCheck={false} placeholder={t('workloads.classControls.leaveEmptyIfPrivateTo')} onChange={(event) => onSharedId(event.target.value)} /></label>
    </div>
    {index === 0 && metadata === 'loading' ? <p role="status" className="feature-note">{t('workloads.classControls.loadingClassMetadata')}</p> : null}
    {index === 0 && metadata === 'error' ? <div className="form-error" role="alert" aria-label={t('workloads.classControls.classMetadataLoadError')}>{t('workloads.classControls.couldNotLoadClassMetadata')}{' '} <Button type="button" onClick={onReload}>{t('workloads.classControls.retryLoadingClassMetadata')}</Button></div> : null}
    {type && classes.length === 0 && metadata === 'ready' ? <p className="feature-note">{t('workloads.classControls.theCatalogHasNotPublished', { key: type.key })}</p> : null}
    {!known ? <p className="feature-note" role="note">{t('workloads.classControls.classClassNameIsNotIn', { className, value: type?.key ?? t('workloads.classControls.this') })}</p> : null}
    <p className="feature-note">{t('workloads.classControls.onlyChooseHowTheResource')}</p>
  </div>;
}
