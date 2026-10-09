#!/usr/bin/env python3
"""Host-independent regression checks; never execute package installation."""
import pathlib
import importlib.util
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
        self.assertIn('dh_installsystemd --no-enable --no-start', rules)


if __name__ == '__main__':
    unittest.main()
