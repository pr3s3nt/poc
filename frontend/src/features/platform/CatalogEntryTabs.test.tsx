import { afterEach, expect, it, vi } from 'vitest';
import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { CatalogEntryTabs } from './CatalogEntryTabs';
afterEach(cleanup);
it('keeps form and import drafts when switching methods using mouse and keyboard', async () => {
  const user = userEvent.setup();
  render(<CatalogEntryTabs kind="resource-types" onRegistered={vi.fn()}><label>Form draft<input defaultValue="kept" /></label></CatalogEntryTabs>);
  expect(screen.getByRole('tab', { name: 'Biểu mẫu' })).toHaveAttribute('aria-selected', 'true');
  await user.click(screen.getByRole('tab', { name: 'Nhập YAML/JSON' }));
  await user.type(screen.getByLabelText('Nội dung YAML/JSON', { exact: true }), 'key: draft');
  await user.click(screen.getByRole('tab', { name: 'Biểu mẫu' }));
  expect(screen.getByLabelText('Form draft')).toHaveValue('kept');
  await user.keyboard('{ArrowRight}');
  expect(screen.getByRole('tab', { name: 'Nhập YAML/JSON' })).toHaveFocus();
  expect(screen.getByLabelText('Nội dung YAML/JSON', { exact: true })).toHaveValue('key: draft');
});
