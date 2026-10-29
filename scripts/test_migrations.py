"""Local tests for upgrade deployment ordering and the Git migration guard."""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]


class MigrationGuardTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='issue23-guard-')
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        (self.root / 'scripts').mkdir()
        shutil.copy(ROOT / 'scripts/check_migrations.py', self.root / 'scripts')
        self.pg = self.root / 'backend/internal/adapters/postgres'
        (self.pg / 'migrations').mkdir(parents=True)
        (self.pg / 'baseline.sql').write_text('CREATE TABLE a(id int);\n')
        (self.pg / 'migrations/version-001.sql').write_text('CREATE TABLE a(id int);\n')
        (self.pg / 'migrations/001-change.sql').write_text('-- initial\n')
        for args in [('init', '-q'), ('add', '.'), ('-c', 'user.name=Test', '-c', 'user.email=test@example.invalid', 'commit', '-qm', 'baseline')]:
            subprocess.run(['git', *args], cwd=self.root, check=True, capture_output=True)

    def check(self):
        return subprocess.run(['python3', 'scripts/check_migrations.py'], cwd=self.root, capture_output=True).returncode

    def test_comments_and_whitespace_do_not_require_version(self):
        (self.pg / 'baseline.sql').write_text('-- comment\nCREATE   TABLE a ( id int );\n')
        self.assertEqual(self.check(), 0)

    def test_changed_schema_requires_migration_and_reference(self):
        (self.pg / 'baseline.sql').write_text('CREATE TABLE a(id text);\n')
        self.assertNotEqual(self.check(), 0)
        (self.pg / 'migrations/version-002.sql').write_text('CREATE TABLE a(id text);\n')
        (self.pg / 'migrations/002-change.sql').write_text('ALTER TABLE a ALTER COLUMN id TYPE text;\n')
        self.assertEqual(self.check(), 0)

    def test_released_artifact_cannot_be_changed(self):
        (self.pg / 'migrations/001-change.sql').write_text('-- modified\n')
        self.assertNotEqual(self.check(), 0)


class UpgradeWorkflowTests(unittest.TestCase):
    def run_workflow(self, fail=''):
        with tempfile.TemporaryDirectory(prefix='issue23-workflow-') as directory:
            path = Path(directory)
            docker = path / 'docker'
            docker.write_text('''#!/bin/bash
printf '%s\\n' "$*" >> "$WORKFLOW_LOG"
case "$*" in
  *"build db-setup backend frontend"*) if [[ $FAIL_STEP == build ]]; then exit 1; fi;;
  *"exec -T postgres sh -c"*) if [[ $FAIL_STEP == backup ]]; then exit 1; fi; printf 'synthetic backup';;
  *"run --rm --no-deps db-setup"*) if [[ $FAIL_STEP == setup ]]; then exit 1; fi;;
esac
''')
            docker.chmod(0o700)
            log = path / 'commands'
            backup = path / 'backup.dump'
            result = subprocess.run(['bash', str(ROOT / 'deploy/local/upgrade-database.sh'), str(backup), '-p', 'isolated-test'], env={**os.environ, 'PATH': str(path) + ':' + os.environ['PATH'], 'WORKFLOW_LOG': str(log), 'FAIL_STEP': fail}, capture_output=True)
            if fail in ('build', 'backup'):
                self.assertFalse(backup.exists())
            else:
                self.assertTrue(backup.exists())
                self.assertEqual(backup.stat().st_mode & 0o777, 0o600)
            return result.returncode, log.read_text().splitlines()

    def test_backup_then_setup_then_start(self):
        code, commands = self.run_workflow()
        self.assertEqual(code, 0)
        self.assertIn('stop frontend backend', commands[1])
        self.assertIn('exec -T postgres sh -c', commands[2])
        self.assertIn('pg_restore --list', commands[3])
        self.assertIn('run --rm --no-deps db-setup', commands[4])
        self.assertIn('up -d --no-deps --wait backend frontend', commands[5])
        self.assertTrue(all('-p isolated-test' in command for command in commands))

    def test_build_failure_keeps_writers_running(self):
        code, commands = self.run_workflow('build')
        self.assertNotEqual(code, 0)
        self.assertEqual(len(commands), 1)

    def test_backup_failure_blocks_migration(self):
        code, commands = self.run_workflow('backup')
        self.assertNotEqual(code, 0)
        self.assertFalse(any('run --rm' in command or 'up -d' in command for command in commands))

    def test_setup_failure_blocks_start(self):
        code, commands = self.run_workflow('setup')
        self.assertNotEqual(code, 0)
        self.assertFalse(any('up -d' in command for command in commands))


if __name__ == '__main__':
    unittest.main()
