import type { DeploymentReviewData } from './DeploymentReview';
import { reviewMetrics } from './reviewMetrics';

export type GraphPosition = { x: number; y: number };
export type GraphConnection = {
  provider: string; consumer: string;
  reasons: DeploymentReviewData['edges'];
  points: GraphPosition[];
  indirectLane: boolean;
};

function endpointPairs(edges: DeploymentReviewData['edges']) {
  const pairs = new Map<string, DeploymentReviewData['edges']>();
  for (const edge of edges) {
    const key = JSON.stringify([edge.provider, edge.consumer]);
    pairs.set(key, [...(pairs.get(key) ?? []), edge]);
  }
  return [...pairs.values()];
}

export function reviewRowGap(edges: DeploymentReviewData['edges'], layers: { id: string }[][]): number {
  const rows = new Map(layers.flatMap((layer, row) => layer.map(n => [n.id, row] as const)));
  const bands = new Map<number, number>();
  for (const [edge] of endpointPairs(edges)) {
    const from = rows.get(edge!.provider), to = rows.get(edge!.consumer);
    if (from === undefined || to === undefined) continue;
    bands.set(from, (bands.get(from) ?? 0) + 1);
    if (to !== from + 1) bands.set(to - 1, (bands.get(to - 1) ?? 0) + 1);
  }
  return Math.max(reviewMetrics.rowGap, ...[...bands.values()].map(n => n * reviewMetrics.laneGap + 36));
}

// One connector per endpoint pair, with every original reason retained. Long
// connectors use an outer lane so they cannot masquerade as a Namespace edge.
export function routeReviewEdges(edges: DeploymentReviewData['edges'], positions: ReadonlyMap<string, GraphPosition>): GraphConnection[] {
  const grouped = endpointPairs(edges).filter(([edge]) => positions.has(edge!.provider) && positions.has(edge!.consumer)).sort((a, b) => {
    const left = a[0]!, right = b[0]!;
    const lp = positions.get(left.provider)!, rp = positions.get(right.provider)!;
    return lp.y - rp.y || lp.x - rp.x || left.provider.localeCompare(right.provider) || left.consumer.localeCompare(right.consumer);
  });
  const result: GraphConnection[] = [];
  const { nodeWidth: width, nodeHeight: height, laneGap } = reviewMetrics;
  const levels = [...new Set([...positions.values()].map(p => p.y))].sort((a, b) => a - b);
  const tracks = new Map<number, string[]>();
  const keyOf = (edge: DeploymentReviewData['edges'][number]) => JSON.stringify([edge.provider, edge.consumer]);
  const indirect = (edge: DeploymentReviewData['edges'][number]) => levels.indexOf(positions.get(edge.consumer)!.y) !== levels.indexOf(positions.get(edge.provider)!.y) + 1;
  for (const [edge] of grouped) {
    const from = levels.indexOf(positions.get(edge!.provider)!.y), to = levels.indexOf(positions.get(edge!.consumer)!.y);
    tracks.set(from, [...(tracks.get(from) ?? []), keyOf(edge!) + ':start']);
    if (indirect(edge!)) tracks.set(to - 1, [...(tracks.get(to - 1) ?? []), keyOf(edge!) + ':end']);
  }
  const trackY = (row: number, key: string) => (levels[row] ?? levels[0] ?? 0) + height + 18 + (tracks.get(row)?.indexOf(key) ?? 0) * laneGap;
  const columns = new Map<number, { x: number; owner: string }[]>();
  const reservePort = (row: number, node: GraphPosition, wanted: number, owner: string) => {
    const occupied = columns.get(row) ?? [];
    const candidates = [wanted, ...Array.from({ length: 20 }, (_, i) => [wanted + (i + 1) * 12, wanted - (i + 1) * 12]).flat()].filter(x => x >= node.x + 12 && x <= node.x + width - 12);
    // Opposite ends of different edges must not occupy the same vertical stem
    // in a row gap, even when their cards happen to be aligned.
    const selected = candidates.find(x => occupied.every(p => p.owner === owner || Math.abs(p.x - x) >= 12)) ?? wanted;
    columns.set(row, [...occupied, { x: selected, owner }]);
    return selected;
  };
  let leftLanes = 0, rightLanes = 0;
  for (const reasons of grouped) {
    const edge = reasons[0]!;
    const provider = positions.get(edge.provider), consumer = positions.get(edge.consumer);
    if (!provider || !consumer) continue;
    const outgoing = grouped.filter(group => group[0]!.provider === edge.provider);
    const incoming = grouped.filter(group => group[0]!.consumer === edge.consumer);
    const start = { x: reservePort(levels.indexOf(provider.y), provider, provider.x + width * (outgoing.indexOf(reasons) + 1) / (outgoing.length + 1), keyOf(edge)), y: provider.y + height };
    const end = { x: reservePort(levels.indexOf(consumer.y) - 1, consumer, consumer.x + width * (incoming.indexOf(reasons) + 1) / (incoming.length + 1), keyOf(edge)), y: consumer.y - 6 };
    const indirectLane = indirect(edge);
    const departure = trackY(levels.indexOf(provider.y), keyOf(edge) + ':start');
    let points: GraphPosition[];
    if (indirectLane) {
      const left = consumer.x < provider.x;
      const laneOffset = 24 + (left ? leftLanes++ : rightLanes++) * laneGap;
      const lane = left ? Math.min(...[...positions.values()].map(p => p.x)) - laneOffset : Math.max(...[...positions.values()].map(p => p.x + width)) + laneOffset;
      const arrival = trackY(levels.indexOf(consumer.y) - 1, keyOf(edge) + ':end');
      points = [start, { x: start.x, y: departure }, { x: lane, y: departure }, { x: lane, y: arrival }, { x: end.x, y: arrival }, end];
    } else {
      points = [start, { x: start.x, y: departure }, { x: end.x, y: departure }, end];
    }
    result.push({ provider: edge.provider, consumer: edge.consumer, reasons, points, indirectLane });
  }
  return result;
}

export function connectionPath(connection: GraphConnection): string {
  return connection.points.map((point, index) => `${index ? 'L' : 'M'}${point.x},${point.y}`).join(' ');
}
