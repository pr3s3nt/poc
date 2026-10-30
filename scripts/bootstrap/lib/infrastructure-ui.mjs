import { expect } from '@playwright/test';
import { readPrivateFile } from './console.mjs';
import { assertAccountTarget, desiredInfrastructureDefinition } from './infrastructure-config.mjs';
import { assertSameDefinition } from './render-bundle-config.mjs';

export async function ensureInfrastructureAccount({ page, baseURL, account, act, get, responseFor, kubeconfigFile }) {
  if (!account) return;
  const { accounts } = await get('/driver-accounts');
  if (!Array.isArray(accounts)) throw new Error('Danh sách accounts không hợp lệ');
  const matches = accounts.filter(item => item.name === account.name);
  if (matches.length > 1) throw new Error('Có nhiều account cùng tên; cần kiểm tra Console');
  if (matches.length) {
    assertAccountTarget(matches[0], account);
    console.log(`PASS: account ${account.name} READY và đúng target; không đọc/upload kubeconfig`);
    return matches[0];
  }
  const file = kubeconfigFile || account.kubeconfigFile;
  if (!file) throw new Error('Account chưa có; cần ORCH_KUBECONFIG_FILE hoặc account.kubeconfigFile');
  const content = readPrivateFile(file, 'kubeconfig');
  const secrets = [...content.matchAll(/^\s*(?:token|client-key-data):\s*(\S+)\s*$/gm)].map(match => match[1].replace(/^["']|["']$/g, '')).filter(value => value.length > 12);
  const leaks = text => secrets.some(value => text.includes(value));
  console.log(`UI: đăng ký account ${account.name} bằng upload kubeconfig`);
  await page.goto(`${baseURL}/ui/platform/driver-accounts`);
  const form = page.getByRole('group', { name: 'Đăng ký tài khoản mới', exact: true });
  await act(form.getByLabel('Tên tài khoản', { exact: true }), 'fill', account.name);
  const inspectReply = responseFor('/driver-accounts/internal/inspect', 'POST');
  await act(form.locator('input[type="file"]'), 'setInputFiles', file);
  const inspected = await inspectReply;
  if (!inspected.ok()) throw new Error(`Kubeconfig inspect HTTP ${inspected.status()}`);
  const { contexts } = await inspected.json();
  if (!Array.isArray(contexts) || !contexts.some(context => context.name === account.context && context.endpoint === account.endpoint)) throw new Error('Kubeconfig không có đúng context/endpoint đã cấu hình; không đăng ký');
  if (contexts.length > 1) await act(form.getByLabel('Context', { exact: true }), 'selectOption', account.context);
  await expect(form.getByLabel('Đích đã chọn', { exact: true })).toContainText(account.endpoint);
  const reply = responseFor('/driver-accounts/internal', 'POST');
  await act(form.getByRole('button', { name: 'Kiểm tra và lưu', exact: true }), 'click');
  const saved = await reply;
  if (leaks(await saved.text()) || leaks(await page.locator('body').innerText())) throw new Error('Phát hiện credential bị hiển thị; dừng');
  if (!saved.ok()) throw new Error(`Account registration HTTP ${saved.status()}; kiểm tra Console trước khi chạy lại`);
  const persisted = (await get('/driver-accounts')).accounts.find(item => item.name === account.name);
  if (!persisted) throw new Error('Không tìm thấy account sau đăng ký');
  assertAccountTarget(persisted, account);
  console.log(`PASS: account ${account.name} READY, đúng endpoint/context`);
  return persisted;
}

async function chooseSource({ page, section, act }, name, source) {
  const card = section.locator('.source-card').filter({ has: page.locator('strong').filter({ hasText: new RegExp(`^${name.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}$`) }) });
  for (const details of await card.locator('xpath=ancestor::details').all()) {
    if (await details.getAttribute('open') === null) await act(details.locator(':scope > summary'), 'click');
  }
  await act(card.getByRole('button'), 'click');
  const dialog = page.getByRole('dialog', { name: 'Chọn nguồn giá trị', exact: true });
  if (source.context) {
    await act(dialog.getByRole('radio', { name: 'Lấy từ ngữ cảnh triển khai', exact: true }), 'check');
    await act(dialog.getByLabel('Giá trị ngữ cảnh', { exact: true }), 'selectOption', source.context);
  } else if (source.resource) {
    await act(dialog.getByRole('radio', { name: 'Lấy từ tài nguyên khác', exact: true }), 'check');
    for (const [label, value] of [['Loại tài nguyên nguồn', source.resource.type], ['Nhóm cấu hình nguồn', source.resource.class], ['Phạm vi nguồn', source.resource.scope], ['Output cần lấy', source.resource.output]]) await act(dialog.getByLabel(label, { exact: true }), 'selectOption', value);
  } else {
    await act(dialog.getByRole('radio', { name: 'Nhập giá trị', exact: true }), 'check');
    const input = dialog.getByLabel(`Giá trị ${name}`, { exact: true });
    await act(input, typeof source.literal === 'boolean' ? 'selectOption' : 'fill', String(source.literal));
  }
  await act(dialog.getByRole('button', { name: 'Áp dụng', exact: true }), 'click');
}

async function applyCriteria(section, criteria, act) {
  await act(section.getByRole('tab', { name: /^Điều kiện áp dụng/ }), 'click');
  const labels = { env_type: 'Loại môi trường', app_id: 'ID ứng dụng', env_id: 'ID môi trường', res_id: 'ID tài nguyên', class: 'Class' };
  for (const [index, row] of criteria.entries()) {
    if (index > 0) await act(section.getByRole('button', { name: '+ Thêm trường hợp áp dụng', exact: true }), 'click');
    for (const [position, [key, value]] of Object.entries(row).entries()) {
      const matchingCase = section.locator('.criteria-case').nth(index);
      await act(matchingCase.getByRole('button', { name: '+ Thêm điều kiện', exact: true }), 'click');
      await act(section.getByLabel(`Trường hợp ${index + 1} thuộc tính ${position + 1}`, { exact: true }), 'selectOption', key);
      await act(section.getByLabel(`Điều kiện ${index + 1} ${labels[key]}`, { exact: true }), 'fill', value);
    }
  }
}

export async function ensureResourceDefinitions({ page, baseURL, definitions, act, get, responseFor }) {
  for (const item of definitions) {
    console.log(`UI: kiểm tra Resource Definition ${item.key}`);
    const accounts = (await get('/driver-accounts')).accounts;
    const account = item.driverType === 'existing-cluster' ? accounts.find(account => account.name === item.accountName && account.type === 'INTERNAL' && account.status === 'READY') : undefined;
    if (item.driverType === 'existing-cluster' && !account) throw new Error(`${item.key}: chưa có Internal account READY ${item.accountName}`);
    let capability;
    if (item.module) {
      const caps = (await get('/resource-definition-capabilities')).capabilities.filter(cap => cap.resourceType === item.resourceType && cap.driverType === 'terraform' && cap.module === item.module && cap.source?.sourceId === 'platform');
      if (caps.length !== 1) throw new Error(`${item.key}: installed module không duy nhất hoặc không có`);
      capability = caps[0];
    }
    const existing = (await get('/resource-definitions')).resourceDefinitions.find(def => def.key === item.key);
    if (existing && !item.source) {
      assertSameDefinition(existing, desiredInfrastructureDefinition(item, capability, account));
      console.log(`PASS: dùng lại Definition ${item.key}, toàn bộ cấu hình khớp`);
      continue;
    }
    await page.goto(`${baseURL}/ui/platform/resource-definitions`);
    await act(page.getByRole('button', { name: 'Tạo cấu hình', exact: true }), 'click');
    const section = page.locator('.definition-detail:visible');
    await act(section.getByLabel('ID cấu hình (Definition ID)', { exact: true }), 'fill', item.key);
    await act(section.getByLabel('Tên hiển thị', { exact: true }), 'fill', item.name);
    await act(section.getByRole('combobox', { name: 'Loại tài nguyên', exact: true }), 'selectOption', item.resourceType);
    if (item.driverType === 'existing-cluster') {
      await act(section.getByRole('combobox', { name: 'Cách cung cấp cluster', exact: true }), 'selectOption', 'existing-cluster');
      await act(section.getByLabel('Tài khoản hạ tầng', { exact: true }), 'selectOption', account.key);
    } else {
      if (item.source) {
        await act(section.getByLabel('Git HTTPS URL', { exact: true }), 'fill', item.source.url);
        await act(section.getByLabel('Thư mục module', { exact: true }), 'fill', item.source.path);
        await act(section.getByLabel('Tag hoặc commit', { exact: true }), 'fill', item.source.ref);
        const reply = responseFor('/terraform-modules/inspect', 'POST');
        await act(section.getByRole('button', { name: 'Kiểm tra module', exact: true }), 'click');
        const inspected = await reply;
        if (!inspected.ok()) throw new Error(`${item.key}: module inspect HTTP ${inspected.status()}`);
        capability = (await inspected.json()).capability;
        if (capability?.source?.repositoryUrl !== item.source.url || capability.source.subdirectory !== item.source.path || capability.source.revision !== item.source.ref) throw new Error(`${item.key}: module Git pin không khớp`);
      } else {
        await act(section.getByRole('button', { name: 'Module đã đăng ký', exact: true }), 'click');
        const select = section.getByRole('combobox', { name: 'Module đã kiểm chứng', exact: true });
        const value = [capability.resourceType, capability.driverType, capability.module, capability.source.sourceId, capability.source.revision, capability.source.subdirectory, capability.source.contentSha256].join('|');
        if (await select.isEnabled()) await act(select, 'selectOption', value);
        await expect(select).toHaveValue(value);
      }
      if (capability.account.required || !capability.providerRequirements.some(provider => provider.binding === 'kubernetes') || capability.providerRequirements.some(provider => provider.binding === 'aws')) throw new Error('Chỉ hỗ trợ Kubernetes Terraform qua resource binding, không AWS');
      const desired = desiredInfrastructureDefinition(item, capability, account);
      if (existing) {
        assertSameDefinition(existing, desired);
        console.log(`PASS: dùng lại Definition ${item.key}, Git pin/cấu hình khớp`);
        continue;
      }
      const advanced = section.locator('details.definition-advanced');
      if (await advanced.getAttribute('open') === null) await act(advanced.locator(':scope > summary'), 'click');
      for (const [param, variable] of Object.entries(item.inputMapping)) await act(section.getByLabel(`Biến module nhận tham số ${param}`, { exact: true }), 'selectOption', variable);
      for (const [output, moduleOutput] of Object.entries(item.outputMapping)) await act(section.getByLabel(`Output module cung cấp ${output}`, { exact: true }), 'selectOption', moduleOutput);
      for (const provider of capability.providerRequirements.filter(provider => provider.binding === 'kubernetes')) await act(section.getByLabel(`Nhóm cluster của provider ${provider.name}`, { exact: true }), 'selectOption', item.clusterClass);
      for (const [name, source] of Object.entries(item.variables)) await chooseSource({ page, section, act }, name, source);
    }
    const desired = desiredInfrastructureDefinition(item, capability, account);
    await applyCriteria(section, item.criteria, act);
    const reply = responseFor('/resource-definitions', 'POST');
    await act(section.getByRole('button', { name: 'Đăng ký cấu hình tài nguyên', exact: true }), 'click');
    const saved = await reply;
    if (!saved.ok()) throw new Error(`${item.key}: Definition registration HTTP ${saved.status()}; không tự retry POST`);
    const persisted = (await get('/resource-definitions')).resourceDefinitions.find(def => def.key === item.key);
    if (!persisted) throw new Error(`${item.key}: không tìm thấy sau registration`);
    assertSameDefinition(persisted, desired);
    console.log(`PASS: đăng ký Definition ${item.key}, full config/pin/mappings khớp`);
  }
}
