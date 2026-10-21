// Standalone review server. Not imported by the product or its build.
import { createServer } from 'node:http';
import { readFileSync } from 'node:fs';
const routes = new Map([
  ['/', ['index.html', 'text/html; charset=utf-8']],
  ['/index.html', ['index.html', 'text/html; charset=utf-8']],
  ['/advanced.js', ['advanced.js', 'text/javascript; charset=utf-8']],
  ['/configuration.png', ['configuration.png', 'image/png']],
  ['/matching.png', ['matching.png', 'image/png']],
  ['/advanced.png', ['advanced.png', 'image/png']],
]);
const host = process.env.HOST || '127.0.0.1';
const port = Number(process.env.PORT || 18089);
createServer((request, response) => {
  const route = routes.get(new URL(request.url, 'http://localhost').pathname);
  if (!route || !['GET', 'HEAD'].includes(request.method)) {
    response.writeHead(404); response.end('Not found'); return;
  }
  response.writeHead(200, { 'Content-Type': route[1], 'Cache-Control': 'no-store' });
  response.end(request.method === 'HEAD' ? undefined : readFileSync(new URL(route[0], import.meta.url)));
}).listen(port, host, () => process.stdout.write(`v3 prototype: http://${host}:${port}/\n`));
