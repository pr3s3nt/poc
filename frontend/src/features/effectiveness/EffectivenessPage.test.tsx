import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Application } from '../../shared/types/application';
import { apiError, stubApi } from '../../test/api';
import { bothConfigured } from '../../test/targets';
import type { Metric, Report, WorkRecord } from './api';
import { addDays, periodOf } from './api';
import { buildCsv, csvFileName } from './csv';
import { EffectivenessPage } from './EffectivenessPage';
import { formatDuration, metricValue } from './labels';

const applications: Application[] = [
  { id: 'shop', name: 'Shop', subdomain: 'shop', environments: bothConfigured(), workloads: { staging: [], production: [] } },
  { id: 'blog', name: 'Blog', subdomain: 'blog', environments: bothConfigured(), workloads: { staging: [], production: [] } },
];

const notCollected = (id: string, group: Metric['group'], requiredSource: string): Metric => ({ id, group, unit: 'SECONDS', status: 'NOT_COLLECTED', requiredSource });

function report(overrides: Partial<Report> = {}): Report {
  return {
    scope: { level: 'ORGANIZATION' }, period: { from: '2026-09-24', to: '2026-10-23', timezone: 'Asia/Ho_Chi_Minh', days: 30 },
    dataOrigin: 'TEST_INSTALLATION', generatedAt: '2026-10-23T05:00:00Z',
    sources: [{ id: 'deployments', available: true }, { id: 'resource_cleanups', available: false }],
    metrics: [
      { id: 'deployment_frequency', group: 'DORA', unit: 'COUNT', status: 'COLLECTED', value: 12, numerator: 12, denominator: 30, details: { perWeek: 2.8, bandDaily: 0, bandWeekly: 1, bandMonthly: 0, bandRare: 0, bandNone: 1 } },
      notCollected('change_lead_time', 'DORA', 'COMMIT_LINK'),
      { id: 'change_fail_rate', group: 'DORA', unit: 'RATIO', status: 'NOT_COLLECTED', requiredSource: 'INCIDENTS' },
      notCollected('failed_deployment_recovery_time', 'DORA', 'INCIDENTS'),
      { id: 'deployment_rework_rate', group: 'DORA', unit: 'RATIO', status: 'NOT_COLLECTED', requiredSource: 'INCIDENTS' },
      { id: 'active_applications', group: 'USAGE', unit: 'COUNT', status: 'COLLECTED', value: 1, numerator: 1, denominator: 2 },
      { id: 'active_users', group: 'USAGE', unit: 'COUNT', status: 'COLLECTED', value: 3, details: { automationRuns: 4 } },
      { id: 'completed_work', group: 'USAGE', unit: 'COUNT', status: 'COLLECTED', value: 15, details: { openWork: 2 } },
      { id: 'execution_attempts', group: 'USAGE', unit: 'COUNT', status: 'COLLECTED', value: 21, numerator: 18, denominator: 15, details: { failedRuns: 4, attemptsPerItem: 1.2 } },
      { id: 'work_duration', group: 'SELF_SERVICE', unit: 'SECONDS', status: 'COLLECTED', value: 1500, coverage: { covered: 14, total: 15 }, details: { p90Seconds: 3600 } },
      { id: 'work_duration_vs_baseline', group: 'VALUE', unit: 'SECONDS', status: 'NO_DATA' },
      notCollected('request_lead_time', 'SELF_SERVICE', 'REQUESTS'),
      { id: 'self_service_rate', group: 'SELF_SERVICE', unit: 'RATIO', status: 'NOT_COLLECTED', requiredSource: 'SUPPORT_RECORDS' },
      notCollected('ops_effort', 'SELF_SERVICE', 'EFFORT_RECORDS'),
      notCollected('onboarding_time', 'SELF_SERVICE', 'ONBOARDING_START'),
      notCollected('labor_hours_saved', 'VALUE', 'EFFORT_RECORDS'),
    ],
    breakdown: { level: 'APPLICATION', rows: [
      { key: 'blog', name: 'Blog', productionDeployments: 0, frequencyBand: 'NONE', completedWork: 0, openWork: 0, attempts: 0, failedAttempts: 0, medianDurationSeconds: null, activeUsers: 0 },
      { key: 'shop', name: 'Shop', productionDeployments: 12, frequencyBand: 'WEEKLY', completedWork: 15, openWork: 2, attempts: 21, failedAttempts: 4, medianDurationSeconds: 1500, activeUsers: 3 },
    ] },
    trend: { bucket: 'WEEK', points: [{ start: '2026-09-21', productionDeployments: 2, completedWork: 3 }, { start: '2026-09-28', productionDeployments: 10, completedWork: 12 }] },
    baseline: null,
    ...overrides,
  };
}

const record: WorkRecord = {
  id: 'd1', kind: 'DEPLOYMENT', applicationKey: 'shop', applicationName: 'Shop', environmentKey: 'production', workloadId: 'api', action: 'UPDATE',
  state: 'COMPLETED', attempts: 3, failedAttempts: 2, attemptsInPeriod: 3, firstStartedAt: '2026-10-10T03:00:00Z', completedAt: '2026-10-10T03:40:00Z',
  durationSeconds: 2400, actors: [{ name: 'platform-engineer', kind: 'USER' }, { name: 'ci-pipeline', kind: 'AUTOMATION' }], lastRunId: 'd3',
};

const json = (body: unknown, status = 200) => Response.json(body, { status });

afterEach(() => { vi.unstubAllGlobals(); window.history.replaceState({}, '', '/'); });

describe('EffectivenessPage', () => {
  it('opens on the organization with collected and not-collected metrics, never zero for missing data', async () => {
    const calls = stubApi((call) => (call.url.startsWith('/api/v1/effectiveness/report') ? json(report()) : undefined));
    render(<EffectivenessPage applications={applications} />);
    expect(screen.getByRole('status')).toHaveTextContent('Đang tải báo cáo…');
    expect(await screen.findByRole('button', { name: 'Tần suất triển khai: 12. Xem chi tiết' })).toBeInTheDocument();
    const { from, to } = periodOf(30);
    expect(calls[0]?.url).toBe(`/api/v1/effectiveness/report?from=${from}&to=${to}`);
    expect(screen.getByRole('heading', { name: 'Hiệu quả platform', level: 1 })).toBeInTheDocument();
    expect(screen.getByText('Dữ liệu kiểm thử')).toBeInTheDocument();
    expect(screen.queryByRole('tab')).toBeNull();
    expect(screen.queryByText('Dữ liệu', { selector: 'button' })).toBeNull();
    expect(screen.getByRole('button', { name: 'Commit đến production: Chưa thu thập. Xem chi tiết' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Ứng dụng hoạt động: 1/2. Xem chi tiết' })).toBeInTheDocument();
    expect(screen.getByText('Độ phủ 14/15')).toBeInTheDocument();
    expect(screen.getByText('Chưa có mốc so sánh')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Tự phục vụ: Chưa thu thập. Xem chi tiết' })).toBeInTheDocument();
    const table = screen.getByRole('table');
    expect(within(table).getByRole('button', { name: 'Shop' })).toBeInTheDocument();
    expect(within(table).getByText('Hằng tuần')).toBeInTheDocument();
    expect(within(table).getByText('Chưa có dữ liệu')).toBeInTheDocument();
    expect(screen.getByText('Không có', { selector: 'span' })).toBeInTheDocument();
  });

  it('drills down to an application through the URL and back to the organization', async () => {
    window.history.replaceState({}, '', '/ui/platform/effectiveness');
    stubApi((call) => (call.url.startsWith('/api/v1/effectiveness/report') ? json(report()) : undefined));
    render(<EffectivenessPage applications={applications} />);
    await userEvent.setup().click(await screen.findByRole('button', { name: 'Shop' }));
    expect(window.location.pathname + window.location.search).toBe('/ui/platform/effectiveness?application=shop');
  });

  it('shows the application scope with workload breakdown and a locked baseline at workload level', async () => {
    stubApi((call) => {
      if (!call.url.startsWith('/api/v1/effectiveness/report')) return undefined;
      return json(report({ scope: { level: 'WORKLOAD', applicationKey: 'shop', applicationName: 'Shop', workloadId: 'api' }, breakdown: null }));
    });
    const user = userEvent.setup();
    render(<EffectivenessPage applications={applications} application="shop" workload="api" />);
    expect(await screen.findByRole('navigation', { name: 'Phạm vi' })).toHaveTextContent('Toàn tổ chức›Shop›api');
    expect(screen.getByText('Mốc so sánh áp dụng cho tổ chức hoặc ứng dụng')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Mốc so sánh' }));
    expect(within(screen.getByRole('dialog')).getByText('Mốc so sánh áp dụng cho tổ chức hoặc ứng dụng.')).toBeInTheDocument();
  });

  it('opens metric evidence with the formula and links records to the deployment history', async () => {
    window.history.replaceState({}, '', '/ui/platform/effectiveness');
    const calls = stubApi((call) => {
      if (call.url.startsWith('/api/v1/effectiveness/report')) return json(report());
      if (call.url.startsWith('/api/v1/effectiveness/records')) return json({ records: [record] });
      return undefined;
    });
    const user = userEvent.setup();
    render(<EffectivenessPage applications={applications} />);
    await user.click(await screen.findByRole('button', { name: 'Tần suất triển khai: 12. Xem chi tiết' }));
    const dialog = screen.getByRole('dialog', { name: 'Tần suất triển khai' });
    expect(dialog).toHaveTextContent('Các lần thử lại của cùng thay đổi tính một lần');
    expect(dialog).toHaveTextContent('12 lượt trong 30 ngày');
    expect(await within(dialog).findByText('api')).toBeInTheDocument();
    expect(calls.some((call) => call.url.includes('/effectiveness/records?') && call.url.includes('metric=deployment_frequency'))).toBe(true);
    expect(within(dialog).getByText('platform-engineer, ci-pipeline (tự động)')).toBeInTheDocument();
    await user.click(within(dialog).getByRole('button', { name: 'Mở api' }));
    expect(window.location.pathname).toBe('/ui/applications/shop/environments/production/deployments/d3');
  });

  it('shows the required source for a metric that is not collected, without loading records', async () => {
    const calls = stubApi((call) => (call.url.startsWith('/api/v1/effectiveness/report') ? json(report()) : undefined));
    const user = userEvent.setup();
    render(<EffectivenessPage applications={applications} />);
    await user.click(await screen.findByRole('button', { name: 'Commit đến production: Chưa thu thập. Xem chi tiết' }));
    expect(screen.getByRole('dialog')).toHaveTextContent('Cần liên kết deployment với commit và thời điểm commit.');
    await user.keyboard('{Escape}');
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(calls.some((call) => call.url.includes('/records'))).toBe(false);
  });

  it('saves a baseline version with the expected version and reports a stale conflict', async () => {
    let posts = 0;
    const calls = stubApi((call) => {
      if (call.url.startsWith('/api/v1/effectiveness/report')) return json(report());
      if (call.url === '/api/v1/effectiveness/baselines' && call.method === 'GET') return json({ currentVersion: 2, baselines: [] });
      if (call.url === '/api/v1/effectiveness/baselines' && call.method === 'POST') return ++posts === 1 ? apiError(409, 'STALE_VERSION') : json({}, 201);
      return undefined;
    });
    const user = userEvent.setup();
    render(<EffectivenessPage applications={applications} />);
    await user.click(await screen.findByRole('button', { name: 'Thêm mốc so sánh' }));
    const dialog = screen.getByRole('dialog', { name: 'Mốc so sánh' });
    await user.type(await within(dialog).findByLabelText('Thời gian thực hiện trung vị (phút)'), '45');
    await user.type(within(dialog).getByLabelText('Nguồn đo'), 'Đo tay 10 yêu cầu');
    await user.click(within(dialog).getByRole('button', { name: 'Lưu phiên bản mới' }));
    expect(await within(dialog).findByText(/Mốc so sánh vừa được thay đổi/)).toBeInTheDocument();
    const post = calls.find((call) => call.method === 'POST');
    expect(post?.body).toMatchObject({ metric: 'work_duration', expectedVersion: 2, valueSeconds: 2700, source: 'Đo tay 10 yêu cầu' });
    expect(post?.body).not.toHaveProperty('applicationKey');
    await user.click(within(dialog).getByRole('button', { name: 'Lưu phiên bản mới' }));
    expect(await screen.findByText('Đã lưu mốc so sánh.')).toBeInTheDocument();
    await waitFor(() => expect(calls.filter((call) => call.url.startsWith('/api/v1/effectiveness/report'))).toHaveLength(2));
  });

  it('shows a permission error without data', async () => {
    stubApi(() => apiError(403, 'FORBIDDEN'));
    render(<EffectivenessPage applications={applications} />);
    expect(await screen.findByRole('alert')).toHaveTextContent('Bạn không có quyền xem báo cáo này.');
    expect(screen.queryByRole('button', { name: /Xem chi tiết/ })).toBeNull();
  });
});

describe('effectiveness formatting and CSV', () => {
  it('formats missing data and durations without inventing zeros', () => {
    expect(metricValue({ id: 'work_duration', group: 'SELF_SERVICE', unit: 'SECONDS', status: 'NO_DATA' })).toBe('Chưa có dữ liệu');
    expect(metricValue({ id: 'change_fail_rate', group: 'DORA', unit: 'RATIO', status: 'NOT_COLLECTED' })).toBe('Chưa thu thập');
    expect(formatDuration(-600)).toBe('−10 phút');
    expect(formatDuration(5400)).toBe('1,5 giờ');
    expect(addDays('2026-03-01', -1)).toBe('2026-02-28');
  });

  it('exports scope, period, origin, sources, units and every record', () => {
    const csv = buildCsv(report({ scope: { level: 'APPLICATION', applicationKey: 'shop', applicationName: 'Shop' } }), [record]);
    expect(csv.startsWith('﻿')).toBe(true);
    expect(csv).toContain('"Phạm vi","Shop"');
    expect(csv).toContain('"Kỳ","2026-09-24 — 2026-10-23","Múi giờ","Asia/Ho_Chi_Minh"');
    expect(csv).toContain('Dữ liệu kiểm thử');
    expect(csv).toContain('"Xóa tài nguyên: không khả dụng"');
    expect(csv).toContain('"change_lead_time","Commit đến production","Chưa thu thập","","giây","","","","Cần liên kết deployment với commit và thời điểm commit."');
    expect(csv).toContain('"work_duration","Thời gian thực hiện","Có dữ liệu","1500","giây","","","14/15",""');
    expect(csv).toContain('"d1","Shop","production","api","","Cập nhật","Hoàn thành","3","2","3"');
    expect(csv).toContain('platform-engineer; ci-pipeline (tự động)');
    expect(csvFileName(report())).toBe('hieu-qua-platform-to-chuc-2026-09-24-2026-10-23.csv');
  });
});
