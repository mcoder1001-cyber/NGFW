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
        self.assertIn('CapabilityBoundingSet=\n', api)
        self.assertNotIn('AF_NETLINK', api)
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
        self.assertIn('rsyslog-gnutls', control)
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
