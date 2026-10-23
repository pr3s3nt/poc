import type { DeploymentReviewData } from './DeploymentReview';

export type GraphPosition = { x: number; y: number };
export type GraphConnection = {
  provider: string; consumer: string;
  reasons: DeploymentReviewData['edges'];
  points: GraphPosition[];
  indirectLane: boolean;
};

// One connector per endpoint pair, with every original reason retained. Long
// connectors use an outer lane so they cannot masquerade as a Namespace edge.
export function routeReviewEdges(edges: DeploymentReviewData['edges'], positions: ReadonlyMap<string, GraphPosition>): GraphConnection[] {
  const pairs = new Map<string, DeploymentReviewData['edges']>();
  for (const edge of edges) {
    const key = JSON.stringify([edge.provider, edge.consumer]);
    pairs.set(key, [...(pairs.get(key) ?? []), edge]);
  }
  const grouped = [...pairs.values()].sort((a, b) => {
    const left = a[0]!, right = b[0]!;
    return left.provider.localeCompare(right.provider) || left.consumer.localeCompare(right.consumer);
  });
  const result: GraphConnection[] = [];
  const width = 238, height = 100;
  for (const reasons of grouped) {
    const edge = reasons[0]!;
    const provider = positions.get(edge.provider), consumer = positions.get(edge.consumer);
    if (!provider || !consumer) continue;
    const outgoing = grouped.filter(group => group[0]!.provider === edge.provider);
    const incoming = grouped.filter(group => group[0]!.consumer === edge.consumer);
    const start = { x: provider.x + width * (outgoing.indexOf(reasons) + 1) / (outgoing.length + 1), y: provider.y + height };
    const end = { x: consumer.x + width * (incoming.indexOf(reasons) + 1) / (incoming.length + 1), y: consumer.y - 4 };
    const indirectLane = consumer.y - provider.y > 160 || consumer.y <= provider.y;
    let points: GraphPosition[];
    if (indirectLane) {
      const between = [...positions.entries()].filter(([id, p]) => id !== edge.provider && id !== edge.consumer && p.y + height > Math.min(start.y, end.y) && p.y < Math.max(start.y, end.y)).map(([, p]) => p);
      const left = consumer.x < provider.x;
      const laneOffset = 18 + (result.filter(item => item.indirectLane).length % 3) * 6;
      const lane = left ? Math.min(provider.x, consumer.x, ...between.map(p => p.x)) - laneOffset : Math.max(provider.x + width, consumer.x + width, ...between.map(p => p.x + width)) + laneOffset;
      points = [start, { x: start.x, y: start.y + 12 }, { x: lane, y: start.y + 12 }, { x: lane, y: end.y - 10 }, { x: end.x, y: end.y - 10 }, end];
    } else {
      const middle = (start.y + end.y) / 2;
      points = [start, { x: start.x, y: middle }, { x: end.x, y: middle }, end];
    }
    result.push({ provider: edge.provider, consumer: edge.consumer, reasons, points, indirectLane });
  }
  return result;
}

export function connectionPath(connection: GraphConnection): string {
  return connection.points.map((point, index) => `${index ? 'L' : 'M'}${point.x},${point.y}`).join(' ');
}
