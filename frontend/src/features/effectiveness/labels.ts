import type { FrequencyBand, Metric } from './api';

// Vietnamese copy of the UC-17 catalog. Unknown IDs from a newer server still render with their ID.
type MetricCopy = { name: string; term?: string; formula: string };

export const metricCopy: Readonly<Record<string, MetricCopy>> = {
  deployment_frequency: { name: 'Tần suất triển khai', term: 'Deployment frequency', formula: 'Số lần triển khai hoặc cập nhật hoàn thành trên production trong kỳ. Các lần thử lại của cùng thay đổi tính một lần; gỡ bỏ workload không tính.' },
  change_lead_time: { name: 'Commit đến production', term: 'Change lead time', formula: 'Thời gian từ commit đến khi thay đổi chạy thành công trên production.' },
  change_fail_rate: { name: 'Thay đổi gây lỗi', term: 'Change fail rate', formula: 'Tỷ lệ deployment production cần can thiệp ngay (rollback, hotfix). Không dùng trạng thái Thất bại của lượt chạy thay thế.' },
  failed_deployment_recovery_time: { name: 'Thời gian khôi phục', term: 'Failed deployment recovery time', formula: 'Thời gian từ khi deployment gây lỗi được phát hiện đến khi khôi phục.' },
  deployment_rework_rate: { name: 'Triển khai khắc phục', term: 'Deployment rework rate', formula: 'Tỷ lệ deployment ngoài kế hoạch để khắc phục sự cố production. Không đồng nhất với thử lại.' },
  active_applications: { name: 'Ứng dụng hoạt động', formula: 'Ứng dụng có ít nhất một công việc hoàn thành trong kỳ, ở bất kỳ môi trường nào, trên số ứng dụng hiện có.' },
  active_users: { name: 'Người dùng hoạt động', formula: 'Số tài khoản khác nhau đã bắt đầu ít nhất một lượt chạy trong kỳ. Mỗi người tính một lần dù làm ở nhiều ứng dụng; lượt do CI hoặc hệ thống chạy đếm riêng.' },
  completed_work: { name: 'Công việc hoàn thành', formula: 'Số công việc triển khai, cập nhật, gỡ bỏ và xóa tài nguyên hoàn thành trong kỳ. Các lần thử lại của cùng công việc tính là một.' },
  execution_attempts: { name: 'Lượt chạy', formula: 'Số lượt chạy bắt đầu trong kỳ, gồm lượt thất bại và đang chạy. Lần thử mỗi công việc = tổng lượt của các công việc hoàn thành ÷ số công việc hoàn thành.' },
  work_duration: { name: 'Thời gian thực hiện', formula: 'Trung vị thời gian từ lượt chạy đầu tiên đến khi hoàn thành, của các công việc hoàn thành trong kỳ. Không gồm thời gian chờ trước khi bắt đầu và không phải giờ công.' },
  work_duration_vs_baseline: { name: 'Thời gian rút ngắn', formula: 'Mốc so sánh trừ trung vị thời gian thực hiện của cùng phạm vi. Số âm là chậm hơn mốc. Đây là thời gian quy trình, không phải giờ công.' },
  request_lead_time: { name: 'Yêu cầu đến hoàn thành', formula: 'Thời gian từ khi có yêu cầu đến khi hoàn thành.' },
  self_service_rate: { name: 'Tự phục vụ', formula: 'Tỷ lệ công việc hoàn thành không cần Ops hỗ trợ, trên các công việc đã ghi nhận hỗ trợ.' },
  ops_effort: { name: 'Công sức Ops', formula: 'Giờ công Ops hỗ trợ và bảo trì.' },
  onboarding_time: { name: 'Onboarding', formula: 'Thời gian từ khi bắt đầu onboarding đến lần triển khai production thành công đầu tiên.' },
  labor_hours_saved: { name: 'Giờ công tiết kiệm', formula: 'Công sức theo mốc so sánh trừ công sức qua platform cho cùng loại công việc.' },
};

export const requiredSourceLabels: Readonly<Record<string, string>> = {
  COMMIT_LINK: 'Cần liên kết deployment với commit và thời điểm commit.',
  INCIDENTS: 'Cần dữ liệu sự cố sau triển khai: deployment gây lỗi, thời điểm phát hiện và khôi phục.',
  REQUESTS: 'Cần thời điểm tạo yêu cầu (ticket hoặc issue).',
  SUPPORT_RECORDS: 'Cần bản ghi hỗ trợ cho từng công việc. Thiếu phản hồi không được tính là tự phục vụ.',
  EFFORT_RECORDS: 'Cần bản ghi công sức, cả theo mốc so sánh và qua platform.',
  ONBOARDING_START: 'Cần mốc bắt đầu onboarding của ứng dụng.',
};

const own = <T,>(table: Readonly<Record<string, T>>, key: string): T | undefined => (Object.hasOwn(table, key) ? table[key] : undefined);

export const metricName = (id: string) => own(metricCopy, id)?.name ?? id;
export const metricFormula = (id: string) => own(metricCopy, id)?.formula ?? '';
export const metricTerm = (id: string) => own(metricCopy, id)?.term;
export const requiredSourceLabel = (source?: string) => (source ? own(requiredSourceLabels, source) ?? '' : '');

export const bandLabels: Readonly<Record<FrequencyBand, string>> = { DAILY: 'Hằng ngày', WEEKLY: 'Hằng tuần', MONTHLY: 'Hằng tháng', RARE: 'Thưa hơn', NONE: 'Không có' };
export const bandDetailKeys: Readonly<Record<FrequencyBand, string>> = { DAILY: 'bandDaily', WEEKLY: 'bandWeekly', MONTHLY: 'bandMonthly', RARE: 'bandRare', NONE: 'bandNone' };

export const actionLabels: Readonly<Record<string, string>> = { DEPLOY: 'Triển khai', UPDATE: 'Cập nhật', REMOVE: 'Gỡ bỏ', RESOURCE_CLEANUP: 'Xóa tài nguyên' };
export const stateLabels: Readonly<Record<string, string>> = { COMPLETED: 'Hoàn thành', RUNNING: 'Đang chạy', FAILED_OPEN: 'Thất bại, chưa xử lý' };
export const actionLabel = (action: string) => own(actionLabels, action) ?? action;
export const stateLabel = (state: string) => own(stateLabels, state) ?? state;

const number = new Intl.NumberFormat('vi-VN', { maximumFractionDigits: 1 });
export const formatNumber = (value: number) => number.format(value);

/** Elapsed time in the largest readable unit; the sign is kept. */
export function formatDuration(seconds: number): string {
  const sign = seconds < 0 ? '−' : '';
  const abs = Math.abs(seconds);
  if (abs < 60) return `${sign}${formatNumber(abs)} giây`;
  if (abs < 3600) return `${sign}${formatNumber(abs / 60)} phút`;
  if (abs < 86400 * 2) return `${sign}${formatNumber(abs / 3600)} giờ`;
  return `${sign}${formatNumber(abs / 86400)} ngày`;
}

export const formatDate = (date: string) => {
  const [y, m, d] = date.split('-');
  return y && m && d ? `${d}/${m}/${y}` : date;
};

export const formatDateTime = (value?: string) => {
  if (!value) return '—';
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? '—' : parsed.toLocaleString('vi-VN', { timeZone: 'Asia/Ho_Chi_Minh' });
};

/** The value shown on a metric card; missing data is never shown as zero. */
export function metricValue(metric: Metric): string {
  if (metric.status === 'NOT_COLLECTED') return 'Chưa thu thập';
  if (metric.status === 'NO_DATA' || metric.value === undefined) return 'Chưa có dữ liệu';
  if (metric.unit === 'SECONDS') return formatDuration(metric.value);
  if (metric.unit === 'RATIO') return `${formatNumber(metric.value * 100)}%`;
  if (metric.numerator !== undefined && metric.denominator !== undefined && metric.id === 'active_applications') return `${formatNumber(metric.numerator)}/${formatNumber(metric.denominator)}`;
  return formatNumber(metric.value);
}

/** The short secondary line of a metric card, if any. */
export function metricNote(metric: Metric): { text: string; warning?: boolean } | undefined {
  if (metric.status !== 'COLLECTED') return undefined;
  const d = metric.details ?? {};
  switch (metric.id) {
    case 'deployment_frequency': return { text: `${formatNumber(d.perWeek ?? 0)} lần/tuần · production` };
    case 'active_users': return d.automationRuns ? { text: `+${formatNumber(d.automationRuns)} lượt tự động` } : undefined;
    case 'completed_work': return d.openWork ? { text: `Đang mở: ${formatNumber(d.openWork)}` } : undefined;
    case 'execution_attempts': return { text: `${d.attemptsPerItem !== undefined ? `${formatNumber(d.attemptsPerItem)} lần/công việc · ` : ''}${formatNumber(d.failedRuns ?? 0)} thất bại` };
    case 'work_duration': {
      const coverage = metric.coverage;
      if (coverage && coverage.covered < coverage.total) return { text: `Độ phủ ${coverage.covered}/${coverage.total}`, warning: true };
      return { text: `Trung vị · p90 ${formatDuration(d.p90Seconds ?? 0)}` };
    }
    default: return undefined;
  }
}
