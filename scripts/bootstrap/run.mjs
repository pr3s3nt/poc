import { readFileSync } from 'node:fs';
import { parseArgs } from 'node:util';
import { loadBootstrapConfig, BootstrapError } from './lib/bootstrap-config.mjs';
import { runBootstrap } from './lib/runner.mjs';
// Preserve the existing catalog-only CLI. New full bootstrap accepts one config file.
if(process.argv.includes('--target') || process.argv.some(v=>v.startsWith('--target='))) {
 await import('./legacy-run.mjs');
} else {
 try {
  const {values}=parseArgs({options:{config:{type:'string'},help:{type:'boolean'}}});
  if(values.help)console.log('node scripts/bootstrap/run.mjs --config scripts/bootstrap/targets/dev.json\nAll inputs and runtime.mode belong in the config file. Legacy catalog-only: --target dev|staging.');
  else {
   if(!values.config)throw new BootstrapError('--config is required');
   if(process.env.DEBUG||process.env.NODE_DEBUG||process.env.PWDEBUG&&process.env.PWDEBUG!=='0')throw new BootstrapError('Disable DEBUG/NODE_DEBUG/PWDEBUG before handling credentials');
   const raw=JSON.parse(readFileSync(values.config,'utf8'));
   if(raw.schemaVersion===undefined&&raw.catalog&&raw.resourceTypes)await import('./legacy-run.mjs');
   else {const config=loadBootstrapConfig(values.config);await runBootstrap(config);}
  }
 } catch(e) {
  console.error(e instanceof BootstrapError?e.message:'Bootstrap failed; inspect private report and Console; credentials are not logged.');process.exitCode=1;
 }
}
