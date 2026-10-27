// Opens the configured Console and signs in; changes nothing in the platform.
// Headed (default): keeps the window open until the user closes it.
// Optional ORCH_EXPECT_ROLE=PLATFORM_ENGINEER|DEVELOPER checks the account role.
import { closeConsole, openConsole, readConsoleConfig } from '../../lib/console.mjs';

let session;
let config;
try {
  config = readConsoleConfig();
  console.log(`Console: ${config.baseURL}`);
  session = await openConsole(config, { role: process.env.ORCH_EXPECT_ROLE || undefined });
  const { user, page, browser } = session;
  console.log(`PASS: đã đăng nhập ${user.username} (${user.role}), tổ chức ${user.organization}`);
  if (!config.headless) {
    console.log('Đóng cửa sổ trình duyệt để kết thúc.');
    await Promise.race([page.waitForEvent('close', { timeout: 0 }), new Promise((resolve) => browser.once('disconnected', resolve))]);
  }
} catch (error) {
  console.error(`FAIL: ${error.message}`);
  process.exitCode = 1;
} finally {
  if (session) await closeConsole(session, config);
}
