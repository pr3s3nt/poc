// Stable role/label locators for refactor scenarios. Each page group has one
// table of logical keys, and each page has its own group; an entry names the Vietnamese label (`vi`) and the
// label the page shows today (`current`). A group switches to `vi` only when
// the task that translates that page flips its entry in TRANSLATED, so a
// scenario never guesses a language and the product has no English mode.
import { expect } from '@playwright/test';

export const TRANSLATED = {
  signIn: true,
  shell: true,
  applications: true,
  resourceTypes: true,
  resourceDefinitions: true,
  renderingTemplates: true,
  connections: true,
  secretStores: true,
  workloadEditor: true,
};

const TABLES = {
  signIn: {
    username: { role: 'label', vi: 'Tên đăng nhập', current: 'Tên đăng nhập' },
    password: { role: 'label', vi: 'Mật khẩu', current: 'Mật khẩu' },
    submit: { role: 'button', vi: 'Đăng nhập', current: 'Đăng nhập' },
  },
  shell: {
    signOut: { role: 'button', vi: 'Đăng xuất', current: 'Đăng xuất' },
    nav: { role: 'navigation', vi: 'Điều hướng chính', current: 'Điều hướng chính' },
    navApplications: { role: 'link', vi: 'Ứng dụng', current: 'Ứng dụng' },
    navResourceTypes: { role: 'link', vi: 'Loại tài nguyên', current: 'Loại tài nguyên' },
    navResourceDefinitions: { role: 'link', vi: 'Cấu hình tài nguyên', current: 'Cấu hình tài nguyên' },
    navRenderingTemplates: { role: 'link', vi: 'Mẫu dựng ứng dụng', current: 'Mẫu dựng ứng dụng' },
    navConnections: { role: 'link', vi: 'Kết nối', current: 'Kết nối' },
    navSecretStores: { role: 'link', vi: 'Kho bí mật', current: 'Kho bí mật' },
  },
  applications: {
    heading: { role: 'heading', level: 1, vi: 'Ứng dụng của bạn', current: 'Ứng dụng của bạn' },
    create: { role: 'button', vi: '+ Tạo ứng dụng', current: '+ Tạo ứng dụng' },
    loading: { role: 'status', vi: 'Đang tải ứng dụng', current: 'Đang tải ứng dụng' },
    emptyHeading: { role: 'heading', level: 2, vi: 'Bắt đầu với ứng dụng mới', current: 'Bắt đầu với ứng dụng mới' },
    retry: { role: 'button', vi: 'Thử lại', current: 'Thử lại' },
  },
  // T04 translated this page; `current` equals `vi` for its entries.
  resourceTypes: {
    heading: { role: 'heading', level: 1, vi: 'Loại tài nguyên', current: 'Loại tài nguyên' },
    id: { role: 'label', vi: 'ID loại tài nguyên', current: 'ID loại tài nguyên' },
    submit: { role: 'button', vi: 'Đăng ký loại tài nguyên', current: 'Đăng ký loại tài nguyên' },
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
  // T06 translated this page; `current` equals `vi` for its entries.
  connections: {
    heading: { role: 'heading', level: 1, vi: 'Kết nối', current: 'Kết nối' },
    name: { role: 'label', vi: 'Tên kết nối', current: 'Tên kết nối' },
    sourceUpload: { role: 'label', vi: 'Tải lên file kubeconfig', current: 'Tải lên file kubeconfig' },
    sourcePaste: { role: 'label', vi: 'Dán nội dung kubeconfig', current: 'Dán nội dung kubeconfig' },
    file: { role: 'label', vi: 'File kubeconfig', current: 'File kubeconfig' },
    content: { role: 'label', vi: 'Nội dung kubeconfig', current: 'Nội dung kubeconfig' },
    showContent: { role: 'label', vi: 'Hiện nội dung', current: 'Hiện nội dung' },
    context: { role: 'label', wrapsOptions: true, vi: 'Context', current: 'Context' },
    retryInspect: { role: 'button', vi: 'Kiểm tra lại kubeconfig', current: 'Kiểm tra lại kubeconfig' },
    submit: { role: 'button', vi: 'Kiểm tra và lưu', current: 'Kiểm tra và lưu' },
    retry: { role: 'button', vi: 'Thử lại', current: 'Thử lại' },
  },
  // T08 translated the Workload editor; `current` equals `vi` for its entries.
  workloadEditor: {
    name: { role: 'label', vi: 'Tên Workload', current: 'Tên Workload' },
    image: { role: 'label', vi: 'Ảnh Container', current: 'Ảnh Container' },
    formTab: { role: 'button', vi: 'Nhập bằng form', current: 'Nhập bằng form' },
    importTab: { role: 'button', vi: 'Nhập Score', current: 'Nhập Score' },
    scoreFile: { role: 'label', vi: 'File Score', current: 'File Score' },
    cpuRequest: { role: 'label', vi: 'Yêu cầu CPU', current: 'Yêu cầu CPU' },
    cpuLimit: { role: 'label', vi: 'Giới hạn CPU', current: 'Giới hạn CPU' },
    addPort: { role: 'button', vi: '+ Thêm cổng', current: '+ Thêm cổng' },
    portName: { role: 'label', vi: 'Tên cổng Service', current: 'Tên cổng Service' },
    portNumber: { role: 'label', vi: 'Cổng Service', current: 'Cổng Service' },
    targetPort: { role: 'label', vi: 'Cổng đích của Container', current: 'Cổng đích của Container' },
    addRoute: { role: 'button', vi: '+ Thêm đường dẫn công khai', current: '+ Thêm đường dẫn công khai' },
    routePath: { role: 'label', vi: 'Đường dẫn công khai', current: 'Đường dẫn công khai' },
    routePort: { role: 'label', vi: 'Cổng Service công khai', current: 'Cổng Service công khai' },
    addResource: { role: 'button', vi: '+ Thêm tài nguyên', current: '+ Thêm tài nguyên' },
    resourceAlias: { role: 'label', vi: 'Alias tài nguyên', current: 'Alias tài nguyên' },
    resourceType: { role: 'label', vi: 'Loại tài nguyên', current: 'Loại tài nguyên' },
    resourceClass: { role: 'label', vi: 'Lớp tài nguyên', current: 'Lớp tài nguyên' },
    save: { role: 'button', vi: 'Lưu draft Workload', current: 'Lưu draft Workload' },
    cancel: { role: 'button', vi: 'Hủy', current: 'Hủy' },
  },
  // T05 translated this page; `current` equals `vi` for its entries.
  secretStores: {
    heading: { role: 'heading', level: 1, vi: 'Kho bí mật', current: 'Kho bí mật' },
    name: { role: 'label', vi: 'Tên kho', current: 'Tên kho' },
    backend: { role: 'label', vi: 'Địa chỉ backend', current: 'Địa chỉ backend' },
    sameAddress: { role: 'label', vi: 'Dùng cùng địa chỉ backend', current: 'Dùng cùng địa chỉ backend' },
    workload: { role: 'label', vi: 'Địa chỉ workload', current: 'Địa chỉ workload' },
    token: { role: 'label', vi: 'Token Vault', current: 'Token Vault' },
    showAdvanced: { role: 'button', vi: 'Hiện nâng cao', current: 'Hiện nâng cao' },
    hideAdvanced: { role: 'button', vi: 'Ẩn nâng cao', current: 'Ẩn nâng cao' },
    mount: { role: 'label', vi: 'Mount KV v2', current: 'Mount KV v2' },
    authMount: { role: 'label', vi: 'Mount xác thực Kubernetes', current: 'Mount xác thực Kubernetes' },
    ca: { role: 'label', vi: 'Chứng chỉ CA tùy chỉnh (PEM, tùy chọn)', current: 'Chứng chỉ CA tùy chỉnh (PEM, tùy chọn)' },
    submit: { role: 'button', vi: 'Xác minh và đăng ký', current: 'Xác minh và đăng ký' },
    retry: { role: 'button', vi: 'Thử lại', current: 'Thử lại' },
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
