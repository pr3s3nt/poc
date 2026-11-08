#!/usr/bin/env node
import { loadConfig, seed, prepareOrganization, SeedError } from './lib/api-seed.mjs';
try {
 const args = process.argv.slice(2);
 if (args.length === 4 && args[0] === '--config' && args[2] === '--prepare-organization') {
  prepareOrganization(args[1], args[3]);
  console.log('Organization identity file prepared; no API calls performed');
 } else {
  if (args.length !== 2 || args[0] !== '--config') throw new SeedError('Usage: node scripts/bootstrap/seed-api.mjs --config /private/dev.yaml [--prepare-organization /private/organization.json]');
  await seed(loadConfig(args[1]));
 }
} catch (e) {
 console.error(e instanceof SeedError ? e.message : 'Seed failed; inspect the current step without logging credentials');
 process.exitCode = 1;
}
