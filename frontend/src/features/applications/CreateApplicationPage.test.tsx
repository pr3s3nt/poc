import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { CreateApplicationPage, suggestSubdomain } from './CreateApplicationPage';
import { ApiError } from '../../shared/api/client';

afterEach(() => vi.unstubAllGlobals());

const nameInput = () => screen.getByLabelText('Tên ứng dụng');
const subdomainInput = () => screen.getByLabelText('Subdomain');
const submitButton = () => screen.getByRole('button', { name: 'Tạo ứng dụng' });

async function fill(name: string, subdomain: string) {
  const user = userEvent.setup();
  if (name) await user.type(nameInput(), name);
  if (subdomain) await user.type(subdomainInput(), subdomain);
  await user.click(submitButton());
}

it('asks only for name and subdomain and never loads or offers a connection', async () => {
  const fetchSpy = vi.fn();
  vi.stubGlobal('fetch', fetchSpy);
  render(<CreateApplicationPage onCreate={vi.fn()} />);
  expect(screen.getAllByRole('textbox')).toHaveLength(2);
  expect(screen.queryByLabelText('Connection')).not.toBeInTheDocument();
  expect(screen.queryByLabelText('Kết nối')).not.toBeInTheDocument();
  expect(screen.queryByRole('combobox')).not.toBeInTheDocument();
  expect(screen.getByRole('heading', { name: 'Tạo ứng dụng', level: 1 })).toBeInTheDocument();
  expect(screen.getByRole('form', { name: 'Tạo ứng dụng' })).toBeInTheDocument();
  expect(screen.getByText(/Chọn kết nối cho từng môi trường sau, trong Cài đặt môi trường/)).toBeInTheDocument();
  expect(screen.getByRole('group', { name: 'Endpoint sẽ được tạo' })).toBeInTheDocument();
  expect(screen.getByText('staging.ten-ung-dung.example.com')).toBeInTheDocument();
  expect(screen.getByText('ten-ung-dung.example.com')).toBeInTheDocument();
  expect(submitButton()).toBeEnabled();
  expect(fetchSpy).not.toHaveBeenCalled();
});

it('builds a DNS-label slug: Vietnamese accents, đ/Đ, separators, trimmed hyphens, 63 chars', () => {
  expect(suggestSubdomain('Thanh toán Điện tử')).toBe('thanh-toan-dien-tu');
  expect(suggestSubdomain('đường Đỗ')).toBe('duong-do');
  expect(suggestSubdomain('  --Pay_ment  v2!! ')).toBe('pay-ment-v2');
  expect(suggestSubdomain('!!! ???')).toBe('');
  expect(suggestSubdomain('Ứng dụng')).toBe('ung-dung');
  const long = suggestSubdomain(`${'a'.repeat(62)} bcd`);
  expect(long).toHaveLength(62);
  expect(long.endsWith('-')).toBe(false);
  expect(suggestSubdomain('b'.repeat(80))).toHaveLength(63);
});

it('suggests the subdomain from the name, follows name changes and shows the endpoint preview', async () => {
  const user = userEvent.setup();
  render(<CreateApplicationPage onCreate={vi.fn()} />);
  await user.type(nameInput(), 'Thanh toán');
  expect(subdomainInput()).toHaveValue('thanh-toan');
  expect(screen.getByText('staging.thanh-toan.example.com')).toBeInTheDocument();
  await user.type(nameInput(), ' Đơn');
  expect(subdomainInput()).toHaveValue('thanh-toan-don');
  await user.clear(nameInput());
  expect(subdomainInput()).toHaveValue('');
  expect(screen.getByText('staging.ten-ung-dung.example.com')).toBeInTheDocument();
});

it('leaves the suggestion empty when the name has no slug and requires a valid subdomain', async () => {
  const onCreate = vi.fn();
  const user = userEvent.setup();
  render(<CreateApplicationPage onCreate={onCreate} />);
  await user.type(nameInput(), '???');
  expect(subdomainInput()).toHaveValue('');
  await user.click(submitButton());
  expect(screen.getByText('Nhập subdomain.')).toBeInTheDocument();
  expect(onCreate).not.toHaveBeenCalled();
});

it('keeps a manual subdomain when the name changes, including a cleared one', async () => {
  const user = userEvent.setup();
  render(<CreateApplicationPage onCreate={vi.fn()} />);
  await user.type(nameInput(), 'Catalog');
  await user.type(subdomainInput(), '-v2');
  expect(subdomainInput()).toHaveValue('catalog-v2');
  await user.type(nameInput(), ' Mới');
  expect(subdomainInput()).toHaveValue('catalog-v2');
  await user.clear(subdomainInput());
  await user.type(nameInput(), ' Nữa');
  expect(subdomainInput()).toHaveValue('');
  expect(screen.getByText('staging.ten-ung-dung.example.com')).toBeInTheDocument();
});

it('keeps a custom subdomain typed before any name', async () => {
  const user = userEvent.setup();
  render(<CreateApplicationPage onCreate={vi.fn()} />);
  await user.type(subdomainInput(), 'custom');
  await user.type(nameInput(), 'Thanh toán');
  expect(subdomainInput()).toHaveValue('custom');
});

it('submits the normalized name and subdomain only', async () => {
  const onCreate = vi.fn().mockResolvedValue('app-1');
  render(<CreateApplicationPage onCreate={onCreate} />);
  await fill('  Catalog ', '');
  expect(onCreate).toHaveBeenCalledTimes(1);
  expect(onCreate.mock.calls[0]).toEqual(['Catalog', 'catalog']);
});

it('normalizes an uppercase manual subdomain before submit', async () => {
  const onCreate = vi.fn().mockResolvedValue('app-1');
  const user = userEvent.setup();
  render(<CreateApplicationPage onCreate={onCreate} />);
  await user.type(nameInput(), 'Catalog');
  await user.clear(subdomainInput());
  await user.type(subdomainInput(), ' Catalog-X ');
  await user.click(submitButton());
  expect(onCreate.mock.calls[0]).toEqual(['Catalog', 'catalog-x']);
});

it('validates inline without calling the API and keeps the URL preview', async () => {
  const onCreate = vi.fn();
  render(<CreateApplicationPage onCreate={onCreate} />);
  await fill('', 'bad_label');
  expect(screen.getByText('Nhập tên ứng dụng.')).toBeInTheDocument();
  expect(screen.getByText(/Chỉ dùng chữ thường không dấu, số và dấu gạch ngang; bắt đầu/)).toBeInTheDocument();
  expect(screen.getByText('staging.bad_label.example.com')).toBeInTheDocument();
  expect(onCreate).not.toHaveBeenCalled();
});

it('rejects an empty form with both field errors and no API call', async () => {
  const onCreate = vi.fn();
  render(<CreateApplicationPage onCreate={onCreate} />);
  await fill('', '');
  expect(screen.getByText('Nhập tên ứng dụng.')).toBeInTheDocument();
  expect(screen.getByText('Nhập subdomain.')).toBeInTheDocument();
  expect(onCreate).not.toHaveBeenCalled();
});

it('prevents duplicate submit and maps a duplicate subdomain to its field keeping the draft', async () => {
  let fail: (error: unknown) => void = () => undefined;
  const onCreate = vi.fn(() => new Promise<string>((_, reject) => { fail = reject; }));
  const user = userEvent.setup();
  render(<CreateApplicationPage onCreate={onCreate} />);
  await fill('Catalog', '');
  expect(onCreate).toHaveBeenCalledWith('Catalog', 'catalog');
  expect(screen.getByRole('button', { name: 'Đang tạo ứng dụng…' })).toBeDisabled();
  expect(nameInput()).toBeDisabled();
  expect(subdomainInput()).toBeDisabled();
  fail(new ApiError(409, 'application: subdomain is already in use', 'subdomain'));
  expect(await screen.findByText('Subdomain này đã được sử dụng.')).toBeInTheDocument();
  expect(nameInput()).toHaveValue('Catalog');
  expect(subdomainInput()).toHaveValue('catalog');
  expect(screen.queryByText(/already in use/)).not.toBeInTheDocument();
  expect(onCreate).toHaveBeenCalledTimes(1);
  // Editing the name after the failure keeps following the untouched suggestion; nothing is sent until submit.
  await user.type(nameInput(), ' 2');
  expect(subdomainInput()).toHaveValue('catalog-2');
  expect(onCreate).toHaveBeenCalledTimes(1);
});

it('maps a duplicate name and shows a safe Vietnamese fallback only on explicit retry', async () => {
  const onCreate = vi.fn()
    .mockRejectedValueOnce(new ApiError(409, 'application: application name already exists in the organization', 'name'))
    .mockRejectedValueOnce(new ApiError(500, 'boom'));
  const user = userEvent.setup();
  render(<CreateApplicationPage onCreate={onCreate} />);
  await fill('Catalog', '');
  expect(await screen.findByText('Tên ứng dụng này đã tồn tại.')).toBeInTheDocument();
  expect(onCreate).toHaveBeenCalledTimes(1);
  await user.click(submitButton());
  expect(await screen.findByRole('alert')).toHaveTextContent('Không thể tạo ứng dụng. Hãy thử lại.');
  expect(screen.queryByText(/boom/)).not.toBeInTheDocument();
  expect(onCreate).toHaveBeenCalledTimes(2);
  expect(subdomainInput()).toHaveValue('catalog');
  expect(nameInput()).toHaveValue('Catalog');
});

it('maps invalid backend field errors to Vietnamese field messages', async () => {
  const onCreate = vi.fn().mockRejectedValueOnce(new ApiError(400, 'invalid subdomain', 'subdomain'));
  render(<CreateApplicationPage onCreate={onCreate} />);
  await fill('Catalog', '');
  expect(await screen.findByText(/Chỉ dùng chữ thường không dấu, số và dấu gạch ngang; bắt đầu/)).toBeInTheDocument();
  expect(screen.queryByText('invalid subdomain')).not.toBeInTheDocument();
});
