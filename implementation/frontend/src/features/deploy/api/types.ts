// JSON contract of the orchestrator API. The console owns these types; the Go
// backend owns the API itself.
export interface EnvironmentSummary {
  readonly key: string;
  readonly name: string;
  readonly environmentType: string;
  readonly namespaceIdentity: string;
  readonly currentDeploymentSetId: string;
  readonly version: number;
}

export interface ApplicationSummary {
  readonly key: string;
  readonly name: string;
  readonly executionProfile: string;
  readonly runtimeStatus: string;
  readonly region?: string;
  readonly environments: readonly EnvironmentSummary[];
}

export interface ApplicationsResponse {
  readonly applications: readonly ApplicationSummary[];
}

export interface ScoreSamplesResponse {
  readonly order: readonly string[];
  readonly samples: Readonly<Record<string, unknown>>;
}

export interface DeploymentCreated {
  readonly deploymentId: string;
  readonly status: string;
  readonly planHash: string;
  readonly workloadId: string;
}

export interface DeploymentRecord {
  readonly id: string;
  readonly applicationKey: string;
  readonly environmentKey: string;
  readonly executionProfile: string;
  readonly action: string;
  readonly workloadId: string;
  readonly actorRef: string;
  readonly status: string;
  readonly startedAt: string;
  readonly finishedAt?: string;
  readonly failureReason?: string;
}

export interface DeploymentsResponse {
  readonly deployments: readonly DeploymentRecord[] | null;
}

export interface GraphNode {
  readonly descriptor: string;
  readonly kind: string;
  readonly resourceType: string;
  readonly origins: readonly string[];
}

export interface GraphEdge {
  readonly consumer: string;
  readonly provider: string;
  readonly reason: string;
}

export interface DeploymentGraph {
  readonly nodes: readonly GraphNode[];
  readonly edges: readonly GraphEdge[];
}

export interface ResourceRow {
  readonly descriptor: string;
  readonly resourceType: string;
  readonly definitionKey: string;
  readonly status: string;
  readonly batchIndex: number;
  readonly outputs: Readonly<Record<string, unknown>>;
}

export interface WorkloadRow {
  readonly workloadId: string;
  readonly status: string;
  readonly manifestDigest: string;
  readonly lastDeploymentId: string;
  readonly observedAt: string;
  readonly targetRef: Readonly<Record<string, unknown>>;
}

export interface SharedResourceEntry {
  readonly type: string;
  readonly class: string;
  readonly params?: Readonly<Record<string, unknown>>;
}

export interface DeploymentSetDocument {
  readonly modules: Readonly<Record<string, unknown>>;
  readonly shared: Readonly<Record<string, SharedResourceEntry>>;
}

export interface DeploymentView {
  readonly deployment: DeploymentRecord;
  readonly deploymentSet: DeploymentSetDocument;
  readonly deploymentSetId: string;
  readonly graph: DeploymentGraph | null;
  readonly batches: readonly (readonly string[])[] | null;
  readonly matches: Readonly<Record<string, { readonly definitionKey: string; readonly driverType: string }>> | null;
  readonly planHash: string;
  readonly resources: readonly ResourceRow[];
  readonly workloads: readonly WorkloadRow[];
}
