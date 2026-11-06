#!/usr/bin/env node
import { loadConfig, seed, SeedError } from './lib/api-seed.mjs';
try {
 const args = process.argv.slice(2);
 if (args.length !== 2 || args[0] !== '--config') throw new SeedError('Usage: node scripts/bootstrap/seed-api.mjs --config /private/dev.yaml');
 await seed(loadConfig(args[1]));
} catch (e) {
 console.error(e instanceof SeedError ? e.message : 'Seed failed; inspect the current step without logging credentials');
 process.exitCode = 1;
}
