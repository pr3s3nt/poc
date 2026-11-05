import { t } from '../../shared/i18n';
import { Button } from '../../shared/ui/Button';
import type { Execution, PodVolume } from './templates';

export function LocalVolumeFields({ value, onChange, containers, disabled }: { value: Execution; onChange(value: Execution): void; containers: string[]; disabled: boolean }) {
  const text = (key: string) => t(`workloads.volumes.${key}`);
  const volumes = value.volumes ?? [];
  function update(index: number, volume: PodVolume) { onChange({ ...value, volumes: volumes.map((v, i) => i === index ? volume : v) }); }
  const containerOptions = (selected: string) => <>{!containers.includes(selected) ? <option value={selected}>{selected}</option> : null}{containers.map(name => <option key={name} value={name}>{name}</option>)}</>;
  return <>
    <div className="section-header"><h3>{text('sources')}</h3><Button disabled={disabled} type="button" onClick={() => {
      let name = 'volume';
      for (let suffix = 2; volumes.some(v => v.name === name); suffix += 1) name = `volume-${suffix}`;
      onChange({ ...value, volumes: [...volumes, { name, emptyDir: {} }] });
    }}>{text('addVolume')}</Button></div>
    {volumes.map((volume, index) => <div className="content-panel editor-grid" key={index}>
      <div className="field-grid"><label>{text('name')}<input value={volume.name} onChange={e => update(index, { ...volume, name: e.target.value })} /></label>
      <label>{text('type')}<select aria-label={text('type')} value={volume.hostPath ? 'hostPath' : volume.configMap ? 'configMap' : 'emptyDir'} onChange={e => update(index, { name: volume.name, ...(e.target.value === 'hostPath' ? { hostPath: { path: '', type: 'Directory' } } : e.target.value === 'emptyDir' ? { emptyDir: {} } : { configMap: { files: [{ name: 'app.conf', value: '' }] } }) })}><option value="emptyDir">{text('emptyDir')}</option><option value="configMap">{text('configMap')}</option><option value="hostPath">{text('hostPath')}</option></select></label></div>
      {volume.hostPath ? <>
        <p className="muted">{text('hostPathHelp')}</p>
        <div className="field-grid">
          <label>{text('hostPathPath')}<input value={volume.hostPath.path} placeholder="/var/local/app" onChange={e => update(index, { ...volume, hostPath: { ...volume.hostPath!, path: e.target.value } })} /></label>
          <label>{text('hostPathType')}<select aria-label={text('hostPathType')} value={volume.hostPath.type ?? ''} onChange={e => update(index, { ...volume, hostPath: { ...volume.hostPath!, type: e.target.value } })}>
            <option value="">{text('hostPathUnchecked')}</option>
            {['Directory', 'DirectoryOrCreate', 'File', 'FileOrCreate', 'Socket', 'CharDevice', 'BlockDevice'].map(type => <option key={type} value={type}>{text(`hostPath${type}`)}</option>)}
          </select></label>
        </div>
      </> : null}
      {volume.emptyDir ? <div className="field-grid"><label>{text('medium')}<select aria-label={text('medium')} value={volume.emptyDir.medium ?? ''} onChange={e => update(index, { ...volume, emptyDir: { ...volume.emptyDir, medium: e.target.value } })}><option value="">{text('disk')}</option><option value="Memory">{text('memory')}</option></select></label><label>{text('sizeLimit')}<input value={volume.emptyDir.sizeLimit ?? ''} placeholder="256Mi" onChange={e => update(index, { ...volume, emptyDir: { ...volume.emptyDir, sizeLimit: e.target.value } })} /></label></div> : null}
      {volume.configMap ? <>
        {(volume.configMap.files ?? []).map((file, fi) => <div className="editor-grid" key={fi}>
          <label>{text('fileName')}<input value={file.name} onChange={e => update(index, { ...volume, configMap: { ...volume.configMap!, files: volume.configMap!.files.map((f, i) => i === fi ? { ...f, name: e.target.value } : f) } })} /></label>
          <label>{text('content')}<textarea rows={5} value={file.value} onChange={e => update(index, { ...volume, configMap: { ...volume.configMap!, files: volume.configMap!.files.map((f, i) => i === fi ? { ...f, value: e.target.value } : f) } })} /></label>
          <Button disabled={disabled} type="button" tone="quiet" onClick={() => update(index, { ...volume, configMap: { ...volume.configMap!, files: volume.configMap!.files.filter((_, i) => i !== fi) } })}>{text('removeFile')}</Button>
        </div>)}
        <Button disabled={disabled} type="button" onClick={() => update(index, { ...volume, configMap: { ...volume.configMap!, files: [...volume.configMap!.files, { name: '', value: '' }] } })}>{text('addFile')}</Button>
        <label>{text('fileMode')}<select aria-label={text('fileMode')} value={volume.configMap.defaultMode ?? 292} onChange={e => update(index, { ...volume, configMap: { ...volume.configMap!, defaultMode: Number(e.target.value) } })}><option value={292}>0444</option><option value={288}>0440</option><option value={256}>0400</option></select></label>
      </> : null}
      <Button disabled={disabled} type="button" tone="quiet" onClick={() => onChange({ ...value, volumes: volumes.filter((_, i) => i !== index) })}>{text('removeVolume')}</Button>
    </div>)}
    <div className="section-header"><h3>{text('mounts')}</h3><Button disabled={disabled || !volumes.length} type="button" onClick={() => onChange({ ...value, volumeMounts: [...(value.volumeMounts ?? []), { volume: volumes[0]!.name, container: containers[0] ?? '', path: '/shared', readOnly: !!(volumes[0]?.configMap || volumes[0]?.hostPath) }] })}>{text('addMount')}</Button></div>
    {(value.volumeMounts ?? []).map((mount, index) => {
      const patch = (p: Partial<typeof mount>) => onChange({ ...value, volumeMounts: value.volumeMounts!.map((m, i) => i === index ? { ...m, ...p } : m) });
      return <div className="editor-grid" key={index}><div className="field-grid">
        <label>{text('volume')}<select aria-label={text('volume')} value={mount.volume} onChange={e => patch({ volume: e.target.value, readOnly: volumes.some(v => v.name === e.target.value && !!(v.configMap || v.hostPath)) })}>{!volumes.some(v => v.name === mount.volume) ? <option value={mount.volume}>{mount.volume}</option> : null}{volumes.map((v, i) => <option key={i} value={v.name}>{v.name}</option>)}</select></label>
        <label>{text('container')}<select aria-label={text('container')} value={mount.container} onChange={e => patch({ container: e.target.value })}>{containerOptions(mount.container)}</select></label>
        <label>{text('path')}<input value={mount.path} onChange={e => patch({ path: e.target.value })} /></label>
        <label>{text('subPath')}<input value={mount.subPath ?? ''} onChange={e => patch({ subPath: e.target.value })} /></label>
        <label className="checkbox-row"><input type="checkbox" checked={mount.readOnly} disabled={disabled || !!volumes.find(v => v.name === mount.volume)?.configMap} onChange={e => patch({ readOnly: e.target.checked })} />{text('readOnly')}</label>
      </div><Button disabled={disabled} type="button" tone="quiet" onClick={() => onChange({ ...value, volumeMounts: value.volumeMounts!.filter((_, i) => i !== index) })}>{text('removeMount')}</Button></div>;
    })}
  </>;
}
