#!/usr/bin/env python3
"""Create private development credentials once. Values never go to stdout/disk."""
import json
import secrets
import subprocess
import sys

if len(sys.argv) != 2:
    sys.exit('Usage: python3 deploy/k8s/init-secrets.py <explicit-kube-context>')
context = sys.argv[1]
namespace = 'orchestrator-demo'
label = {'app.kubernetes.io/managed-by': 'poc-k8s-manifests'}

def kubectl(*args, data=None):
    result = subprocess.run(
        ['kubectl', '--context', context, *args],
        input=json.dumps(data) if data is not None else None,
        text=True, capture_output=True,
    )
    if result.returncode:
        # API responses could echo submitted Secret values; never print them.
        sys.exit('kubectl failed during credential provisioning; inspect resource status separately')
    return json.loads(result.stdout) if result.stdout.strip() else None

ns = kubectl('get', 'namespace', namespace, '--ignore-not-found', '-o', 'json')
if ns is None:
    kubectl('create', '-f', '-', '-o', 'json', data={
        'apiVersion': 'v1', 'kind': 'Namespace',
        'metadata': {'name': namespace, 'labels': label},
    })
elif ns['metadata'].get('labels', {}).get(next(iter(label))) != next(iter(label.values())):
    sys.exit('Refusing to reuse a namespace without this deployment ownership label')

expected = {
    'postgres-passwords': {'owner', 'console', 'state', 'superuser'},
    'database-owner': {'database-url'},
    'backend-runtime': {'console-database-url', 'state-database-url', 'state-key'},
    'vault-tokens': {'connection-token', 'app-token', 'output-token'},
}
existing = {name: kubectl('-n', namespace, 'get', 'secret', name, '--ignore-not-found', '-o', 'json')
            for name in expected}
if any(existing.values()):
    for name, keys in expected.items():
        item = existing[name]
        if not item or not keys.issubset(item.get('data', {})) or any(not item['data'][key] for key in keys):
            sys.exit('Incomplete existing credentials; restore matching Secrets, do not regenerate against retained data')
    print('Existing credentials retained unchanged')
    sys.exit(0)
claims = kubectl('-n', namespace, 'get', 'pvc', '-o', 'json')['items']
if claims:
    sys.exit('PVCs already exist without credentials; restore matching Secrets before continuing')
passwords = {name: secrets.token_hex(24) for name in expected['postgres-passwords']}
def url(role):
    return f'postgres://orchestrator_{role}:{passwords[role]}@postgres:5432/orchestrator?sslmode=disable'
values = {
    'postgres-passwords': passwords,
    'database-owner': {'database-url': url('owner')},
    'backend-runtime': {'console-database-url': url('console'), 'state-database-url': url('state'),
                        'state-key': secrets.token_hex(32)},
    'vault-tokens': {key: secrets.token_hex(24) for key in expected['vault-tokens']},
}
for name, content in values.items():
    kubectl('create', '-f', '-', '-o', 'json', data={
        'apiVersion': 'v1', 'kind': 'Secret', 'type': 'Opaque',
        'metadata': {'name': name, 'namespace': namespace, 'labels': label},
        'stringData': content,
    })
print('Created four Secrets; credential values were not printed or saved to disk')
