import { within } from '@testing-library/react';

// Follow the same disclosure interaction as a user before editing hidden fields.
export async function openWorkloadGroup(user: { click(element: Element): Promise<void> }, title: string, root: HTMLElement = document.body) {
  const label = await within(root).findByText(title, { exact: true });
  const group = label.closest('details');
  if (!group) throw new Error(`Missing workload group: ${title}`);
  if (!group.open) await user.click(label);
}
