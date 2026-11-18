import { readFileSync, writeFileSync, existsSync, mkdirSync, statSync, lstatSync } from 'node:fs';
import { resolve, dirname, join } from 'node:path';
import { randomBytes, randomUUID, createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { parseAllDocuments, stringify } from 'yaml';
export const root = resolve(new URL('../..', import.meta.url).pathname);
export const owner = 'orchestrator-installer';
export function requireThat(test, message) { if (!test) throw new Error(message); }
export function privateDirectory(dir) {
 dir=resolve(dir);for(let p=dir;;p=dirname(p)){requireThat(!existsSync(join(p,'.git')),'Private directory must be outside Git');if(p===dirname(p))break;}
 if(!existsSync(dir))mkdirSync(dir,{recursive:true,mode:0o700});
 const s=lstatSync(dir);requireThat(s.isDirectory()&&!s.isSymbolicLink()&&!(s.mode&0o077),'Private directory must be mode 0700');return dir;
}
export function privateRead(file) { const s=lstatSync(file);requireThat(s.isFile()&&!s.isSymbolicLink()&&!(s.mode&0o077),'Private input must be a regular owner-only file');return readFileSync(file); }
export function savePrivate(file,data,exclusive=false) { writeFileSync(file,data,{mode:0o600,flag:exclusive?'wx':'w'}); }
export function digest(value){return createHash('sha256').update(typeof value==='string'?value:JSON.stringify(value)).digest('hex');}
export function loadConfig(file,{complete=true}={}) {
 const c=JSON.parse(readFileSync(file,'utf8'));
 requireThat(['kind','staging','production'].includes(c.environment),'Unknown environment');
 requireThat(/^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/.test(c.namespace),'Invalid namespace');
 requireThat(typeof c.context==='string'&&c.context&&c.gateway&&c.images&&c.storage,'Missing target/configuration');
 if(complete){requireThat(!JSON.stringify(c).includes('CHANGE_ME'),'Resolve configuration placeholders');
  requireThat(!/[\s\/:?#]/.test(c.hostname)&&c.hostname.includes('.'),'Hostname must be a DNS name');
  requireThat(['http','https'].includes(c.scheme),'Invalid route scheme');
  requireThat(c.environment==='kind'||c.scheme==='https','Staging/production require an HTTPS Gateway listener');
  for(const image of Object.values(c.images))requireThat(typeof image==='string'&&image&&!/\s/.test(image),'Invalid image');
  if(c.environment!=='kind')for(const image of Object.values(c.images))requireThat(/@sha256:[a-f0-9]{64}$/.test(image),'Staging/production require immutable image digests');
  for(const n of ['postgres','vault','backend'])requireThat(/^\d+(Gi|Mi)$/.test(c.storage[n]),'Invalid PVC size');
 }
 return c;
}
export function readDocuments(text) { return parseAllDocuments(text,{uniqueKeys:true}).map(d=>{requireThat(!d.errors.length,'Invalid rendered YAML');return d.toJS()}).filter(Boolean); }
export function render(c,installationID='unprepared') {
 const yaml=execFileSync('kubectl',['kustomize',join(root,'deploy/kubernetes/overlays',c.environment)],{encoding:'utf8',maxBuffer:8<<20});
 const docs=readDocuments(yaml);
 for(const d of docs){
  d.metadata.labels={...d.metadata.labels,'app.kubernetes.io/managed-by':owner,'orchestrator.dev/installation':installationID};
  if(d.kind==='Namespace')d.metadata.name=c.namespace;else d.metadata.namespace=c.namespace;
  if(d.kind==='ConfigMap'&&d.metadata.name==='installation-settings')d.data={CONTEXT:c.context,HOSTNAME:c.hostname,GATEWAY:c.gateway.name,GATEWAY_NAMESPACE:c.gateway.namespace,LISTENER:c.gateway.listener,SCHEME:c.scheme,STORAGE_CLASS:c.storage.class};
  if(d.kind==='PersistentVolumeClaim'){d.spec.storageClassName=c.storage.class;d.spec.resources.requests.storage=c.storage[d.metadata.name.replace('-data','')];}
  if(d.kind==='HTTPRoute'){d.spec.hostnames=[c.hostname];d.spec.parentRefs=[{name:c.gateway.name,namespace:c.gateway.namespace,sectionName:c.gateway.listener}];}
  const pod=d.kind==='Deployment'?d.spec.template.spec:d.kind==='Job'?d.spec.template.spec:undefined;
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
 dir=privateDirectory(dir);const file=join(dir,'installation.json');
 if(existsSync(file)){const state=JSON.parse(privateRead(file));requireThat(state.context===c.context&&state.namespace===c.namespace&&state.environment===c.environment,'Private state belongs to another target');return state;}
 requireThat(!existsSync(join(dir,'prepare-intent')),'Interrupted preparation requires operator recovery');savePrivate(join(dir,'prepare-intent'),'Preparing independent installation\n',true);
 const hex=n=>randomBytes(n).toString('hex');
 const s={id:randomUUID(),context:c.context,namespace:c.namespace,environment:c.environment,passwords:{postgres:hex(24),owner:hex(24),console:hex(24),state:hex(24)},stateKey:hex(32),setupToken:hex(32),vaultTokens:{'connection-token':'orch-'+hex(32),'output-token':'orch-'+hex(32),'app-token':'orch-'+hex(32)}};
 if(testCertificates){
  requireThat(c.environment==='kind','Test CA generation is restricted to kind');
  const ca=join(dir,'ca.crt'),key=join(dir,'ca.key');
  runSSL(['req','-x509','-newkey','rsa:3072','-nodes','-keyout',key,'-out',ca,'-days','30','-subj','/CN=Orchestrator kind fixture']);
  s.tls={};
  for(const name of ['postgres','vault']){
   const prefix=join(dir,name);savePrivate(prefix+'.ext',`subjectAltName=DNS:${name},DNS:${name}.${c.namespace}.svc,DNS:${name}.${c.namespace}.svc.cluster.local,IP:127.0.0.1\nextendedKeyUsage=serverAuth\n`);
   runSSL(['req','-newkey','rsa:3072','-nodes','-keyout',prefix+'.key','-out',prefix+'.csr','-subj','/CN='+name]);
   runSSL(['x509','-req','-in',prefix+'.csr','-CA',ca,'-CAkey',key,'-CAcreateserial','-out',prefix+'.crt','-days','30','-extfile',prefix+'.ext']);
   s.tls[name]={ca,cert:prefix+'.crt',key:prefix+'.key'};
  }
 }else{s.tls=c.tls;requireThat(s.tls?.postgres&&s.tls?.vault,'Supply PostgreSQL/Vault TLS file paths');}
 for(const name of ['postgres','vault']){const tls=s.tls[name];requireThat(tls.ca&&tls.cert&&tls.key,'Missing TLS file');privateRead(tls.key);runSSL(['verify','-CAfile',tls.ca,tls.cert]);runSSL(['x509','-in',tls.cert,'-noout','-checkhost',name]);}
 savePrivate(file,JSON.stringify(s,null,2)+'\n',true);savePrivate(join(dir,'setup-token'),s.setupToken+'\n',true);return s;
}
export function desiredSecrets(c,s,{vault=false}={}) {
 const url=role=>`postgres://orchestrator_${role}:${s.passwords[role]}@postgres:5432/orchestrator?sslmode=verify-full&sslrootcert=/private/postgres-ca.crt`;
 const values={'orchestrator-postgres':{password:s.passwords.postgres},'orchestrator-db-roles':{owner:s.passwords.owner,console:s.passwords.console,state:s.passwords.state},'orchestrator-runtime':{'console-database-url':url('console'),'state-database-url':url('state')},'orchestrator-owner':{'database-url':url('owner')},'orchestrator-state-key':{key:s.stateKey},'orchestrator-setup':{'setup-token':s.setupToken},'orchestrator-trust':{'postgres-ca.crt':readFileSync(s.tls.postgres.ca),'vault-ca.crt':readFileSync(s.tls.vault.ca)}};
 for(const n of ['postgres','vault'])values[n+'-tls']={'tls.crt':readFileSync(s.tls[n].cert),'tls.key':privateRead(s.tls[n].key)};
 if(vault)values['orchestrator-vault-runtime']=s.vaultTokens;
 return Object.entries(values).map(([name,data])=>({apiVersion:'v1',kind:'Secret',type:'Opaque',metadata:{name,namespace:c.namespace,labels:{'app.kubernetes.io/managed-by':owner,'orchestrator.dev/installation':s.id}},data:Object.fromEntries(Object.entries(data).map(([k,v])=>[k,Buffer.from(v).toString('base64')]))}));
}
export function sameSecret(existing,desired){return JSON.stringify(Object.entries(existing.data??{}).sort())===JSON.stringify(Object.entries(desired.data).sort());}
export function assertNamespace(ns,c,s){requireThat(ns.metadata.name===c.namespace&&ns.metadata.labels?.['app.kubernetes.io/managed-by']===owner&&ns.metadata.labels?.['orchestrator.dev/installation']===s.id,'Namespace ownership mismatch');if(s.namespaceUID)requireThat(ns.metadata.uid===s.namespaceUID,'Namespace UID changed');}
