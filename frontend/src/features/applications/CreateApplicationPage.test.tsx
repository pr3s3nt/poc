import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { CreateApplicationPage } from './CreateApplicationPage';
import { ApiError } from '../../shared/api/client';

afterEach(() => vi.unstubAllGlobals());

const nameInput = () => screen.getByLabelText('Tên ứng dụng');
const submitButton = () => screen.getByRole('button', { name: 'Tạo ứng dụng' });

// ADR-037: an Application has a name only; no subdomain or derived address.
it('asks only for the name and never loads or offers a connection', async () => {
  const fetchSpy = vi.fn();
  vi.stubGlobal('fetch', fetchSpy);
  render(<CreateApplicationPage onCreate={vi.fn()} />);
  expect(screen.getAllByRole('textbox')).toHaveLength(1);
  expect(screen.queryByLabelText('Subdomain')).not.toBeInTheDocument();
  expect(screen.queryByRole('combobox')).not.toBeInTheDocument();
  expect(screen.queryByRole('group', { name: 'Endpoint sẽ được tạo' })).not.toBeInTheDocument();
  expect(fetchSpy).not.toHaveBeenCalled();
});

it('submits the trimmed name only and validates an empty name inline', async () => {
  const onCreate = vi.fn().mockResolvedValue('id');
  const user = userEvent.setup();
  render(<CreateApplicationPage onCreate={onCreate} />);
  await user.click(submitButton());
  expect(onCreate).not.toHaveBeenCalled();
  await user.type(nameInput(), '  Catalog  ');
  await user.click(submitButton());
  expect(onCreate).toHaveBeenCalledWith('Catalog');
});

it('maps a duplicate name to its field and keeps the draft', async () => {
  const onCreate = vi.fn().mockRejectedValueOnce(new ApiError(409, 'DUPLICATE', 'name'));
  const user = userEvent.setup();
  render(<CreateApplicationPage onCreate={onCreate} />);
  await user.type(nameInput(), 'Catalog');
  await user.click(submitButton());
  expect(await screen.findByText('Tên ứng dụng này đã tồn tại.')).toBeInTheDocument();
  expect(nameInput()).toHaveValue('Catalog');
});
