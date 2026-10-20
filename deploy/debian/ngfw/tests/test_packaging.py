#!/usr/bin/env python3
"""Host-independent regression checks; never execute package installation."""
import pathlib
import importlib.util
import shlex
import os
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

SOURCE = pathlib.Path(__file__).resolve().parents[1]
ROOT = SOURCE.parents[2]
SPEC = importlib.util.spec_from_file_location('api_storage', SOURCE / 'assets/provision-api-storage.py')
STORAGE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(STORAGE)
SCRIPTS_SPEC = importlib.util.spec_from_file_location('maintainer_scripts', SOURCE / 'tests/maintainer_scripts.py')
MAINTAINER = importlib.util.module_from_spec(SCRIPTS_SPEC)
SCRIPTS_SPEC.loader.exec_module(MAINTAINER)
PROVISIONING_UNITS = {'ngfw-firstboot.service', 'ngfw-firewall-bootstrap.service'}


class Packaging(unittest.TestCase):
    def setUp(self):
        # Hosted unit runners are unprivileged. Exercise the real descriptor and
        # mode handling while mapping requested root ownership to fixture owner.
        # The separate privilege test below runs actual uid boundaries as root.
        if os.geteuid() != 0:
            real_chown = os.fchown
            patcher = mock.patch.object(STORAGE.os, 'fchown',
                side_effect=lambda fd, uid, gid: real_chown(fd, os.getuid() if uid == 0 else uid, gid))
            patcher.start()
            self.addCleanup(patcher.stop)

    def test_api_storage_parent_requests_root_ownership(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = pathlib.Path(temporary)
            with mock.patch.object(STORAGE.os, 'fchown') as chown:
                STORAGE.provision(root / 'api', root / 'data', 1234, 5678)
            self.assertEqual([call.args[1:] for call in chown.call_args_list],
                             [(1234, 5678), (0, 5678), (1234, 5678), (1234, 5678), (1234, 5678)])

    def test_api_cannot_replace_root_state_but_can_write_owned_children(self):
        if os.geteuid() != 0:
            result = subprocess.run(["sudo", "-n", sys.executable, str(pathlib.Path(__file__).resolve()),
                "Packaging.test_api_cannot_replace_root_state_but_can_write_owned_children"],
                capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, "real UID boundary fixture requires passwordless sudo: " + result.stderr)
            self.assertIn("Ran 1 test", result.stderr)
            self.assertNotIn("skipped", result.stderr)
            return
        with tempfile.TemporaryDirectory() as temporary:
            root = pathlib.Path(temporary)
            root.chmod(0o755)
            data = root / 'data'
            STORAGE.provision(root / 'api', data, 65534, 65534)
            state = data / 'ngfw-upgrade'
            state.mkdir(mode=0o700)
            (state / 'state.json').write_bytes(b'protected')
            script = """import os,sys
from pathlib import Path
data=Path(sys.argv[1])
try:
    os.rename(data/'ngfw-upgrade', data/'replacement')
except PermissionError:
    pass
else:
    raise SystemExit('API renamed privileged state')
for name in ('backups','updates','support'):
    (data/name/'api-owned').write_bytes(b'allowed')
"""
            def unprivileged():
                os.setgroups([])
                os.setgid(65534)
                os.setuid(65534)
            result = subprocess.run(['python3', '-c', script, str(data)], preexec_fn=unprivileged,
                                    capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual((state / 'state.json').read_bytes(), b'protected')
            for name in ('backups', 'updates', 'support'):
                self.assertEqual((data / name / 'api-owned').read_bytes(), b'allowed')

    def test_api_storage_reconfigure_preserves_existing_data(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = pathlib.Path(temporary)
            def configure():
                STORAGE.provision(root / 'api', root / 'data', os.getuid(), os.getgid())
            configure()
            preserved = root / 'data/backups/operator-backup'
            preserved.write_bytes(b'existing backup data')
            preserved.chmod(0o600)
            metadata = preserved.stat()
            configure()
            self.assertEqual(preserved.read_bytes(), b'existing backup data')
            self.assertEqual(preserved.stat().st_mode, metadata.st_mode)
            self.assertEqual(preserved.stat().st_uid, metadata.st_uid)
            self.assertEqual(preserved.stat().st_gid, metadata.st_gid)
            for name in ['', 'backups', 'updates', 'support']:
                directory = root / 'data' / name
                self.assertEqual(directory.stat().st_mode & 0o777, 0o750)

    def test_build_refuses_missing_verified_inputs(self):
        with tempfile.TemporaryDirectory() as temporary:
            result = subprocess.run(
                ['make', '-f', str(SOURCE / 'debian/rules'), 'override_dh_auto_configure'],
                cwd=temporary, capture_output=True, text=True, check=False,
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertIn('Missing verified VPP artifact version', result.stderr)

    def test_api_storage_helper_is_shipped_with_direct_python_dependency(self):
        postinst = (SOURCE / 'debian/ngfw-api.postinst').read_text()
        self.assertIn('/usr/bin/python3 /usr/lib/ngfw/provision-api-storage.py', postinst)
        self.assertNotIn('install -d', postinst)
        self.assertIn('assets/provision-api-storage.py usr/lib/ngfw/',
                      (SOURCE / 'debian/ngfw-api.install').read_text())
        control = (SOURCE / 'debian/control').read_text().split('Package: ngfw-api\n')[1].split('\n\n')[0]
        self.assertIn('python3', control)

    def test_api_storage_refuses_links_without_touching_targets(self):
        for location in ('api', 'data', 'data/backups', 'data/updates', 'data/support'):
            with self.subTest(location=location), tempfile.TemporaryDirectory() as temporary:
                root = pathlib.Path(temporary)
                target = root / 'external'
                target.mkdir(mode=0o700)
                (target / 'operator-data').write_bytes(b'keep')
                before = target.stat()
                link = root / location
                link.parent.mkdir(parents=True, exist_ok=True)
                link.symlink_to(target, target_is_directory=True)
                with self.assertRaises(OSError):
                    STORAGE.provision(root / 'api', root / 'data', os.getuid(), os.getgid())
                after = target.stat()
                self.assertEqual((after.st_mode, after.st_uid, after.st_gid),
                                 (before.st_mode, before.st_uid, before.st_gid))
                self.assertEqual((target / 'operator-data').read_bytes(), b'keep')
                self.assertTrue(link.is_symlink())

    def test_api_storage_child_swap_cannot_redirect_permissions(self):
        for before_open in (True, False):
            with self.subTest(before_open=before_open), tempfile.TemporaryDirectory() as temporary:
                root = pathlib.Path(temporary)
                data = root / 'data'
                data.mkdir()
                (data / 'backups').mkdir()
                target = root / 'external'
                target.mkdir(mode=0o700)
                before = target.stat()
                original_open = os.open
                swapped = False
                def racing_open(path, flags, *args, **kwargs):
                    nonlocal swapped
                    if path != 'backups' or swapped:
                        return original_open(path, flags, *args, **kwargs)
                    descriptor = None if before_open else original_open(path, flags, *args, **kwargs)
                    (data / 'backups').rename(data / 'original-backups')
                    (data / 'backups').symlink_to(target, target_is_directory=True)
                    swapped = True
                    return original_open(path, flags, *args, **kwargs) if before_open else descriptor
                with mock.patch.object(STORAGE.os, 'open', side_effect=racing_open):
                    if before_open:
                        with self.assertRaises(OSError):
                            STORAGE.provision(root / 'api', data, os.getuid(), os.getgid())
                    else:
                        STORAGE.provision(root / 'api', data, os.getuid(), os.getgid())
                self.assertTrue(swapped)
                after = target.stat()
                self.assertEqual((after.st_mode, after.st_uid, after.st_gid),
                                 (before.st_mode, before.st_uid, before.st_gid))
                self.assertTrue((data / 'backups').is_symlink())

    def test_units_preserve_privilege_boundary(self):
        units = ROOT / 'deploy/systemd'
        if not units.is_dir():
            units = SOURCE / 'stage/usr/lib/systemd/system'
        agent = (units / 'ngfw-agent.service').read_text()
        api = (units / 'ngfw-api.service').read_text()
        self.assertIn('Requires=vpp.service', agent)
        self.assertIn('Environment=NGFW_VPP_ID_RANGE=all', agent)
        self.assertNotIn('NGFW_VPP_TABLE_BASE=', agent)
        self.assertIn('CapabilityBoundingSet=CAP_NET_ADMIN CAP_SYS_ADMIN CAP_IPC_LOCK CAP_CHOWN CAP_DAC_OVERRIDE\n', agent)
        self.assertIn('AF_NETLINK', agent)
        self.assertIn('/etc/ngfw/rsyslog-tls', agent)
        self.assertIn('/var/lib/ngfw/captures', agent)
        if (ROOT / 'apps/agent').is_dir():
            renderer = (ROOT / 'apps/agent/internal/renderers/rsyslog/paths.go').read_text()
            capture = (ROOT / 'apps/agent/internal/actions/capture-trace/capture.go').read_text()
            self.assertIn('TLSDir:      "/etc/ngfw/rsyslog-tls"', renderer)
            self.assertIn('c.Dir = "/var/lib/ngfw/captures"', capture)
        self.assertIn('CapabilityBoundingSet=\n', api)
        self.assertNotIn('AF_NETLINK', api)
        self.assertNotIn('MemoryDenyWriteExecute=yes', api)
        self.assertNotIn('ProtectHostname=yes', agent)
        self.assertNotIn('ProtectClock=yes', agent)
        self.assertNotIn('PrivateDevices=yes', agent)
        self.assertNotIn('RestrictNamespaces=yes', agent)
        self.assertIn('RestrictNamespaces=~cgroup', agent)
        for unit in [agent, api]:
            self.assertIn('ConditionPathExists=/var/lib/ngfw/firstboot-complete', unit)
            self.assertIn('ProtectSystem=strict', unit)
            self.assertIn('ProtectHome=yes', unit)
            self.assertIn('NoNewPrivileges=yes', unit)

    def test_api_unit_enables_default_nic_registration(self):
        # D-192/D-252: the appliance registers its data NICs at every API start. systemd applies Environment= lines in
        # order (the last assignment wins), EnvironmentFile= overrides them whatever the line order, and
        # UnsetEnvironment= removes a variable: check the effective value, not a line position.
        units = ROOT / 'deploy/systemd'
        if not units.is_dir():
            units = SOURCE / 'stage/usr/lib/systemd/system'
        texts = [(units / 'ngfw-api.service').read_text()]
        for dropin in sorted((ROOT / 'deploy/hardening/systemd/ngfw-api.service.d').glob('*.conf')):
            texts.append(dropin.read_text())
        value = None
        for text in texts:
            for line in text.splitlines():
                key, sep, rest = line.strip().partition('=')
                if not sep:
                    continue
                if key == 'Environment':
                    if rest.strip() == '':
                        value = None  # an empty assignment resets the whole environment list
                    for assignment in shlex.split(rest):
                        name, eq, assigned = assignment.partition('=')
                        if eq and name == 'NGFW_SEED_DEFAULT_NICS':
                            value = assigned
                elif key == 'UnsetEnvironment':
                    self.assertNotIn('NGFW_SEED_DEFAULT_NICS', shlex.split(rest),
                                     'the appliance unit must not unset the default NIC registration opt-in')
        self.assertIn(value, ('1', 'true'),
                      'the appliance API unit (and its hardening drop-in) must leave NGFW_SEED_DEFAULT_NICS enabled')

    def test_pppoe_runtime_dependencies_belong_to_agent(self):
        # The agent renders pppd/rp-pppoe units and starts DHCPv6 helpers.
        # Direct dependencies keep standalone ngfw-agent installs usable too.
        control = (SOURCE / 'debian/control').read_text()
        agent = control.split('Package: ngfw-agent\n', 1)[1].split('\n\n', 1)[0]
        dependencies = agent.split('Depends: ', 1)[1].split('\n', 1)[0].split(', ')
        for package in ('ppp', 'pppoe', 'dhcpcd-base', 'python3'):
            with self.subTest(package=package):
                self.assertIn(package, dependencies)

    def test_runtime_dependency_contract(self):
        control = (SOURCE / 'debian/control').read_text()
        self.assertIn('vpp (= ${ngfw:VppVersion})', control)
        self.assertNotIn('kea-ctrl-agent', control)
        self.assertNotIn('vpp-dev', control)
        self.assertIn('rsyslog-openssl', control)
        self.assertIn('nodejs (<< 23)', control)

    def test_http_never_proxies_and_tls_has_ws_and_api_routes(self):
        config = (SOURCE / 'assets/nginx.conf').read_text()
        cleartext, secure = config.split('server {', 2)[1:]
        self.assertIn('listen 80', cleartext)
        self.assertIn('return 308 https://', cleartext)
        self.assertNotIn('proxy_pass', cleartext)
        self.assertIn('listen 443 ssl', secure)
        self.assertIn('proxy_set_header Upgrade $http_upgrade', secure)
        self.assertIn('proxy_set_header X-Forwarded-For $remote_addr', secure)
        self.assertNotIn('$proxy_add_x_forwarded_for', secure)
        self.assertIn('location /restconf', secure)
        self.assertIn('location = /.well-known/host-meta', secure)

    def test_tls_first_run_retry_and_partial_pair_refusal(self):
        script = SOURCE / 'assets/tls-bootstrap.sh'
        with tempfile.TemporaryDirectory() as directory:
            tls = pathlib.Path(directory) / 'tls'
            subprocess.run(['bash', str(script), '--directory', str(tls)], check=True)
            key, cert = (tls / 'server.key'), (tls / 'server.crt')
            before = (key.read_bytes(), cert.read_bytes())
            self.assertEqual(key.stat().st_mode & 0o777, 0o600)
            subprocess.run(['bash', str(script), '--directory', str(tls)], check=True)
            self.assertEqual(before, (key.read_bytes(), cert.read_bytes()))
            cert.unlink()
            result = subprocess.run(['bash', str(script), '--directory', str(tls)],
                                    capture_output=True, check=False)
            self.assertNotEqual(result.returncode, 0)
            self.assertEqual(before[0], key.read_bytes())
            self.assertFalse(cert.exists())

    def test_product_startup_apply_finds_packaged_helpers(self):
        install = (SOURCE / 'debian/ngfw-agent.install').read_text()
        self.assertIn('stage/usr/sbin/ngfw-startupgen usr/lib/ngfw/bin/', install)
        self.assertIn('stage/usr/sbin/ngfw-vppcheck usr/lib/ngfw/bin/', install)
        if (ROOT / 'deploy/vpp/apply-startup.sh').is_file():
            apply = (ROOT / 'deploy/vpp/apply-startup.sh').read_text()
            self.assertIn('${NGFW_LIB_BIN:=/usr/lib/ngfw/bin}', apply)

    def test_vpp_waits_for_successful_firstboot_not_only_ordering(self):
        dropin = (SOURCE / 'assets/vpp-firstboot.conf').read_text()
        self.assertIn('Requires=ngfw-firstboot.service', dropin)
        self.assertIn('After=ngfw-firstboot.service', dropin)
        meta = (SOURCE / 'debian/ngfw-meta.postinst').read_text()
        self.assertIn('deb-systemd-helper enable', meta)
        self.assertNotIn('systemctl start', meta)
        self.assertNotIn('systemctl restart', meta)

    def test_meta_only_registers_fixed_boot_units_without_starting_services(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = pathlib.Path(temporary)
            commands = root / 'commands'
            helper = root / 'deb-systemd-helper'
            helper.write_text('#!/bin/sh\nprintf "%s\\n" "$*" >> "$FIXTURE_COMMANDS"\n')
            helper.chmod(0o755)
            env = {**os.environ, 'PATH': str(root), 'FIXTURE_COMMANDS': str(commands)}
            subprocess.run(['/bin/sh', str(SOURCE / 'debian/ngfw-meta.postinst'), 'configure'],
                           env=env, check=True, capture_output=True)
            self.assertEqual(commands.read_text().splitlines(), [
                'enable ngfw-ra-openfile.socket', 'enable apply-executor.socket',
                'enable ngfw-firstboot.service', 'enable ngfw-agent.service', 'enable ngfw-api.service',
            ])
            commands.unlink()
            subprocess.run(['/bin/sh', str(SOURCE / 'debian/ngfw-meta.postinst'), 'abort-upgrade'],
                           env=env, check=True, capture_output=True)
            self.assertFalse(commands.exists())

    def test_maintainer_scripts_are_valid_and_no_destructive_actions(self):
        for path in (SOURCE / 'debian').glob('*.postinst'):
            subprocess.run(['sh', '-n', str(path)], check=True)
            script = path.read_text()
            self.assertNotIn('rm ', script)
            self.assertNotIn('systemctl', script)
        rules = (SOURCE / 'debian/rules').read_text()
        self.assertIn('dh_installsystemd --no-enable --no-start --no-stop-on-upgrade', rules)


class MaintainerScripts(unittest.TestCase):
    """The scripts debhelper generates from debian/rules, run against recording fakes (D-253)."""

    @classmethod
    def setUpClass(cls):
        missing = MAINTAINER.missing_tools()
        if missing:
            raise unittest.SkipTest('debhelper is required to generate the maintainer scripts; missing: ' + ' '.join(missing))
        cls.generated = MAINTAINER.generate()
        cls.packaged = [package for package in MAINTAINER.packages() if MAINTAINER.maintscript_units(package)]

    def upgrade_calls(self, package):
        """(script, dpkg arguments, recorded fake calls) for every maintainer-script invocation of an upgrade."""
        calls = []
        with tempfile.TemporaryDirectory() as temporary:
            recorder = MAINTAINER.Recorder(temporary)
            for script, invocations in MAINTAINER.UPGRADE_PATHS.items():
                text = self.generated[package].get(script, '')
                for arguments in invocations:
                    calls.append((script, arguments, recorder.run(text, *arguments) if text else {}))
        return calls

    def test_four_packages_and_their_units_are_generated(self):
        self.assertEqual(sorted(self.generated), ['ngfw-agent', 'ngfw-api', 'ngfw-meta', 'ngfw-web'])
        self.assertEqual(sorted(self.packaged), ['ngfw-agent', 'ngfw-api', 'ngfw-meta'])
        self.assertEqual(MAINTAINER.maintscript_units('ngfw-meta'), sorted(PROVISIONING_UNITS))
        self.assertIn('ngfw-agent.service', MAINTAINER.maintscript_units('ngfw-agent'))
        self.assertIn('apply-executor.socket', MAINTAINER.maintscript_units('ngfw-agent'))
        self.assertEqual(self.generated['ngfw-web'], {})

    def test_upgrade_paths_never_stop_start_or_restart_a_unit(self):
        # --no-start --no-stop-on-upgrade: an upgrade changes files on disk and nothing about running
        # units. Daemons keep running the previous version until the operator restarts them
        # (deploy/deploy-hosts.sh restarts agent and API); sockets and the completed provisioning
        # oneshots keep their state. Before D-253 the preinst stopped every shipped unit on upgrade
        # and --no-start never brought the sockets back.
        for package in self.packaged:
            with self.subTest(package=package):
                for script, arguments, calls in self.upgrade_calls(package):
                    self.assertEqual(calls.get('deb-systemd-invoke', []), [], (script, arguments))
                for section in MAINTAINER.debhelper_sections(self.generated[package]['postinst']):
                    self.assertNotIn('deb-systemd-invoke', section)

    def test_removal_still_stops_every_shipped_unit(self):
        # Positive control: the fixture runs the real snippets, and `apt-get remove` still stops the units.
        for package in self.packaged:
            with self.subTest(package=package), tempfile.TemporaryDirectory() as temporary:
                calls = MAINTAINER.Recorder(temporary).run(self.generated[package]['prerm'], 'remove')
                self.assertEqual(MAINTAINER.stopped_units(calls), set(MAINTAINER.maintscript_units(package)))
                self.assertEqual(len(calls.get('deb-systemd-invoke', [])), 1)

    def test_upgrade_never_stops_a_unit_vpp_requires(self):
        # vpp.service carries Requires=ngfw-firstboot.service (assets/vpp-firstboot.conf). An explicit
        # stop of that unit stops VPP, the agent and the API (systemd.unit(5), Requires=): the
        # 2026-10-13 and 2026-10-19 upgrades of appliance 37 restarted VPP for about nine seconds this
        # way. Before D-255 the closure went on through firstboot's Requires= to nftables.service,
        # ngfw-firewall-bootstrap.service, postgresql.service and valkey-server.service; now the
        # provisioning oneshot is the only unit whose stop reaches VPP (ngfw-upgrade-prepare.service
        # joins it on A/B images, where it is linked into ngfw-firstboot.service.requires/).
        guarded = MAINTAINER.stop_propagation_closure('vpp.service', enabled=True)
        shipped = {unit for package in self.packaged for unit in MAINTAINER.maintscript_units(package)}
        self.assertEqual(MAINTAINER.stop_propagation_closure('vpp.service'), {'ngfw-firstboot.service'})
        self.assertEqual(guarded, {'ngfw-firstboot.service', 'ngfw-upgrade-prepare.service'})
        self.assertLessEqual(guarded, shipped)
        for daemon in ('nftables.service', 'postgresql.service', 'valkey-server.service'):
            self.assertNotIn(daemon, guarded)
        for package in self.packaged:
            with self.subTest(package=package):
                for script, arguments, calls in self.upgrade_calls(package):
                    self.assertEqual(MAINTAINER.stopped_units(calls) & guarded, set(), (script, arguments))
        with tempfile.TemporaryDirectory() as temporary:  # not vacuous: removing the packages does stop them
            recorder = MAINTAINER.Recorder(temporary)
            self.assertEqual(MAINTAINER.stopped_units(recorder.run(self.generated['ngfw-meta']['prerm'], 'remove')),
                             PROVISIONING_UNITS)
            self.assertIn('ngfw-upgrade-prepare.service',
                          MAINTAINER.stopped_units(recorder.run(self.generated['ngfw-agent']['prerm'], 'remove')))

    def test_rules_without_the_flag_would_stop_the_provisioning_units_on_upgrade(self):
        # Negative control: the same fixture with the pre-D-253 arguments reproduces the stop, whichever
        # script this debhelper puts it in, so a lost flag is caught rather than passed vacuously.
        previous = MAINTAINER.generate(['--no-enable', '--no-start'])
        stopped = set()
        with tempfile.TemporaryDirectory() as temporary:
            recorder = MAINTAINER.Recorder(temporary)
            for script, invocations in MAINTAINER.UPGRADE_PATHS.items():
                for arguments in invocations:
                    if script in previous['ngfw-meta']:
                        stopped |= MAINTAINER.stopped_units(recorder.run(previous['ngfw-meta'][script], *arguments))
        self.assertEqual(stopped, PROVISIONING_UNITS)


if __name__ == '__main__':
    unittest.main()
