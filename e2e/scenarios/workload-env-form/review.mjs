// Read-only UI review of the existing run, in both languages and desktop sizes.
import { expect } from '@playwright/test';
import { mkdir, writeFile } from 'node:fs/promises';
import { readConsoleConfig, openConsole, closeConsole } from '../../lib/console.mjs';
const config = readConsoleConfig();
const app = process.env.ORCH_APP_ID, evidence = process.env.ORCH_EVIDENCE_DIR;
if (!app || !evidence) throw Error('ORCH_APP_ID and outside-Git ORCH_EVIDENCE_DIR required');
await mkdir(evidence, { recursive: true, mode: 0o700 });
const session = await openConsole(config, { role: 'DEVELOPER' });
const { page } = session;
const passed = [];

try {
  await page.goto(`${config.baseURL}/ui/applications/${app}`);
  await page.getByRole('button', { name: 'Sửa', exact: true }).first().click();
  await expect(page.getByLabel('Tên Workload', { exact: true })).toHaveValue('backend');
  for (const locale of ['vi', 'en']) {
    if (locale === 'en') {
      await page.locator('.language-select select').selectOption('en');
      await expect(page.getByRole('heading', { name: 'Basic information', exact: true })).toBeVisible();
    }
    for (const width of [1512,1280]) {
      await page.setViewportSize({ width, height: 980 });
      const region = page.getByRole('region', { name: locale === 'vi' ? 'Biến môi trường của container main' : 'Environment variables of container main', exact: true });
      await expect(region.getByRole('columnheader', { name: locale === 'vi' ? 'Giá trị' : 'Value', exact: true })).toBeVisible();
      await expect(region.getByRole('combobox')).toHaveCount(0);
      const secret = region.locator('tr').filter({ has: page.locator('input[value="ACCEPTANCE_SECRET"]') });
      await expect(secret).toContainText(locale === 'vi' ? 'Ứng dụng / ACCEPTANCE_SECRET' : 'Application / ACCEPTANCE_SECRET');
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth + 1)).toBe(true);
      await region.scrollIntoViewIfNeeded();
      await page.screenshot({ path: `${evidence}/${locale}-${width}-table.png`, fullPage: true });
      await region.getByRole('button', { name: /^(Chọn|Đổi) nguồn, dòng 1$|^(Choose|Change) source, row 1$/ }).click();
      const dialog = page.getByRole('dialog');
      await expect(dialog.getByRole('searchbox')).toBeFocused();
      await expect(dialog.getByRole('button', { name: locale === 'vi' ? 'Ứng dụng / ACCEPTANCE_CONFIG' : 'Application / ACCEPTANCE_CONFIG', exact: true })).toBeVisible();
      expect(await dialog.locator('button').allTextContents()).not.toContain('ENV_UI_UPDATED');
      await page.screenshot({ path: `${evidence}/${locale}-${width}-picker.png` });
      await page.keyboard.press('Escape'); await expect(dialog).not.toBeVisible();
      passed.push(`${locale} ${width}: direct-entry, scoped references, keyboard, no page overflow`);
    }
  }
  await writeFile(`${evidence}/review.json`, JSON.stringify({ passed }, null, 2));
  console.log(`PASS ${passed.length} locale/viewport reviews`);
} finally { await closeConsole(session, config); }
