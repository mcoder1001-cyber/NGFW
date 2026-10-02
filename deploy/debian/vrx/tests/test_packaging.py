#!/usr/bin/env python3
"""Host-independent regression checks; never execute package installation."""
import pathlib
import subprocess
import tempfile
import unittest

SOURCE = pathlib.Path(__file__).resolve().parents[1]
ROOT = SOURCE.parents[2]


class Packaging(unittest.TestCase):
    def test_build_refuses_missing_verified_inputs(self):
        with tempfile.TemporaryDirectory() as temporary:
            result = subprocess.run(
                ['make', '-f', str(SOURCE / 'debian/rules'), 'override_dh_auto_configure'],
                cwd=temporary, capture_output=True, text=True, check=False,
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertIn('Missing verified VPP artifact version', result.stderr)

    def test_units_preserve_privilege_boundary(self):
        units = ROOT / 'deploy/systemd'
        if not units.is_dir():
            units = SOURCE / 'stage/usr/lib/systemd/system'
        agent = (units / 'vrx-agent.service').read_text()
        api = (units / 'vrx-api.service').read_text()
        self.assertIn('Requires=vpp.service', agent)
        self.assertIn('CapabilityBoundingSet=CAP_NET_ADMIN CAP_SYS_ADMIN CAP_IPC_LOCK', agent)
        self.assertIn('AF_NETLINK', agent)
        self.assertIn('/etc/vrx/rsyslog-tls', agent)
        self.assertIn('/var/lib/vrx/captures', agent)
        if (ROOT / 'apps/agent').is_dir():
            renderer = (ROOT / 'apps/agent/internal/renderers/rsyslog/paths.go').read_text()
            capture = (ROOT / 'apps/agent/internal/actions/capture-trace/capture.go').read_text()
            self.assertIn('TLSDir:      "/etc/vrx/rsyslog-tls"', renderer)
            self.assertIn('c.Dir = "/var/lib/vrx/captures"', capture)
        self.assertIn('CapabilityBoundingSet=\n', api)
        self.assertNotIn('AF_NETLINK', api)
        self.assertNotIn('MemoryDenyWriteExecute=yes', api)
        self.assertNotIn('ProtectHostname=yes', agent)
        self.assertNotIn('ProtectClock=yes', agent)
        self.assertNotIn('PrivateDevices=yes', agent)
        self.assertNotIn('RestrictNamespaces=yes', agent)
        self.assertIn('RestrictNamespaces=~cgroup', agent)
        for unit in [agent, api]:
            self.assertIn('ConditionPathExists=/var/lib/vrx/firstboot-complete', unit)
            self.assertIn('ProtectSystem=strict', unit)
            self.assertIn('ProtectHome=yes', unit)
            self.assertIn('NoNewPrivileges=yes', unit)

    def test_runtime_dependency_contract(self):
        control = (SOURCE / 'debian/control').read_text()
        self.assertIn('vpp (= ${vrx:VppVersion})', control)
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
        install = (SOURCE / 'debian/vrx-agent.install').read_text()
        self.assertIn('stage/usr/sbin/vrx-startupgen usr/lib/vrx/bin/', install)
        self.assertIn('stage/usr/sbin/vrx-vppcheck usr/lib/vrx/bin/', install)
        if (ROOT / 'deploy/vpp/apply-startup.sh').is_file():
            apply = (ROOT / 'deploy/vpp/apply-startup.sh').read_text()
            self.assertIn('${VRX_LIB_BIN:=/usr/lib/vrx/bin}', apply)

    def test_vpp_waits_for_successful_firstboot_not_only_ordering(self):
        dropin = (SOURCE / 'assets/vpp-firstboot.conf').read_text()
        self.assertIn('Requires=vrx-firstboot.service', dropin)
        self.assertIn('After=vrx-firstboot.service', dropin)
        meta = (SOURCE / 'debian/vrx-meta.postinst').read_text()
        self.assertIn('deb-systemd-helper enable', meta)
        self.assertNotIn('systemctl start', meta)
        self.assertNotIn('systemctl restart', meta)

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
