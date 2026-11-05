import { fireEvent, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, it } from 'vitest';
import { UserGuidePage } from './UserGuidePage';
import { canReadGuide, guideSections } from './content';

it.each(['DEVELOPER', 'PLATFORM_ENGINEER', 'ADMIN'])('lets %s read the developer walkthrough and its initial empty-state image', role => {
  render(<UserGuidePage role={role} />);
  expect(screen.getByRole('heading', { name: 'DEV — Triển khai acceptance app từng bước' })).toBeVisible();
  const initial = screen.getByRole('img', { name: /Ngay sau khi tạo: ứng dụng acceptance-dev-guide chưa có workload/ });
  expect(initial).toHaveAttribute('src', '/ui/guides/acceptance-app/new-01-empty.png');
  for (const image of screen.getAllByRole('img')) expect(image.getAttribute('src')).toMatch(/^\/ui\/guides\/acceptance-app\//);
  expect(screen.queryByText('Nguồn đối chiếu và phạm vi xác minh')).not.toBeInTheDocument();
  for (const section of guideSections) {
    expect(screen.getByRole('link', { name: section.title })).toHaveAttribute('href', `#${section.id}`);
    expect(document.getElementById(section.id)).toHaveTextContent(section.title);
  }
});

it('keeps the developer article out of unknown roles and platform articles out of Developer access', () => {
  render(<UserGuidePage role="UNKNOWN" />);
  expect(screen.getByText('Chưa có hướng dẫn cho vai trò của bạn.')).toBeVisible();
  expect(screen.queryByRole('article')).not.toBeInTheDocument();
  expect(canReadGuide('DEVELOPER', 'platform')).toBe(false);
  expect(canReadGuide('PLATFORM_ENGINEER', 'platform')).toBe(true);
  expect(canReadGuide('ADMIN', 'platform')).toBe(true);
});

it('opens a screenshot by keyboard and restores focus after Escape', async () => {
  const user = userEvent.setup();
  render(<UserGuidePage role="DEVELOPER" />);
  const trigger = screen.getByRole('button', { name: /Xem ảnh: Ngay sau khi tạo/ });
  trigger.focus();
  await user.keyboard('{Enter}');
  expect(screen.getByRole('dialog', { name: /Ngay sau khi tạo/ })).toBeVisible();
  await user.keyboard('{Escape}');
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  expect(trigger).toHaveFocus();
});

it('keeps the text readable and lets the user retry a failed illustration', async () => {
  const user = userEvent.setup();
  render(<UserGuidePage role="DEVELOPER" />);
  const image = screen.getByRole('img', { name: /Ngay sau khi tạo/ });
  fireEvent.error(image);
  expect(screen.getByText('Không tải được ảnh minh họa.')).toBeVisible();
  expect(screen.getByRole('heading', { name: '1. Tạo ứng dụng' })).toBeVisible();
  await user.click(screen.getByRole('button', { name: 'Thử tải lại ảnh' }));
  expect(screen.getByRole('img', { name: /Ngay sau khi tạo/ })).toHaveAttribute('src', '/ui/guides/acceptance-app/new-01-empty.png');
  expect(screen.queryByText('Không tải được ảnh minh họa.')).not.toBeInTheDocument();
});
