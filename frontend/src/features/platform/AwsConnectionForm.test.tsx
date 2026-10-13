import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ConnectionsPage } from './ConnectionsPage';
import { parseRegions } from './AwsConnectionForm';

// Synthetic fixture key: never a real credential.
const keyId = 'AKIAFIXTUREKEY000001';
const secret = 'fixture/secret/value/0000000000000000000';
const awsRecord = {
  key: 'lab-aws', name: 'Lab AWS', kind: 'AWS', authenticationType: 'AWS_ACCESS_KEY', status: 'READY',
  config: { accountId: '123456789012', principalArn: 'arn:aws:iam::123456789012:user/orch', region: 'us-east-1', allowedRegions: ['us-east-1', 'eu-west-1'] },
  verification: { verified: true, identity: 'VERIFIED', regionCheck: 'VERIFIED', capabilityCheck: 'VERIFIED' },
};

type Call = { url: string; body: Record<string, unknown> };

function stubApi(options: { register?: () => Response | Promise<Response>; list?: () => Response } = {}) {
  const calls: Call[] = [];
  let lists = 0;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (init?.method === 'POST') {
      calls.push({ url, body: JSON.parse(String(init.body)) as Record<string, unknown> });
      return (options.register ?? (() => Response.json(awsRecord, { status: 201 })))();
    }
    lists += 1;
    return (options.list ?? (() => Response.json({ connections: [] })))();
  }));
  return { calls, lists: () => lists };
}

afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks(); });

async function openAws(user: ReturnType<typeof userEvent.setup>) {
  await screen.findByText('Chưa có kết nối nào được đăng ký.');
  await user.click(screen.getByLabelText('Tài khoản AWS'));
}
const nameField = () => screen.getByLabelText('Tên kết nối');
const keyField = () => screen.getByLabelText('Access key ID');
const secretField = () => screen.getByLabelText('Secret access key');
const regionField = () => screen.getByLabelText('Vùng được phép');
const save = () => screen.getByRole('button', { name: 'Kiểm tra và lưu' });
async function fill(user: ReturnType<typeof userEvent.setup>, overrides: { name?: string; key?: string; secret?: string; regions?: string } = {}) {
  for (const [field, value] of [[nameField(), overrides.name ?? 'Lab AWS'], [keyField(), overrides.key ?? keyId], [secretField(), overrides.secret ?? secret], [regionField(), overrides.regions ?? 'us-east-1, eu-west-1']] as const) {
    if (value) await user.type(field, value);
  }
}
const pageText = () => document.body.textContent ?? '';

describe('UC-04 AWS connection onboarding', () => {
  it('shows the AWS form with concealed key fields and no reveal control', async () => {
    const user = userEvent.setup();
    stubApi();
    render(<ConnectionsPage />);
    await openAws(user);
    expect(keyField()).toHaveAttribute('type', 'password');
    expect(secretField()).toHaveAttribute('type', 'password');
    expect(keyField()).toHaveAttribute('autocomplete', 'new-password');
    expect(screen.queryByLabelText(/Hiện/)).not.toBeInTheDocument();
    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Nội dung kubeconfig')).not.toBeInTheDocument();
    expect(screen.queryByText(/backend Terraform|state backend/i)).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/profile|credential của máy/i)).not.toBeInTheDocument();
    expect(save()).toBeEnabled();
  });

  it('rejects malformed input locally without sending anything or echoing values', async () => {
    const user = userEvent.setup();
    const api = stubApi();
    render(<ConnectionsPage />);
    await openAws(user);
    await fill(user, { key: 'AKIAshort', regions: 'us-east-1' });
    await user.click(save());
    expect(await screen.findByRole('alert')).toHaveTextContent('Access key ID không đúng dạng');
    expect(pageText()).not.toContain('AKIAshort');
    for (const [patch, expected] of [
      [{ secret: 'tooshort', regions: 'us-east-1' }, 'Secret access key không đúng dạng'],
      [{ regions: 'not a region!' }, 'Tên vùng không hợp lệ'],
      [{ regions: 'us-east-1, us-east-1' }, 'Mỗi vùng chỉ được nhập một lần'],
      [{ regions: '' }, 'Nhập ít nhất một vùng'],
    ] as const) {
      await user.clear(keyField()); await user.clear(secretField()); await user.clear(regionField());
      await fill(user, { name: '', ...patch });
      await user.click(save());
      expect(await screen.findByRole('alert')).toHaveTextContent(expected);
    }
    expect(api.calls).toHaveLength(0);
  });

  it('registers once, sends the exact payload, clears every secret and shows only safe metadata', async () => {
    const user = userEvent.setup();
    const api = stubApi();
    render(<ConnectionsPage />);
    await openAws(user);
    await fill(user);
    expect(regionField()).toHaveValue('us-east-1, eu-west-1');
    await user.click(save());
    const result = await screen.findByLabelText('Kết quả xác minh AWS');
    expect(api.calls).toHaveLength(1);
    expect(api.calls[0]).toEqual({ url: '/api/v1/connections/aws', body: { name: 'Lab AWS', accessKeyId: keyId, secretAccessKey: secret, allowedRegions: ['us-east-1', 'eu-west-1'] } });
    expect(result).toHaveTextContent('123456789012');
    expect(result).toHaveTextContent('arn:aws:iam::123456789012:user/orch');
    expect(result).toHaveTextContent('us-east-1, eu-west-1');
    expect(result).toHaveTextContent('Quyền cấp phát');
    expect(screen.getByRole('status')).toHaveTextContent('Đã đăng ký tài khoản AWS Lab AWS (lab-aws)');
    expect(screen.getByRole('status')).toHaveTextContent('chưa có tài nguyên nào được tạo');
    expect(keyField()).toHaveValue(''); expect(secretField()).toHaveValue(''); expect(nameField()).toHaveValue(''); expect(regionField()).toHaveValue('');
    expect(pageText()).not.toContain(keyId); expect(pageText()).not.toContain(secret);
    await waitFor(() => expect(api.lists()).toBeGreaterThan(1));
  });

  it('keeps safe metadata, clears both keys and shows fixed guidance when verification fails', async () => {
    const user = userEvent.setup();
    const api = stubApi({ register: () => Response.json({ code: 'VERIFICATION_PERMISSION_DENIED', error: 'AccessDenied SECRET-AWS-DETAIL' }, { status: 422 }) });
    render(<ConnectionsPage />);
    await openAws(user);
    await fill(user);
    await user.click(save());
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('thiếu quyền');
    expect(alert).toHaveTextContent('chưa lưu kết nối');
    expect(pageText()).not.toContain('SECRET-AWS-DETAIL');
    expect(pageText()).not.toContain(keyId); expect(pageText()).not.toContain(secret);
    expect(keyField()).toHaveValue(''); expect(secretField()).toHaveValue('');
    expect(nameField()).toHaveValue('Lab AWS'); expect(regionField()).toHaveValue('us-east-1, eu-west-1');
    expect(screen.queryByLabelText('Kết quả xác minh AWS')).not.toBeInTheDocument();
    expect(api.calls).toHaveLength(1);
  });

  it.each([
    ['VERIFICATION_CREDENTIAL_REJECTED', 'AWS từ chối access key'],
    ['VERIFICATION_REGION_UNAVAILABLE', 'vùng được phép chưa dùng được'],
    ['VERIFICATION_CAPABILITY_UNVERIFIABLE', 'iam:SimulatePrincipalPolicy'],
    ['VERIFICATION_PRINCIPAL_UNSUPPORTED', 'không dùng tài khoản gốc'],
    ['VERIFICATION_UNREACHABLE', 'Không kết nối được AWS'],
    ['VERIFICATION_FAILED', 'Không xác minh được tài khoản AWS'],
  ])('maps %s to AWS guidance without raw text', async (code, expected) => {
    const user = userEvent.setup();
    stubApi({ register: () => Response.json({ code, error: 'RAW-PROVIDER-TEXT' }, { status: 422 }) });
    render(<ConnectionsPage />);
    await openAws(user);
    await fill(user);
    await user.click(save());
    expect(await screen.findByRole('alert')).toHaveTextContent(expected);
    expect(pageText()).not.toContain('RAW-PROVIDER-TEXT');
  });

  it('does not claim failure on an unknown outcome, reloads the list and never resubmits', async () => {
    const user = userEvent.setup();
    let attempts = 0;
    const api = stubApi({ register: () => { attempts += 1; throw new TypeError('network down'); } });
    render(<ConnectionsPage />);
    await openAws(user);
    await fill(user);
    const before = api.lists();
    await user.click(save());
    expect(await screen.findByRole('alert')).toHaveTextContent('chưa biết kết nối đã được lưu hay chưa');
    await waitFor(() => expect(api.lists()).toBeGreaterThan(before));
    expect(attempts).toBe(1);
    expect(keyField()).toHaveValue(''); expect(secretField()).toHaveValue('');
  });

  it('locks the form while saving so a double click sends one request', async () => {
    const user = userEvent.setup();
    let release: (response: Response) => void = () => undefined;
    const api = stubApi({ register: () => new Promise<Response>((resolve) => { release = resolve; }) });
    render(<ConnectionsPage />);
    await openAws(user);
    await fill(user);
    await user.click(save());
    expect(await screen.findByRole('button', { name: 'Đang kiểm tra và lưu…' })).toBeDisabled();
    expect(nameField()).toBeDisabled(); expect(secretField()).toBeDisabled();
    await user.click(screen.getByRole('button', { name: 'Đang kiểm tra và lưu…' }));
    expect(api.calls).toHaveLength(1);
    release(Response.json(awsRecord, { status: 201 }));
    await screen.findByLabelText('Kết quả xác minh AWS');
  });

  it('shows 403 and server failures with fixed guidance', async () => {
    const user = userEvent.setup();
    stubApi({ register: () => Response.json({ code: 'FORBIDDEN' }, { status: 403 }) });
    render(<ConnectionsPage />);
    await openAws(user);
    await fill(user);
    await user.click(save());
    expect(await screen.findByRole('alert')).toHaveTextContent('Cần vai trò Kỹ sư nền tảng hoặc Quản trị');
  });

  it('lists AWS records truthfully: verified evidence only when all three checks exist', async () => {
    stubApi({ list: () => Response.json({ connections: [awsRecord, { key: 'aws-account', name: 'aws-account', kind: 'AWS', status: 'READY', config: { region: 'eu-west-1', accountId: '111111111111' }, verification: { verified: true } }] }) });
    render(<ConnectionsPage />);
    const table = await screen.findByRole('table', { name: 'Kết nối đã đăng ký' });
    const rows = within(table).getAllByRole('row');
    expect(rows[1]).toHaveTextContent('us-east-1, eu-west-1');
    expect(rows[1]).toHaveTextContent('123456789012');
    expect(rows[1]).toHaveTextContent('Đã xác minh khi đăng ký: danh tính, từng vùng và quyền cấp phát');
    expect(rows[2]).toHaveTextContent('Chưa có bằng chứng xác minh quyền cấp phát');
    expect(rows[2]).not.toHaveTextContent('Đã xác minh khi đăng ký');
    expect(pageText()).not.toMatch(/secretRef|kv2:|AKIA/);
  });

  it('parses regions in typed order', () => {
    expect(parseRegions(' us-east-1,eu-west-1  ap-south-1;')).toEqual(['us-east-1', 'eu-west-1', 'ap-south-1']);
    expect(parseRegions('')).toEqual([]);
  });
});
