// Render nonsecret installation manifests. Applying is an operator action.
import { readFileSync } from 'node:fs';
const phase = process.argv[2];
if (!['storage', 'setup', 'console'].includes(phase)) throw Error('Expected storage|setup|console');
const ns = 'orchestrator-dev-3070', tag = 'seed-80315d31';
const labels = { 'app.kubernetes.io/part-of': ns, 'app.kubernetes.io/managed-by': 'poc-local-installer' };
const meta = name => ({ name, namespace: ns, labels });
const volume = { name: 'storage', persistentVolumeClaim: { claimName: 'platform-storage' } };
const scripts = { name: 'scripts', configMap: { name: 'installation-scripts', defaultMode: 0o444 } };
const mount = (path, subPath, readOnly = false) => ({ name: 'storage', mountPath: path, subPath, readOnly });
const localScripts = { name: 'scripts', mountPath: '/local', readOnly: true };
const probe = command => ({ exec: { command: ['sh', '-ec', command] }, periodSeconds: 5, timeoutSeconds: 4, failureThreshold: 24 });
const service = (name, port) => ({ apiVersion: 'v1', kind: 'Service', metadata: meta(name), spec: { selector: { app: name }, ports: [{ port, targetPort: port }] } });
const deployment = (name, pod) => ({ apiVersion: 'apps/v1', kind: 'Deployment', metadata: meta(name), spec: { replicas: 1, strategy: { type: 'Recreate' }, selector: { matchLabels: { app: name } }, template: { metadata: { labels: { app: name, ...labels } }, spec: { automountServiceAccountToken: false, ...pod } } } });
const envs = values => Object.entries(values).map(([name, value]) => ({ name, value }));
let items;
if (phase === 'storage') {
 const files = ['init-db-credentials.sh', 'postgres-init-roles.sh', 'start-vault.sh', 'vault.hcl', 'connections-policy.hcl', 'applications-policy.hcl', 'outputs-policy.hcl'];
 const data = Object.fromEntries(files.map(f => [f, readFileSync(new URL(`../local/${f}`, import.meta.url), 'utf8')]));
 const prepare = `set -eu; umask 077
for dir in postgres role-passwords owner runtime state-key vault-data vault-bootstrap connection-token app-token output-token backend-data; do mkdir -p /storage/$dir; done
if [ ! -s /storage/superuser ]; then
  test ! -e /storage/postgres/PG_VERSION
  test ! -e /storage/postgres/pgdata/PG_VERSION
  od -An -N24 -tx1 /dev/urandom | tr -d ' \\n' > /storage/superuser
fi
chown $(id -u postgres):$(id -g postgres) /storage/postgres /storage/superuser
chmod 700 /storage/postgres
mkdir -p /storage/backend-data/recovery /storage/backend-data/delivery-git /storage/backend-data/module-cache /storage/backend-data/template-source-cache /storage/backend-data/tfroot
chown -R 65532:65532 /storage/backend-data
chmod 700 /storage/backend-data /storage/backend-data/*`;
 items = [
  { apiVersion: 'v1', kind: 'Namespace', metadata: { name: ns, labels } },
  { apiVersion: 'v1', kind: 'ConfigMap', metadata: meta('installation-scripts'), data },
  { apiVersion: 'v1', kind: 'PersistentVolumeClaim', metadata: meta('platform-storage'), spec: { accessModes: ['ReadWriteOnce'], resources: { requests: { storage: '8Gi' } } } },
  service('postgres', 5432), service('vault', 8200),
  deployment('postgres', {
   initContainers: [
    { name: 'prepare', image: 'postgres:16-alpine', command: ['sh', '-ec', prepare], volumeMounts: [{ name: 'storage', mountPath: '/storage' }] },
    { name: 'credentials', image: 'postgres:16-alpine', command: ['sh', '/local/init-db-credentials.sh'], volumeMounts: [localScripts, mount('/role-passwords', 'role-passwords'), mount('/owner', 'owner'), mount('/runtime', 'runtime'), mount('/state-key', 'state-key'), mount('/pgdata', 'postgres', true)] },
   ],
   containers: [{ name: 'postgres', securityContext: { runAsUser: 0 }, image: 'postgres:16-alpine', env: envs({ POSTGRES_DB: 'orchestrator', POSTGRES_USER: 'orchestrator', POSTGRES_PASSWORD_FILE: '/run/postgres-superuser', PGDATA: '/var/lib/postgresql/data/pgdata' }), volumeMounts: [mount('/var/lib/postgresql/data', 'postgres'), mount('/run/orchestrator/role-passwords', 'role-passwords', true), mount('/run/postgres-superuser', 'superuser', true), { name: 'scripts', mountPath: '/docker-entrypoint-initdb.d/10-orchestrator-roles.sh', subPath: 'postgres-init-roles.sh', readOnly: true }], readinessProbe: probe('pg_isready -U orchestrator -d orchestrator'), resources: { requests: { cpu: '50m', memory: '128Mi' }, limits: { memory: '768Mi' } } }], volumes: [volume, scripts],
  }),
  deployment('vault', { containers: [{ name: 'vault', image: 'hashicorp/vault:1.21', securityContext: { runAsUser: 0 }, command: ['sh', '/local/start-vault.sh'], env: envs({ VAULT_ADDR: 'http://127.0.0.1:8200' }), volumeMounts: [localScripts, ...[['/vault/data', 'vault-data'], ['/vault/bootstrap', 'vault-bootstrap'], ['/connection-token', 'connection-token'], ['/app-token', 'app-token'], ['/output-token', 'output-token']].map(([p, s]) => mount(p, s))], readinessProbe: probe('test -f /tmp/vault-ready && vault status >/dev/null 2>&1'), resources: { requests: { cpu: '50m', memory: '128Mi' }, limits: { memory: '768Mi' } } }], volumes: [volume, scripts] }),
 ];
} else if (phase === 'setup') {
 items = [{ apiVersion: 'batch/v1', kind: 'Job', metadata: meta('db-setup'), spec: { backoffLimit: 0, activeDeadlineSeconds: 180, template: { metadata: { labels }, spec: { automountServiceAccountToken: false, restartPolicy: 'Never', containers: [{ name: 'setup', image: `orchestrator-backend:${tag}`, imagePullPolicy: 'Never', command: ['/usr/local/bin/orchestrator', '-setup-database', '-database-owner-url-file', '/run/owner/database-url', '-console-database-role', 'orchestrator_console', '-state-database-role', 'orchestrator_state'], volumeMounts: [mount('/run/owner', 'owner', true)] }], volumes: [volume] } } } }];
} else {
 items = [service('backend', 8080), service('frontend', 8080),
 deployment('backend', { containers: [{ name: 'backend', image: `orchestrator-backend:${tag}`, imagePullPolicy: 'Never', env: envs({
  ORCHESTRATOR_LISTEN_ADDR: '0.0.0.0:8080', ORCHESTRATOR_ADAPTERS: 'kubernetes',
  ORCHESTRATOR_DATABASE_URL_FILE: '/run/runtime/console-database-url', ORCHESTRATOR_TERRAFORM_STATE_DATABASE_URL_FILE: '/run/runtime/state-database-url',
  ORCHESTRATOR_TERRAFORM_STATE_KEY_FILE: '/run/state-key/key', ORCHESTRATOR_TERRAFORM_STATE_KEY_ID: 'dev-3070-key-1',
  ORCHESTRATOR_CONNECTION_CREDENTIAL_STORE: 'vault', ORCHESTRATOR_CONNECTION_VAULT_ADDR: 'http://vault:8200', ORCHESTRATOR_CONNECTION_VAULT_TOKEN_FILE: '/run/connection-token/token',
  ORCHESTRATOR_OUTPUT_SECRET_STORE: 'vault', ORCHESTRATOR_OUTPUT_SECRET_VAULT_ADDR: 'http://vault:8200', ORCHESTRATOR_OUTPUT_SECRET_VAULT_TOKEN_FILE: '/run/output-token/token',
  ORCHESTRATOR_TERRAFORM_RECOVERY_DIR: '/var/lib/orchestrator/recovery', ORCHESTRATOR_TERRAFORM_MODULE_CACHE_DIR: '/var/lib/orchestrator/module-cache', ORCHESTRATOR_TEMPLATE_SOURCE_CACHE_DIR: '/var/lib/orchestrator/template-source-cache',
  ORCHESTRATOR_DELIVERY_GIT_ROOT: '/var/lib/orchestrator/delivery-git',
 }), volumeMounts: [mount('/run/runtime', 'runtime', true), mount('/run/state-key', 'state-key', true), mount('/run/connection-token', 'connection-token', true), mount('/run/output-token', 'output-token', true), mount('/var/lib/orchestrator', 'backend-data')], readinessProbe: probe('wget -q -O /dev/null http://127.0.0.1:8080/api/v1/healthz'), resources: { requests: { cpu: '100m', memory: '256Mi' }, limits: { memory: '2Gi' } } }], volumes: [volume] }),
 deployment('frontend', { containers: [{ name: 'frontend', image: `orchestrator-frontend:${tag}`, imagePullPolicy: 'Never', env: envs({ BACKEND_URL: 'http://backend:8080' }), readinessProbe: probe('wget -q -O /dev/null http://127.0.0.1:8080/healthz'), resources: { requests: { cpu: '20m', memory: '32Mi' }, limits: { memory: '128Mi' } } }] }),
 ];
}
console.log(JSON.stringify({ apiVersion: 'v1', kind: 'List', items }));
