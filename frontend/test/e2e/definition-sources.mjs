// Drives the shared "Chọn nguồn giá trị" chooser of the Resource Definition form (UC-03 UI11-02) with real
// browser input: open the parameter card, pick the source, apply. The catalog flags `presetName` and
// `presetNamespace` of the acceptance catalog keep their meaning; they now name explicit source choices.
import { expect } from '@playwright/test';
import { applyOverrideValue } from './terraform-v2-steps.mjs';

const escape = (text) => text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
export const NAMESPACE_CONTEXT = { context: 'env.namespace' };
export const NAMESPACE_OUTPUT = { resource: { type: 'k8s-namespace', class: 'default', scope: 'environments.@app.@env', output: 'name' } };

async function openChooser({ page, h }, name) {
  await h.click(page.getByRole('button', { name: new RegExp(`^(Cung cấp giá trị|Thay đổi) ${escape(name)}$`) }));
  const dialog = page.getByRole('dialog', { name: 'Chọn nguồn giá trị' });
  await expect(dialog).toBeVisible();
  return dialog;
}

/** source: { context } | { resource: { type, class, scope, output } } | { literal } */
export async function provideSource(kit, name, source) {
  const { h } = kit;
  const dialog = await openChooser(kit, name);
  if (source.context !== undefined) {
    await h.click(dialog.getByRole('radio', { name: 'Lấy từ ngữ cảnh triển khai' }));
    await dialog.getByLabel('Giá trị ngữ cảnh', { exact: true }).selectOption(source.context);
  } else if (source.resource !== undefined) {
    const { type, class: klass, scope, output } = source.resource;
    await h.click(dialog.getByRole('radio', { name: 'Lấy từ tài nguyên khác' }));
    await dialog.getByLabel('Loại tài nguyên nguồn', { exact: true }).selectOption(type);
    await dialog.getByLabel('Nhóm cấu hình nguồn', { exact: true }).selectOption(klass);
    await dialog.getByLabel('Phạm vi nguồn', { exact: true }).selectOption(scope);
    await dialog.getByLabel('Output cần lấy', { exact: true }).selectOption(output);
  } else {
    await h.click(dialog.getByRole('radio', { name: 'Nhập giá trị' }));
    await applyOverrideValue(h, dialog.getByLabel(`Giá trị ${name}`, { exact: true }), source.literal);
  }
  await h.click(dialog.getByRole('button', { name: 'Áp dụng', exact: true }));
  await expect(dialog).toBeHidden();
}

/** The acceptance catalog flags as explicit source choices. */
export async function applyCatalogSources(kit, { presetName, presetNamespace }) {
  if (presetName) await provideSource(kit, presetName, NAMESPACE_CONTEXT);
  if (presetNamespace) await provideSource(kit, 'namespace', NAMESPACE_OUTPUT);
}
