import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, it, vi } from 'vitest';
import { SearchableSelect } from './SearchableSelect';

it('filters long catalogs by name without changing or hiding the selected value', async () => {
  const user = userEvent.setup();
  const onChange = vi.fn();
  render(<SearchableSelect label="Key" value="key-0" options={Array.from({ length: 12 }, (_, i) => ({ value: `key-${i}`, label: `key-${i}` }))} placeholder="Choose" disabled={false} onChange={onChange} />);
  await user.type(screen.getByRole('searchbox'), 'key-11');
  const select = screen.getByRole('combobox', { name: 'Key' });
  expect(select).toHaveValue('key-0');
  expect(within(select).getAllByRole('option')).toHaveLength(3);
  expect(onChange).not.toHaveBeenCalled();
  await user.selectOptions(select, 'key-11');
  expect(onChange).toHaveBeenCalledWith('key-11');
  expect(screen.getByRole('searchbox')).toHaveValue('');
});

it('keeps short catalogs as a single native selector', () => {
  render(<SearchableSelect label="Resource" value="" options={[{ value: 'db', label: 'db' }]} placeholder="Choose" disabled={false} onChange={() => {}} />);
  expect(screen.queryByRole('searchbox')).not.toBeInTheDocument();
  expect(screen.getByRole('combobox')).toBeInTheDocument();
});
