import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useState } from 'react';
import { describe, expect, it, vi } from 'vitest';
import { CriteriaEditor, draftFromCriteria, draftToCriteria, emptyDraft, type ApplicationOption, type CriteriaDraft, type CriteriaSource } from './CriteriaEditor';

const apps: ApplicationOption[] = [
  { key: 'shop', name: 'Cửa hàng', environments: [{ key: 'staging', name: 'Staging', environmentType: 'staging' }, { key: 'production', name: 'Production', environmentType: 'production' }] },
  { key: 'blog', name: 'blog', environments: [{ key: 'dev', name: 'Development', environmentType: 'development' }] },
];
const sources: CriteriaSource[] = [
  { key: 'pg-adv', name: 'PG nâng cao', resourceType: 'postgres', criteria: [{ env_type: 'staging', class: 'fast' }, { app_id: 'shop' }] },
  { key: 'pg-env', resourceType: 'postgres', criteria: [{ env_type: 'production' }] },
  { key: 'ns-any', resourceType: 'k8s-namespace', criteria: [{}] },
];

type Props = Partial<Parameters<typeof CriteriaEditor>[0]> & { initial?: CriteriaDraft; onDraft?(draft: CriteriaDraft): void };
function Harness({ initial = emptyDraft(), onDraft, ...props }: Props) {
  const [draft, setDraft] = useState(initial);
  return <CriteriaEditor draft={draft} onChange={(next) => { setDraft(next); onDraft?.(next); }}
    applications={{ items: apps, loading: false, error: '', onRetry: vi.fn() }} sources={{ items: sources, loading: false, error: '', onRetry: vi.fn() }} {...props} />;
}
const mode = (name: RegExp | string) => screen.getByRole('radio', { name });
const payload = (draft: CriteriaDraft) => draftToCriteria(draft).criteria;

describe('criteria draft projection', () => {
  it('serializes wildcard as [{}] and drops empty fields', () => {
    expect(draftToCriteria(emptyDraft())).toEqual({ criteria: [{}], problem: '' });
    expect(payload({ mode: 'env_type', rows: [{ env_type: 'staging', app_id: 'leftover', env_id: '', res_id: '', class: '' }] })).toEqual([{ env_type: 'staging' }]);
    expect(payload({ mode: 'app_env', rows: [{ env_type: '', app_id: 'shop', env_id: 'staging', res_id: '', class: '' }] })).toEqual([{ app_id: 'shop', env_id: 'staging' }]);
  });
  it('preserves literal whitespace criteria instead of replacing them with wildcard', () => {
    const criteria = [{ env_type: ' ', class: ' custom ' }, { res_id: '\t' }];
    expect(payload(draftFromCriteria([{ env_type: ' ' }]))).toEqual([{ env_type: ' ' }]);
    expect(payload(draftFromCriteria(criteria))).toEqual(criteria);
  });
  it('never turns an incomplete simple mode into a silent wildcard', () => {
    expect(draftToCriteria({ mode: 'env_type', rows: emptyDraft().rows }).problem).toMatch(/loại môi trường/);
    expect(draftToCriteria({ mode: 'app_env', rows: emptyDraft().rows }).problem).toMatch(/ứng dụng/);
  });
  it('classifies only losslessly simple criteria and keeps the rest advanced', () => {
    expect(draftFromCriteria(undefined).mode).toBe('all');
    expect(draftFromCriteria([{}]).mode).toBe('all');
    expect(draftFromCriteria([{ env_type: 'staging' }]).mode).toBe('env_type');
    expect(draftFromCriteria([{ app_id: 'shop', env_id: 'staging' }]).mode).toBe('app_env');
    expect(draftFromCriteria([{ env_type: 'staging', app_id: 'shop' }]).mode).toBe('advanced');
    expect(draftFromCriteria([{ class: 'fast' }]).mode).toBe('advanced');
    expect(draftFromCriteria([{ env_type: 'a' }, { env_type: 'b' }]).mode).toBe('advanced');
  });
});

describe('criteria draft row preservation', () => {
  it('keeps every row: a wildcard row beside a specific one is advanced and serializes unchanged', () => {
    const draft = draftFromCriteria([{}, { env_type: 'staging' }]);
    expect(draft.mode).toBe('advanced');
    expect(payload(draft)).toEqual([{}, { env_type: 'staging' }]);
  });
  it('keeps several blank rows as advanced instead of collapsing them to one wildcard', () => {
    const draft = draftFromCriteria([{}, {}]);
    expect(draft.mode).toBe('advanced');
    expect(draft.rows).toHaveLength(2);
    expect(payload(draft)).toEqual([{}, {}]);
  });
  it('keeps an empty list as zero rows and refuses to serialize it as a wildcard', () => {
    const draft = draftFromCriteria([]);
    expect(draft).toMatchObject({ mode: 'advanced', rows: [] });
    expect(draftToCriteria(draft).problem).toMatch(/ít nhất một điều kiện/);
  });
  it('reads only the five own fields, ignoring prototype and unknown keys', () => {
    const hostile = JSON.parse('{"__proto__":{"class":"x"},"constructor":"y","toString":"z","env_type":"a","extra":"b"}') as Record<string, string>;
    const draft = draftFromCriteria([hostile]);
    expect(payload(draft)).toEqual([{ env_type: 'a' }]);
    expect(Object.keys(draft.rows[0]!).sort()).toEqual(['app_id', 'class', 'env_id', 'env_type', 'res_id']);
  });
});

describe('CriteriaEditor', () => {
  it('asks before collapsing blank or mixed wildcard rows and copies them intact', async () => {
    const user = userEvent.setup();
    const seen: CriteriaDraft[] = [];
    const mixed: CriteriaSource[] = [{ key: 'mixed', resourceType: 'postgres', criteria: [{}, { env_type: 'staging' }] }, { key: 'blanks', resourceType: 'postgres', criteria: [{}, {}] }];
    render(<Harness sources={{ items: mixed, loading: false, error: '', onRetry: vi.fn() }} onDraft={(draft) => seen.push(draft)} />);
    await user.selectOptions(screen.getByLabelText('Sao chép điều kiện từ cấu hình khác'), 'mixed');
    await user.click(screen.getByRole('button', { name: 'Sao chép điều kiện' }));
    expect(mode(/Tùy chỉnh nâng cao/)).toBeChecked();
    expect(payload(seen.at(-1)!)).toEqual([{}, { env_type: 'staging' }]);
    await user.click(mode(/Theo loại môi trường/));
    expect(screen.getByRole('group', { name: 'Xác nhận thay đổi điều kiện' })).toBeInTheDocument();
    expect(payload(seen.at(-1)!)).toEqual([{}, { env_type: 'staging' }]);
    await user.click(screen.getByRole('button', { name: 'Giữ điều kiện nâng cao' }));
    await user.selectOptions(screen.getByLabelText('Sao chép điều kiện từ cấu hình khác'), 'blanks');
    await user.click(screen.getByRole('button', { name: 'Sao chép điều kiện' }));
    await user.click(screen.getByRole('button', { name: 'Thay bằng bản sao chép' }));
    expect(payload(seen.at(-1)!)).toEqual([{}, {}]);
    await user.click(mode(/Mọi nơi/));
    expect(screen.getByRole('group', { name: 'Xác nhận thay đổi điều kiện' })).toBeInTheDocument();
    expect(mode(/Tùy chỉnh nâng cao/)).toBeChecked();
  });

  it('shows a retryable error when the source list fails', async () => {
    const user = userEvent.setup();
    const retry = vi.fn();
    render(<Harness sources={{ items: [], loading: false, error: 'lỗi', onRetry: retry }} />);
    const alert = screen.getByRole('alert', { name: 'Lỗi tải cấu hình để sao chép' });
    await user.click(within(alert).getByRole('button', { name: 'Thử lại' }));
    expect(retry).toHaveBeenCalledTimes(1);
  });

  it('has no profile reasoning and no browser-only scope preview: the sample context only feeds the server matching panel', async () => {
    const user = userEvent.setup();
    const typed: ApplicationOption[] = [{ key: 'shop', name: 'shop', environments: [{ key: 'prod', name: 'prod', environmentType: 'production' }, { key: 'dev', name: 'dev', environmentType: 'development' }] }];
    render(<Harness applications={{ items: typed, loading: false, error: '', onRetry: vi.fn() }} initial={draftFromCriteria([{ app_id: 'shop' }])} />);
    expect(screen.queryByRole('group', { name: /Xem trước phạm vi/ })).not.toBeInTheDocument();
    const context = screen.getByRole('group', { name: 'Ngữ cảnh kiểm tra matching' });
    await user.selectOptions(screen.getByLabelText('Ứng dụng kiểm tra'), 'shop');
    await user.selectOptions(screen.getByLabelText('Môi trường kiểm tra'), 'prod');
    await user.type(screen.getByLabelText('ID tài nguyên kiểm tra (res_id)'), 'db');
    await user.type(screen.getByLabelText('Class kiểm tra'), 'fast');
    expect(context).not.toHaveTextContent('Cần thêm ngữ cảnh');
    expect(context.textContent).not.toMatch(/thỏa context|không thỏa|executionProfile|phạm vi triển khai|thắng|winner|ambiguous/i);
    expect(context).toHaveTextContent('env_type được máy chủ lấy từ môi trường');
  });

  it('shows the four Vietnamese modes and only the controls of the selected one', async () => {
    const user = userEvent.setup();
    const seen: CriteriaDraft[] = [];
    render(<Harness onDraft={(draft) => seen.push(draft)} />);
    for (const name of ['Mọi nơi', 'Theo loại môi trường', 'Theo ứng dụng (+ môi trường)', 'Tùy chỉnh nâng cao']) expect(mode(new RegExp(name.replace(/[()+]/g, '\\$&')))).toBeInTheDocument();
    expect(mode(/Mọi nơi/)).toBeChecked();
    expect(screen.queryByLabelText('Loại môi trường')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Ứng dụng')).not.toBeInTheDocument();
    await user.click(mode(/Theo loại môi trường/));
    await user.type(screen.getByLabelText('Loại môi trường'), 'staging');
    expect(payload(seen.at(-1)!)).toEqual([{ env_type: 'staging' }]);
    await user.click(mode(/Theo ứng dụng/));
    expect(screen.queryByLabelText('Loại môi trường')).not.toBeInTheDocument();
    await user.selectOptions(screen.getByLabelText('Ứng dụng'), 'shop');
    expect(within(screen.getByLabelText('Môi trường (tùy chọn)')).getAllByRole('option').map((option) => option.getAttribute('value'))).toEqual(['', 'staging', 'production']);
    await user.selectOptions(screen.getByLabelText('Môi trường (tùy chọn)'), 'production');
    expect(payload(seen.at(-1)!)).toEqual([{ app_id: 'shop', env_id: 'production' }]);
    // Another application resets an environment that belongs to the previous one.
    await user.selectOptions(screen.getByLabelText('Ứng dụng'), 'blog');
    expect(screen.getByLabelText('Môi trường (tùy chọn)')).toHaveValue('');
    expect(payload(seen.at(-1)!)).toEqual([{ app_id: 'blog' }]);
    await user.click(mode(/Mọi nơi/));
    expect(payload(seen.at(-1)!)).toEqual([{}]);
  });

  it('keeps all five fields and several rows in advanced mode', async () => {
    const user = userEvent.setup();
    const seen: CriteriaDraft[] = [];
    render(<Harness onDraft={(draft) => seen.push(draft)} />);
    await user.click(mode(/Tùy chỉnh nâng cao/));
    for (const label of ['Loại môi trường', 'ID ứng dụng', 'ID môi trường', 'ID tài nguyên', 'Class']) expect(screen.getByLabelText(`Điều kiện 1 ${label}`)).toBeInTheDocument();
    await user.type(screen.getByLabelText('Điều kiện 1 Loại môi trường'), 'staging');
    await user.type(screen.getByLabelText('Điều kiện 1 Class'), 'fast');
    await user.click(screen.getByRole('button', { name: '+ Thêm điều kiện' }));
    await user.type(screen.getByLabelText('Điều kiện 2 ID ứng dụng'), 'shop');
    await user.type(screen.getByLabelText('Điều kiện 2 ID tài nguyên'), 'db');
    expect(payload(seen.at(-1)!)).toEqual([{ env_type: 'staging', class: 'fast' }, { app_id: 'shop', res_id: 'db' }]);
    await user.click(screen.getByRole('button', { name: 'Xóa điều kiện 1' }));
    expect(payload(seen.at(-1)!)).toEqual([{ app_id: 'shop', res_id: 'db' }]);
  });

  it('blocks a lossy switch from advanced until the user confirms, and a lossless one carries over', async () => {
    const user = userEvent.setup();
    const seen: CriteriaDraft[] = [];
    render(<Harness onDraft={(draft) => seen.push(draft)} />);
    await user.click(mode(/Tùy chỉnh nâng cao/));
    await user.type(screen.getByLabelText('Điều kiện 1 Loại môi trường'), 'staging');
    // One env_type-only row is representable: no prompt, value carried.
    await user.click(mode(/Theo loại môi trường/));
    expect(screen.queryByRole('group', { name: 'Xác nhận thay đổi điều kiện' })).not.toBeInTheDocument();
    expect(screen.getByLabelText('Loại môi trường')).toHaveValue('staging');
    await user.click(mode(/Tùy chỉnh nâng cao/));
    await user.type(screen.getByLabelText('Điều kiện 1 Class'), 'fast');
    await user.click(screen.getByRole('button', { name: '+ Thêm điều kiện' }));
    await user.type(screen.getByLabelText('Điều kiện 2 ID ứng dụng'), 'shop');
    const before = seen.length;
    await user.click(mode(/Mọi nơi/));
    const prompt = screen.getByRole('group', { name: 'Xác nhận thay đổi điều kiện' });
    expect(prompt).toHaveTextContent(/sẽ bị bỏ/);
    expect(mode(/Tùy chỉnh nâng cao/)).toBeChecked();
    expect(seen).toHaveLength(before);
    await user.click(within(prompt).getByRole('button', { name: 'Giữ điều kiện nâng cao' }));
    expect(screen.getByLabelText('Điều kiện 2 ID ứng dụng')).toHaveValue('shop');
    await user.click(mode(/Theo ứng dụng/));
    expect(screen.getByRole('group', { name: 'Xác nhận thay đổi điều kiện' })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Bỏ điều kiện và chuyển chế độ' }));
    expect(mode(/Theo ứng dụng/)).toBeChecked();
    expect(screen.queryByRole('group', { name: 'Xác nhận thay đổi điều kiện' })).not.toBeInTheDocument();
  });

  it('copies criteria as an independent value and asks before replacing typed criteria', async () => {
    const user = userEvent.setup();
    const seen: CriteriaDraft[] = [];
    const mutable: CriteriaSource[] = [{ key: 'pg-adv', resourceType: 'postgres', criteria: [{ env_type: 'staging', class: 'fast' }, { app_id: 'shop' }] }];
    render(<Harness sources={{ items: mutable, loading: false, error: '', onRetry: vi.fn() }} onDraft={(draft) => seen.push(draft)} />);
    await user.selectOptions(screen.getByLabelText('Sao chép điều kiện từ cấu hình khác'), 'pg-adv');
    await user.click(screen.getByRole('button', { name: 'Sao chép điều kiện' }));
    expect(mode(/Tùy chỉnh nâng cao/)).toBeChecked();
    expect(payload(seen.at(-1)!)).toEqual([{ env_type: 'staging', class: 'fast' }, { app_id: 'shop' }]);
    await user.type(screen.getByLabelText('Điều kiện 2 ID tài nguyên'), 'db');
    expect(mutable[0]?.criteria).toEqual([{ env_type: 'staging', class: 'fast' }, { app_id: 'shop' }]);
    mutable[0]!.criteria![0]!.class = 'slow'; // later source change must not leak into the copy
    expect(screen.getByLabelText('Điều kiện 1 Class')).toHaveValue('fast');
    await user.click(screen.getByRole('button', { name: 'Sao chép điều kiện' }));
    const prompt = screen.getByRole('group', { name: 'Xác nhận thay đổi điều kiện' });
    expect(screen.getByLabelText('Điều kiện 2 ID tài nguyên')).toHaveValue('db');
    await user.click(within(prompt).getByRole('button', { name: 'Thay bằng bản sao chép' }));
    expect(screen.getByLabelText('Điều kiện 1 Class')).toHaveValue('slow');
    expect(screen.getByLabelText('Điều kiện 2 ID tài nguyên')).toHaveValue('');
  });

  it('shows app/env scope and no deployment profile; env_type stays its own criterion', async () => {
    const user = userEvent.setup();
    render(<Harness initial={draftFromCriteria([{ app_id: 'shop', env_id: 'staging' }])} />);
    const summary = screen.getByRole('group', { name: 'Tóm tắt phạm vi áp dụng' });
    expect(summary).toHaveTextContent('Ứng dụng (app_id): Cửa hàng (shop)');
    expect(summary).toHaveTextContent('Môi trường (env_id): staging');
    expect(summary).not.toHaveTextContent('Loại môi trường (env_type):');
    expect(summary).not.toHaveTextContent(/executionProfile|Phạm vi triển khai|profile/i);
    await user.click(mode(/Theo loại môi trường/));
    await user.type(screen.getByLabelText('Loại môi trường'), 'production');
    expect(summary).toHaveTextContent('Loại môi trường (env_type): production');
  });

  it('explains AND within a row, OR across rows, the weights, an explicit wildcard versus no criteria, and Definition ID versus res_id', () => {
    render(<Harness />);
    const rules = screen.getByRole('note', { name: 'Quy tắc so khớp' });
    expect(rules).toHaveTextContent('AND trong một dòng, OR giữa các dòng');
    expect(rules).toHaveTextContent('env_type 1 · app_id 2 · env_id 4 · res_id 8 · class 16');
    expect(rules).toHaveTextContent('Không có điều kiện (danh sách trống) bị từ chối');
    expect(rules).toHaveTextContent('Dòng trống {} là wildcard tường minh, khớp mọi nơi với điểm 0');
    expect(rules).toHaveTextContent('hòa điểm cao nhất và không có cấu hình nào khớp là hai kết quả khác nhau');
    expect(rules).toHaveTextContent('ID cấu hình (Definition ID) khác ID tài nguyên res_id');
  });

  it('asks for more context before the server check and never evaluates or ranks in the browser', async () => {
    const user = userEvent.setup();
    render(<Harness initial={draftFromCriteria([{ app_id: 'shop', class: 'fast' }])} />);
    const context = screen.getByRole('group', { name: 'Ngữ cảnh kiểm tra matching' });
    expect(context).toHaveTextContent(/Cần thêm ngữ cảnh: ứng dụng và môi trường, ID tài nguyên \(res_id\), Class/);
    await user.selectOptions(screen.getByLabelText('Ứng dụng kiểm tra'), 'shop');
    await user.selectOptions(screen.getByLabelText('Môi trường kiểm tra'), 'staging');
    await user.type(screen.getByLabelText('ID tài nguyên kiểm tra (res_id)'), 'db');
    expect(context).toHaveTextContent(/Cần thêm ngữ cảnh: Class/);
    await user.type(screen.getByLabelText('Class kiểm tra'), 'fast');
    expect(context).not.toHaveTextContent('Cần thêm ngữ cảnh');
    await user.clear(screen.getByLabelText('Class kiểm tra'));
    await user.type(screen.getByLabelText('Class kiểm tra'), 'slow');
    expect(context.textContent).not.toMatch(/thỏa|không thỏa|ambiguous|thắng|winner/i);
  });

  it('warns about possible overlap only for the same Resource Type, without profile reasoning or a winner', () => {
    render(<Harness resourceType="postgres" initial={draftFromCriteria([{ class: 'fast' }])} />);
    const warning = screen.getByRole('group', { name: 'Cảnh báo nguy cơ chồng lấn' });
    expect(warning).toHaveTextContent('pg-adv');
    expect(warning).toHaveTextContent('pg-env'); // no profile guard: any same-type overlap candidate may be listed
    expect(warning).not.toHaveTextContent('ns-any'); // other type
    expect(warning).toHaveTextContent(/chỉ “Kiểm tra matching” trên máy chủ mới cho biết cấu hình thắng, hòa hay không khớp/);
    expect(warning.textContent).not.toMatch(/ambiguous|mơ hồ|executionProfile|phạm vi triển khai/i);
  });

  it('never offers the Definition being edited as its own copy or overlap source', () => {
    render(<Harness resourceType="postgres" excludeKey="pg-adv" initial={draftFromCriteria([{ class: 'fast' }])} />);
    expect(screen.getByRole('group', { name: 'Cảnh báo nguy cơ chồng lấn' })).not.toHaveTextContent('pg-adv');
    expect(screen.queryByRole('option', { name: /pg-adv/ })).not.toBeInTheDocument();
  });

  it('keeps edits when the application list loads late, fails or is retried', async () => {
    const user = userEvent.setup();
    const retry = vi.fn();
    const { rerender } = render(<Harness initial={draftFromCriteria([{ app_id: 'gone', env_id: 'x' }])} applications={{ items: [], loading: true, error: '', onRetry: retry }} />);
    expect(screen.getByLabelText('Ứng dụng')).toHaveValue('gone');
    expect(screen.getByRole('option', { name: 'gone (không có trong danh sách hiện tại)' })).toBeInTheDocument();
    rerender(<Harness initial={draftFromCriteria([{ app_id: 'gone', env_id: 'x' }])} applications={{ items: [], loading: false, error: 'lỗi', onRetry: retry }} />);
    const alert = screen.getByRole('alert', { name: 'Lỗi tải ứng dụng' });
    expect(screen.getByLabelText('Ứng dụng')).toHaveValue('gone');
    await user.click(within(alert).getByRole('button', { name: 'Thử lại' }));
    expect(retry).toHaveBeenCalledTimes(1);
  });
});
