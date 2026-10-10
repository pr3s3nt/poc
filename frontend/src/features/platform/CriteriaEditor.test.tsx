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
  { key: 'pg-adv', resourceType: 'postgres', executionProfile: 'internal-k8s', criteria: [{ env_type: 'staging', class: 'fast' }, { app_id: 'shop' }] },
  { key: 'pg-env', resourceType: 'postgres', executionProfile: 'aws-eks', criteria: [{ env_type: 'production' }] },
  { key: 'ns-any', resourceType: 'k8s-namespace', executionProfile: '', criteria: [{}] },
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

  it('does not suggest applicability when the sample environment profile differs or is unknown', async () => {
    const user = userEvent.setup();
    const profiled: ApplicationOption[] = [{ key: 'shop', name: 'shop', environments: [{ key: 'prod', name: 'prod', environmentType: 'production', executionProfile: 'aws-eks' }, { key: 'dev', name: 'dev', environmentType: 'development', executionProfile: '' }, { key: 'ci', name: 'ci', environmentType: 'ci', executionProfile: 'internal-k8s' }] }];
    render(<Harness applications={{ items: profiled, loading: false, error: '', onRetry: vi.fn() }} profile="internal-k8s" initial={draftFromCriteria([{ app_id: 'shop' }])} />);
    const preview = screen.getByRole('group', { name: /Xem trước phạm vi/ });
    await user.selectOptions(screen.getByLabelText('Ứng dụng mẫu'), 'shop');
    await user.type(screen.getByLabelText('ID tài nguyên mẫu (res_id)'), 'db');
    await user.type(screen.getByLabelText('Class mẫu'), 'fast');
    await user.selectOptions(screen.getByLabelText('Môi trường mẫu'), 'prod');
    expect(preview).toHaveTextContent(/khác phạm vi internal-k8s.*không được xét/);
    expect(preview).not.toHaveTextContent('thỏa context mẫu');
    await user.selectOptions(screen.getByLabelText('Môi trường mẫu'), 'dev');
    expect(preview).toHaveTextContent(/Chưa biết phạm vi triển khai.*chưa có nghĩa là cấu hình được xét/);
    await user.selectOptions(screen.getByLabelText('Môi trường mẫu'), 'ci');
    expect(preview).toHaveTextContent('phù hợp với cấu hình này');
    expect(preview).toHaveTextContent('Điều kiện 1: thỏa context mẫu');
    expect(preview.textContent).not.toMatch(/ambiguous|thắng|winner/i);
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
    const mutable: CriteriaSource[] = [{ key: 'pg-adv', resourceType: 'postgres', executionProfile: '', criteria: [{ env_type: 'staging', class: 'fast' }, { app_id: 'shop' }] }];
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

  it('shows app/env scope and the deployment profile apart from env_type', async () => {
    const user = userEvent.setup();
    render(<Harness profile="internal-k8s" profileLabel="Cluster nội bộ" initial={draftFromCriteria([{ app_id: 'shop', env_id: 'staging' }])} />);
    const summary = screen.getByRole('group', { name: 'Tóm tắt phạm vi áp dụng' });
    expect(summary).toHaveTextContent('Ứng dụng (app_id): Cửa hàng (shop)');
    expect(summary).toHaveTextContent('Môi trường (env_id): staging');
    expect(summary).not.toHaveTextContent('Loại môi trường (env_type):');
    expect(summary).toHaveTextContent(/Phạm vi triển khai \(executionProfile\): Cluster nội bộ.*không phải Loại môi trường \(env_type\)/);
    await user.click(mode(/Theo loại môi trường/));
    await user.type(screen.getByLabelText('Loại môi trường'), 'production');
    expect(summary).toHaveTextContent('Loại môi trường (env_type): production');
  });

  it('asks for more context before previewing and never claims an exact match', async () => {
    const user = userEvent.setup();
    render(<Harness initial={draftFromCriteria([{ app_id: 'shop', class: 'fast' }])} />);
    const preview = screen.getByRole('group', { name: /Xem trước phạm vi/ });
    expect(preview).toHaveTextContent(/Cần thêm context: ứng dụng và môi trường mẫu, ID tài nguyên \(res_id\), Class/);
    await user.selectOptions(screen.getByLabelText('Ứng dụng mẫu'), 'shop');
    await user.selectOptions(screen.getByLabelText('Môi trường mẫu'), 'staging');
    await user.type(screen.getByLabelText('ID tài nguyên mẫu (res_id)'), 'db');
    expect(preview).toHaveTextContent(/Cần thêm context: Class/);
    await user.type(screen.getByLabelText('Class mẫu'), 'fast');
    expect(preview).not.toHaveTextContent('Cần thêm context');
    expect(preview).toHaveTextContent('Điều kiện 1: thỏa context mẫu');
    expect(preview).toHaveTextContent(/không phải kết quả matching chính xác/);
    await user.clear(screen.getByLabelText('Class mẫu'));
    await user.type(screen.getByLabelText('Class mẫu'), 'slow');
    expect(preview).toHaveTextContent('Điều kiện 1: không thỏa context mẫu');
    expect(preview.textContent).not.toMatch(/ambiguous|thắng|winner/i);
  });

  it('warns about possible overlap only for the same type and profile eligibility, without claiming a winner', () => {
    const { rerender } = render(<Harness resourceType="postgres" profile="internal-k8s" initial={draftFromCriteria([{ env_type: 'staging' }])} />);
    const warning = screen.getByRole('group', { name: 'Cảnh báo nguy cơ chồng lấn' });
    expect(warning).toHaveTextContent('pg-adv');
    expect(warning).not.toHaveTextContent('pg-env'); // other profile
    expect(warning).not.toHaveTextContent('ns-any'); // other type
    expect(warning).toHaveTextContent(/chưa phải kết quả matching chính xác/);
    expect(warning.textContent).not.toMatch(/ambiguous|thắng|winner|mơ hồ/i);
    rerender(<Harness resourceType="postgres" profile="aws-eks" initial={draftFromCriteria([{ env_type: 'development' }])} />);
    expect(screen.queryByRole('group', { name: 'Cảnh báo nguy cơ chồng lấn' })).not.toBeInTheDocument();
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
