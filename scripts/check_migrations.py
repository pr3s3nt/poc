#!/usr/bin/env python3
"""Guard released schema references/migrations and require versioned schema changes."""
import argparse
from pathlib import Path
import re
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]
DIRECTORY = 'backend/internal/adapters/postgres/migrations/'


def git(*args):
    return subprocess.check_output(['git', *args], cwd=ROOT)


def tokens(sql):
    # Preserve quoted values/identifiers and dollar-quoted function bodies.
    pattern = r"('(?:''|[^'])*'|\"(?:\"\"|[^\"])*\"|\$([A-Za-z_0-9]*)\$.*?\$\2\$|--[^\n]*|/\*.*?\*/|\s+|[A-Za-z_][A-Za-z_0-9$]*|[0-9]+|.)"
    return [m.group(1) for m in re.finditer(pattern, sql, re.S)
            if not m.group(1).isspace() and not m.group(1).startswith(('--', '/*'))]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base', default='HEAD')
    args = parser.parse_args()
    base = git('rev-parse', '--verify', args.base).decode().strip()
    released = git('ls-tree', '-r', '--name-only', base, DIRECTORY).decode().splitlines()
    errors = []
    for name in released:
        current = ROOT / name
        if not current.exists() or current.read_bytes() != git('show', f'{base}:{name}'):
            errors.append(f'Released migration/reference changed: {name}; add a new version instead.')
    baseline = 'backend/internal/adapters/postgres/baseline.sql'
    changed = tokens(git('show', f'{base}:{baseline}').decode()) != tokens((ROOT / baseline).read_text())
    new = {str(p.relative_to(ROOT)) for p in (ROOT / DIRECTORY).glob('*.sql')} - set(released)
    if changed and not (any('/version-' in p for p in new) and any('/0' in p for p in new)):
        errors.append('Schema changed: add a new immutable version reference and migration SQL, register them and test the upgrade.')
    if errors:
        print('\n'.join(errors), file=sys.stderr)
        return 1
    print('Migration immutability/version guard passed. PostgreSQL tests verify structural equivalence and upgrades.')
    return 0


if __name__ == '__main__':
    sys.exit(main())
