/* global process, console, URL */
import http from "node:http";
import { readFile } from "node:fs/promises";
import { resolve, extname, sep } from "node:path";
const root = resolve(process.argv[2] || "/tmp/platform-effectiveness-dist");
const host = process.argv[3] || "127.0.0.1";
const port = Number(process.argv[4] || 18097);
const types = {
  ".html": "text/html; charset=utf-8",
  ".js": "application/javascript; charset=utf-8",
  ".css": "text/css; charset=utf-8",
};
http
  .createServer(async (req, res) => {
    if (req.method !== "GET" && req.method !== "HEAD") {
      res.writeHead(405);
      res.end();
      return;
    }
    try {
      const pathname = decodeURIComponent(
        new URL(req.url, "http://localhost").pathname,
      );
      const file = resolve(
        root,
        "." + (pathname === "/" ? "/index.html" : pathname),
      );
      if (!file.startsWith(root + sep) || !types[extname(file)]) {
        res.writeHead(404);
        res.end();
        return;
      }
      const body = await readFile(file);
      res.writeHead(200, {
        "Content-Type": types[extname(file)],
        "Cache-Control": "no-store",
        "X-Content-Type-Options": "nosniff",
        "Referrer-Policy": "no-referrer",
      });
      res.end(req.method === "HEAD" ? undefined : body);
    } catch {
      res.writeHead(404);
      res.end();
    }
  })
  .listen(port, host, () => console.log(`Mock UI: http://${host}:${port}`));
