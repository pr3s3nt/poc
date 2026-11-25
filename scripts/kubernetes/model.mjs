import { readFileSync, writeFileSync, existsSync, mkdirSync, statSync, lstatSync } from 'node:fs';
import { resolve, dirname, join } from 'node:path';
import { randomBytes, randomUUID, createHash, createPublicKey, X509Certificate } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { parseAllDocuments, stringify } from 'yaml';
export const root = resolve(new URL('../..', import.meta.url).pathname);
export const owner = 'orchestrator-installer';
export const vaultReviewerMount = '/var/run/secrets/kubernetes.io/serviceaccount';
const vaultReviewerVolume = 'vault-kubernetes-reviewer';
const imageDigest = /^[^@]+@sha256:[a-f0-9]{64}$/;
const imageTag = /(?:^|\/)[^:@/]+:\w[\w.-]{0,127}$/;
// pg_restore --list can finish after the TOC. Drain the remaining stream so
// kubectl does not wait forever on a stdin copier, preserving the tool status.
export const archiveListingScript = 'pg_restore --list; status=$?; cat >/dev/null; exit "$status"';
export function requireThat(test, message) { if (!test) throw new Error(message); }
export function privateDirectory(dir) {
 dir=resolve(dir);for(let p=dir;;p=dirname(p)){requireThat(!existsSync(join(p,'.git')),'Private directory must be outside Git');if(p===dirname(p))break;}
 if(!existsSync(dir))mkdirSync(dir,{recursive:true,mode:0o700});
 const s=lstatSync(dir);requireThat(s.isDirectory()&&!s.isSymbolicLink()&&!(s.mode&0o077),'Private directory must be mode 0700');return dir;
}
export function privateRead(file) { const s=lstatSync(file);requireThat(s.isFile()&&!s.isSymbolicLink()&&!(s.mode&0o077),'Private input must be a regular owner-only file');return readFileSync(file); }
export function savePrivate(file,data,exclusive=false) { writeFileSync(file,data,{mode:0o600,flag:exclusive?'wx':'w'}); }
export function digest(value){return createHash('sha256').update(typeof value==='string'?value:JSON.stringify(value)).digest('hex');}
function usesInternalTLS(config) {
 return config.environment !== 'staging' || config.tls !== undefined;
}
export function assertInstallationConfig(config, state) {
 requireThat(state.context === config.context && state.namespace === config.namespace && state.environment === config.environment,
  'Private inputs belong to another target');
 requireThat(Boolean(state.tls) === usesInternalTLS(config), 'Cannot change internal TLS mode for a prepared installation');
 requireThat(!state.vaultKubernetesAuth || config.vaultKubernetesAuth === true,
  'Removing managed Vault Kubernetes Auth requires a separate operator procedure');
 requireThat(!state.vaultUIRouted || config.vaultUI?.hostname,
  'Removing the Vault UI route requires a separate operator procedure');
}
export function loadConfig(file,{complete=true}={}) {
 const c=JSON.parse(readFileSync(file,'utf8'));
 requireThat(['kind','staging','production'].includes(c.environment),'Unknown environment');
 requireThat(/^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/.test(c.namespace),'Invalid namespace');
 requireThat(typeof c.context==='string'&&c.context&&c.gateway&&c.images&&c.storage,'Missing target/configuration');
 requireThat(c.vaultKubernetesAuth === undefined || typeof c.vaultKubernetesAuth === 'boolean',
  'vaultKubernetesAuth must be a boolean');
 requireThat(c.oidc === undefined || typeof c.oidc === 'boolean', 'oidc must be a boolean');
 if (c.vaultUI !== undefined) {
  requireThat(c.vaultUI && typeof c.vaultUI === 'object' && !Array.isArray(c.vaultUI) &&
   Object.keys(c.vaultUI).every(key => key === 'hostname'), 'vaultUI must be an object with an optional hostname');
  if (c.vaultUI.hostname !== undefined) {
   const hostname = c.vaultUI.hostname;
   requireThat(typeof hostname === 'string' && /^(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$/.test(hostname) &&
    hostname !== c.hostname, 'Vault UI requires its own DNS hostname');
   requireThat(c.scheme === 'https' && !usesInternalTLS(c), 'The Vault UI Gateway route requires HTTPS at the Gateway and HTTP to Vault');
  }
 }
 if(complete){requireThat(!JSON.stringify(c).includes('CHANGE_ME'),'Resolve configuration placeholders');
  requireThat(!/[\s\/:?#]/.test(c.hostname)&&c.hostname.includes('.'),'Hostname must be a DNS name');
  requireThat(['http','https'].includes(c.scheme),'Invalid route scheme');
  requireThat(c.environment==='kind'||c.scheme==='https','Staging/production require an HTTPS Gateway listener');
  for(const name of ['backend','frontend','postgres','vault'])requireThat(typeof c.images[name]==='string'&&c.images[name],'All four images are required');
  for(const image of Object.values(c.images))requireThat(typeof image==='string'&&image&&!/\s/.test(image),'Invalid image');
  for(const image of Object.values(c.images)){
   if(c.environment==='production')requireThat(imageDigest.test(image),'Production requires immutable image digests');
   if(c.environment==='staging')requireThat(imageDigest.test(image)||(!image.includes('@')&&imageTag.test(image)),'Staging images require an explicit tag or SHA-256 digest');
  }
  for(const n of ['postgres','vault','backend'])requireThat(/^\d+(Gi|Mi)$/.test(c.storage[n]),'Invalid PVC size');
 }
 return c;
}
export function loadOIDC(c, dir) {
 if (!c.oidc) return undefined;
 const input = JSON.parse(privateRead(join(dir, 'oidc.json')));
 requireThat(input && typeof input === 'object' && !Array.isArray(input), 'Invalid OIDC configuration');
 for (const field of ['issuer', 'clientId', 'clientSecret', 'organizationKey', 'publicUrl']) {
  requireThat(typeof input[field] === 'string' && input[field].trim(), 'Missing OIDC field: ' + field);
 }
 requireThat(input.publicUrl === c.scheme + '://' + c.hostname, 'OIDC publicUrl must match the Console origin');
 const issuer = URL.parse(input.issuer);
 const allowHTTP = c.environment === 'kind' && input.allowHttp === true;
 requireThat(input.allowHttp === undefined || input.allowHttp === false || allowHTTP,
  'OIDC allowHttp is restricted to kind');
 requireThat((c.scheme === 'https' || allowHTTP) && issuer &&
  (issuer.protocol === 'https:' || (allowHTTP && issuer.protocol === 'http:')) &&
  !issuer.username && !issuer.password && !issuer.search && !issuer.hash && !input.issuer.endsWith('/'),
  'OIDC requires HTTPS and a canonical issuer URL');
 return input;
}
export function readDocuments(text) { return parseAllDocuments(text,{uniqueKeys:true}).map(d=>{requireThat(!d.errors.length,'Invalid rendered YAML');return d.toJS()}).filter(Boolean); }
function configureVaultReviewer(pod, reviewerName) {
 pod.serviceAccountName = reviewerName;
 pod.volumes.push({
  name: vaultReviewerVolume,
  projected: {
   defaultMode: 0o440,
   sources: [
    {serviceAccountToken: {path:'token', expirationSeconds:3600}},
    {configMap: {name:'kube-root-ca.crt', items:[{key:'ca.crt', path:'ca.crt'}]}},
   ],
  },
 });
 pod.containers.find(container => container.name === 'vault').volumeMounts.push({
  name:vaultReviewerVolume, mountPath:vaultReviewerMount, readOnly:true,
 });
}
function configurePlaintext(document, pod) {
 const name = document.metadata.name;
 if (document.kind === 'ConfigMap' && name.startsWith('vault-config-')) {
  document.data['vault.hcl'] = document.data['vault.hcl']
   .replace(/^[ \t]*tls_(?:cert|key)_file[ \t]*=.*\n/gm, '')
   .replace('listener "tcp" {', 'listener "tcp" {\n  tls_disable = true')
   .replace('https://vault:8200', 'http://vault:8200');
 }
 if (!pod || name === 'frontend') return;
 if (name === 'vault') {
  delete pod.initContainers;
  pod.volumes = pod.volumes.filter(volume => !['private', 'scripts', 'source'].includes(volume.name));
  const [vault] = pod.containers;
  vault.env = [{name:'VAULT_ADDR', value:'http://127.0.0.1:8200'}];
  vault.volumeMounts = vault.volumeMounts.filter(mount => mount.name !== 'private');
  return;
 }
 const source = pod.volumes.find(volume => volume.name === 'source').projected;
 source.sources = source.sources.filter(entry => !['postgres-tls', 'orchestrator-trust'].includes(entry.secret?.name));
 for (const container of pod.containers) {
  if (container.env) {
   container.env = container.env.filter(entry => !['SSL_CERT_FILE', 'VAULT_CACERT'].includes(entry.name));
   for (const entry of container.env) {
    if (entry.value === 'https://vault:8200') entry.value = 'http://vault:8200';
   }
  }
  container.volumeMounts = container.volumeMounts.filter(mount => !['postgres-require-tls.sh', 'vault-ca.crt'].includes(mount.subPath));
 }
 if (name === 'postgres') pod.containers[0].args = ['-c', 'ssl=off'];
}
export function render(c,installationID='unprepared') {
 const yaml=execFileSync('kubectl',['kustomize',join(root,'deploy/kubernetes/overlays',c.environment)],{encoding:'utf8',maxBuffer:8<<20});
 const docs=readDocuments(yaml);
 const optional = file => readDocuments(readFileSync(join(root,'deploy/kubernetes/base',file),'utf8'));
 const reviewerName = 'orchestrator-vault-reviewer-' + installationID;
 if(c.vaultKubernetesAuth){
  const resources = optional('vault-kubernetes-auth.yaml');
  for(const resource of resources)resource.metadata.name = reviewerName;
  const binding = resources.find(resource => resource.kind === 'ClusterRoleBinding');
  binding.roleRef.name = reviewerName;
  binding.subjects[0] = {...binding.subjects[0], name:reviewerName, namespace:c.namespace};
  docs.push(...resources);
 }
 if(c.vaultUI?.hostname)docs.push(...optional('httproute-vault.yaml'));
 for(const d of docs){
  d.metadata.labels={...d.metadata.labels,'app.kubernetes.io/managed-by':owner,'orchestrator.dev/installation':installationID};
  if(d.kind==='Namespace')d.metadata.name=c.namespace;
  else if(!['ClusterRole','ClusterRoleBinding'].includes(d.kind))d.metadata.namespace=c.namespace;
  if(d.kind==='ConfigMap'&&d.metadata.name==='installation-settings')d.data={CONTEXT:c.context,HOSTNAME:c.hostname,GATEWAY:c.gateway.name,GATEWAY_NAMESPACE:c.gateway.namespace,LISTENER:c.gateway.listener,SCHEME:c.scheme,STORAGE_CLASS:c.storage.class};
  if(d.kind==='PersistentVolumeClaim'){d.spec.storageClassName=c.storage.class;d.spec.resources.requests.storage=c.storage[d.metadata.name.replace('-data','')];}
  if(d.kind==='HTTPRoute'){d.spec.hostnames=[d.metadata.name==='vault-ui'?c.vaultUI.hostname:c.hostname];d.spec.parentRefs=[{name:c.gateway.name,namespace:c.gateway.namespace,sectionName:c.gateway.listener}];}
  const pod=d.kind==='Deployment'?d.spec.template.spec:d.kind==='Job'?d.spec.template.spec:undefined;
  if(!usesInternalTLS(c))configurePlaintext(d,pod);
  if(c.vaultUI && d.kind==='ConfigMap' && d.data?.['vault.hcl']){
   d.data['vault.hcl']=d.data['vault.hcl'].replace(/^ui = false$/m,'ui = true');
  }
  if(c.vaultKubernetesAuth && d.kind==='Deployment' && d.metadata.name==='vault')configureVaultReviewer(pod,reviewerName);
  if(c.oidc && d.kind==='Deployment' && d.metadata.name==='backend'){
   pod.volumes.find(volume=>volume.name==='source').projected.sources.push({secret:{name:'orchestrator-oidc'}});
   pod.containers.find(container=>container.name==='backend').env.push({
    name:'ORCHESTRATOR_OIDC_CONFIG_FILE',value:'/private/oidc.json'
   });
  }
  if(pod){
   d.spec.template.metadata.labels={...d.spec.template.metadata.labels,'app.kubernetes.io/managed-by':owner,'orchestrator.dev/installation':installationID};
   if(c.imagePullSecrets?.length)pod.imagePullSecrets=c.imagePullSecrets.map(name=>({name}));
   for(const container of [...pod.containers,...pod.initContainers??[]]){
    const original=container.image;
    container.image=original.startsWith('orchestrator-backend:')?c.images.backend:original.startsWith('orchestrator-frontend:')?c.images.frontend:original.startsWith('postgres:')?c.images.postgres:original.startsWith('hashicorp/vault:')?c.images.vault:original;
    container.imagePullPolicy=c.environment==='kind'?'IfNotPresent':'Always';
   }
   d.spec.template.metadata.annotations={'orchestrator.dev/release':digest({images:c.images,configuration:c}).slice(0,20)};
  }
 }
 const release=digest({configuration:c,manifests:docs}).slice(0,20);
 for(const d of docs)if(d.kind==='Deployment'||d.kind==='Job')d.spec.template.metadata.annotations['orchestrator.dev/release']=release;
 return docs;
}
export const serialize=docs=>docs.map(d=>stringify(d)).join('---\n');
function runSSL(args){try{return execFileSync('openssl',args,{stdio:['ignore','pipe','pipe']});}catch{throw new Error('Certificate preparation/validation failed');}}
export function prepare(dir,c,{testCertificates=false}={}) {
 requireThat(!testCertificates || c.environment === 'kind','Test CA generation is restricted to kind');
 dir=privateDirectory(dir);const file=join(dir,'installation.json');
 if(existsSync(file)){const state=JSON.parse(privateRead(file));assertInstallationConfig(c,state);return state;}
 requireThat(!existsSync(join(dir,'prepare-intent')),'Interrupted preparation requires operator recovery');savePrivate(join(dir,'prepare-intent'),'Preparing independent installation\n',true);
 const hex=n=>randomBytes(n).toString('hex');
 const s={id:randomUUID(),context:c.context,namespace:c.namespace,environment:c.environment,passwords:{postgres:hex(24),owner:hex(24),console:hex(24),state:hex(24)},stateKey:hex(32),setupToken:hex(32),vaultTokens:{'connection-token':'orch-'+hex(32),'output-token':'orch-'+hex(32),'app-token':'orch-'+hex(32)}};
 if(testCertificates){
  const ca=join(dir,'ca.crt'),key=join(dir,'ca.key');
  runSSL(['req','-x509','-newkey','rsa:3072','-nodes','-keyout',key,'-out',ca,'-days','30','-subj','/CN=Orchestrator kind fixture']);
  s.tls={};
  for(const name of ['postgres','vault']){
   const prefix=join(dir,name);savePrivate(prefix+'.ext',`subjectAltName=DNS:${name},DNS:${name}.${c.namespace}.svc,DNS:${name}.${c.namespace}.svc.cluster.local,IP:127.0.0.1\nextendedKeyUsage=serverAuth\n`);
   runSSL(['req','-newkey','rsa:3072','-nodes','-keyout',prefix+'.key','-out',prefix+'.csr','-subj','/CN='+name]);
   runSSL(['x509','-req','-in',prefix+'.csr','-CA',ca,'-CAkey',key,'-CAcreateserial','-out',prefix+'.crt','-days','30','-extfile',prefix+'.ext']);
   s.tls[name]={ca,cert:prefix+'.crt',key:prefix+'.key'};
  }
 }else if(usesInternalTLS(c)){s.tls=c.tls;}
 if(usesInternalTLS(c)){
  requireThat(s.tls?.postgres&&s.tls?.vault,'Supply PostgreSQL/Vault TLS file paths');
  for(const name of ['postgres','vault']){const tls=s.tls[name];requireThat(tls.ca&&tls.cert&&tls.key,'Missing TLS file');const key=privateRead(tls.key);let matches=false;try{matches=new X509Certificate(readFileSync(tls.cert)).publicKey.export({type:'spki',format:'der'}).equals(createPublicKey(key).export({type:'spki',format:'der'}));}catch{throw new Error('Invalid TLS certificate/key material');}requireThat(matches,'TLS certificate and private key do not match');runSSL(['verify','-CAfile',tls.ca,tls.cert]);runSSL(['x509','-in',tls.cert,'-noout','-checkhost',name]);}
 }
 savePrivate(file,JSON.stringify(s,null,2)+'\n',true);savePrivate(join(dir,'setup-token'),s.setupToken+'\n',true);return s;
}
export function desiredSecrets(c,s,{vault=false,oidc}={}) {
 const ssl=s.tls?'sslmode=verify-full&sslrootcert=/private/postgres-ca.crt':'sslmode=disable';
 const url=role=>`postgres://orchestrator_${role}:${s.passwords[role]}@postgres:5432/orchestrator?${ssl}`;
 const values={'orchestrator-postgres':{password:s.passwords.postgres},'orchestrator-db-roles':{owner:s.passwords.owner,console:s.passwords.console,state:s.passwords.state},'orchestrator-runtime':{'console-database-url':url('console'),'state-database-url':url('state')},'orchestrator-owner':{'database-url':url('owner')},'orchestrator-state-key':{key:s.stateKey},'orchestrator-setup':{'setup-token':s.setupToken}};
 if(s.tls){
  values['orchestrator-trust']={'postgres-ca.crt':readFileSync(s.tls.postgres.ca),'vault-ca.crt':readFileSync(s.tls.vault.ca)};
  for(const n of ['postgres','vault'])values[n+'-tls']={'tls.crt':readFileSync(s.tls[n].cert),'tls.key':privateRead(s.tls[n].key)};
 }
 if(vault)values['orchestrator-vault-runtime']=s.vaultTokens;
 if(oidc)values['orchestrator-oidc']={'oidc.json':JSON.stringify(oidc)};
 return Object.entries(values).map(([name,data])=>({apiVersion:'v1',kind:'Secret',type:'Opaque',metadata:{name,namespace:c.namespace,labels:{'app.kubernetes.io/managed-by':owner,'orchestrator.dev/installation':s.id}},data:Object.fromEntries(Object.entries(data).map(([k,v])=>[k,Buffer.from(v).toString('base64')]))}));
}
export function sameSecret(existing,desired){return JSON.stringify(Object.entries(existing.data??{}).sort())===JSON.stringify(Object.entries(desired.data).sort());}
export function assertNamespace(ns,c,s){requireThat(ns.metadata.name===c.namespace&&ns.metadata.labels?.['app.kubernetes.io/managed-by']===owner&&ns.metadata.labels?.['orchestrator.dev/installation']===s.id,'Namespace ownership mismatch');if(s.namespaceUID)requireThat(ns.metadata.uid===s.namespaceUID,'Namespace UID changed');}
