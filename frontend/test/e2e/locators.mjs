// Stable role/label locators for refactor scenarios. Each page group has one
// table of logical keys, and each page has its own group; an entry names the Vietnamese label (`vi`) and the
// label the page shows today (`current`). A group switches to `vi` only when
// the task that translates that page flips its entry in TRANSLATED, so a
// scenario never guesses a language and the product has no English mode.
import { expect } from '@playwright/test';

export const TRANSLATED = {
  signIn: false,
  shell: false,
  applications: false,
  resourceTypes: false,
  resourceDefinitions: true,
  renderingTemplates: true,
  connections: false,
  secretStores: false,
};

const TABLES = {
  signIn: {
    username: { role: 'label', vi: 'Tên đăng nhập', current: 'Username' },
    password: { role: 'label', vi: 'Mật khẩu', current: 'Password' },
    submit: { role: 'button', vi: 'Đăng nhập', current: 'Sign in' },
  },
  shell: {
    signOut: { role: 'button', vi: 'Đăng xuất', current: 'Sign out' },
    nav: { role: 'navigation', vi: 'Điều hướng chính', current: 'Main navigation' },
    navApplications: { role: 'link', vi: 'Ứng dụng', current: 'Applications' },
    navResourceTypes: { role: 'link', vi: 'Loại tài nguyên', current: 'Resource types' },
    navResourceDefinitions: { role: 'link', vi: 'Cấu hình tài nguyên', current: 'Resource definitions' },
    navRenderingTemplates: { role: 'link', vi: 'Mẫu dựng ứng dụng', current: 'Mẫu dựng ứng dụng' },
    navConnections: { role: 'link', vi: 'Kết nối', current: 'Connections' },
    navSecretStores: { role: 'link', vi: 'Kho bí mật', current: 'Secret stores' },
  },
  applications: {
    heading: { role: 'heading', level: 1, vi: 'Ứng dụng của bạn', current: 'Your applications' },
  },
  resourceTypes: {
    heading: { role: 'heading', level: 1, vi: 'Loại tài nguyên', current: 'Resource types' },
  },
  // T02 translated this page; `current` equals `vi` for its entries.
  resourceDefinitions: {
    heading: { role: 'heading', level: 1, vi: 'Cấu hình tài nguyên', current: 'Resource definitions' },
    id: { role: 'label', vi: 'ID cấu hình', current: 'ID cấu hình' },
    type: { role: 'label', wrapsOptions: true, vi: 'Loại tài nguyên', current: 'Loại tài nguyên' },
    driver: { role: 'label', wrapsOptions: true, vi: 'Cách tạo', current: 'Cách tạo' },
    scope: { role: 'label', wrapsOptions: true, vi: 'Phạm vi triển khai', current: 'Phạm vi triển khai' },
    awsConnection: { role: 'label', wrapsOptions: true, vi: 'Kết nối AWS', current: 'Kết nối AWS' },
    kubeConnection: { role: 'label', wrapsOptions: true, vi: 'Kết nối riêng cho cluster (tùy chọn)', current: 'Kết nối riêng cho cluster (tùy chọn)' },
    variables: { role: 'label', wrapsOptions: true, vi: 'Tham số cấu hình (JSON object)', current: 'Tham số cấu hình (JSON object)' },
    criterionClass: { role: 'label', vi: 'Điều kiện 1 Class', current: 'Điều kiện 1 Class' },
    showAdvanced: { role: 'button', vi: 'Hiện nâng cao', current: 'Hiện nâng cao' },
    hideAdvanced: { role: 'button', vi: 'Ẩn nâng cao', current: 'Ẩn nâng cao' },
    submit: { role: 'button', vi: 'Đăng ký cấu hình tài nguyên', current: 'Đăng ký cấu hình tài nguyên' },
  },
  // T03 page is Vietnamese from the start; `current` equals `vi`.
  renderingTemplates: {
    heading: { role: 'heading', level: 1, vi: 'Mẫu dựng ứng dụng', current: 'Mẫu dựng ứng dụng' },
    id: { role: 'label', vi: 'ID mẫu', current: 'ID mẫu' },
    bundle: { role: 'label', vi: 'ID bundle dựng ứng dụng', current: 'ID bundle dựng ứng dụng' },
    submit: { role: 'button', vi: 'Đăng ký mẫu dựng ứng dụng', current: 'Đăng ký mẫu dựng ứng dụng' },
  },
  connections: {
    heading: { role: 'heading', level: 1, vi: 'Kết nối', current: 'Connections' },
  },
  secretStores: {
    heading: { role: 'heading', level: 1, vi: 'Kho bí mật', current: 'Secret stores' },
  },
};

export function labelFor(group, key) {
  const entry = TABLES[group]?.[key];
  if (!entry) throw new Error(`unknown locator ${group}.${key}`);
  return TRANSLATED[group] ? entry.vi : entry.current;
}

// Accessible-name locator: exact match (links: label at the end of the name).
export function locate(page, group, key) {
  const entry = TABLES[group]?.[key];
  if (!entry) throw new Error(`unknown locator ${group}.${key}`);
  const name = labelFor(group, key);
  // A label that wraps a <select> also contains the option texts, so Playwright
  // cannot match it exactly; those entries use a substring match (`wrapsOptions`).
  if (entry.role === 'label') return page.getByLabel(name, { exact: !entry.wrapsOptions });
  // A nav link's name starts with a decorative glyph ("◇ Resource types"), so
  // it matches the label as the last words of the name.
  const options = { name: entry.role === 'link' ? new RegExp(`(^|\\s)${name.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}$`) : name, exact: true };
  if (entry.level) options.level = entry.level;
  return page.getByRole(entry.role, options);
}

export function locators(page) {
  return (group, key) => locate(page, group, key);
}

// Fails the scenario when a table entry has no Vietnamese label, so a page
// cannot be marked translated with an empty entry.
export function assertTablesComplete() {
  for (const [group, table] of Object.entries(TABLES)) {
    for (const [key, entry] of Object.entries(table)) {
      expect(entry.vi, `${group}.${key} vi`).toBeTruthy();
      expect(entry.current, `${group}.${key} current`).toBeTruthy();
    }
  }
}
