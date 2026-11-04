import { lstatSync, readFileSync } from 'node:fs';
import { createHash } from 'node:crypto';
import { validateGitInput } from './git-inputs.mjs';
import { requireConfig } from './bootstrap-config.mjs';
export function createPreparedInputs(path) {
 const receipts=[];
 return {receipts,read(input){
  validateGitInput(input);
  requireConfig(typeof input.preparedFile==='string'&&input.preparedFile.length>0,'Console bootstrap requires preparedFile for each Git business document; run prepare-enterprise-input.mjs first');
  const file=path(input.preparedFile),stat=lstatSync(file);
  requireConfig(stat.isFile()&&!stat.isSymbolicLink()&&stat.size<=1024*1024,'Prepared input must be one bounded regular file');
  const bytes=readFileSync(file);
  requireConfig(createHash('sha256').update(bytes).digest('hex')===input.sha256,'Prepared input SHA256 mismatch');
  const {preparedFile,...source}=input;
  receipts.push({...source,verification:'local-sha256',gitCommitVerified:false});
  return file;
 }};
}
