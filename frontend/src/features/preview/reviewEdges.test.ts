import { expect, it } from 'vitest';
import { routeReviewEdges, type GraphPosition } from './reviewEdges';

const positions = new Map<string, GraphPosition>([
  ['cluster', { x: 185, y: 58 }], ['namespace', { x: 185, y: 196 }],
  ['api', { x: 60, y: 334 }], ['web', { x: 310, y: 334 }],
]);
const edges = [
  { provider: 'cluster', consumer: 'namespace', reason: 'provider-binding' },
  { provider: 'cluster', consumer: 'api', reason: 'resource-reference' },
  { provider: 'cluster', consumer: 'web', reason: 'resource-reference' },
  { provider: 'namespace', consumer: 'api', reason: 'resource-reference' },
  { provider: 'namespace', consumer: 'web', reason: 'resource-reference' },
];
function crossesCard(a: GraphPosition, b: GraphPosition, card: GraphPosition): boolean {
  if (a.x === b.x) return a.x > card.x && a.x < card.x + 238 && Math.max(a.y, b.y) > card.y && Math.min(a.y, b.y) < card.y + 100;
  return a.y > card.y && a.y < card.y + 100 && Math.max(a.x, b.x) > card.x && Math.min(a.x, b.x) < card.x + 238;
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
  const wider = new Map(positions);wider.set('api', { x: 185, y: 472 });
  const result = routeReviewEdges([edges[1]!], wider)[0]!;
  for (let i = 1; i < result.points.length; i++) {
    for (const id of ['namespace', 'web']) expect(crossesCard(result.points[i - 1]!, result.points[i]!, wider.get(id)!)).toBe(false);
  }
});
