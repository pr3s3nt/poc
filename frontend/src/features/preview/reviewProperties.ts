import { parse } from 'yaml';
import type { ReviewArtifact, ReviewNode } from './DeploymentReview';
import { t } from '../../shared/i18n';

export type NodeFact = [label: string, value: string];
const labels: Record<string, string> = { database: t('preview.reviewProperties.databaseName'), namespace: 'Namespace', name: t('preview.reviewProperties.resourceName'), port: 'Port', storage: t('preview.reviewProperties.storage'), engineVersion: t('preview.reviewProperties.version') };

// Read only artifacts already bounded and sanitized by their owner, never raw
// Terraform source, environment values or secret references.
export function nodeFacts(node: ReviewNode, artifacts: ReviewArtifact[]): NodeFact[] {
  const facts: NodeFact[] = [];
  if (Array.isArray(node.properties)) for (const property of node.properties) {
    if (!Object.hasOwn(labels, property.key)) continue;
    if (property.availability === 'UNKNOWN') facts.push([labels[property.key]!, t('preview.reviewProperties.waitingForResourceOutputs')]);
    else if (property.availability === 'AVAILABLE' && typeof property.value === 'string' && property.value.length <= 96) facts.push([labels[property.key]!, property.value]);
  }
  if (node.kind === 'workload') {
    const source = artifacts.find(a => a.kind === 'kubernetes-manifest' && a.path.startsWith('Deployment/') && a.after.text)?.after.text;
    if (source) try {
      const doc = parse(source), spec = doc?.spec, containers = spec?.template?.spec?.containers;
      if (Array.isArray(containers)) {
        const images = containers.flatMap(c => typeof c?.image === 'string' ? [c.image] : []);
        const ports = containers.flatMap(c => Array.isArray(c?.ports) ? c.ports.flatMap((p: { containerPort?: unknown }) => typeof p?.containerPort === 'number' ? [String(p.containerPort)] : []) : []);
        if (ports.length) facts.push(['Port', [...new Set<string>(ports)].join(', ')]);
        if (images.length) facts.push(['Image', images.join(', ')]);
        if (typeof spec.replicas === 'number') facts.push(['Replicas', String(spec.replicas)]);
        const requests = containers[0]?.resources?.requests;
        if (requests) facts.push([t('preview.reviewProperties.cPURAMPerPod'), [requests.cpu, requests.memory].filter(v => typeof v === 'string').join(' / ')]);
      }
    } catch { /* Unsupported artifacts keep public metadata. */ }
  }
  if (node.resourceType === 'redis') facts.unshift(['ID cache', node.id.split('#')[1] ?? node.name]);
  const account = node.placement?.account || node.account;
  if (account) facts.push([t('preview.reviewProperties.account'), account]);
  const region = node.placement?.region || node.region;
  if (region) facts.push([t('preview.reviewProperties.region'), region]);
  if (node.module?.module) facts.push(['Module', node.module.module]);
  if (node.class) facts.push(['Class', node.class]);
  const priorities = node.resourceType === 'postgres' ? [t('preview.reviewProperties.databaseName'), 'Port'] : node.resourceType === 'redis' ? ['ID cache', 'Port'] : node.resourceType === 'k8s-namespace' ? ['Namespace', t('preview.reviewProperties.account')] : node.kind === 'workload' ? ['Port', 'Image'] : [t('preview.reviewProperties.resourceName'), t('preview.reviewProperties.account'), t('preview.reviewProperties.region')];
  return facts.sort((a, b) => (priorities.includes(a[0]) ? priorities.indexOf(a[0]) : 100) - (priorities.includes(b[0]) ? priorities.indexOf(b[0]) : 100));
}
