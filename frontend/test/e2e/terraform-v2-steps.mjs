// Current shared acceptance form helpers. Legacy Environment/profile authoring
// and retired scenarios are preserved only in historical Git/verification records.
import { expect } from '@playwright/test';
export const SHORT_READ = 2000;
export const FIXTURE_PASSWORD = 'test-password'; // isolated synthetic account; masked input

export function decodeCatalogOverride(encoded, substitutions = {}) {
  const expanded = Object.entries(substitutions).reduce((value, [key, replacement]) => value.replaceAll(`@${key}@`, replacement), encoded);
  return JSON.parse(expanded);
}

export function formOverrideText(value) {
  return typeof value === 'string' ? value : JSON.stringify(value);
}

// Boolean capability inputs are native selects. Other typed controls accept
// their raw textual or JSON representation through a paste-like insert.
export async function applyOverrideValue(h, field, value) {
  if (typeof value === 'boolean') {
    await h.choose(field, String(value));
    return;
  }
  await h.paste(field, formOverrideText(value));
}

export function steps({ page, h, caption, rec }) {
  const nav = (name) => page.getByRole('link', { name: new RegExp(`(^|\\s)${name}$`) });
  async function registerSecretStore(store, token) {
    caption('Kỹ sư nền tảng vào "Kho bí mật" và đăng ký Vault cấu hình hiện có: địa chỉ backend, địa chỉ workload và token có phạm vi (dán vào ô mật khẩu, luôn bị che)');
    await h.click(nav('Kho bí mật'));
    await expect(page.getByRole('heading', { name: 'Kho bí mật', level: 1 })).toBeVisible();
    await h.pause(SHORT_READ);
    await h.type(page.getByLabel('Tên kho', { exact: true }), store.name);
    await h.type(page.getByRole('textbox', { name: 'Địa chỉ backend' }), store.backendAddress);
    await h.type(page.getByLabel('Địa chỉ workload', { exact: true }), store.workloadAddress);
    await h.click(page.getByRole('button', { name: 'Hiện nâng cao' }));
    await h.type(page.getByLabel('Mount KV v2', { exact: true }), store.mount, { replace: true });
    await h.type(page.getByLabel('Mount xác thực Kubernetes', { exact: true }), store.authMount, { replace: true });
    await h.paste(page.getByLabel('Token Vault', { exact: true }), token);
    await expect(page.getByLabel('Token Vault', { exact: true })).toHaveAttribute('type', 'password');
    expect(await page.locator('body').innerText(), 'the Vault token must not be visible').not.toContain(token);
    const registered = page.waitForResponse((r) => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/secret-stores', { timeout: 120_000 });
    await h.click(page.getByRole('button', { name: 'Xác minh và đăng ký' }));
    const response = await registered;
    const text = await response.text();
    if (text.includes(token)) throw new Error('the registration response echoed the Vault token');
    if (response.status() !== 201) {
      let diagnostic = `HTTP ${response.status()}`;
      try {
        const body = JSON.parse(text);
        if (typeof body.code === 'string' && /^[A-Z][A-Z0-9_]{0,63}$/.test(body.code)) diagnostic += ` code=${body.code}`;
        if (typeof body.field === 'string' && /^[A-Za-z][A-Za-z0-9_.-]{0,63}$/.test(body.field)) diagnostic += ` field=${body.field}`;
      } catch { /* retain only the status when the response is not the fixed public error shape */ }
      throw new Error(`registering the secret store was refused (${diagnostic})`);
    }
    const body = JSON.parse(text);
    const key = body.key ?? body.secretStore?.key;
    if (!key) throw new Error('the registered secret store has no key');
    await expect(page.getByRole('status')).toContainText(`"${store.name}" đã sẵn sàng`);
    const row = page.locator('.settings-table-row').filter({ hasText: store.name });
    await expect(row).toContainText('Sẵn sàng');
    await h.moveTo(row);
    await h.pause(SHORT_READ);
    expect(await page.locator('body').innerText(), 'the Vault token must not be visible after registration').not.toContain(token);
    expect(await page.getByLabel('Token Vault', { exact: true }).inputValue(), 'the token field is cleared').toBe('');
    rec.record('ui.secret-store.ready', 'FORM', 'ui', true, `secret store ${key} READY; token masked and never echoed`);
    return key;
  }

  return { registerSecretStore };
}
