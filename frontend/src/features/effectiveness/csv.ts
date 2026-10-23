import type { Report, WorkRecord } from './api';
import { actionLabel, metricName, requiredSourceLabel, stateLabel } from './labels';

// Formats the server report and its records; nothing is recomputed here.

const cell = (value: string | number | undefined | null) => `"${String(value ?? '').replaceAll('"', '""')}"`;
const row = (...values: (string | number | undefined | null)[]) => values.map(cell).join(',');

const statusText: Readonly<Record<string, string>> = { COLLECTED: 'Có dữ liệu', NO_DATA: 'Chưa có dữ liệu', NOT_COLLECTED: 'Chưa thu thập' };
const unitText: Readonly<Record<string, string>> = { COUNT: 'số lượng', RATIO: 'tỷ lệ 0..1', SECONDS: 'giây' };
const sourceText: Readonly<Record<string, string>> = { deployments: 'Lịch sử triển khai', resource_cleanups: 'Xóa tài nguyên' };
const lookup = (table: Readonly<Record<string, string>>, key: string) => (Object.hasOwn(table, key) ? table[key] : key);

export function scopeLabel(report: Report): string {
  if (report.scope.level === 'ORGANIZATION') return 'Toàn tổ chức';
  const app = report.scope.applicationName || report.scope.applicationKey || '';
  return report.scope.level === 'WORKLOAD' ? `${app} › ${report.scope.workloadId ?? ''}` : app;
}

export function buildCsv(report: Report, records: WorkRecord[]): string {
  const lines = [
    row('Báo cáo hiệu quả platform'),
    row('Phạm vi', scopeLabel(report)),
    row('Kỳ', `${report.period.from} — ${report.period.to}`, 'Múi giờ', report.period.timezone),
    row('Nguồn dữ liệu', report.dataOrigin === 'PRODUCTION' ? 'Production' : 'Dữ liệu kiểm thử (bản cài đặt không phải production)'),
    row('Tạo lúc', report.generatedAt),
    row('Nguồn', ...report.sources.map((source) => `${lookup(sourceText, source.id)}: ${source.available ? 'có' : 'không khả dụng'}`)),
    '',
    row('ID chỉ số', 'Chỉ số', 'Trạng thái', 'Giá trị', 'Đơn vị', 'Tử số', 'Mẫu số', 'Độ phủ', 'Nguồn cần có'),
    ...report.metrics.map((m) => row(m.id, metricName(m.id), lookup(statusText, m.status), m.value, lookup(unitText, m.unit), m.numerator, m.denominator,
      m.coverage ? `${m.coverage.covered}/${m.coverage.total}` : '', requiredSourceLabel(m.requiredSource))),
  ];
  if (report.baseline) {
    const b = report.baseline;
    lines.push('', row('Mốc so sánh', `phiên bản ${b.version}`, `${b.valueSeconds ?? ''} giây (trung vị)`, `áp dụng từ ${b.effectiveFrom}`, b.source));
  }
  lines.push('', row('ID công việc', 'Ứng dụng', 'Môi trường', 'Workload', 'Tài nguyên', 'Loại', 'Trạng thái', 'Lần thử', 'Lần thất bại', 'Lần thử trong kỳ',
    'Bắt đầu', 'Hoàn thành', 'Thời gian thực hiện (giây)', 'Người thực hiện', 'Lượt cuối'));
  for (const r of records) {
    lines.push(row(r.id, r.applicationName || r.applicationKey, r.environmentKey, r.workloadId, r.resourceId, actionLabel(r.action), stateLabel(r.state),
      r.attempts, r.failedAttempts, r.attemptsInPeriod, r.firstStartedAt, r.completedAt, r.durationSeconds,
      r.actors.map((a) => (a.kind === 'USER' ? a.name : `${a.name} (tự động)`)).join('; '), r.lastRunId));
  }
  return `\uFEFF${lines.join('\r\n')}\r\n`;
}

export function csvFileName(report: Report): string {
  const scope = report.scope.level === 'ORGANIZATION' ? 'to-chuc' : [report.scope.applicationKey, report.scope.workloadId].filter(Boolean).join('-');
  return `hieu-qua-platform-${scope.replace(/[^A-Za-z0-9._-]+/g, '-')}-${report.period.from}-${report.period.to}.csv`;
}
