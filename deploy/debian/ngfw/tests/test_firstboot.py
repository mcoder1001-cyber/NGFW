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
        for path in ['etc/ngfw', 'etc/nginx/sites-enabled', 'var/lib/ngfw', 'run/lock',
                     'usr/lib/ngfw/api', 'usr/lib/ngfw/bin', 'bin']:
            (root / path).mkdir(parents=True)
        credentials = root / 'etc/ngfw/bootstrap.env'
        credentials.write_text('bootstrap fixture; no real credentials\n')
        credentials.chmod(0o600)
        def command(path, script):
            file = root / path
            file.write_text('#!/usr/bin/env bash\nset -eu\n' + script + '\n')
            file.chmod(0o755)
        command('bin/runuser', '''if [[ "$*" == *psql* ]]; then cat >/dev/null; exit 0; fi
[[ ${FAIL_STAGE:-} != db ]]''')
        command('bin/chown', ':')
        command('bin/openssl', '''if [[ ${FAIL_STAGE:-} == random && "$*" == "rand -hex 32" ]]; then exit 1; fi; exec /usr/bin/openssl "$@"''')
        command('bin/install', '''args=(); while (( $# )); do case $1 in -o|-g) shift 2;; *) args+=("$1"); shift;; esac; done; /usr/bin/install "${args[@]}"''')
        command('bin/stat', '''if [[ "$*" == *secret.key* ]]; then echo 32:600:ngfw; elif [[ "$*" == *bootstrap.env* || "$*" == *api.env* ]]; then echo 0:600; else /usr/bin/stat "$@"; fi''')
        command('bin/systemctl', 'echo "$*" >> "$FIXTURE_ROOT/service-commands"')
        command('usr/lib/ngfw/tls-bootstrap.sh', '[[ ${FAIL_STAGE:-} != tls ]]')
        command('usr/lib/ngfw/bin/ngfw-startupgen', '[[ ${FAIL_STAGE:-} != startup ]]')
        command('nginx', '[[ ${FAIL_STAGE:-} != nginx ]]')
        script = (SOURCE / 'assets/firstboot.sh').read_text()
        # Model root/unit identity in the redirected fixture so debhelper tests
        # can run as an unprivileged builder. The shipped root guard is intact.
        script = script.replace('[[ $EUID == 0 ]]', '[[ 1 == 1 ]]', 1)
        # A test-local copy redirects every absolute product path to a private
        # fixture; production script itself has no root bypass/environment hook.
        for prefix in ['/var/lib/ngfw', '/usr/lib/ngfw', '/run/lock', '/etc/ngfw', '/etc/nginx']:
            script = script.replace(prefix, str(root) + prefix)
        script = script.replace('/usr/sbin/nginx', str(root / 'nginx'))
        entry = root / 'firstboot.sh'
        entry.write_text(script)
        env = dict(os.environ, PATH=str(root / 'bin') + ':' + os.environ['PATH'],
                   FIXTURE_ROOT=str(root), NGFW_BOOTSTRAP_ADMIN_USER='fixture',
                   NGFW_BOOTSTRAP_ADMIN_PASSWORD='fixture-only-bootstrap')
        return root, credentials, entry, env

    def test_completed_marker_still_runs_unit_cleanup_after_crash(self):
        with tempfile.TemporaryDirectory() as directory:
            root, credentials, entry, env = self.fixture(directory)
            (root / 'var/lib/ngfw/firstboot-complete').write_text('completed\n')
            unit_path = SOURCE.parents[2] / 'deploy/systemd/ngfw-firstboot.service'
            if not unit_path.exists():
                unit_path = SOURCE / 'stage/usr/lib/systemd/system/ngfw-firstboot.service'
            unit = unit_path.read_text()
            self.assertNotIn('ConditionPathExists=!/var/lib/ngfw/firstboot-complete', unit)
            self.assertIn('EnvironmentFile=-/etc/ngfw/bootstrap.env', unit)
            subprocess.run(['bash', str(entry)], env=env, check=True, capture_output=True)
            self.assertFalse(credentials.exists())
            self.assertFalse((root / 'service-commands').exists())

    def test_invalid_existing_jwt_and_random_failure_retain_credentials(self):
        for corruption in ['missing', 'empty', 'duplicate', 'valid-empty', 'empty-valid', 'random-failure']:
            with self.subTest(corruption=corruption), tempfile.TemporaryDirectory() as directory:
                root, credentials, entry, env = self.fixture(directory)
                if corruption == 'random-failure':
                    env['FAIL_STAGE'] = 'random'
                else:
                    env['FAIL_STAGE'] = 'nginx'
                    subprocess.run(['bash', str(entry)], env=env, capture_output=True)
                    settings = root / 'etc/ngfw/api.env'
                    lines = settings.read_text().splitlines()
                    jwt = next(line for line in lines if line.startswith('NGFW_JWT_SECRET='))
                    lines = [line for line in lines if not line.startswith('NGFW_JWT_SECRET=')]
                    if corruption == 'empty': lines.append('NGFW_JWT_SECRET=')
                    if corruption == 'duplicate': lines.extend([jwt, jwt])
                    if corruption == 'valid-empty': lines.extend([jwt, 'NGFW_JWT_SECRET='])
                    if corruption == 'empty-valid': lines.extend(['NGFW_JWT_SECRET=', jwt])
                    settings.write_text('\n'.join(lines) + '\n')
                    del env['FAIL_STAGE']
                result = subprocess.run(['bash', str(entry)], env=env, capture_output=True)
                self.assertNotEqual(result.returncode, 0)
                self.assertTrue(credentials.exists())
                self.assertFalse((root / 'var/lib/ngfw/firstboot-complete').exists())

    def test_critical_environment_duplicates_refuse_before_completion(self):
        for field in ['NGFW_DATABASE_URL', 'NGFW_SECRET_KEY_FILE']:
            for value in ['alternate', '']:
                for original_first in [True, False]:
                    with self.subTest(field=field, value=value, original_first=original_first), tempfile.TemporaryDirectory() as directory:
                        root, credentials, entry, env = self.fixture(directory)
                        env['FAIL_STAGE'] = 'nginx'
                        subprocess.run(['bash', str(entry)], env=env, capture_output=True)
                        settings = root / 'etc/ngfw/api.env'
                        lines = settings.read_text().splitlines()
                        original = next(line for line in lines if line.startswith(field + '='))
                        lines = [line for line in lines if not line.startswith(field + '=')]
                        duplicate = field + '=' + value
                        lines.extend([original, duplicate] if original_first else [duplicate, original])
                        settings.write_text('\n'.join(lines) + '\n')
                        del env['FAIL_STAGE']
                        result = subprocess.run(['bash', str(entry)], env=env, capture_output=True)
                        self.assertNotEqual(result.returncode, 0)
                        self.assertTrue(credentials.exists())
                        self.assertFalse((root / 'var/lib/ngfw/firstboot-complete').exists())

    def test_key_file_override_refuses_even_with_valid_secret(self):
        with tempfile.TemporaryDirectory() as directory:
            root, credentials, entry, env = self.fixture(directory)
            env['FAIL_STAGE'] = 'nginx'
            subprocess.run(['bash', str(entry)], env=env, capture_output=True)
            settings = root / 'etc/ngfw/api.env'
            with settings.open('a') as file:
                file.write('NGFW_JWT_KEY_FILE=/nonexistent/firstboot-keyring\n')
            del env['FAIL_STAGE']
            result = subprocess.run(['bash', str(entry)], env=env, capture_output=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertTrue(credentials.exists())
            self.assertFalse((root / 'var/lib/ngfw/firstboot-complete').exists())

    def test_failures_retain_credentials_and_do_not_publish_completion(self):
        for stage in ['db', 'tls', 'startup', 'nginx']:
            with self.subTest(stage=stage), tempfile.TemporaryDirectory() as directory:
                root, credentials, entry, env = self.fixture(directory)
                env['FAIL_STAGE'] = stage
                result = subprocess.run(['bash', str(entry)], env=env, capture_output=True)
                self.assertNotEqual(result.returncode, 0)
                self.assertTrue(credentials.exists())
                self.assertFalse((root / 'var/lib/ngfw/firstboot-complete').exists())
                self.assertFalse((root / 'service-commands').exists())

    def test_retry_preserves_secrets_and_cleans_credentials_only_after_success(self):
        with tempfile.TemporaryDirectory() as directory:
            root, credentials, entry, env = self.fixture(directory)
            env['FAIL_STAGE'] = 'nginx'
            failed = subprocess.run(['bash', str(entry)], env=env, capture_output=True)
            self.assertTrue((root / 'var/lib/ngfw/secret.key').exists(), failed.stderr.decode())
            key = (root / 'var/lib/ngfw/secret.key').read_bytes()
            settings = (root / 'etc/ngfw/api.env').read_bytes()
            del env['FAIL_STAGE']
            subprocess.run(['bash', str(entry)], env=env, check=True, capture_output=True)
            self.assertFalse(credentials.exists())
            self.assertTrue((root / 'var/lib/ngfw/firstboot-complete').exists())
            self.assertEqual(key, (root / 'var/lib/ngfw/secret.key').read_bytes())
            self.assertEqual(settings, (root / 'etc/ngfw/api.env').read_bytes())
            subprocess.run(['bash', str(entry)], env=env, check=True, capture_output=True)
            self.assertEqual('disable ngfw-firstboot.service\n', (root / 'service-commands').read_text())


if __name__ == '__main__':
    unittest.main()
