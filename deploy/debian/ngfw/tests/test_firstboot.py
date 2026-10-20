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
# The daemons the first boot needs up before it runs (D-255: Wants= + After= in the unit, is-active in the script).
GATED_UNITS = ('postgresql.service', 'valkey-server.service', 'nftables.service')
# The PostgreSQL gate itself: the unit is only a wrapper on Debian, the cluster answers pg_isready.
PG_ISREADY = '-u postgres -- pg_isready -q'


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
        command('bin/runuser', '''echo "$*" >> "$FIXTURE_ROOT/runuser-commands"
if [[ "$*" == *pg_isready* ]]; then [[ ${FAIL_STAGE:-} != cluster ]]; exit 0; fi
if [[ "$*" == *psql* ]]; then cat >/dev/null; exit 0; fi
[[ ${FAIL_STAGE:-} != db ]]''')
        command('bin/chown', ':')
        command('bin/openssl', '''if [[ ${FAIL_STAGE:-} == random && "$*" == "rand -hex 32" ]]; then exit 1; fi; exec /usr/bin/openssl "$@"''')
        command('bin/install', '''args=(); while (( $# )); do case $1 in -o|-g) shift 2;; *) args+=("$1"); shift;; esac; done; /usr/bin/install "${args[@]}"''')
        command('bin/stat', '''if [[ "$*" == *secret.key* ]]; then echo 32:600:ngfw; elif [[ "$*" == *bootstrap.env* || "$*" == *api.env* ]]; then echo 0:600; else /usr/bin/stat "$@"; fi''')
        # `systemctl --quiet is-active <unit>` (the D-255 first-boot gate) is recorded separately and
        # fails for the units named in INACTIVE_UNITS; every other systemctl call is a state change.
        command('bin/systemctl', '''if [[ $1 == --quiet && $2 == is-active ]]; then
  echo "$*" >> "$FIXTURE_ROOT/service-queries"
  for unit in ${INACTIVE_UNITS:-}; do [[ $3 != "$unit" ]] || exit 3; done
  exit 0
fi
echo "$*" >> "$FIXTURE_ROOT/service-commands"''')
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
            # D-255: a provisioned appliance never gates on the daemons (no is-active query).
            self.assertFalse((root / 'service-queries').exists())
            self.assertFalse((root / 'runuser-commands').exists())

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
        for stage in ['cluster', 'db', 'tls', 'startup', 'nginx']:
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

    def test_fresh_boot_refuses_an_inactive_dependency_before_provisioning(self):
        # D-255: the unit pulls PostgreSQL, Valkey and nftables in with Wants= (an explicit
        # restart of one of them must not propagate to VPP), so the script keeps the first boot
        # fail-closed itself: an inactive dependency stops it before pg_isready and any database,
        # key or file work, credentials stay for the retry and no unit state is changed.
        for unit in GATED_UNITS:
            with self.subTest(unit=unit), tempfile.TemporaryDirectory() as directory:
                root, credentials, entry, env = self.fixture(directory)
                env['INACTIVE_UNITS'] = unit
                result = subprocess.run(['bash', str(entry)], env=env, capture_output=True, text=True)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn('first boot requires an active ' + unit, result.stderr)
                self.assertTrue(credentials.exists())
                self.assertFalse((root / 'var/lib/ngfw/firstboot-complete').exists())
                self.assertFalse((root / 'var/lib/ngfw/secret.key').exists())
                self.assertFalse((root / 'etc/ngfw/api.env').exists())
                self.assertFalse((root / 'runuser-commands').exists())
                self.assertFalse((root / 'service-commands').exists())
                queries = (root / 'service-queries').read_text().splitlines()
                self.assertEqual(queries[-1], '--quiet is-active ' + unit)
                self.assertEqual(queries, ['--quiet is-active ' + name for name in GATED_UNITS[:len(queries)]])

    def test_fresh_boot_queries_each_dependency_separately_then_provisions(self):
        # One unit per is-active call: with several names systemctl succeeds when any one of
        # them is active. The three queries come before the first database command.
        with tempfile.TemporaryDirectory() as directory:
            root, credentials, entry, env = self.fixture(directory)
            subprocess.run(['bash', str(entry)], env=env, check=True, capture_output=True)
            self.assertEqual((root / 'service-queries').read_text().splitlines(),
                             ['--quiet is-active ' + name for name in GATED_UNITS])
            commands = (root / 'runuser-commands').read_text().splitlines()
            self.assertEqual(commands[0], PG_ISREADY)
            self.assertIn('psql', commands[1])
            self.assertTrue((root / 'var/lib/ngfw/firstboot-complete').exists())
            self.assertFalse(credentials.exists())

    def test_fresh_boot_refuses_a_cluster_that_does_not_accept_connections(self):
        # On Debian postgresql.service is an ExecStart=/bin/true wrapper and the cluster runs in
        # postgresql@<version>-main.service, so an active wrapper proves nothing: pg_isready (as
        # postgres, over the socket) is the PostgreSQL gate, after the unit queries and before the
        # first psql; the psql calls would stop on a down cluster too, pg_isready names the cause.
        with tempfile.TemporaryDirectory() as directory:
            root, credentials, entry, env = self.fixture(directory)
            env['FAIL_STAGE'] = 'cluster'
            result = subprocess.run(['bash', str(entry)], env=env, capture_output=True, text=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn('first boot requires a PostgreSQL cluster accepting connections', result.stderr)
            self.assertTrue(credentials.exists())
            self.assertFalse((root / 'var/lib/ngfw/firstboot-complete').exists())
            self.assertFalse((root / 'var/lib/ngfw/secret.key').exists())
            self.assertFalse((root / 'etc/ngfw/api.env').exists())
            self.assertFalse((root / 'service-commands').exists())
            self.assertEqual((root / 'service-queries').read_text().splitlines(),
                             ['--quiet is-active ' + name for name in GATED_UNITS])
            self.assertEqual((root / 'runuser-commands').read_text().splitlines(), [PG_ISREADY])


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


class FirstbootDependencies(unittest.TestCase):
    """How ngfw-firstboot.service depends on PostgreSQL, Valkey and nftables (D-255)."""

    def unit(self, name):
        return MAINTAINER.locate_unit('stage/usr/lib/systemd/system/' + name, name).read_text()

    def test_firstboot_wants_and_orders_after_the_daemons_and_before_the_consumers(self):
        # Wants= pulls the three in on every boot, After= runs firstboot once they are up (or
        # failed), Before= keeps VPP, agent, API and nginx behind it; no propagating dependency
        # type names them, so their explicit restart never restarts firstboot.
        unit = self.unit('ngfw-firstboot.service')
        self.assertEqual(MAINTAINER.unit_setting(unit, 'Wants'), list(GATED_UNITS))
        self.assertEqual(MAINTAINER.unit_setting(unit, 'After'), list(GATED_UNITS))
        self.assertEqual(MAINTAINER.unit_setting(unit, 'Before'),
                         ['vpp.service', 'ngfw-agent.service', 'ngfw-api.service', 'nginx.service'])
        for key in MAINTAINER.DEPENDENCY_KEYS:
            self.assertEqual(MAINTAINER.unit_setting(unit, key), [], key)
        self.assertEqual(MAINTAINER.unit_setting(unit, 'WantedBy', 'Install'), ['multi-user.target'])
        self.assertNotIn('ConditionPathExists', unit)

    def test_vpp_agent_and_api_keep_requiring_a_completed_firstboot(self):
        # The gate below firstboot is unchanged (D-253): a fresh appliance runs firstboot before
        # VPP, a failed firstboot blocks VPP, agent and API, and agent/API also carry the marker condition.
        dropin = (SOURCE / 'assets/vpp-firstboot.conf').read_text()
        self.assertEqual(MAINTAINER.unit_setting(dropin, 'Requires'), ['ngfw-firstboot.service'])
        self.assertEqual(MAINTAINER.unit_setting(dropin, 'After'), ['ngfw-firstboot.service'])
        agent, api = self.unit('ngfw-agent.service'), self.unit('ngfw-api.service')
        self.assertLessEqual({'vpp.service', 'ngfw-firstboot.service', 'nftables.service'}, set(MAINTAINER.unit_setting(agent, 'Requires')))
        self.assertLessEqual({'vpp.service', 'ngfw-firstboot.service', 'nftables.service'}, set(MAINTAINER.unit_setting(agent, 'After')))
        self.assertLessEqual({'ngfw-agent.service', 'ngfw-firstboot.service'}, set(MAINTAINER.unit_setting(api, 'Requires')))
        self.assertLessEqual({'ngfw-agent.service', 'ngfw-firstboot.service', 'postgresql.service', 'valkey-server.service'},
                             set(MAINTAINER.unit_setting(api, 'After')))
        for text in (agent, api):
            self.assertIn('ConditionPathExists=/var/lib/ngfw/firstboot-complete', text)

    def test_explicit_restart_of_a_daemon_no_longer_reaches_vpp(self):
        # systemd.unit(5): only Requires=/Requisite=/BindsTo=/PartOf= propagate an explicit stop
        # or restart. Before D-255 the closure of vpp.service held nftables, firewall-bootstrap,
        # postgresql and valkey-server through firstboot's Requires=; the valkey-server and
        # nftables packages restart their units in their own upgrade postinst, and
        # `systemctl restart postgresql` is routine. Now only the provisioning oneshots reach VPP
        # (the closure counts [Install] RequiredBy= and the A/B .requires/ links, so
        # ngfw-upgrade-prepare.service is always in it).
        self.assertEqual(MAINTAINER.stop_propagation_closure('vpp.service'),
                         {'ngfw-firstboot.service', 'ngfw-upgrade-prepare.service'})
        # The agent keeps its own Requires= on the base firewall: an nftables restart restarts the
        # agent and the API (which re-converge VPP), never VPP itself.
        agent = {'vpp.service', 'ngfw-firstboot.service', 'nftables.service', 'ngfw-firewall-bootstrap.service',
                 'ngfw-upgrade-prepare.service'}
        self.assertEqual(MAINTAINER.stop_propagation_closure('ngfw-agent.service'), agent)
        self.assertEqual(MAINTAINER.stop_propagation_closure('ngfw-api.service'), agent | {'ngfw-agent.service'})
        for consumer in ('vpp.service', 'ngfw-agent.service', 'ngfw-api.service'):
            reached = MAINTAINER.stop_propagation_closure(consumer)
            self.assertNotIn('postgresql.service', reached, consumer)
            self.assertNotIn('valkey-server.service', reached, consumer)
        # Negative control: the pre-D-255 unit text (Requires=) puts all four back into VPP's closure.
        unit = self.unit('ngfw-firstboot.service')
        self.assertEqual(unit.count('\nWants='), 1)
        previous = {'ngfw-firstboot.service': unit.replace('\nWants=', '\nRequires=')}
        self.assertLessEqual({'nftables.service', 'ngfw-firewall-bootstrap.service', 'postgresql.service', 'valkey-server.service'},
                             MAINTAINER.stop_propagation_closure('vpp.service', overrides=previous))

    def test_upgrade_prepare_snapshot_wants_postgresql_and_stays_a_gate_when_enabled(self):
        # The A/B upgrade's pre-migration snapshot is linked into ngfw-firstboot.service.requires/
        # and ngfw-api.service.requires/ by deploy/upgrade/provision and ngfw-upgrade; with
        # Requires=postgresql.service a PostgreSQL restart reached VPP through it on A/B images.
        unit = self.unit('ngfw-upgrade-prepare.service')
        self.assertEqual(MAINTAINER.unit_setting(unit, 'Wants'), ['postgresql.service'])
        self.assertEqual(MAINTAINER.unit_setting(unit, 'After'), ['postgresql.service'])
        self.assertLessEqual({'ngfw-firstboot.service', 'ngfw-api.service'}, set(MAINTAINER.unit_setting(unit, 'Before')))
        for key in MAINTAINER.DEPENDENCY_KEYS:
            self.assertEqual(MAINTAINER.unit_setting(unit, key), [], key)
        self.assertEqual(MAINTAINER.unit_setting(unit, 'RequiredBy', 'Install'), ['ngfw-api.service', 'ngfw-firstboot.service'])

    def test_closure_helper_sees_every_propagating_setting_and_only_those(self):
        # Not vacuous: the helper must not pass because it ignores a setting.
        self.assertEqual(MAINTAINER.DEPENDENCY_KEYS, ('Requires', 'Requisite', 'BindsTo', 'PartOf'))
        text = ('[Unit]\nRequires=a.service\nWants=b.service\nAfter=b.service\nPartOf=c.service\n'
                'BindsTo=d.service\nRequisite=e.service\n# Requires=comment.service\n[Install]\nRequiredBy=f.service\n')
        self.assertEqual(MAINTAINER.unit_dependencies(text), {'a.service', 'c.service', 'd.service', 'e.service'})
        self.assertEqual(MAINTAINER.unit_setting(text, 'RequiredBy', 'Install'), ['f.service'])
        self.assertEqual(MAINTAINER.unit_setting(text, 'RequiredBy'), [])
        self.assertEqual(MAINTAINER.unit_setting('[Unit]\nAfter=a.service b.service\nAfter=\nAfter=c.service\n', 'After'), ['c.service'])


if __name__ == '__main__':
    unittest.main()
