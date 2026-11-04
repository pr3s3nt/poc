import { readFileSync } from 'node:fs';
import { parseArgs } from 'node:util';
import { loadBootstrapConfig, BootstrapError } from './lib/bootstrap-config.mjs';
import { runBootstrap } from './lib/runner.mjs';
// Preserve the existing catalog-only CLI. Bootstrap accepts one environment-specific config file.
if(process.argv.includes('--target') || process.argv.some(v=>v.startsWith('--target='))) {
 await import('./legacy-run.mjs');
} else {
 try {
  const {values}=parseArgs({options:{config:{type:'string'},help:{type:'boolean'}}});
  if(values.help)console.log('node scripts/bootstrap/run.mjs --config /private/bootstrap/dev.json\nSchema 3 Git-only: git-targets/dev.json (Git Definitions + final PVC acceptance); compatible local-document presets: direct-targets/dev.json; modes all/check/bootstrap.\nSchema 2: targets/dev.json (Catalog). All inputs and runtime.mode belong in the config file.\nLegacy catalog-only: --target dev|staging.');
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
