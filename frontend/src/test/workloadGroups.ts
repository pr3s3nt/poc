import { within, waitFor } from '@testing-library/react';

export async function openAdvancedFields(user: { click(element: Element): Promise<void> }, root: HTMLElement = document.body) {
  await waitFor(()=>{if(!root.querySelector('button[data-workload-advanced]'))throw new Error('Advanced fields are not ready');});
  for (const button of root.querySelectorAll<HTMLButtonElement>('button[data-workload-advanced][aria-expanded="false"]')) await user.click(button);
}

// Follow the same disclosure interaction as a user before editing hidden fields.
export async function openWorkloadGroup(user: { click(element: Element): Promise<void> }, title: string, root: HTMLElement = document.body) {
  await openAdvancedFields(user,root);
  const label = await within(root).findByText(title, { exact: true });
  const group = label.closest('details');
  if (!group) throw new Error(`Missing workload group: ${title}`);
  if (!group.open) await user.click(label);
}
