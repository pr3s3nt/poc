import http from 'node:http';
import {readFile} from 'node:fs/promises';
const types={'/':'text/html; charset=utf-8','/index.html':'text/html; charset=utf-8','/app.js':'text/javascript; charset=utf-8','/style.css':'text/css; charset=utf-8','/catalog.json':'application/json; charset=utf-8'};
const port=Number(process.env.PORT||18090);
http.createServer(async(req,res)=>{const path=new URL(req.url,'http://localhost').pathname;if(!Object.hasOwn(types,path)){res.writeHead(404);res.end();return;}try{const body=await readFile(new URL('.'+(path==='/'?'/index.html':path),import.meta.url));res.writeHead(200,{'Content-Type':types[path],'Cache-Control':'no-store'});res.end(body);}catch{res.writeHead(500);res.end('Cannot read prototype asset');}}).listen(port,'127.0.0.1',()=>console.log(`Mock: http://127.0.0.1:${port}/#module-section`));
