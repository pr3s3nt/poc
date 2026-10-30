import { readFileSync } from 'node:fs';
import { homedir } from 'node:os';
import { join } from 'node:path';

// Console credentials remain private files; tracked presets contain no password.
export function loadTarget(options, env = process.env, home = homedir()) {
  if (options.target && options.config) throw Error('Chọn --target hoặc --config, không dùng cả hai');
  if (!options.target) {
    if (!options.config) throw Error('Cần --target dev|staging hoặc --config /path/catalog.json');
    return { raw: JSON.parse(readFileSync(options.config, 'utf8')), env: { ...env, ...(options.kubeconfig ? { ORCH_KUBECONFIG_FILE: options.kubeconfig } : {}) } };
  }
  if (!['dev', 'staging'].includes(options.target)) throw Error('--target chỉ nhận dev hoặc staging');
  const preset = JSON.parse(readFileSync(new URL(`../targets/${options.target}.json`, import.meta.url), 'utf8'));
  const privateDir = join(home, '.orch', 'bootstrap', options.target);
  return { raw: preset.seed, env: {
    ...env,
    ORCH_PLATFORM_URL: env.ORCH_PLATFORM_URL ?? preset.console.url,
    ORCH_PLATFORM_USERNAME: env.ORCH_PLATFORM_USERNAME ?? preset.console.username,
    ORCH_PLATFORM_PASSWORD_FILE: env.ORCH_PLATFORM_PASSWORD_FILE ?? join(privateDir, 'password'),
    ORCH_KUBECONFIG_FILE: options.kubeconfig ?? env.ORCH_KUBECONFIG_FILE ?? join(privateDir, 'kubeconfig'),
    ORCH_HEADLESS: env.ORCH_HEADLESS ?? '1',
    ORCH_STEP_DELAY_MS: env.ORCH_STEP_DELAY_MS ?? '0',
  } };
}
