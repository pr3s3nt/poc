// The dependency graph of a plan (ADR-017) as the Console shows it: every ordering edge with its
// reason, the nodes that were only selected (no ordering edge), and the consumers a failed
// provider held back. Descriptors, reasons and safe codes only: never a param value, output
// value, secret reference or error text.
import { t } from '../i18n';
export type GraphNode = { descriptor: string; kind: string; resourceType?: string; origins?: string[]; paramKeys?: string[] };
export type GraphEdge = { consumer: string; provider: string; reason: string; path?: string };
export type BlockedConsumer = { consumer: string; provider: string; reason: string; code: string };

// Every edge of a plan is an ORDERING edge: the provider runs first. The label says why it exists.
const reasonLabels: Record<string, string> = {
  'resource-reference': t('shared.dependencyGraph.referencesAnOutputOfAnother'),
  'workload-placeholder': t('shared.dependencyGraph.workloadReadsAResourceOutput'),
  'resource-input-placeholder': t('shared.dependencyGraph.resourceParameterReadsAnOutput'),
  'deployment-set-private': t('shared.dependencyGraph.privateResourceDeclaredInThe'),
  'provider-binding': t('shared.dependencyGraph.theModulesKubernetesProviderRuns'),
  'co-provision-is-dependent': t('shared.dependencyGraph.companionResourceRunsAfterThe'),
  'co-provision-match-dependents': t('shared.dependencyGraph.everyConsumerOfTheOwner'),
  'execution-profile': t('shared.dependencyGraph.executionRecordEdgeOfPlanner'),
};
export const reasonLabel = (reason: string) => (Object.hasOwn(reasonLabels, reason) ? reasonLabels[reason] : t('shared.dependencyGraph.otherDependency'));

// Failure codes are fixed allowlist values; anything else renders generically.
const codeLabels: Record<string, string> = {
  REJECTED_BEFORE_RUN: t('shared.dependencyGraph.rejectedBeforeTheModuleRan'),
  OUTPUT_CONTRACT: t('shared.dependencyGraph.outputsDoNotMatchThe'),
  NOT_READY: t('shared.dependencyGraph.notReady'), READINESS_UNAVAILABLE: t('shared.dependencyGraph.readinessCouldNotBeChecked'), AUTH: t('shared.dependencyGraph.authenticationError'), INPUT: t('shared.dependencyGraph.invalidInput'),
  TIMEOUT: t('shared.dependencyGraph.timedOut'), FAILED: t('shared.dependencyGraph.failed'), OUTCOME_UNKNOWN: t('shared.dependencyGraph.outcomeUnknown'), SECRET_UNAVAILABLE: t('shared.dependencyGraph.couldNotReadARequired'),
  DESTRUCTIVE_PLAN: t('shared.dependencyGraph.planWouldDestroyResources'), STATE_BACKEND: t('shared.dependencyGraph.stateStoreError'), STATE_NOT_ACKNOWLEDGED: t('shared.dependencyGraph.stateNotConfirmedAsSaved'),
  TRANSIENT: t('shared.dependencyGraph.transientError'), CANCELLED: t('shared.dependencyGraph.destroyed'), PHYSICAL_RESOURCE_REMAINS: t('shared.dependencyGraph.physicalResourcesStillExist'),
};
export const codeLabel = (code: string) => (Object.hasOwn(codeLabels, code) ? codeLabels[code] : t('shared.dependencyGraph.unknownError'));

const originLabels: Record<string, string> = { 'co-provision': t('shared.dependencyGraph.companionResource'), 'resource-reference': t('shared.dependencyGraph.referenced'), 'provider-binding': t('shared.dependencyGraph.providerCluster'), 'shared-dependency': t('shared.dependencyGraph.shared'), 'private-dependency': t('shared.dependencyGraph.privateToTheWorkload'), 'implicit-workload': 'workload', 'execution-profile': t('shared.dependencyGraph.plannerSourceReferencingHistory') };
const originLabel = (origin: string) => (Object.hasOwn(originLabels, origin) ? originLabels[origin] : t('shared.dependencyGraph.otherSource'));

export function DependencyGraph({ nodes, edges, blocked = [], definitionOf }: { nodes: GraphNode[]; edges: GraphEdge[]; blocked?: BlockedConsumer[]; definitionOf?: (descriptor: string) => string | undefined }) {
  const touched = new Set(edges.flatMap((edge) => [edge.consumer, edge.provider]));
  // A node that exists only because a rule selected it (no flag, no reference) has no ordering edge.
  const selectionOnly = nodes.filter((node) => !touched.has(node.descriptor));
  return <div className="dependency-graph" aria-label={t('shared.dependencyGraph.planDependencies')}>
    <p>{t('shared.dependencyGraph.nodesCountNodesEdgesCountOrderingEdges', { nodesCount: nodes.length, edgesCount: edges.length })}</p>
    <h3>{t('shared.dependencyGraph.nodes')}</h3>
    <ul>{nodes.map((node) => <li key={node.descriptor}>{node.descriptor} · {node.kind}{definitionOf?.(node.descriptor) ? ` · ${definitionOf(node.descriptor)}` : ''}{node.paramKeys?.length ? t('shared.dependencyGraph.parametersValue', { value: node.paramKeys.join(', ') }) : ''}</li>)}</ul>
    {edges.length ? <><h3>{t('shared.dependencyGraph.dependenciesConsumerProvider')}</h3>
      <ul>{edges.map((edge, index) => <li key={`${edge.consumer}-${edge.provider}-${edge.reason}-${index}`} data-reason={edge.reason}>{edge.consumer} → {edge.provider} <small>({reasonLabel(edge.reason)})</small></li>)}</ul></> : null}
    {selectionOnly.length ? <><h3>{t('shared.dependencyGraph.selectedOnlyNoExecutionOrder')}</h3>
      <ul>{selectionOnly.map((node) => <li key={node.descriptor}>{node.descriptor} <small>{t('shared.dependencyGraph.valueNoDependencyEdgeForces', { value: (node.origins ?? []).map(originLabel).join(', ') || t('shared.dependencyGraph.selected') })}</small></li>)}</ul></> : null}
    {blocked.length ? <section aria-label={t('shared.dependencyGraph.blockedComponents')}><h3>{t('shared.dependencyGraph.blockedByAFailedDependency')}</h3>
      <ul>{blocked.map((item, index) => <li key={`${item.consumer}-${item.provider}-${index}`} data-code={item.code}>{item.consumer} <strong>{t('shared.dependencyGraph.didNotRun')}</strong> {' '}{t('shared.dependencyGraph.because')}{' '} {item.provider} {' '}{t('shared.dependencyGraph.failed2')}{' '} {codeLabel(item.code)} <small>({reasonLabel(item.reason)})</small></li>)}</ul></section> : null}
  </div>;
}
