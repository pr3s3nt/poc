import { readFileSync, statSync } from 'node:fs';
import { homedir } from 'node:os';
import { join } from 'node:path';
import { X509Certificate } from 'node:crypto';
import { readPrivateFile } from './console.mjs';

export class GitAccessError extends Error {}
const fail = message => { throw new GitAccessError(message); };
const expand = (file, home) => file.startsWith('~/') ? join(home, file.slice(2)) : file;
export function gitURL(raw) {
  if (typeof raw !== 'string' || !raw || raw.length > 2048 || /[\s\x00-\x1f\x7f\\%?#]/u.test(raw)) fail('Git access: URL phải là HTTPS, không chứa credential hoặc đường dẫn mơ hồ');
  // Check raw path before WHATWG URL removes dot segments.
  const path = raw.replace(/^https:\/\/[^/]+/u, '');
  if (path.includes('//') || path.split('/').some(segment => segment === '.' || segment === '..')) fail('Git access: đường dẫn URL không hợp lệ');
  let url;
  try { url = new URL(raw); } catch { fail('Git access: URL không hợp lệ'); }
  if (url.protocol !== 'https:' || url.username || url.password || raw.split('/')[2]?.includes('@') || !url.hostname) fail('Git access: cần URL HTTPS không chứa credential');
  return 'https://' + raw.split('/')[2].toLowerCase() + url.pathname.replace(/\/+$/u, '');
}
export function inGitScope(scope, repo) {
  const a = new URL(gitURL(scope)), b = new URL(gitURL(repo));
  return gitURL(scope).split('/')[2] === gitURL(repo).split('/')[2] && (a.pathname === '/' || a.pathname === b.pathname || b.pathname.startsWith(a.pathname + '/'));
}
export function parseGitAccess(raw, options = {}, home = homedir()) {
  const value = raw.gitAccess;
  if (value === undefined) return null;
  if (!value || Array.isArray(value) || typeof value !== 'object' || Object.keys(value).some(key => !['enabled', 'url', 'username', 'tokenFile', 'caPemFile', 'repositoryUrl', 'ref'].includes(key))) fail('gitAccess: cấu hình hoặc trường không được hỗ trợ');
  if (value.enabled !== undefined && typeof value.enabled !== 'boolean') fail('gitAccess.enabled phải là boolean');
  if (value.enabled === false) return null;
  if (typeof value.username !== 'string' || !value.username.trim() || value.username.length > 256 || /[:\x00-\x1f\x7f-\x9f]/u.test(value.username)) fail('gitAccess.username không hợp lệ');
  const sources = [...(raw.resourceDefinitions ?? []).map(item => item.source), ...(raw.bundles ?? []).map(item => item.source)].filter(Boolean);
  const urls = [...new Set(sources.map(source => source.url))];
  const repositoryUrl = gitURL(value.repositoryUrl ?? options['git-url'] ?? (urls.length === 1 ? urls[0] : undefined));
  const url = gitURL(value.url ?? options['git-url'] ?? repositoryUrl);
  if (!inGitScope(url, repositoryUrl) || sources.some(source => !inGitScope(url, source.url))) fail('Git access: repo catalog phải thuộc phạm vi URL đã cấu hình');
  const refs = [...new Set(sources.filter(source => gitURL(source.url) === repositoryUrl).map(source => source.ref))];
  const ref = value.ref ?? options['git-ref'] ?? (refs.length === 1 ? refs[0] : undefined);
  if (typeof ref !== 'string' || !/^[A-Za-z0-9][A-Za-z0-9._/-]{0,199}$/u.test(ref) || /\.\.|\/\/|[/.]$/u.test(ref)) fail('gitAccess.ref: cần một nhánh/tag/commit kiểm tra rõ ràng');
  if (typeof value.tokenFile !== 'string' || !value.tokenFile) fail('gitAccess.tokenFile: cần đường dẫn file token riêng tư');
  const tokenFile = expand(value.tokenFile, home);
  let caPem = '';
  if (value.caPemFile !== undefined && value.caPemFile !== '') {
    if (typeof value.caPemFile !== 'string') fail('gitAccess.caPemFile phải là đường dẫn');
    try {
      const file = expand(value.caPemFile, home);
      const stat = statSync(file);
      if (!stat.isFile() || stat.size > 65536) fail('Git access: file CA không hợp lệ hoặc vượt 64 KiB');
      caPem = readFileSync(file, 'utf8').trim();
      let remaining = caPem, count = 0;
      while (remaining) {
        const block = /^-----BEGIN CERTIFICATE-----[\s\S]*?-----END CERTIFICATE-----/u.exec(remaining)?.[0];
        if (!block) fail('Git access: CA phải chỉ chứa chứng chỉ PEM');
        new X509Certificate(block); count++;
        remaining = remaining.slice(block.length).trim();
      }
      if (!count) fail('Git access: file CA rỗng');
    } catch { fail('Git access: không đọc được CA PEM hợp lệ'); }
  }
  return { url, username: value.username, tokenFile, caPem, repositoryUrl, ref };
}
export function readGitToken(file) {
  try {
    if (statSync(file).size > 4098) fail('Git token quá lớn');
    const token = readPrivateFile(file, 'Git token');
    if (Buffer.byteLength(token) > 4096 || /[\x00-\x1f\x7f-\x9f]/u.test(token)) fail('Git token không hợp lệ');
    return token;
  } catch { fail('Git access: cần file token không rỗng, tối đa 4096 byte, quyền 0600/0400'); }
}
export function gitAccessAction(current, desired, update) {
  if (!current || !Number.isSafeInteger(current.version) || current.version < 0) fail('Git access: metadata hiện tại không hợp lệ');
  if (current.version === 0) return 'create';
  const same = current.url === desired.url && current.username === desired.username && current.authenticated === true && current.tokenConfigured === true && (current.caPem ?? '').trim() === desired.caPem;
  if (update) return 'update'; // Explicit rotation also replaces an unreadable/expired token.
  if (same) return 'reuse';
  fail('Git access: cấu hình khác; cần --update-git-access để cập nhật');
}
