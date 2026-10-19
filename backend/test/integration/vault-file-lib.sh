#!/usr/bin/env bash
# Run-owned, file-backed Vault fixture. Caller uses umask077 and owns its EXIT trap.
# Init/unseal/root material stays private so original cleanup survives host restart.
unseal_file_vault() {
  local vault_dir="$1" vault_addr="$2"
  node -e 'const fs=require("fs");const [dir]=process.argv.slice(1);const j=JSON.parse(fs.readFileSync(`${dir}/init.json`,"utf8"));if(!j.keys_base64?.[0])process.exit(1);fs.writeFileSync(`${dir}/unseal-request.json`,JSON.stringify({key:j.keys_base64[0]}),{mode:0o600});' "${vault_dir}"
  curl --max-time 10 -fsS -X PUT "${vault_addr}/v1/sys/unseal" -d @"${vault_dir}/unseal-request.json" > "${vault_dir}/unseal-response.json"
  rm -f "${vault_dir}/unseal-request.json" "${vault_dir}/unseal-response.json"
  curl --max-time 10 -fsS "${vault_addr}/v1/sys/health" > /dev/null
}

start_file_vault() {
  local vault_dir="$1" vault_addr="$2" vault_header="$3" vault_pid_var="$4" vault_status="" vault_attempt
  mkdir -m0700 "${vault_dir}" "${vault_dir}/data"
  node -e 'const fs=require("fs");const [dir,addr]=process.argv.slice(1);const u=new URL(addr);if(u.protocol!=="http:")process.exit(1);fs.writeFileSync(`${dir}/config.json`,JSON.stringify({disable_mlock:true,ui:false,api_addr:addr,storage:{file:{path:`${dir}/data`}},listener:[{tcp:{address:u.host,tls_disable:1}}]}),{mode:0o600});' "${vault_dir}" "${vault_addr}"
  setsid "${VAULT_BIN}" server -config="${vault_dir}/config.json" > "${vault_dir}/server.log" 2>&1 &
  printf -v "${vault_pid_var}" '%s' "$!"
  printf '%s\n' "$!" > "${vault_dir}/server.pid"
  for vault_attempt in $(seq 1 60); do
    vault_status="$(curl --max-time 2 -s -o /dev/null -w '%{http_code}' "${vault_addr}/v1/sys/health" || true)"
    [[ "${vault_status}" == 501 ]] && break
    sleep 0.5
  done
  [[ "${vault_status}" == 501 ]] || { echo 'run-owned file Vault did not reach uninitialized state' >&2; return 1; }
  curl --max-time 10 -fsS -X PUT "${vault_addr}/v1/sys/init" -d '{"secret_shares":1,"secret_threshold":1}' > "${vault_dir}/init.json"
  node -e 'const fs=require("fs");const [dir,header]=process.argv.slice(1);const j=JSON.parse(fs.readFileSync(`${dir}/init.json`,"utf8"));if(!j.root_token)process.exit(1);fs.writeFileSync(header,`X-Vault-Token: ${j.root_token}\n`,{mode:0o600});' "${vault_dir}" "${vault_header}"
  unseal_file_vault "${vault_dir}" "${vault_addr}"
  curl --max-time 10 -fsS -H @"${vault_header}" -X POST "${vault_addr}/v1/sys/mounts/secret" -d '{"type":"kv","options":{"version":"2"}}' > /dev/null
}
