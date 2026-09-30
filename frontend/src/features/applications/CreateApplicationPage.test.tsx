import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, it, vi } from 'vitest';
import { CreateApplicationPage } from './CreateApplicationPage';
import { ApiError } from '../../shared/api/client';

async function fill(name: string, subdomain: string) {
  const user = userEvent.setup();
  if (name) await user.type(screen.getByLabelText('Application name'), name);
  if (subdomain) await user.type(screen.getByLabelText('Subdomain'), subdomain);
  await user.click(screen.getByRole('button', { name: 'Create application' }));
}

it('offers only name and subdomain and previews both endpoints', () => {
  render(<CreateApplicationPage onCreate={vi.fn()} />);
  expect(screen.getAllByRole('textbox')).toHaveLength(2);
  expect(screen.getByText('staging.your-app.example.com')).toBeInTheDocument();
});

it('validates inline without calling the API and keeps the URL preview', async () => {
  const onCreate = vi.fn();
  render(<CreateApplicationPage onCreate={onCreate} />);
  await fill('', 'bad_label');
  expect(screen.getByText('Enter an application name.')).toBeInTheDocument();
  expect(screen.getByText(/Use lowercase letters, numbers and hyphens;/)).toBeInTheDocument();
  expect(screen.getByText('staging.bad_label.example.com')).toBeInTheDocument();
  expect(onCreate).not.toHaveBeenCalled();
});

it('prevents duplicate submit and maps a duplicate subdomain to its field', async () => {
  let fail: (error: unknown) => void = () => undefined;
  const onCreate = vi.fn(() => new Promise<string>((_, reject) => { fail = reject; }));
  render(<CreateApplicationPage onCreate={onCreate} />);
  await fill('Catalog', 'Catalog');
  expect(onCreate).toHaveBeenCalledWith('Catalog', 'catalog');
  expect(screen.getByRole('button', { name: 'Creating application…' })).toBeDisabled();
  fail(new ApiError(409, 'application: subdomain is already in use', 'subdomain'));
  expect(await screen.findByText('This subdomain is already in use.')).toBeInTheDocument();
  expect(screen.getByLabelText('Application name')).toHaveValue('Catalog');
  expect(screen.getByLabelText('Subdomain')).toHaveValue('Catalog');
  expect(onCreate).toHaveBeenCalledTimes(1);
});

it('maps a duplicate name and shows form-level API errors', async () => {
  const onCreate = vi.fn()
    .mockRejectedValueOnce(new ApiError(409, 'application: application name already exists in the organization', 'name'))
    .mockRejectedValueOnce(new ApiError(422, 'application: default execution target is not ready'))
    .mockRejectedValueOnce(new ApiError(500, 'boom'));
  const user = userEvent.setup();
  render(<CreateApplicationPage onCreate={onCreate} />);
  await fill('Catalog', 'catalog');
  expect(await screen.findByText('An application with this name already exists.')).toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Create application' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('The platform default target is not ready.');
  await user.click(screen.getByRole('button', { name: 'Create application' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('Could not create the application. Try again.');
  expect(screen.getByLabelText('Subdomain')).toHaveValue('catalog');
});
