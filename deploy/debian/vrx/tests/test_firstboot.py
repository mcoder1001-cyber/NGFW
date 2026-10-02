#!/usr/bin/env python3
"""Execute firstboot control flow using fixture commands, never a live appliance."""
import os
import pathlib
import subprocess
import tempfile
import unittest

SOURCE = pathlib.Path(__file__).resolve().parents[1]


class Firstboot(unittest.TestCase):
    def fixture(self, directory):
        root = pathlib.Path(directory)
        for path in ['etc/vrx', 'etc/nginx/sites-enabled', 'var/lib/vrx', 'run/lock',
                     'usr/lib/vrx/api', 'usr/lib/vrx/bin', 'bin']:
            (root / path).mkdir(parents=True)
        credentials = root / 'etc/vrx/bootstrap.env'
        credentials.write_text('bootstrap fixture; no real credentials\n')
        credentials.chmod(0o600)
        def command(path, script):
            file = root / path
            file.write_text('#!/usr/bin/env bash\nset -eu\n' + script + '\n')
            file.chmod(0o755)
        command('bin/runuser', '''if [[ "$*" == *psql* ]]; then cat >/dev/null; exit 0; fi
[[ ${FAIL_STAGE:-} != db ]]''')
        command('bin/chown', ':')
        command('bin/install', '''args=(); while (( $# )); do case $1 in -o|-g) shift 2;; *) args+=("$1"); shift;; esac; done; /usr/bin/install "${args[@]}"''')
        command('bin/stat', '''if [[ "$*" == *secret.key* ]]; then echo 32:600:vrx; else /usr/bin/stat "$@"; fi''')
        command('bin/systemctl', 'echo "$*" >> "$FIXTURE_ROOT/service-commands"')
        command('usr/lib/vrx/tls-bootstrap.sh', '[[ ${FAIL_STAGE:-} != tls ]]')
        command('usr/lib/vrx/bin/vrx-startupgen', '[[ ${FAIL_STAGE:-} != startup ]]')
        command('nginx', '[[ ${FAIL_STAGE:-} != nginx ]]')
        script = (SOURCE / 'assets/firstboot.sh').read_text()
        # A test-local copy redirects every absolute product path to a private
        # fixture; production script itself has no root bypass/environment hook.
        for prefix in ['/var/lib/vrx', '/usr/lib/vrx', '/run/lock', '/etc/vrx', '/etc/nginx']:
            script = script.replace(prefix, str(root) + prefix)
        script = script.replace('/usr/sbin/nginx', str(root / 'nginx'))
        entry = root / 'firstboot.sh'
        entry.write_text(script)
        env = dict(os.environ, PATH=str(root / 'bin') + ':' + os.environ['PATH'],
                   FIXTURE_ROOT=str(root), VRX_BOOTSTRAP_ADMIN_USER='fixture',
                   VRX_BOOTSTRAP_ADMIN_PASSWORD='fixture-only-bootstrap')
        return root, credentials, entry, env

    def test_completed_marker_still_runs_unit_cleanup_after_crash(self):
        with tempfile.TemporaryDirectory() as directory:
            root, credentials, entry, env = self.fixture(directory)
            (root / 'var/lib/vrx/firstboot-complete').write_text('completed\n')
            unit_path = SOURCE.parents[2] / 'deploy/systemd/vrx-firstboot.service'
            if not unit_path.exists():
                unit_path = SOURCE / 'stage/usr/lib/systemd/system/vrx-firstboot.service'
            unit = unit_path.read_text()
            self.assertNotIn('ConditionPathExists=!/var/lib/vrx/firstboot-complete', unit)
            self.assertIn('EnvironmentFile=-/etc/vrx/bootstrap.env', unit)
            subprocess.run(['bash', str(entry)], env=env, check=True, capture_output=True)
            self.assertFalse(credentials.exists())
            self.assertFalse((root / 'service-commands').exists())

    def test_failures_retain_credentials_and_do_not_publish_completion(self):
        for stage in ['db', 'tls', 'startup', 'nginx']:
            with self.subTest(stage=stage), tempfile.TemporaryDirectory() as directory:
                root, credentials, entry, env = self.fixture(directory)
                env['FAIL_STAGE'] = stage
                result = subprocess.run(['bash', str(entry)], env=env, capture_output=True)
                self.assertNotEqual(result.returncode, 0)
                self.assertTrue(credentials.exists())
                self.assertFalse((root / 'var/lib/vrx/firstboot-complete').exists())
                self.assertFalse((root / 'service-commands').exists())

    def test_retry_preserves_secrets_and_cleans_credentials_only_after_success(self):
        with tempfile.TemporaryDirectory() as directory:
            root, credentials, entry, env = self.fixture(directory)
            env['FAIL_STAGE'] = 'nginx'
            failed = subprocess.run(['bash', str(entry)], env=env, capture_output=True)
            self.assertTrue((root / 'var/lib/vrx/secret.key').exists(), failed.stderr.decode())
            key = (root / 'var/lib/vrx/secret.key').read_bytes()
            settings = (root / 'etc/vrx/api.env').read_bytes()
            del env['FAIL_STAGE']
            subprocess.run(['bash', str(entry)], env=env, check=True, capture_output=True)
            self.assertFalse(credentials.exists())
            self.assertTrue((root / 'var/lib/vrx/firstboot-complete').exists())
            self.assertEqual(key, (root / 'var/lib/vrx/secret.key').read_bytes())
            self.assertEqual(settings, (root / 'etc/vrx/api.env').read_bytes())
            subprocess.run(['bash', str(entry)], env=env, check=True, capture_output=True)
            self.assertEqual('disable vrx-firstboot.service\n', (root / 'service-commands').read_text())


if __name__ == '__main__':
    unittest.main()
