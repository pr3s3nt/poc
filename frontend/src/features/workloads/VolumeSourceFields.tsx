import { t } from '../../shared/i18n';
import { Button } from '../../shared/ui/Button';
import type { PodVolume } from './templates';

export function VolumeSourceFields({ volume, onChange, disabled, creating = false }: { volume: PodVolume; onChange(value: PodVolume): void; disabled: boolean; creating?: boolean }) {
  const text = (key: string) => t(`workloads.volumes.${key}`);
  return <fieldset disabled={disabled} className="editor-grid volume-fields">
    <label>{text('name')}<input value={volume.name} disabled={!creating} onChange={e => onChange({ ...volume, name: e.target.value })} /></label>
      {volume.hostPath ? <>
        <p className="muted">{text('hostPathHelp')}</p>
        <div className="field-grid">
          <label>{text('hostPathPath')}<input value={volume.hostPath.path} placeholder="/var/local/app" onChange={e => onChange({ ...volume, hostPath: { ...volume.hostPath!, path: e.target.value } })} /></label>
          <label>{text('hostPathType')}<select aria-label={text('hostPathType')} value={volume.hostPath.type ?? ''} onChange={e => onChange({ ...volume, hostPath: { ...volume.hostPath!, type: e.target.value } })}>
            <option value="">{text('hostPathUnchecked')}</option>
            {['Directory', 'DirectoryOrCreate', 'File', 'FileOrCreate', 'Socket', 'CharDevice', 'BlockDevice'].map(type => <option key={type} value={type}>{text(`hostPath${type}`)}</option>)}
          </select></label>
        </div>
      </> : null}
      {volume.emptyDir ? <div className="field-grid"><label>{text('medium')}<select aria-label={text('medium')} value={volume.emptyDir.medium ?? ''} onChange={e => onChange({ ...volume, emptyDir: { ...volume.emptyDir, medium: e.target.value } })}><option value="">{text('disk')}</option><option value="Memory">{text('memory')}</option></select></label><label>{text('sizeLimit')}<input value={volume.emptyDir.sizeLimit ?? ''} placeholder="256Mi" onChange={e => onChange({ ...volume, emptyDir: { ...volume.emptyDir, sizeLimit: e.target.value } })} /></label></div> : null}
      {volume.configMap ? <>
        {(volume.configMap.files ?? []).map((file, fi) => <div className="editor-grid volume-fields" key={fi}>
          <label>{text('fileName')}<input value={file.name} onChange={e => onChange({ ...volume, configMap: { ...volume.configMap!, files: volume.configMap!.files.map((f, i) => i === fi ? { ...f, name: e.target.value } : f) } })} /></label>
          <label>{text('content')}<textarea rows={5} value={file.value} onChange={e => onChange({ ...volume, configMap: { ...volume.configMap!, files: volume.configMap!.files.map((f, i) => i === fi ? { ...f, value: e.target.value } : f) } })} /></label>
          <Button disabled={disabled} type="button" tone="quiet" onClick={() => onChange({ ...volume, configMap: { ...volume.configMap!, files: volume.configMap!.files.filter((_, i) => i !== fi) } })}>{text('removeFile')}</Button>
        </div>)}
        <Button disabled={disabled} type="button" onClick={() => onChange({ ...volume, configMap: { ...volume.configMap!, files: [...volume.configMap!.files, { name: '', value: '' }] } })}>{text('addFile')}</Button>
        <label>{text('fileMode')}<select aria-label={text('fileMode')} value={volume.configMap.defaultMode ?? 292} onChange={e => onChange({ ...volume, configMap: { ...volume.configMap!, defaultMode: Number(e.target.value) } })}><option value={292}>0444</option><option value={288}>0440</option><option value={256}>0400</option></select></label>
      </> : null}
  </fieldset>;
}
