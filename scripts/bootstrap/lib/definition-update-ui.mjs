import { assertSameDefinition } from './render-bundle-config.mjs';

export function validateUpdatePayload(body, existing, desired) {
  if (body.version !== existing.version) throw Error(`${desired.key}: version UI đã đổi: Console=${body.version}; đã đọc=${existing.version}; dừng và đọc lại trước khi chạy lại`);
  try { assertSameDefinition(body, desired); } catch (error) {
    throw Error(`${desired.key}: payload UI không khớp cấu hình đã kiểm tra; không gửi cập nhật. ${error.message}`);
  }
}

// Keep the browser form's normal PUT/CAS flow, intercept only to validate the
// exact payload before forwarding it. A mismatch never reaches the backend.
export async function saveDefinitionUpdate({ page, baseURL, act, button, existing, desired }) {
  const url = `${baseURL}/api/v1/resource-definitions/${encodeURIComponent(desired.key)}`;
  let blocked;
  const guard = async route => {
    if (route.request().method() !== 'PUT') return route.fallback();
    try { validateUpdatePayload(route.request().postDataJSON(), existing, desired); } catch (error) {
      blocked = error;
      return route.fulfill({ status: 422, contentType: 'application/json', body: JSON.stringify({ code: 'BOOTSTRAP_UPDATE_BLOCKED' }) });
    }
    await route.fallback();
  };
  await page.route(url, guard);
  try {
    const [response] = await Promise.all([
      page.waitForResponse(reply => reply.url() === url && reply.request().method() === 'PUT', { timeout: 180_000 }),
      act(button, 'click'),
    ]);
    if (blocked) throw blocked;
    if (!response.ok()) throw Error(`${desired.key}: update HTTP ${response.status()}; không tự retry, đọc lại cấu hình trước khi chạy lại`);
  } finally { await page.unroute(url, guard); }
}

export async function updateRenderDefinitionSource({ page, baseURL, act, existing, desired, bundleId }) {
  await act(page.getByRole('button', { name: `Sửa mẫu ${desired.key}`, exact: true }), 'click');
  const editor = page.getByRole('form', { name: 'Sửa mẫu dựng ứng dụng', exact: true });
  await act(editor.getByRole('button', { name: 'Bundle đã xác minh', exact: true }), 'click');
  await act(editor.locator('select[aria-describedby="template-bundle-hint"]'), 'selectOption', bundleId);
  await saveDefinitionUpdate({ page, baseURL, act, button: editor.getByRole('button', { name: 'Lưu thay đổi', exact: true }), existing, desired });
}
