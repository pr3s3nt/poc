import type { ResourceType, Workload } from './api';
import type { Dependency } from './DependencyFields';
import { t } from '../../shared/i18n';

export type SharedResource = { id: string; label: string; dependency?: Dependency };
// getRandomValues also works on the Console's non-HTTPS internal origins.
export const newSharedId = (): string => `resource-${Array.from(crypto.getRandomValues(new Uint8Array(16)), (byte) => byte.toString(16).padStart(2, '0')).join('')}`;

// Compare authored content independent of dictionary order and omitted defaults.
function canonical(value: unknown): string {
  if (Array.isArray(value)) return JSON.stringify(value.map(canonical));
  if (value && typeof value === 'object') return JSON.stringify(Object.entries(value).filter(([, item]) => item !== undefined).sort(([a], [b]) => a.localeCompare(b)).map(([key, item]) => [key, canonical(item)]));
  return JSON.stringify(value) ?? '';
}

// Only safe authored Score is used here, never resource outputs or credentials.
// Workload-local references cannot be transplanted into another workload.
export function sharedResources(workloads: Workload[], types: ResourceType[]): SharedResource[] {
  const found = new Map<string, SharedResource & { signature: string }>();
  for (const workload of workloads) {
    if (workload.state === 'PENDING_DELETE') continue;
    const resources = workload.score?.resources;
    if (!resources || typeof resources !== 'object' || Array.isArray(resources)) continue;
    for (const [alias, raw] of Object.entries(resources)) {
      if (!raw || typeof raw !== 'object' || Array.isArray(raw)) continue;
      const spec = raw as Record<string, unknown>;
      if (typeof spec.id !== 'string' || !spec.id || spec.type === 'environment' || spec.type === 'service') continue;
      const type = types.find((item) => item.key === spec.type);
      const params = spec.params ?? {};
      const validParams = !!params && typeof params === 'object' && !Array.isArray(params) && Object.entries(params).every(([key, value]) => {
        const input = type?.inputs.find((item) => item.name === key);
        return value !== '' && input && typeof value === (input.type === 'number' ? 'number' : input.type === 'bool' ? 'boolean' : 'string') &&
          !(typeof value === 'string' && /(?<!\$)(?:\$\$)*\$\{/.test(value));
      });
      const safe = type && validParams && (spec.class === undefined || typeof spec.class === 'string') &&
        Object.keys(spec).every((key) => ['id', 'type', 'class', 'params'].includes(key));
      const signature = canonical({ type: spec.type, class: spec.class || 'default', params });
      const previous = found.get(spec.id);
      if (previous) {
        if (previous.signature !== signature || !safe) previous.dependency = undefined;
        continue;
      }
      found.set(spec.id, {
        id: spec.id, signature, label: `${alias} · ${String(spec.type)} · ${workload.id}`,
        dependency: safe ? { alias, type: type.key, className: typeof spec.class === 'string' ? spec.class : '', sharedId: spec.id,
          params: Object.fromEntries(Object.entries(params).map(([key, value]) => [key, String(value)])) } : undefined,
      });
    }
  }
  return [...found.values()];
}

export function sharedSelectionProblem(dependency: Dependency, choices: SharedResource[]): string | undefined {
  if (!dependency.reuseId) return undefined;
  const selected = choices.find((item) => item.id === dependency.reuseId)?.dependency;
  const content = ({ type, className, sharedId, params }: Dependency) => ({ type, className, sharedId, params });
  if (!selected || canonical(content(selected)) !== canonical(content(dependency))) return t('workloads.resources.selectionChanged');
  return undefined;
}
