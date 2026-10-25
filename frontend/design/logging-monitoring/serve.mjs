/* global process, console, URL */
import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import { resolve, extname, sep } from 'node:path';

// Serves only a built prototype directory, never the repository or an API.
const root = resolve(process.argv[2] || '/tmp/orchestrator-logging-monitoring-dist');
const host = process.argv[3] || '127.0.0.1';
const port = Number(process.argv[4] || 18101);
const types = { '.html': 'text/html; charset=utf-8', '.js': 'text/javascript; charset=utf-8', '.css': 'text/css; charset=utf-8' };
createServer(async (request, response) => {
  if (!['GET', 'HEAD'].includes(request.method)) {
    response.writeHead(405); response.end(); return;
  }
  try {
    const pathname = decodeURIComponent(new URL(request.url, 'http://localhost').pathname);
    const target = resolve(root, '.' + (pathname === '/' ? '/index.html' : pathname));
    if (!target.startsWith(root + sep) || !Object.hasOwn(types, extname(target))) {
      response.writeHead(404); response.end(); return;
    }
    const body = await readFile(target);
    response.writeHead(200, { 'Content-Type': types[extname(target)], 'Cache-Control': 'no-store', 'X-Content-Type-Options': 'nosniff', 'Referrer-Policy': 'no-referrer' });
    response.end(request.method === 'HEAD' ? undefined : body);
  } catch {
    response.writeHead(404); response.end();
  }
}).listen(port, host, () => console.log(`Mock UI: http://${host}:${port}`));
