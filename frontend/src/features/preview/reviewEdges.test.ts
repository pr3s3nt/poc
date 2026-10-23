import { expect, it } from 'vitest';
import { routeReviewEdges, reviewRowGap, type GraphPosition } from './reviewEdges';
import { reviewMetrics } from './reviewMetrics';

const positions = new Map<string, GraphPosition>([
  ['cluster', { x: 218, y: 58 }], ['namespace', { x: 218, y: 318 }],
  ['api', { x: 60, y: 578 }], ['web', { x: 376, y: 578 }],
]);
const edges = [
  { provider: 'cluster', consumer: 'namespace', reason: 'provider-binding' },
  { provider: 'cluster', consumer: 'api', reason: 'resource-reference' },
  { provider: 'cluster', consumer: 'web', reason: 'resource-reference' },
  { provider: 'namespace', consumer: 'api', reason: 'resource-reference' },
  { provider: 'namespace', consumer: 'web', reason: 'resource-reference' },
];
function crossesCard(a: GraphPosition, b: GraphPosition, card: GraphPosition): boolean {
  if (a.x === b.x) return a.x > card.x && a.x < card.x + reviewMetrics.nodeWidth && Math.max(a.y, b.y) > card.y && Math.min(a.y, b.y) < card.y + reviewMetrics.nodeHeight;
  return a.y > card.y && a.y < card.y + reviewMetrics.nodeHeight && Math.max(a.x, b.x) > card.x && Math.min(a.x, b.x) < card.x + reviewMetrics.nodeWidth;
}
it('routes direct Cluster dependencies around Namespace instead of through its card', () => {
  const connections = routeReviewEdges(edges, positions);
  expect(connections).toHaveLength(5);
  for (const connection of connections) {
    expect(connection.indirectLane).toBe(connection.provider === 'cluster' && connection.consumer !== 'namespace');
    for (let i = 1; i < connection.points.length; i++) {
      for (const [id, card] of positions) {
        if (id === connection.provider || id === connection.consumer) continue;
        expect(crossesCard(connection.points[i - 1]!, connection.points[i]!, card), `${connection.provider}→${connection.consumer} crosses ${id}`).toBe(false);
      }
    }
  }
  const api = connections.filter(connection => connection.consumer === 'api');
  expect(api[0]!.points.at(-1)!.x).not.toBe(api[1]!.points.at(-1)!.x);
});
it('draws one connection while preserving every distinct reason and source path', () => {
  const connections = routeReviewEdges([edges[1]!, { ...edges[1]!, reason: 'provider-binding', path: '/binding' }], positions);
  expect(connections).toHaveLength(1);
  expect(connections[0]!.reasons).toHaveLength(2);
  expect(connections[0]!.reasons[1]!.path).toBe('/binding');
});
it('avoids wider intervening rows even when the endpoints are centered', () => {
  const wider = new Map(positions);wider.set('api', { x: 185, y: 838 });
  const result = routeReviewEdges([edges[1]!], wider)[0]!;
  for (let i = 1; i < result.points.length; i++) {
    for (const id of ['namespace', 'web']) expect(crossesCard(result.points[i - 1]!, result.points[i]!, wider.get(id)!)).toBe(false);
  }
});

it('gives dense dependencies independent horizontal tracks and outer lanes', () => {
  const layers = [[{id:'cluster'}],[{id:'namespace'}],[{id:'db'},{id:'redis'},{id:'web'}],[{id:'api'},{id:'worker'}]];
  const graph = [
    ...['namespace','db','redis','web','api','worker'].map(consumer=>({provider:'cluster',consumer,reason:'provider-binding'})),
    ...['db','redis','web','api','worker'].map(consumer=>({provider:'namespace',consumer,reason:'resource-reference'})),
    ...['db','redis'].flatMap(provider=>['api','worker'].map(consumer=>({provider,consumer,reason:'workload-placeholder'}))),
  ];
  const gap = reviewRowGap(graph,layers), pitch=reviewMetrics.nodeHeight+gap;
  const positions = new Map(layers.flatMap((layer,row)=>layer.map((node,col)=>[node.id,{x:80+(3-layer.length)*158+col*316,y:50+row*pitch}] as const)));
  const routes = routeReviewEdges(graph,positions);
  expect(routes).toHaveLength(graph.length);
  for(const route of routes) for(let i=1;i<route.points.length;i++) for(const [id,card] of positions) {
    if(id!==route.provider&&id!==route.consumer) expect(crossesCard(route.points[i-1]!,route.points[i]!,card)).toBe(false);
  }
  const horizontal = routes.flatMap((route,index)=>route.points.slice(1).flatMap((b,i)=>{const a=route.points[i]!;return a.y===b.y&&a.x!==b.x?[{route:index,y:a.y,min:Math.min(a.x,b.x),max:Math.max(a.x,b.x)}]:[];}));
  for(const [i,a] of horizontal.entries()) for(const b of horizontal.slice(i+1)) {
    if(a.route!==b.route&&a.y===b.y) expect(Math.min(a.max,b.max)-Math.max(a.min,b.min)).toBeLessThanOrEqual(0);
  }
});

it('keeps vertical attachment stems separate across opposite ends of dense edges', () => {
  const layers=[[{id:'cluster'}],[{id:'namespace'}],[{id:'db'},{id:'redis'},{id:'web'}],[{id:'api'},{id:'worker'}]];
  const graph=[...['namespace','db','redis','web','api','worker'].map(consumer=>({provider:'cluster',consumer,reason:'provider-binding'})),...['db','redis','web','api','worker'].map(consumer=>({provider:'namespace',consumer,reason:'resource-reference'})),...['db','redis'].flatMap(provider=>['api','worker'].map(consumer=>({provider,consumer,reason:'workload-placeholder'})))];
  const pitch=reviewMetrics.nodeHeight+reviewRowGap(graph,layers);
  const positions=new Map(layers.flatMap((layer,row)=>layer.map((n,col)=>[n.id,{x:80+(3-layer.length)*158+col*316,y:50+row*pitch}] as const)));
  const vertical=routeReviewEdges(graph,positions).flatMap((r,index)=>r.points.slice(1).flatMap((b,i)=>{const a=r.points[i]!;return a.x===b.x&&a.y!==b.y?[{index,x:a.x,min:Math.min(a.y,b.y),max:Math.max(a.y,b.y)}]:[];}));
  for(const [i,a] of vertical.entries())for(const b of vertical.slice(i+1))if(a.index!==b.index&&a.x===b.x)expect(Math.min(a.max,b.max)-Math.max(a.min,b.min)).toBeLessThanOrEqual(0);
});
