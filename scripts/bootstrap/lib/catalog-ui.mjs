import { isDeepStrictEqual } from 'node:util';
export async function ensureCatalog({ page, baseURL, act, get, responseFor, source }) {
 const accounts = (await get('/driver-accounts')).accounts;
 const bindings = {};
 for (const [slot, name] of Object.entries(source.accountBindings)) {
  const found = accounts.filter(value => value.name === name || value.key === name);
  if (found.length !== 1) throw Error('Catalog binding cần đúng một tài khoản hạ tầng đã đăng ký');
  bindings[slot] = found[0].key;
 }
 const desired = { ...source, accountBindings: bindings };
 const sources = (await get('/platform/catalog/sources')).sources;
 const existing = sources.find(item => item.id === source.id);
 await page.goto(`${baseURL}/ui/platform/catalog`);
 if (existing) {
  const actual = Object.fromEntries(Object.keys(desired).map(key => [key, key === 'accountBindings' ? existing[key] ?? {} : existing[key]]));
  if (!isDeepStrictEqual(actual, desired)) throw Error('Catalog source đã có khác cấu hình; sửa có chủ ý trong Console trước khi chạy lại');
 } else {
  await act(page.getByLabel('ID nguồn', { exact:true }), 'fill', source.id);
  await act(page.getByLabel('Repository Git', { exact:true }), 'fill', source.repository);
  await act(page.getByLabel('Nhánh theo dõi', { exact:true }), 'fill', source.branch);
  await act(page.locator('form details summary'), 'click');
  await act(page.getByLabel('Bộ lọc tag', { exact:true }), 'fill', source.tagPattern);
  await act(page.getByLabel('Đường dẫn manifest', { exact:true }), 'fill', source.manifestPath);
  await act(page.getByLabel('Bao gồm bản thử nghiệm', { exact:true }), 'setChecked', source.includePrereleases);
  await act(page.getByLabel('Account bindings', { exact:true }), 'fill', Object.entries(bindings).map(([slot,id])=>`${slot}=${id}`).join('\n'));
  const reply = responseFor(`/platform/catalog/sources/${encodeURIComponent(source.id)}`, 'PUT');
  await act(page.getByRole('button', {name:'Lưu',exact:true}), 'click');
  if (!(await reply).ok()) throw Error('Không xác nhận được lưu Catalog; tải lại Console để kiểm tra trước khi chạy lại');
 }
 const row = page.locator('.section-header').filter({has:page.locator('strong').filter({hasText:source.id})});
 const reply = responseFor(`/platform/catalog/sources/${encodeURIComponent(source.id)}/refresh`, 'POST');
 await act(row.getByRole('button',{name:'Kiểm tra cập nhật',exact:true}), 'click');
 const response = await reply;
 if (!response.ok()) throw Error('Catalog refresh thất bại');
 const result = await response.json();
 if (result.source?.code !== 'READY') throw Error('Catalog chưa READY; kiểm tra trạng thái nguồn/release trong Console');
 const {releases} = await get(`/platform/catalog/sources/${encodeURIComponent(source.id)}/releases`);
 if (!releases?.length) throw Error('Chưa có release hợp lệ; push tag SemVer khớp bộ lọc và thuộc nhánh theo dõi');
 console.log(`PASS: Catalog có ${releases.length} release; chọn phiên bản trong Cài đặt môi trường trước khi Preview`);
}
