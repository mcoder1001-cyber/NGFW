#!/usr/bin/env python3
"""Off-host checks for the packaged fixed carrier and attested helper lifecycle."""
import configparser
import hashlib
import os
import pathlib
import subprocess
import tempfile
import unittest

SOURCE = pathlib.Path(__file__).resolve().parents[1]
REPO = SOURCE.parents[2]


def asset(name):
    if (SOURCE / 'stage').is_dir():
        if name.endswith('.service'):
            return SOURCE / 'stage/usr/lib/systemd/system' / name
        if name.endswith('.conf'):
            return SOURCE / 'stage/usr/lib/tmpfiles.d' / name
        return SOURCE / 'stage/usr/lib/ngfw/pppoe-carrier-hooks' / name
    return REPO / 'scripts/pppoe-carrier-assets' / name


def unit(name):
    # systemd permits repeated keys; these units repeat only bind declarations.
    parsed = configparser.ConfigParser(interpolation=None, strict=False)
    parsed.optionxform = str
    parsed.read_string(asset(name).read_text())
    return parsed


class CarrierAssets(unittest.TestCase):
    def test_dormant_units_and_fixed_execution_graph(self):
        carrier = unit('ngfw-pppoe-carrier@.service')
        broker = unit('ngfw-pppoe-broker@.service')
        self.assertEqual(carrier['Unit']['Requires'], 'vpp.service')
        self.assertIn('vpp.service', carrier['Unit']['After'].split())
        self.assertEqual(broker['Unit']['Requires'], 'systemd-tmpfiles-setup.service')
        self.assertIn('systemd-tmpfiles-setup.service', broker['Unit']['After'].split())
        for parsed, operation in [(carrier, 'run'), (broker, 'broker-execute')]:
            self.assertNotIn('Install', parsed)
            self.assertEqual(parsed['Service']['ExecStart'],
                             '/usr/bin/python3 -I /usr/lib/ngfw/pppoe-carrier.py ' + operation + ' %i')
            self.assertEqual(parsed['Service']['User'], 'root')
            self.assertEqual(parsed['Service']['Group'], 'root')
            self.assertEqual(parsed['Service']['UMask'], '0077')
            self.assertEqual(parsed['Service']['NoNewPrivileges'], 'true')
            self.assertEqual(parsed['Service']['KillMode'], 'control-group')
        self.assertEqual(carrier['Service']['ReadWritePaths'], '/run/ngfw/pppoe/%i')
        self.assertEqual(carrier['Service']['ProtectSystem'], 'strict')
        # A private mount namespace on the broker would hide its persistent netns mounts.
        for option in ['PrivateTmp', 'ProtectSystem', 'ProtectHome']:
            self.assertNotIn(option, broker['Service'])
        rules = (SOURCE / 'debian/rules').read_text()
        self.assertIn('dh_installsystemd --no-enable --no-start', rules)

    def test_private_peer_root_is_writable_under_actual_agent_sandbox(self):
        base = '/var/lib/ngfw/agent/pppoe-carrier'
        staged = (SOURCE / 'stage').is_dir()
        agent_path = (SOURCE / 'stage/usr/lib/systemd/system/ngfw-agent.service'
                      if staged else REPO / 'deploy/systemd/ngfw-agent.service')
        agent = agent_path.read_text()
        self.assertIn('ProtectSystem=strict', agent.splitlines())
        writable = [path.lstrip('-+') for line in agent.splitlines()
                    if line.startswith('ReadWritePaths=') for path in line.split('=', 1)[1].split()]
        self.assertTrue(any(pathlib.PurePosixPath(base).is_relative_to(path) for path in writable),
                        'private PPP peer root is read-only in the packaged agent sandbox')
        carrier = asset('ngfw-pppoe-carrier@.service').read_text().splitlines()
        self.assertIn('BindReadOnlyPaths=' + base + '/%i/ppp:/etc/ppp', carrier)
        self.assertIn('d ' + base + ' 0700 root root -',
                      asset('ngfw-pppoe-carrier.conf').read_text().splitlines())

    def test_exact_private_tmpfiles_and_hook_dispatchers(self):
        rows = [line.split() for line in asset('ngfw-pppoe-carrier.conf').read_text().splitlines()
                if line.strip() and not line.startswith('#')]
        self.assertEqual(rows, [
            ['d', '/run/netns', '0755', 'root', 'root', '-'],
            ['d', '/run/ngfw-pppoe-carrier', '0700', 'root', 'root', '-'],
            ['d', '/run/ngfw/pppoe', '0700', 'root', 'root', '-'],
            ['d', '/run/ngfw/pppoe-broker', '0700', 'root', 'root', '-'],
            ['d', '/var/lib/ngfw/agent/pppoe-carrier', '0700', 'root', 'root', '-'],
        ])
        for name in ['ip-up', 'ip-down', 'ipv6-up', 'ipv6-down']:
            # Execute the actual hook with its subprocess boundary replaced: prove argv
            # preservation for metacharacters without executing any host dispatcher.
            import runpy
            from unittest.mock import patch
            args = [name, 'ppp0', 'a;$(false)', '192.0.2.1', '2001:db8::1']
            with patch('sys.argv', args), patch('subprocess.run') as run:
                runpy.run_path(str(asset(name)), run_name='__main__')
            actual = run.call_args.args[0]
            self.assertEqual(actual, ['/usr/bin/run-parts', '--exit-on-error',
                                     *['--arg=' + value for value in args[1:]], '/etc/ppp/' + name + '.d'])
            self.assertTrue(run.call_args.kwargs['check'])
            self.assertNotIn('shell', run.call_args.kwargs)

    def test_debhelper_transforms_preserve_attested_bytes(self):
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            binary_dir = root / 'debian/fixture/usr/lib/ngfw'
            binary_dir.mkdir(parents=True)
            helpers = ['ngfw-ra-daemon', 'ngfw-ra-namespace-broker', 'ngfw-wan-probe']
            for name in helpers + ['other-native-binary']:
                (binary_dir / name).write_bytes(b'executable-before-transforms')
            before = {name: hashlib.sha256((binary_dir / name).read_bytes()).hexdigest() for name in helpers}
            stub = root / 'stub'
            stub.mkdir()
            # Emulate an observable byte transformation unless the actual rules
            # exclude this filename. The unrelated binary must still be processed.
            for command in ['dh_strip', 'dh_dwz']:
                path = stub / command
                path.write_text('''#!/usr/bin/python3
import pathlib, sys
excluded = [arg[2:] for arg in sys.argv[1:] if arg.startswith('-X')]
for path in pathlib.Path('debian/fixture/usr/lib/ngfw').iterdir():
    if not any(part in str(path) for part in excluded):
        path.write_bytes(path.read_bytes() + b'-transformed')
''')
                path.chmod(0o755)
            result = subprocess.run(['make', '-f', str(SOURCE / 'debian/rules'),
                                     'override_dh_strip', 'override_dh_dwz'], cwd=root,
                                    env={**os.environ, 'PATH': str(stub) + os.pathsep + os.environ['PATH']},
                                    capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            for name in helpers:
                self.assertEqual(hashlib.sha256((binary_dir / name).read_bytes()).hexdigest(), before[name])
            self.assertEqual((binary_dir / 'other-native-binary').read_bytes(),
                             b'executable-before-transforms-transformed-transformed')


if __name__ == '__main__':
    unittest.main()
