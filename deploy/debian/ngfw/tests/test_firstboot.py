#!/usr/bin/env python3
"""Execute firstboot control flow using fixture commands, never a live appliance."""
import importlib.util
import os
import pathlib
import subprocess
import tempfile
import unittest

SOURCE = pathlib.Path(__file__).resolve().parents[1]
SCRIPTS_SPEC = importlib.util.spec_from_file_location('maintainer_scripts', SOURCE / 'tests/maintainer_scripts.py')
MAINTAINER = importlib.util.module_from_spec(SCRIPTS_SPEC)
SCRIPTS_SPEC.loader.exec_module(MAINTAINER)


class Firstboot(unittest.TestCase):
    def fixture(self, directory):
        root = pathlib.Path(directory)
        for path in ['etc/ngfw', 'etc/nginx/sites-enabled', 'var/lib/ngfw', 'run/lock',
                     'usr/lib/ngfw/api', 'usr/lib/ngfw/bin', 'bin']:
            (root / path).mkdir(parents=True)
        credentials = root / 'etc/ngfw/bootstrap.env'
        credentials.write_text('bootstrap fixture; no real credentials\n')
        credentials.chmod(0o600)
        (root / 'usr/lib/ngfw/initial-dataplane.json').write_bytes(
            (SOURCE / 'assets/initial-dataplane.json').read_bytes())
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

    def test_missing_administrator_variables_use_product_defaults(self):
        with tempfile.TemporaryDirectory() as directory:
            root, credentials, entry, env = self.fixture(directory)
            env.pop('NGFW_BOOTSTRAP_ADMIN_USER', None)
            env.pop('NGFW_BOOTSTRAP_ADMIN_PASSWORD', None)
            runuser = root / 'bin/runuser'
            runuser.write_text(runuser.read_text() + '\nif [[ "$*" == *bootstrap-db.mjs* ]]; then [[ $NGFW_BOOTSTRAP_ADMIN_USER == admin && $NGFW_BOOTSTRAP_ADMIN_PASSWORD == admin ]]; fi\n')
            subprocess.run(['bash', str(entry)], env=env, check=True, capture_output=True)
            self.assertFalse(credentials.exists())
            self.assertTrue((root / 'var/lib/ngfw/firstboot-complete').exists())

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


class FirstbootUpgrade(unittest.TestCase):
    """A package upgrade leaves the completed first boot alone (D-253)."""

    @classmethod
    def setUpClass(cls):
        missing = MAINTAINER.missing_tools()
        if missing:
            raise unittest.SkipTest('debhelper is required to generate the maintainer scripts; missing: ' + ' '.join(missing))
        cls.meta = MAINTAINER.generate()['ngfw-meta']

    def test_upgrade_neither_stops_nor_starts_the_firstboot_unit(self):
        # After a successful first boot ngfw-firstboot.service stays active (exited) and disabled;
        # vpp.service, ngfw-agent.service and ngfw-api.service require it. An upgrade must not stop it
        # (that stops the data plane) and must not start or restart it (that re-runs firstboot.sh).
        # Removing ngfw-meta still stops it: the positive control for the fixture.
        with tempfile.TemporaryDirectory() as temporary:
            recorder = MAINTAINER.Recorder(temporary)
            for script, invocations in MAINTAINER.UPGRADE_PATHS.items():
                for arguments in invocations:
                    calls = recorder.run(self.meta[script], *arguments) if script in self.meta else {}
                    for line in calls.get('deb-systemd-invoke', []):
                        self.assertNotIn('ngfw-firstboot.service', line, (script, arguments))
            for section in MAINTAINER.debhelper_sections(self.meta['postinst']):
                self.assertNotIn('deb-systemd-invoke', section)
            if 'prerm' in self.meta:  # debhelper 13.31 emits a remove-only stop; 13.14 emits no prerm for -r
                self.assertIn('ngfw-firstboot.service', MAINTAINER.stopped_units(recorder.run(self.meta['prerm'], 'remove')))

    def test_first_boot_gate_keeps_requires_not_wants(self):
        # The decision keeps the gate: a fresh appliance runs firstboot before VPP and a failed firstboot
        # still blocks VPP (Requires= plus After=, not Wants=); the unit stays a WantedBy oneshot that
        # remains active after it exits, so a later `systemctl restart ngfw-agent` finds it active and
        # does not run firstboot.sh again.
        dropin = (SOURCE / 'assets/vpp-firstboot.conf').read_text()
        self.assertIn('Requires=ngfw-firstboot.service', dropin)
        self.assertIn('After=ngfw-firstboot.service', dropin)
        self.assertNotIn('Wants=', dropin)
        unit = MAINTAINER.locate_unit('stage/usr/lib/systemd/system/ngfw-firstboot.service', 'ngfw-firstboot.service').read_text()
        for line in ('Type=oneshot', 'RemainAfterExit=yes', 'WantedBy=multi-user.target', 'Before=vpp.service'):
            self.assertIn(line, unit)
        self.assertIn('ngfw-firstboot.service', MAINTAINER.stop_propagation_closure('vpp.service'))


if __name__ == '__main__':
    unittest.main()
