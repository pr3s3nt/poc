import type { Probe } from './ProbeFields';
import type { Execution } from './templates';

type Container = { name: string; image: string; role?: string; initOrder?: number; ports?: {name: string; containerPort: number; protocol?: string}[]; readinessProbe?: Probe; livenessProbe?: Probe; startupProbe?: Probe };
const overlap = (a: string, b: string) => a === b || a.startsWith(`${b}/`) || b.startsWith(`${a}/`);
const safePath = (path: string) => path.startsWith('/') && path.length <= 1024 && !/[\0\r\n\\]/.test(path) && path.slice(1).split('/').every(part => part !== '' && part !== '.' && part !== '..') && !['/vault', '/var/run/secrets'].some(prefix => overlap(path, prefix));
const safeSubPath = (path: string) => path === '' || path.length <= 1024 && !/[\0\r\n\\]/.test(path) && path.split('/').every(part => part !== '' && part !== '.' && part !== '..');
export function validRuntimeIntent(containers: Container[], e?: Execution, secrets?: string[]): boolean {
  const names = new Set(containers.map(c => c.name));
  if (names.size !== containers.length || containers.some(c => !c.name || !c.image)) return false;
  if (!containers.some(c => !c.role || c.role === 'app')) return false;
  const orders = new Set<number>();
  for (const c of containers) {
    const ports = c.ports ?? [];
    if (new Set(ports.map(p => p.name)).size !== ports.length || ports.some(p => !p.name || !Number.isInteger(p.containerPort) || p.containerPort < 1 || p.containerPort > 65535 || p.protocol && !['TCP','UDP','SCTP'].includes(p.protocol))) return false;
    for (const [probe, single] of [[c.readinessProbe, false], [c.livenessProbe, true], [c.startupProbe, true]] as const) {
      if (!probe) continue;
      const type = probe.type ?? 'http';
      if (type === 'http' && (!probe.path?.startsWith('/') || !Number.isInteger(probe.port) || probe.port! < 1 || probe.port! > 65535 || probe.command?.length || probe.scheme && !['HTTP','HTTPS'].includes(probe.scheme))) return false;
      if (type === 'tcp' && (!Number.isInteger(probe.port) || probe.port! < 1 || probe.port! > 65535 || probe.path || probe.command?.length || probe.scheme)) return false;
      if (type === 'exec' && (!probe.command?.length || probe.command.some(arg => !arg || arg.includes('\0')) || probe.port || probe.path || probe.scheme)) return false;
      if (!['http','tcp','exec'].includes(type)) return false;
      for (const field of ['initialDelaySeconds','timeoutSeconds','periodSeconds','successThreshold','failureThreshold'] as const) {
        const n = probe[field]; if (n !== undefined && (!Number.isInteger(n) || n < (field === 'initialDelaySeconds' ? 0 : 1))) return false;
      }
      if (single && probe.successThreshold !== undefined && probe.successThreshold !== 1) return false;
    }
    if (c.role === 'init' || c.role === 'sidecar') {
      if (!Number.isInteger(c.initOrder) || c.initOrder! < 1 || c.initOrder! > 999 || orders.has(c.initOrder!)) return false;
      orders.add(c.initOrder!);
      if (c.role === 'init' && (c.readinessProbe || c.livenessProbe || c.startupProbe)) return false;
    } else if (c.role && c.role !== 'app' || c.initOrder !== undefined && c.initOrder !== 0) return false;
  }
  if (!e) return true;
  const volumes = e.volumes ?? [];
  if (volumes.length > 32 || new Set(volumes.map(v => v.name)).size !== volumes.length) return false;
  for (const v of volumes) {
    if (!/^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/.test(v.name) || [v.emptyDir, v.configMap, v.hostPath].filter(Boolean).length !== 1) return false;
    if (v.hostPath) {
      const { path, type } = v.hostPath;
      if (!path.startsWith('/') || new TextEncoder().encode(path).length > 1024 || /[\0\r\n\\]/.test(path) || path !== '/' && path.slice(1).split('/').some(p => !p || p === '.' || p === '..') || !['', 'Directory', 'DirectoryOrCreate', 'File', 'FileOrCreate', 'Socket', 'CharDevice', 'BlockDevice'].includes(type ?? '')) return false;
    }
    if (v.emptyDir) {
      if (v.emptyDir.medium && v.emptyDir.medium !== 'Memory') return false;
      const size = v.emptyDir.sizeLimit;
      if (size && (!/^[0-9]+(?:\.[0-9]+)?(?:Ki|Mi|Gi|Ti|Pi|Ei|[kMGTPE])?$/.test(size) || !/[1-9]/.test(size))) return false;
    }
    if (v.configMap) {
      const files = v.configMap.files;
      if (!files.length || files.length > 128 || new Set(files.map(f => f.name)).size !== files.length || files.some(f => !/^[A-Za-z0-9][A-Za-z0-9._-]{0,252}$/.test(f.name) || f.value.includes('\0'))) return false;
      if (v.configMap.defaultMode !== undefined && ![256, 288, 292].includes(v.configMap.defaultMode)) return false;
    }
  }
  const mounts = [...(e.mounts ?? []), ...(e.volumeMounts ?? []), ...(e.secretFiles ?? [])];
  const paths = new Map<string, string[]>();
  for (const mount of mounts) {
    if (!names.has(mount.container) || !safePath(mount.path) || 'subPath' in mount && !safeSubPath(mount.subPath ?? '')) return false;
    const previous = paths.get(mount.container) ?? [];
    if (previous.some(path => overlap(path, mount.path))) return false;
    paths.set(mount.container, [...previous, mount.path]);
  }
  for (const mount of e.volumeMounts ?? []) {
    const volume = volumes.find(v => v.name === mount.volume);
    if (!volume || volume.configMap && (!mount.readOnly || mount.subPath && !volume.configMap.files.some(f => f.name === mount.subPath))) return false;
  }
  for (const file of e.secretFiles ?? []) {
    const match = /^\$\{resources\.env\.([A-Za-z_][A-Za-z0-9_-]*)\}$/.exec(file.ref);
    if (!match || ![256, 288].includes(file.mode ?? 256) || secrets && !secrets.includes(match[1]!)) return false;
  }
  return true;
}
