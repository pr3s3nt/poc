// Derive acceptance form inputs from the checksum-verified chart render.
// No Kubernetes writes; secret bytes and generated output stay outside Git.
import assert from 'node:assert/strict';
import {readFileSync,writeFileSync} from 'node:fs';
import YAML from 'yaml';
import {outsideRepo} from '../../../scripts/bootstrap/lib/runtime.mjs';
const dir=process.env.ORCH_TEST_RUN_DIR;assert(dir);outsideRepo(dir);
const docs=YAML.parseAllDocuments(readFileSync(dir+'/gitlab-rendered.yaml','utf8')).map(d=>d.toJSON()).filter(Boolean);
const maps=Object.fromEntries(docs.filter(d=>d.kind==='ConfigMap').map(d=>[d.metadata.name,d.data]));
const secretMap={'gitlab-ui-rails-secret':'RAILS_SECRETS','gitlab-ui-gitaly-secret':'GITALY_TOKEN','gitlab-ui-gitlab-shell-secret':'SHELL_SECRET','redis-credentials':'REDIS_PASSWORD','database-credentials':'PG_PASSWORD','object-storage-connection':'S3_CONNECTION','gitlab-ui-gitlab-initial-root-password':'ROOT_PASSWORD','gitlab-ui-gitlab-workhorse-secret':'WORKHORSE_SECRET','gitlab-ui-registry-secret':'REGISTRY_KEY','gitlab-ui-registry-notification':'REGISTRY_NOTIFICATION'};
const specs=[];
const base=(name,kind,containers,more={})=>({name,kind,containers,volumes:[],mounts:[],secrets:[],...more});
const image={postgres:'docker.io/library/postgres:17-bookworm',redis:'docker.io/library/redis:7.2-bookworm',s3:'docker.io/chrislusf/seaweedfs:4.52',aws:'public.ecr.aws/aws-cli/aws-cli:2.31.20'};
const container=(name,image,env={},args,command)=>({name,image,env,args,command,resources:{requests:{cpu:'100m',memory:'256Mi'},limits:{memory:'1Gi'}}});
const secret=key=>({key});
specs.push(base('postgres','StatefulSet',[container('postgres',image.postgres,{POSTGRES_DB:'gitlabhq_production',POSTGRES_USER:'gitlab',POSTGRES_PASSWORD:secret('PG_PASSWORD'),PGDATA:'/var/lib/postgresql/data/pgdata'})],{uid:999,claim:{name:'data',size:'10Gi',container:'postgres',path:'/var/lib/postgresql/data'},service:{name:'postgres',port:5432},volumes:[{name:'bootstrap',files:[{name:'extensions.sql',value:'CREATE EXTENSION IF NOT EXISTS pg_trgm;\nCREATE EXTENSION IF NOT EXISTS btree_gist;\nCREATE EXTENSION IF NOT EXISTS amcheck;\n'}]}],mounts:[{volume:'bootstrap',container:'postgres',path:'/docker-entrypoint-initdb.d'}]}));
specs[0].containers[0].readiness={type:'exec',command:['pg_isready','-U','gitlab','-d','gitlabhq_production']};
specs.push(base('redis','StatefulSet',[container('redis',image.redis,{REDIS_PASSWORD:secret('REDIS_PASSWORD')},['exec redis-server --appendonly yes --maxmemory-policy noeviction --requirepass "$REDIS_PASSWORD"'],['/bin/sh','-c'])],{uid:999,claim:{name:'data',size:'1Gi',container:'redis',path:'/data'},service:{name:'redis',port:6379}}));
specs[1].containers[0].readiness={type:'exec',command:['sh','-c','REDISCLI_AUTH="$REDIS_PASSWORD" redis-cli ping']};
specs.push(base('objectstore','StatefulSet',[container('objectstore',image.s3,{AWS_ACCESS_KEY_ID:secret('S3_USER'),AWS_SECRET_ACCESS_KEY:secret('S3_PASSWORD'),HOME:'/data'},['mini','-dir=/data','-s3.port=9000','-webdav=false','-admin.ui=false','-master.telemetry=false'],['weed'])],{uid:1000,claim:{name:'data',size:'5Gi',container:'objectstore',path:'/data'},service:{name:'s3',port:9000}}));
specs[2].containers[0].readiness={type:'tcp',port:9000};
specs.push(base('object-init','Job',[container('buckets',image.aws,{AWS_ACCESS_KEY_ID:secret('S3_USER'),AWS_SECRET_ACCESS_KEY:secret('S3_PASSWORD'),S3_ENDPOINT:{service:'objectstore',port:'s3'},AWS_DEFAULT_REGION:'us-east-1',AWS_EC2_METADATA_DISABLED:'true',AWS_MAX_ATTEMPTS:'2',HOME:'/tmp'},['set -e; for bucket in gitlab-artifacts git-lfs gitlab-uploads gitlab-packages gitlab-pages gitlab-backups; do aws --cli-connect-timeout 5 --cli-read-timeout 15 --endpoint-url "$S3_ENDPOINT" s3api head-bucket --bucket "$bucket" || aws --cli-connect-timeout 5 --cli-read-timeout 15 --endpoint-url "$S3_ENDPOINT" s3api create-bucket --bucket "$bucket"; done'],['/bin/sh','-c'])],{uid:1000}));
for(const d of docs.filter(d=>['Deployment','StatefulSet','Job'].includes(d.kind))){
 const name=d.metadata.name.includes('migrations')?'migrations':d.metadata.name.includes('webservice')?'webservice':d.metadata.name.includes('sidekiq')?'sidekiq':d.metadata.name.includes('toolbox')?'toolbox':'gitaly';
 const pod=d.spec.template.spec,spec=base(name,d.kind,[],{uid:1000});
 if(name==='gitaly'){spec.claim={name:'repo-data',size:'10Gi',container:'gitaly',path:'/home/git/repositories'};spec.service={name:'grpc',port:8075};}
 if(name==='webservice')spec.service={name:'http',port:8181};
 if(['webservice','sidekiq','toolbox'].includes(name))spec.wait=['migrations'];
 if(name==='migrations')spec.wait=['object-init'];
 for(const v of pod.volumes){
  if(['registry-db-config','openbao-db-config'].includes(v.name))continue;
  const sources=v.projected?.sources??(v.configMap?[{configMap:v.configMap}]:[]);
  const configSources=sources.filter(s=>s.configMap&&maps[s.configMap.name]);
  if(sources.some(s=>s.secret)&&v.name!=='custom-ca-certificates'&&!sources.every(s=>s.secret?.optional))continue;
  const files=configSources.flatMap(s=>{const data=maps[s.configMap.name];return Object.entries(data).map(([name,value])=>({name,value:String(value).replaceAll('gitlab-ui-webservice-default.gitlab-ui-survey.svc','webservice').replaceAll('gitlab-ui-gitaly-0.gitlab-ui-gitaly.gitlab-ui-survey.svc','gitaly')}));});
  spec.volumes.push({name:v.name,...(files.length?{files}:{emptyDir:{...(v.emptyDir?.medium?{medium:v.emptyDir.medium}:{})}})});
 }
 const all=[...pod.containers,...(pod.initContainers??[])];
 for(const c of all){
  const init=(pod.initContainers??[]).includes(c),env=Object.fromEntries((c.env??[]).filter(e=>e.value!=null).map(e=>[e.name,String(e.value)]));
  const out={name:c.name,image:c.image,role:init?'init':'app',...(init?{order:pod.initContainers.indexOf(c)+1}:{}),env,command:c.command,args:c.args?.map(a=>a.replace(/\n\s*/g,' ')),resources:c.resources??{requests:{cpu:'50m',memory:'128Mi'}}};
  for(const[k,p]of[['readiness','readinessProbe'],['startup','startupProbe']])if(c[p]){const q=c[p];out[k]={type:q.httpGet?'http':q.exec?'exec':'tcp',...(q.httpGet??q.exec??q.tcpSocket??q.grpc),...Object.fromEntries(Object.entries(q).filter(([k])=>/Seconds$|Threshold$/.test(k)))};if(out[k].type==='exec')out[k].command=q.exec.command;}
  // Use bound Service values as actual configuration inputs, preserving dependency edges.
  if(name!=='gitaly'){env.PG_ENDPOINT={service:'postgres',port:'postgres'};env.REDIS_ENDPOINT={service:'redis',port:'redis'};}
  for(const m of c.volumeMounts??[]){
   if(['registry-db-config','openbao-db-config'].includes(m.name))continue;
   if(m.name==='repo-data'&&spec.claim)continue;
   const v=pod.volumes.find(v=>v.name===m.name);assert(v);
   const sources=(v.projected?.sources??[]).filter(s=>s.secret);
   if(sources.length&&m.name!=='custom-ca-certificates'&&!sources.every(s=>s.secret.optional)){
    for(const src of sources){if(src.secret.optional)continue;const key=secretMap[src.secret.name];assert(key,'Unknown required secret '+src.secret.name);for(const item of src.secret.items??[]){spec.secrets.push({container:c.name,key,path:m.mountPath+'/'+item.path});}}
   }else spec.mounts.push({volume:m.name,container:c.name,path:m.mountPath.replace(/\/$/,''),subPath:m.subPath,readOnly:!!m.readOnly||spec.volumes.find(v=>v.name===m.name)?.files?.length>0});
  }
  spec.containers.push(out);
 }
 // Make database/Redis reference outputs feed ERB/Gomplate, not unused env placeholders.
 for(const v of spec.volumes)for(const f of v.files??[]){f.value=f.value.replaceAll('host: "postgres"',`host: <%= ENV.fetch('PG_ENDPOINT').split('://',2).last.split(':').first.to_json %>`).replaceAll('@redis:6379',`@<%= ENV.fetch('REDIS_ENDPOINT').split('://',2).last %>`).replaceAll('URL = "redis://redis:6379"','URL = "redis://{% .Env.REDIS_ENDPOINT | strings.TrimPrefix "http://" %}"');}
 specs.push(spec);
}
assert.equal(specs.length,9);
// Providers before consumers also makes them selectable in the real form.
const order=['postgres','redis','objectstore','object-init','gitaly','migrations','webservice','sidekiq','toolbox'];specs.sort((a,b)=>order.indexOf(a.name)-order.indexOf(b.name));
writeFileSync(dir+'/gitlab-profile.json',JSON.stringify(specs,null,2),{mode:0o600});console.log('Prepared',specs.length,'form profiles; no deployment');
