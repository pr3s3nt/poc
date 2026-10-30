import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, it } from 'vitest';
import { PasswordInput } from './PasswordInput';

// The value stays masked by default; the toggle reveals it as plain text and
// never submits the surrounding form.
it('toggles between masked and plain-text display', async () => {
  const user = userEvent.setup();
  render(<form onSubmit={(event) => event.preventDefault()}><PasswordInput aria-label="Mật khẩu" defaultValue="secret-1" /></form>);
  const input = screen.getByLabelText('Mật khẩu');
  expect(input).toHaveAttribute('type', 'password');
  const toggle = screen.getByRole('button', { name: 'Hiện' });
  expect(toggle).toHaveAttribute('type', 'button');
  await user.click(toggle);
  expect(input).toHaveAttribute('type', 'text');
  await user.click(screen.getByRole('button', { name: 'Ẩn' }));
  expect(input).toHaveAttribute('type', 'password');
});
