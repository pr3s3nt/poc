import { useEffect, useRef, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { AppShell } from '../../src/app/AppShell';
import { Button } from '../../src/shared/ui/Button';
import { Status } from '../../src/shared/ui/Status';
import { setCatalogs, t, intlLocale, localeStorageKey } from '../../src/shared/i18n';
import vi from '../../public/locales/vi.json';
import en from '../../public/locales/en.json';
import '../../src/styles/index.css';
import './style.css';

const words = {
  vi: {
    logExpired: 'Thời điểm này đã ngoài thời gian lưu log.', mock: 'Bản mẫu · Dữ liệu giả', scenario: 'Tình huống', healthy: 'Đã nhận dữ liệu', empty: 'Chưa triển khai', waiting: 'Chờ dữ liệu đầu tiên', gap: 'Gián đoạn thu thập', missing: 'Chưa có metrics', error: 'Lỗi tải dữ liệu', loading: 'Đang tải',
    back: 'Về ứng dụng', app: 'Ứng dụng', settings: 'Cài đặt môi trường', home: 'Tổng quan', logs: 'Logs', metrics: 'Metrics', title: 'Logging & Monitoring',
    preview: 'Xem trước thay đổi', add: 'Thêm Workload', edit: 'Sửa', ready: 'Sẵn sàng', name: 'Tên', status: 'Trạng thái', actions: 'Thao tác', workload: 'Workload', all: 'Tất cả workload',
    source: 'Thu thập tự động', latest: 'Dữ liệu mới nhất', collected: 'Workload có dữ liệu', cpu: 'CPU', memory: 'RAM', restarts: 'Restart', pods: 'Pod sẵn sàng', viewLogs: 'Xem logs', viewMetrics: 'Xem metrics', noDeployments: 'Chưa có lần triển khai nào.',
    retention: 'Logs: 7 ngày · Metrics: 15 ngày', open: 'Mở', time: 'Khoảng thời gian', last15: '15 phút qua', last60: '1 giờ qua', last360: '6 giờ qua', last1440: '24 giờ qua', last10080: '7 ngày qua', last21600: '15 ngày qua', custom: 'Tùy chọn', from: 'Từ', to: 'Đến', apply: 'Áp dụng', invalidRange: 'Chọn thời gian kết thúc sau thời gian bắt đầu, trong phạm vi lưu trữ.',
    search: 'Tìm trong nội dung log', searchHint: 'Từ khóa, lỗi hoặc nội dung…', stream: 'Luồng', allStreams: 'Tất cả', follow: 'Theo dõi log mới', pause: 'Tạm dừng', live: 'Đang theo dõi', refresh: 'Làm mới', updated: 'Cập nhật lúc', timeColumn: 'Thời gian', content: 'Nội dung', shown: 'Hiển thị {count} dòng mới nhất', noResults: 'Không có log khớp bộ lọc.', clear: 'Xóa bộ lọc', older: 'Xem log cũ hơn', detail: 'Chi tiết log', close: 'Đóng', copy: 'Sao chép nội dung', copied: 'Đã sao chép', copyFail: 'Không sao chép được; bạn có thể chọn nội dung bên dưới.', previousPod: 'Pod trước redeploy',
    noDeploy: 'Môi trường này chưa có workload được triển khai.', noDeployHelp: 'Dữ liệu sẽ xuất hiện sau khi triển khai và thu thập thành công.', waitingText: 'Chưa nhận được dữ liệu từ workload.', waitingHelp: 'Đã cấu hình thu thập. Đang chờ log và mẫu CPU/RAM đầu tiên.',
    gapText: 'Thiếu dữ liệu từ 14:08 đến 14:12.', gapHelp: 'Phần lịch sử đã lưu vẫn xem được. Biểu đồ để trống khoảng bị thiếu.', missingText: 'Chưa lấy được CPU/RAM từ Metrics Server.', missingHelp: 'Logs vẫn có dữ liệu. Kiểm tra Metrics Server và quyền đọc metrics của cluster.', errorText: 'Không tải được dữ liệu. Thử lại để cập nhật.', retry: 'Thử lại', loadingText: 'Đang tải dữ liệu…',
    historyCPU: 'Lịch sử CPU', historyMemory: 'Lịch sử RAM', unitCPU: 'mCPU', unitMemory: 'MiB', sampleEvery: 'Mẫu mỗi 30 giây', sampleAt: 'Tại', latestValues: 'Mẫu gần nhất theo workload', currentNote: 'Các giá trị trong bảng là mẫu mới nhất, không phải trung bình của khoảng đã chọn.',
    absent: 'Chưa có dữ liệu', countStatus: '3 workload · đã nhận logs và metrics', logsOnly: '3 workload · đã nhận logs', deployment: 'Triển khai gần đây', success: 'Thành công', history: 'Xem tất cả', deployed: 'Triển khai', sampleNotice: 'Tương tác này chỉ minh họa; bản mẫu không gọi API sản phẩm.', logNavigation: 'Chọn một dòng để xem pod, container và nội dung đầy đủ.', tooltipKeyboard: 'Dùng phím trái/phải để xem các điểm thời gian.', removed: 'Lịch sử vẫn gồm pod đã bị thay thế.', chartEmpty: 'Chưa có mẫu CPU/RAM trong khoảng này.', timezone: 'Giờ hiển thị: UTC+7', zoom: 'Xem logs tại thời điểm này', container: 'Container', pod: 'Pod',
  },
  en: {
    logExpired: 'This point is outside log retention.', mock: 'Prototype · Synthetic data', scenario: 'Scenario', healthy: 'Data received', empty: 'Not deployed', waiting: 'Waiting for first data', gap: 'Collection gap', missing: 'Metrics unavailable', error: 'Data load error', loading: 'Loading',
    back: 'Back to application', app: 'Application', settings: 'Environment settings', home: 'Overview', logs: 'Logs', metrics: 'Metrics', title: 'Logging & Monitoring',
    preview: 'Preview changes', add: 'Add Workload', edit: 'Edit', ready: 'Ready', name: 'Name', status: 'Status', actions: 'Actions', workload: 'Workload', all: 'All workloads',
    source: 'Automatic collection', latest: 'Latest data', collected: 'Workloads reporting', cpu: 'CPU', memory: 'Memory', restarts: 'Restarts', pods: 'Ready pods', viewLogs: 'View logs', viewMetrics: 'View metrics', noDeployments: 'No deployments yet.',
    retention: 'Logs: 7 days · Metrics: 15 days', open: 'Open', time: 'Time range', last15: 'Last 15 minutes', last60: 'Last hour', last360: 'Last 6 hours', last1440: 'Last 24 hours', last10080: 'Last 7 days', last21600: 'Last 15 days', custom: 'Custom', from: 'From', to: 'To', apply: 'Apply', invalidRange: 'Choose an end after the start, within the retention period.',
    search: 'Search log content', searchHint: 'Keyword, error or text…', stream: 'Stream', allStreams: 'All', follow: 'Follow new logs', pause: 'Pause', live: 'Following', refresh: 'Refresh', updated: 'Updated at', timeColumn: 'Time', content: 'Message', shown: 'Showing {count} latest lines', noResults: 'No logs match your filters.', clear: 'Clear filters', older: 'Load older logs', detail: 'Log details', close: 'Close', copy: 'Copy message', copied: 'Copied', copyFail: 'Could not copy; select the message below.', previousPod: 'Pod before redeploy',
    noDeploy: 'This environment has no deployed workloads.', noDeployHelp: 'Data appears after deployment and successful collection.', waitingText: 'No workload data has been received yet.', waitingHelp: 'Collection is configured. Waiting for the first logs and CPU/memory samples.',
    gapText: 'Data missing from 14:08 to 14:12.', gapHelp: 'Previously stored history remains available. Charts leave the missing interval blank.', missingText: 'CPU/memory could not be read from Metrics Server.', missingHelp: 'Logs are still available. Check Metrics Server and cluster metrics permissions.', errorText: 'Could not load data. Retry to refresh.', retry: 'Retry', loadingText: 'Loading data…',
    historyCPU: 'CPU history', historyMemory: 'Memory history', unitCPU: 'mCPU', unitMemory: 'MiB', sampleEvery: 'Sampled every 30 seconds', sampleAt: 'At', latestValues: 'Latest sample by workload', currentNote: 'Table values are the latest sample, not the average over the selected range.',
    absent: 'No data yet', countStatus: '3 workloads · logs and metrics received', logsOnly: '3 workloads · logs received', deployment: 'Recent deployments', success: 'Succeeded', history: 'View all', deployed: 'Deploy', sampleNotice: 'This interaction is illustrative; the prototype does not call product APIs.', logNavigation: 'Select a line to view its pod, container and full message.', tooltipKeyboard: 'Use left/right arrows to explore time points.', removed: 'History includes pods that have been replaced.', chartEmpty: 'No CPU/memory samples in this range.', timezone: 'Display time: UTC+7', zoom: 'View logs at this time', container: 'Container', pod: 'Pod',
  },
} as const;
const language = localStorage.getItem(localeStorageKey) === 'en' ? 'en' : 'vi';
const catalog = (base: Record<string, string>, extra: Record<string, string>) => ({ ...base, ...Object.fromEntries(Object.entries(extra).map(([key, value]) => [`obs.${key}`, value])) });
setCatalogs(language, catalog(language === 'en' ? en : vi, words[language]), catalog(vi, words.vi), [{ code: 'vi', name: 'Tiếng Việt', intl: 'vi-VN' }, { code: 'en', name: 'English', intl: 'en-GB' }]);
const text = (key: keyof typeof words.vi, params?: Record<string, unknown>) => t(`obs.${key}`, params);
type View = 'home' | 'logs' | 'metrics';
type Scenario = 'healthy' | 'empty' | 'waiting' | 'gap' | 'missing' | 'error' | 'loading';
type Env = 'staging' | 'production';
type Workload = 'backend' | 'worker' | 'frontend';
const workloads: Workload[] = ['backend', 'worker', 'frontend'];
const colors: Record<Workload, string> = { backend: '#0969da', worker: '#8250df', frontend: '#1a7f64' };
const anchor = Date.UTC(2026, 9, 24, 7, 30);
const time = (value: number, seconds = false) => new Intl.DateTimeFormat(intlLocale(), { timeZone: 'Asia/Bangkok', hour: '2-digit', minute: '2-digit', ...(seconds ? { second: '2-digit' } : {}) }).format(value);
const dateTime = (value: number) => new Intl.DateTimeFormat(intlLocale(), { timeZone: 'Asia/Bangkok', day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hourCycle: 'h23' }).format(value);
const localInput = (value: number) => new Date(value + 7 * 3600000).toISOString().slice(0, 16);
const parseInput = (value: string) => Date.parse(`${value}:00+07:00`);
type Log = { id: string; timestamp: number; workload: Workload; pod: string; container: string; stream: 'stdout' | 'stderr'; message: string; old: boolean };
function makeLog(index: number, environment: Env, now = anchor): Log {
  const workload = workloads[index % workloads.length]!;
  const timestamp = now - index * 90000;
  const old = workload === 'worker' && timestamp < anchor - 18 * 60000;
  const stream = index % 17 === 0 ? 'stderr' : 'stdout';
  const messages: Record<Workload, string> = {
    backend: index % 17 === 0 ? 'ERROR database query timed out; retry scheduled in 2s' : `INFO GET /api/jobs 200 duration=${22 + index % 60}ms environment=${environment}`,
    worker: index % 17 === 0 ? 'WARN Redis connection interrupted; reconnecting' : `INFO job completed job_id=job-${1042 + index} duration=${135 + index % 200}ms queue=default`,
    frontend: index % 17 === 0 ? 'ERROR upstream temporarily unavailable; backend retry' : `INFO GET / ${index % 11 === 0 ? 304 : 200} duration=${3 + index % 20}ms`,
  };
  return { id: `${environment}-${now}-${index}`, timestamp, workload, pod: `${workload}-${old ? '7c98d56f4b' : '6db7f8c95d'}-${old ? 'p4x7b' : 'm8r2k'}`, container: 'main', stream, message: messages[workload], old };
}
function pointValue(workload: Workload, index: number, metric: 'cpu' | 'memory', environment: Env): number {
  const wi = workloads.indexOf(workload);
  const wave = Math.sin(index * 0.43 + wi) * 0.5 + Math.sin(index * 1.11 + wi) * 0.18;
  const peak = index > 18 && index < 29 ? Math.sin((index - 18) / 11 * Math.PI) : 0;
  const factor = environment === 'production' ? 1.45 : 1;
  return Math.round((metric === 'cpu' ? [130, 92, 27][wi]! + wave * [47, 34, 12][wi]! + peak * [140, 230, 20][wi]! : [235, 178, 64][wi]! + wave * [18, 22, 4][wi]! + peak * [32, 65, 8][wi]!) * factor);
}

function Chart({ metric, start, end, environment, selected, gap, onInspect }: { metric: 'cpu' | 'memory'; start: number; end: number; environment: Env; selected: string; gap: boolean; onInspect(value: number): void }) {
  const [hover, setHover] = useState<number | null>(null);
  const items = workloads.filter(w => selected === 'all' || selected === w);
  const width = 540, height = 222, left = 46, top = 18, bottom = 188, right = 524, samples = 61;
  const max = metric === 'cpu' ? (environment === 'production' ? 600 : 400) : (environment === 'production' ? 600 : 400);
  const stamp = (index: number) => start + (end - start) * index / (samples - 1);
  const unavailable = (index: number) => gap && stamp(index) >= anchor - 22 * 60000 && stamp(index) <= anchor - 18 * 60000;
  const paths = (workload: Workload) => {
    let active = false;
    return Array.from({ length: samples }, (_, index) => {
      if (unavailable(index)) { active = false; return ''; }
      const segment = `${active ? 'L' : 'M'}${left + (right - left) * index / (samples - 1)},${bottom - (bottom - top) * pointValue(workload, index, metric, environment) / max}`;
      active = true; return segment;
    }).join(' ');
  };
  return <section className="obs-chart" aria-label={text(metric === 'cpu' ? 'historyCPU' : 'historyMemory')}>
    <div className="obs-chart-heading"><h3>{text(metric === 'cpu' ? 'historyCPU' : 'historyMemory')}</h3><span>{text(metric === 'cpu' ? 'unitCPU' : 'unitMemory')}</span></div>
    <div className="obs-chart-body" tabIndex={0} role="group" aria-label={`${text(metric === 'cpu' ? 'historyCPU' : 'historyMemory')}. ${text('tooltipKeyboard')}`} onKeyDown={event => { if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') { event.preventDefault(); setHover(value => Math.max(0, Math.min(samples - 1, (value ?? samples - 1) + (event.key === 'ArrowLeft' ? -1 : 1)))); } }} onMouseLeave={() => setHover(null)}>
      <svg viewBox={`0 0 ${width} ${height}`} role="img" aria-label={`${text(metric === 'cpu' ? 'historyCPU' : 'historyMemory')}, ${dateTime(start)} – ${dateTime(end)}`} onMouseMove={event => { const rect = event.currentTarget.getBoundingClientRect(); setHover(Math.max(0, Math.min(samples - 1, Math.round(((event.clientX - rect.left) / rect.width * width - left) / (right - left) * (samples - 1))))); }}>
        {[0, 1, 2, 3, 4].map(tick => <g key={tick}><line x1={left} x2={right} y1={bottom - tick / 4 * (bottom - top)} y2={bottom - tick / 4 * (bottom - top)} className="obs-gridline" /><text x={left - 10} y={bottom - tick / 4 * (bottom - top) + 4} textAnchor="end">{tick / 4 * max}</text></g>)}
        {gap && end > anchor - 22 * 60000 && start < anchor - 18 * 60000 ? <rect className="obs-gap-zone" x={left + Math.max(0, (anchor - 22 * 60000 - start) / (end - start)) * (right - left)} y={top} width={Math.max(1, (Math.min(end, anchor - 18 * 60000) - Math.max(start, anchor - 22 * 60000)) / (end - start) * (right - left))} height={bottom - top} /> : null}
        {items.map(w => <path key={w} d={paths(w)} stroke={colors[w]} className="obs-series" />)}
        {[0, 15, 30, 45, 60].map(i => <text key={i} x={left + (right - left) * i / 60} y={213} textAnchor={i === 0 ? 'start' : i === 60 ? 'end' : 'middle'}>{end - start > 86400000 ? new Intl.DateTimeFormat(intlLocale(), { timeZone: 'Asia/Bangkok', day: '2-digit', month: '2-digit' }).format(stamp(i)) : time(stamp(i))}</text>)}
        {hover !== null ? <line x1={left + (right - left) * hover / 60} x2={left + (right - left) * hover / 60} y1={top} y2={bottom} className="obs-crosshair" /> : null}
      </svg>
      {hover !== null ? <div className="obs-chart-tooltip" role="status"><strong>{dateTime(stamp(hover))}</strong>{unavailable(hover) ? <span>{text('absent')}</span> : items.map(w => <span key={w}><i style={{ background: colors[w] }} />{w}<b>{pointValue(w, hover, metric, environment)} {text(metric === 'cpu' ? 'unitCPU' : 'unitMemory')}</b></span>)}{stamp(hover) < anchor - 7 * 86400000 ? <span>{text('logExpired')}</span> : <button onClick={() => onInspect(stamp(hover))}>{text('zoom')} →</button>}</div> : null}
    </div>
    <div className="obs-legend">{items.map(w => <span key={w}><i style={{ background: colors[w] }} />{w}</span>)}</div>
  </section>;
}

function App() {
  const [view, setView] = useState<View>('home');
  const [environment, setEnvironment] = useState<Env>('staging');
  const [scenario, setScenario] = useState<Scenario>('healthy');
  const [workload, setWorkload] = useState('all');
  const [range, setRange] = useState('60');
  const [customStart, setCustomStart] = useState(localInput(anchor - 3600000));
  const [customEnd, setCustomEnd] = useState(localInput(anchor));
  const [windowRange, setWindowRange] = useState({ start: anchor - 3600000, end: anchor });
  const [rangeError, setRangeError] = useState(false);
  const [query, setQuery] = useState('');
  const [stream, setStream] = useState('all');
  const [limit, setLimit] = useState(50);
  const [follow, setFollow] = useState(false);
  const [extraLogs, setExtraLogs] = useState<Log[]>([]);
  const [detail, setDetail] = useState<Log | null>(null);
  const [copyState, setCopyState] = useState('');
  const [notice, setNotice] = useState('');
  const [refreshAt, setRefreshAt] = useState(anchor);
  const logArea = useRef<HTMLDivElement>(null);
  const dialog = useRef<HTMLDialogElement>(null);
  const opener = useRef<HTMLElement | null>(null);
  const now = extraLogs[0]?.timestamp ?? anchor;
  const rangeStart = range === 'custom' ? windowRange.start : now - Number(range) * 60000;
  const rangeEnd = range === 'custom' ? windowRange.end : now;
  const noData = scenario === 'empty' || scenario === 'waiting';
  const metricsMissing = scenario === 'missing';
  const available = !noData && scenario !== 'error' && scenario !== 'loading';
  const title = text(scenario === 'gap' ? 'gap' : scenario === 'missing' ? 'missing' : scenario === 'healthy' ? 'healthy' : scenario);
  const currentLogs = [...extraLogs, ...Array.from({ length: 6720 }, (_, i) => makeLog(i, environment))].filter(row => row.timestamp >= rangeStart && row.timestamp <= rangeEnd && (workload === 'all' || row.workload === workload) && (stream === 'all' || stream === row.stream) && row.message.toLocaleLowerCase().includes(query.toLocaleLowerCase()) && !(scenario === 'gap' && row.timestamp >= anchor - 22 * 60000 && row.timestamp <= anchor - 18 * 60000));
  const filteredLogs = currentLogs.slice(0, limit).reverse();
  function open(next: View, selected = workload) { setView(next); setWorkload(selected); setNotice(''); setFollow(false); setLimit(50); }
  function changeEnvironment(next: Env) { setEnvironment(next); setExtraLogs([]); setFollow(false); setDetail(null); setRefreshAt(anchor); setNotice(''); setLimit(50); }
  function changeRange(next: string) { setRange(next); setRangeError(false); setLimit(50); setFollow(false); }
  function mockNotice() { setNotice(text('sampleNotice')); }
  function openLog(row: Log, trigger: HTMLElement) { opener.current = trigger; setCopyState(''); setDetail(row); }
  useEffect(() => {
    if (detail) dialog.current?.showModal();
    else if (dialog.current?.open) { dialog.current.close(); opener.current?.focus(); }
  }, [detail]);
  useEffect(() => {
    if (!follow || view !== 'logs' || !available) return;
    const timer = window.setInterval(() => setExtraLogs(previous => {
      const stamp = (previous[0]?.timestamp ?? anchor) + 3000;
      const index = previous.length;
      const next = { ...makeLog(index, environment, stamp), timestamp: stamp, id: `${environment}-live-${stamp}`, old: false, pod: `${workloads[index % 3]}-6db7f8c95d-m8r2k` };
      return [next, ...previous].slice(0, 200);
    }), 3000);
    return () => window.clearInterval(timer);
  }, [follow, view, available, environment]);
  useEffect(() => { if (follow && logArea.current) logArea.current.scrollTop = logArea.current.scrollHeight; }, [extraLogs, follow]);
  useEffect(() => {
    const stopNavigation = (event: MouseEvent) => {
      const target = event.target instanceof Element ? event.target.closest('a') : null;
      if (target?.closest('.sidebar')) { event.preventDefault(); event.stopPropagation(); setView('home'); setNotice(text('sampleNotice')); }
    };
    document.addEventListener('click', stopNavigation, true);
    return () => document.removeEventListener('click', stopNavigation, true);
  }, []);
  function clearFilters() { setQuery(''); setWorkload('all'); setStream('all'); setLimit(50); }
  const missingCard = () => <div className="obs-empty"><span className="obs-empty-icon" aria-hidden="true">{scenario === 'empty' ? '◫' : '◷'}</span><h3>{text(scenario === 'empty' ? 'noDeploy' : 'waitingText')}</h3><p>{text(scenario === 'empty' ? 'noDeployHelp' : 'waitingHelp')}</p></div>;
  const banner = scenario === 'gap' ? <div className="obs-warning" role="status"><strong>{text('gapText')}</strong><span>{text('gapHelp')}</span></div> : metricsMissing ? <div className="obs-warning" role="status"><strong>{text('missingText')}</strong><span>{text('missingHelp')}</span></div> : null;
  const controls = <div className="obs-filters"><label>{text('workload')}<select aria-label={text('workload')} value={workload} onChange={event => { setWorkload(event.target.value); setLimit(50); }}><option value="all">{text('all')}</option>{workloads.map(w => <option key={w}>{w}</option>)}</select></label><label>{text('time')}<select aria-label={text('time')} value={range} onChange={event => changeRange(event.target.value)}>{[15, 60, 360, 1440, 10080, ...(view === 'metrics' ? [21600] : [])].map(n => <option key={n} value={n}>{text(`last${n}` as keyof typeof words.vi)}</option>)}<option value="custom">{text('custom')}</option></select></label><span className="obs-filter-spacer" /><span className="obs-stamp">{text('updated')} {time(refreshAt, true)}</span><Button onClick={() => { setRefreshAt(anchor + extraLogs.length * 3000); if (scenario === 'error') setScenario('healthy'); }}>{text('refresh')}</Button></div>;
  return <>
    <div className="obs-prototype-bar"><span><span className="obs-prototype-dot" />{text('mock')}</span><label>{text('scenario')}<select aria-label={text('scenario')} value={scenario} onChange={event => { setScenario(event.target.value as Scenario); setFollow(false); }}>{(['healthy', 'empty', 'waiting', 'gap', 'missing', 'error', 'loading'] as const).map(item => <option key={item} value={item}>{text(item)}</option>)}</select></label></div>
    <AppShell username="developer" role="DEVELOPER" activeRoute="application" onSignOut={mockNotice}>
      <section className="page application-home obs-page">
        <button className="back-link" onClick={() => view === 'home' ? mockNotice() : open('home')}>← {view === 'home' ? t('app.appShell.applications') : text('back')}</button>
        <header className="page-header application-header"><div><p className="eyebrow">{text('app')}</p><h1>JobFlow</h1><p>{environment === 'staging' ? 'staging.' : ''}jobflow.example.com</p></div><Button onClick={mockNotice}>{text('settings')}</Button></header>
        <div className="tabs" role="tablist" aria-label="Environment">{(['staging', 'production'] as const).map(env => <button key={env} role="tab" aria-selected={environment === env} className={`tab${environment === env ? ' tab-active' : ''}`} onClick={() => changeEnvironment(env)}>{env}<span>{env === 'staging' ? 'staging.' : ''}jobflow.example.com</span></button>)}</div>
        {notice ? <div className="form-info obs-notice" role="status">{notice}<button aria-label={text('close')} onClick={() => setNotice('')}>×</button></div> : null}
        {view === 'home' ? <>
          <section className="content-panel"><div className="section-header"><h2>Workload</h2><Button onClick={mockNotice}>{text('add')}</Button></div>{scenario === 'empty' ? missingCard() : <div className="workload-table"><div className="table-head"><span>{text('name')}</span><span>{text('status')}</span><span>{text('actions')}</span></div>{workloads.map(w => <div className="table-row" key={w}><span className="workload-name"><span className="workload-icon">◫</span>{w}</span><Status tone="good">{text('ready')}</Status><span><Button tone="quiet" onClick={() => open('logs', w)}>{text('logs')}</Button><Button tone="quiet" onClick={() => open('metrics', w)}>{text('metrics')}</Button><Button tone="quiet" onClick={mockNotice}>{text('edit')}</Button></span></div>)}</div>}<div className="form-actions"><Button tone="primary" onClick={mockNotice}>{text('preview')}</Button></div></section>
          <section className="content-panel obs-home-card"><div className="section-header"><div><h2>{text('title')}</h2><p>{text('source')} · {environment}</p></div><Status tone={scenario === 'healthy' ? 'good' : 'neutral'}>{title}</Status></div>{noData ? missingCard() : <><div className="obs-home-summary"><div><small>{text('collected')}</small><strong>{scenario === 'error' || scenario === 'loading' ? '—' : '3 / 3'}</strong></div><div><small>{text('cpu')}</small><strong>{metricsMissing || !available ? '—' : workloads.reduce((sum, w) => sum + pointValue(w, 60, 'cpu', environment), 0)} <span>{metricsMissing || !available ? '' : text('unitCPU')}</span></strong></div><div><small>{text('memory')}</small><strong>{metricsMissing || !available ? '—' : workloads.reduce((sum, w) => sum + pointValue(w, 60, 'memory', environment), 0)} <span>{metricsMissing || !available ? '' : text('unitMemory')}</span></strong></div><div><small>{text('latest')}</small><strong>{!available ? '—' : '14:30:00'}</strong></div></div>{banner}</>}<div className="obs-home-footer"><small>{text('retention')}</small><div><Button onClick={() => open('logs')}>{text('viewLogs')} →</Button><Button onClick={() => open('metrics')}>{text('viewMetrics')} →</Button></div></div></section>
          <section className="content-panel"><div className="section-header"><h2>{text('deployment')}</h2><Button tone="quiet" onClick={mockNotice}>{text('history')} →</Button></div>{scenario === 'empty' ? <p className="obs-stamp">{text('noDeployments')}</p> : workloads.map((w, i) => <div className="deployment-entry obs-deployment" key={w}><span><strong>{w}</strong><small>{text('deployed')} · 24/10/2026, {i === 1 ? '14:12' : '13:55'}</small></span><Status tone="good">{text('success')}</Status></div>)}</section>
        </> : <section className="content-panel obs-explorer">
          <div className="section-header"><div><h2>{text('title')}</h2><p>{environment} · {text('source')}</p></div><Status tone={scenario === 'healthy' ? 'good' : 'neutral'}>{title}</Status></div>
          <div className="obs-view-tabs" role="tablist" aria-label={text('title')}>{(['logs', 'metrics'] as const).map(tab => <button key={tab} role="tab" aria-selected={view === tab} className={view === tab ? 'is-active' : ''} onClick={() => { open(tab); if (tab === 'logs' && Number(range) > 10080) changeRange('10080'); }}>{text(tab)}</button>)}</div>
          {controls}
          {range === 'custom' ? <div className="obs-custom-range"><label>{text('from')}<input aria-label={text('from')} type="datetime-local" value={customStart} onChange={event => setCustomStart(event.target.value)} /></label><label>{text('to')}<input aria-label={text('to')} type="datetime-local" value={customEnd} onChange={event => setCustomEnd(event.target.value)} /></label><Button onClick={() => { const start = parseInput(customStart), end = parseInput(customEnd); const valid = Number.isFinite(start) && Number.isFinite(end) && start < end && start >= anchor - (view === 'logs' ? 7 : 15) * 86400000 && end <= now; setRangeError(!valid); if (valid) setWindowRange({ start, end }); }}>{text('apply')}</Button><small>{text('timezone')}</small>{rangeError ? <span role="alert" className="field-error">{text('invalidRange')}</span> : null}</div> : null}
          {banner}
          {scenario === 'loading' ? <div className="obs-empty" role="status" aria-busy="true"><span className="obs-spinner" />{text('loadingText')}</div> : scenario === 'error' ? <div className="obs-empty" role="alert"><h3>{text('errorText')}</h3><Button onClick={() => setScenario('healthy')}>{text('retry')}</Button></div> : noData ? missingCard() : view === 'logs' ? <>
            <div className="obs-log-controls"><label className="obs-search"><svg viewBox="0 0 20 20" aria-hidden="true"><circle cx="8" cy="8" r="5" /><path d="m12 12 5 5" /></svg><input aria-label={text('search')} placeholder={text('searchHint')} value={query} onChange={event => { setQuery(event.target.value); setLimit(50); }} /></label><label className="obs-stream"><span>{text('stream')}</span><select aria-label={text('stream')} value={stream} onChange={event => { setStream(event.target.value); setLimit(50); }}><option value="all">{text('allStreams')}</option><option>stdout</option><option>stderr</option></select></label><Button aria-pressed={follow} disabled={range === 'custom'} onClick={() => setFollow(value => !value)}><span className={`obs-live-dot${follow ? ' is-live' : ''}`} />{text(follow ? 'pause' : 'follow')}</Button></div>
            <div className="obs-log-table" ref={logArea} role="region" aria-label={text('logs')} tabIndex={0}><table><thead><tr><th>{text('timeColumn')}</th><th>{text('workload')}</th><th>{text('content')}</th></tr></thead><tbody>{filteredLogs.map(row => <tr key={row.id}><td><span>{time(row.timestamp, true)}</span><small>{new Intl.DateTimeFormat(intlLocale(), { timeZone: 'Asia/Bangkok', day: '2-digit', month: '2-digit' }).format(row.timestamp)}</small></td><td><span className="obs-workload-label"><i style={{ background: colors[row.workload] }} />{row.workload}</span><small>{row.stream}</small></td><td><button className="obs-log-message" aria-label={`${dateTime(row.timestamp)} · ${row.workload} · ${row.message}`} onClick={event => openLog(row, event.currentTarget)}>{query ? row.message.split(new RegExp(`(${query.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')})`, 'ig')).map((part, i) => part.toLowerCase() === query.toLowerCase() ? <mark key={i}>{part}</mark> : part) : row.message}{row.old ? <span className="obs-old-pod" title={text('previousPod')}>↺</span> : null}</button></td></tr>)}</tbody></table>{!filteredLogs.length ? <div className="obs-empty"><p>{text('noResults')}</p><Button onClick={clearFilters}>{text('clear')}</Button></div> : null}</div>
            <div className="obs-log-footer"><span>{text('shown', { count: filteredLogs.length })}{follow ? <span className="obs-follow-state"> · {text('live')}</span> : null}</span>{currentLogs.length > limit ? <Button tone="quiet" onClick={() => setLimit(value => value + 50)}>{text('older')} ↑</Button> : <small>{text('removed')}</small>}</div>
          </> : <>
            {metricsMissing ? <div className="obs-chart-grid">{(['historyCPU', 'historyMemory'] as const).map(name => <section className="obs-chart" key={name}><h3>{text(name)}</h3><div className="obs-empty obs-chart-unavailable"><strong>—</strong><span>{text('absent')}</span></div></section>)}</div> : <div className="obs-chart-grid">{(['cpu', 'memory'] as const).map(metric => <Chart key={`${metric}-${environment}-${rangeStart}-${rangeEnd}`} metric={metric} start={rangeStart} end={rangeEnd} environment={environment} selected={workload} gap={scenario === 'gap'} onInspect={stamp => { setRange('custom'); const start = Math.max(anchor - 7 * 86400000, stamp - 5 * 60000), end = Math.min(now, stamp + 5 * 60000); setCustomStart(localInput(start)); setCustomEnd(localInput(end)); setWindowRange({ start, end }); open('logs'); }} />)}</div>}
            <div className="obs-metric-table-heading"><h3>{text('latestValues')}</h3><span>{text('sampleEvery')} · 14:30:00</span></div><div className="obs-table-scroll"><table className="obs-metric-table"><thead><tr><th>{text('workload')}</th><th>{text('cpu')} <small>(mCPU)</small></th><th>{text('memory')} <small>(MiB)</small></th><th>{text('restarts')}</th><th>{text('pods')}</th><th /></tr></thead><tbody>{workloads.filter(w => workload === 'all' || workload === w).map(w => <tr key={w}><td><span className="obs-workload-label"><i style={{ background: colors[w] }} />{w}</span></td><td>{metricsMissing ? '—' : pointValue(w, 60, 'cpu', environment)}</td><td>{metricsMissing ? '—' : pointValue(w, 60, 'memory', environment)}</td><td>{w === 'worker' ? '1' : '0'}</td><td><Status tone="good">{environment === 'production' && w === 'backend' ? '2 / 2' : '1 / 1'}</Status></td><td><Button tone="quiet" onClick={() => open('logs', w)}>{text('viewLogs')} →</Button></td></tr>)}</tbody></table></div>
          </>}
          <div className="obs-explorer-footer"><small>{text('retention')}</small><small>{text('timezone')}</small></div>
        </section>}
      </section>
    </AppShell>
    <dialog ref={dialog} className="obs-detail-dialog" onCancel={() => setDetail(null)} onClick={event => { if (event.target === event.currentTarget) setDetail(null); }} aria-labelledby="obs-dialog-title">
      {detail ? <><header><h2 id="obs-dialog-title">{text('detail')}</h2><Button tone="quiet" aria-label={text('close')} onClick={() => setDetail(null)}>×</Button></header><dl><div><dt>{text('timeColumn')}</dt><dd>{dateTime(detail.timestamp)} · UTC+7</dd></div><div><dt>Environment</dt><dd>{environment}</dd></div><div><dt>{text('workload')}</dt><dd>{detail.workload}</dd></div><div><dt>{text('container')}</dt><dd>{detail.container}</dd></div><div className="obs-full-row"><dt>{text('pod')}</dt><dd>{detail.pod}{detail.old ? <span className="obs-pod-badge">{text('previousPod')}</span> : null}</dd></div><div><dt>{text('stream')}</dt><dd>{detail.stream}</dd></div></dl><pre>{detail.message}</pre><footer><span role="status">{copyState}</span><Button onClick={() => { void navigator.clipboard?.writeText(detail.message).then(() => setCopyState(text('copied'))).catch(() => setCopyState(text('copyFail'))); if (!navigator.clipboard) setCopyState(text('copyFail')); }}>{text('copy')}</Button></footer></> : null}
    </dialog>
  </>;
}

createRoot(document.getElementById('root')!).render(<App />);
