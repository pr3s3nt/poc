import type { ApplicationsResponse, DeploymentView, ScoreSamplesResponse } from '../features/deploy/api/types';

export const applicationsFixture: ApplicationsResponse = {
  applications: [
    {
      key: 'acceptance',
      name: 'Acceptance Application',
      executionProfile: 'internal-k8s',
      runtimeStatus: 'READY',
      environments: [
        {
          key: 'dev',
          name: 'Development',
          environmentType: 'development',
          namespaceIdentity: 'acceptance-dev',
          currentDeploymentSetId: 'set-1',
          version: 1,
        },
      ],
    },
  ],
};

export const scoreSamplesFixture: ScoreSamplesResponse = {
  order: ['backend', 'worker', 'frontend'],
  samples: {
    backend: {
      apiVersion: 'score.dev/v1b1',
      metadata: { name: 'backend' },
      containers: { main: { image: 'acceptance-backend:dev' } },
    },
    worker: {
      apiVersion: 'score.dev/v1b1',
      metadata: { name: 'worker' },
      containers: { main: { image: 'acceptance-worker:dev' } },
    },
    frontend: {
      apiVersion: 'score.dev/v1b1',
      metadata: { name: 'frontend' },
      containers: { main: { image: 'acceptance-frontend:dev' } },
    },
  },
};

export const deploymentViewFixture: DeploymentView = {
  deployment: {
    id: 'dep-1',
    applicationKey: 'acceptance',
    environmentKey: 'dev',
    executionProfile: 'internal-k8s',
    action: 'DEPLOY',
    workloadId: 'backend',
    actorRef: 'web-console',
    status: 'SUCCEEDED',
    startedAt: '2026-09-21T09:00:00Z',
    finishedAt: '2026-09-21T09:01:00Z',
  },
  deploymentSet: {
    modules: { backend: {}, worker: {}, frontend: {} },
    shared: {
      'acceptance-db': { type: 'postgres', class: 'default', params: { database: 'acceptance', username: 'app' } },
    },
  },
  deploymentSetId: 'set-2',
  graph: {
    nodes: [
      {
        descriptor: 'workload.default#modules.backend',
        kind: 'workload',
        resourceType: 'workload',
        origins: ['implicit-workload'],
      },
      {
        descriptor: 'postgres.default#shared.acceptance-db',
        kind: 'resource',
        resourceType: 'postgres',
        origins: ['shared-dependency'],
      },
    ],
    edges: [
      {
        consumer: 'workload.default#modules.backend',
        provider: 'postgres.default#shared.acceptance-db',
        reason: 'workload-placeholder',
      },
    ],
  },
  batches: [
    ['k8s-cluster.internal#connections.internal-cluster'],
    ['postgres.default#shared.acceptance-db'],
  ],
  matches: {
    'postgres.default#shared.acceptance-db': {
      definitionKey: 'postgres-internal-statefulset',
      driverType: 'kubernetes',
    },
  },
  planHash: 'abcdef1234567890',
  resources: [
    {
      descriptor: 'postgres.default#shared.acceptance-db',
      resourceType: 'postgres',
      definitionKey: 'postgres-internal-statefulset',
      status: 'READY',
      batchIndex: 1,
      outputs: { host: 'acceptance-db', port: 5432, password: '***redacted***' },
    },
  ],
  workloads: [
    {
      workloadId: 'backend',
      status: 'READY',
      manifestDigest: 'deadbeefcafebabe',
      lastDeploymentId: 'dep-1',
      observedAt: '2026-09-21T09:01:00Z',
      targetRef: { namespace: 'acceptance-dev', cluster: 'idp-internal' },
    },
  ],
};
