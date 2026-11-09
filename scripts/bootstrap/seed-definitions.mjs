#!/usr/bin/env node
import { loadDefinitionConfig, seedDefinitions } from './lib/definition-seed.mjs';
import { SeedError } from './lib/api-seed.mjs';

try {
 const args = process.argv.slice(2);
 if (args.length !== 2 || args[0] !== '--config') throw new SeedError('Usage: node scripts/bootstrap/seed-definitions.mjs --config /private/definitions.yaml');
 await seedDefinitions(loadDefinitionConfig(args[1]));
} catch (error) {
 console.error(error instanceof SeedError ? error.message : 'Definition seed failed; inspect server state without logging credentials');
 process.exitCode = 1;
}
