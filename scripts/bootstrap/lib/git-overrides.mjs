// Catalog releases replace per-component source overrides.
export function applyGitOverrides(raw, options) {
 const result=structuredClone(raw);
 if (options['git-ref'] !== undefined) throw Error('Dùng catalog.branch và tag SemVer thay cho --git-ref');
 if (options['git-url'] !== undefined) {
  const url=new URL(options['git-url']);
  if(url.protocol!=='https:'||url.username||url.password||url.search||url.hash)throw Error('--git-url phải là HTTPS không credential/query/hash');
  if(!result.catalog)throw Error('Cần cấu hình catalog');
  result.catalog.repository=options['git-url'];
 }
 return result;
}
