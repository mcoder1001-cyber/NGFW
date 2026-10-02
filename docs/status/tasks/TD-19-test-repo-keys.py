#!/usr/bin/env python3
"""Exercise actual public-key gate/parser with controlled GPG output, no key retrieval."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[3]
A = 'A' * 40
B = 'B' * 40


def primary(fingerprint):
    return 'pub:-:2048:1:0123456789ABCDEF:1:0:::::scSC:\nfpr:::::::::' + fingerprint + ':\nuid:::::::::Fixture:\n'


class RepoKeys(unittest.TestCase):
    def run_gate(self, identities=None, packets=':public key packet:', kind='regular', expected=A):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            stubs = root / 'stubs'
            stubs.mkdir()
            gpg = stubs / 'gpg'
            gpg.write_text('''#!/usr/bin/python3
import json,os,pathlib,sys
args=sys.argv[1:]
with open(os.environ['CALL_LOG'],'a') as log: log.write(json.dumps(args)+'\\n')
assert args[0]=='--no-options' and args[1]=='--homedir' and args[3]=='--batch'
if '--list-packets' in args: print(os.environ['PACKETS'])
elif '--show-keys' in args:
    assert args[4:7]==['--with-colons','--with-fingerprint','--show-keys']
    print(os.environ['IDENTITIES'])
elif '--dearmor' in args:
    assert args[4:7]==['--yes','--dearmor','--output']
    pathlib.Path(args[7]).write_bytes(b'fixture-public-keyring')
else: raise SystemExit(91)
''')
            gpg.chmod(0o755)
            key = root / 'input.key'
            key.write_bytes(b'public-key fixture input')
            if kind == 'symlink':
                key.unlink(); key.symlink_to(root / 'foreign'); (root / 'foreign').write_bytes(b'fixture')
            elif kind == 'empty': key.write_bytes(b'')
            elif kind == 'oversize': key.write_bytes(b'x' * (1024 * 1024 + 1))
            home = root / 'home'; home.mkdir(mode=0o700)
            output = root / 'output.gpg'
            source = (ROOT / 'scripts/00-add-repos.sh').read_text()
            start = source.index('check_key_pins() {')
            end = source.index('if [[ ${1:-} == --check-artifacts ]]', start)
            harness = 'set -euo pipefail\n' + source[start:end] + '\nverify_repo_key "$KEY" "$EXPECTED" "$KEY_HOME" "$KEY_OUTPUT"\n'
            log = root / 'calls'
            result = subprocess.run(['bash', '-c', harness], text=True, capture_output=True, env=dict(os.environ,
                PATH=f'{stubs}:/usr/bin:/bin', CALL_LOG=str(log), PACKETS=packets, IDENTITIES=identities or primary(A),
                KEY=str(key), EXPECTED=expected, KEY_HOME=str(home), KEY_OUTPUT=str(output)))
            return result, [json.loads(line) for line in log.read_text().splitlines()] if log.exists() else [], output.exists()

    def test_exact_primary_set_fixed_argv_and_private_output(self):
        result, calls, output = self.run_gate(identities=primary(A)+primary(B), expected=A+','+B)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(output)
        self.assertEqual(len(calls), 3)
        self.assertIn('--list-packets', calls[0])
        self.assertIn('--show-keys', calls[1])
        self.assertIn('--dearmor', calls[2])
        self.assertTrue(all('/home' in call[2] for call in calls))

    def test_unmatched_extra_missing_duplicate_or_malformed_primary_refused(self):
        for identities in (primary(B), primary(A)+primary(B), primary(A)+primary(A), primary('short'), 'pub:-:2048:1:fixture:\nuid:::::::::Fixture:\n', ''):
            with self.subTest(identities=identities):
                # Empty output must remain genuinely empty rather than default fixture.
                result, calls, output = self.run_gate(identities=identities or '\n')
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(output)
                self.assertFalse(any('--dearmor' in call for call in calls))

    def test_secret_packets_or_secret_identity_refused(self):
        for packets, identities in ((':secret key packet:', primary(A)), (':secret sub key packet:', primary(A)), (':public key packet:', 'sec:-:2048:1:fixture:\n'+primary(A))):
            with self.subTest(packets=packets, identities=identities):
                result, calls, output = self.run_gate(packets=packets, identities=identities)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(output)
                self.assertFalse(any('--dearmor' in call for call in calls))

    def test_nonregular_empty_oversize_refused_before_gpg(self):
        for kind in ('symlink', 'empty', 'oversize'):
            with self.subTest(kind=kind):
                result, calls, output = self.run_gate(kind=kind)
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(calls, [])
                self.assertFalse(output)

    def test_missing_malformed_pin_configuration_before_network_or_apt(self):
        source = (ROOT / 'scripts/00-add-repos.sh').read_text()
        start = source.index('check_key_pins() {'); end = source.index('verify_repo_key()', start)
        for expected in ('', 'a'*40, 'A'*39, A+','+A, A+';echo unsafe'):
            with self.subTest(expected=expected):
                result = subprocess.run(['bash','-c','set -euo pipefail\n'+source[start:end]+'\ncheck_key_pins'],capture_output=True,text=True,
                    env=dict(os.environ, VRX_FRR_KEY_FINGERPRINTS=expected,VRX_NODESOURCE_KEY_FINGERPRINTS=B))
                self.assertNotEqual(result.returncode,0)
                self.assertIn('trusted exact primary',result.stderr)
        self.assertLess(source.index('\ncheck_key_pins\n'),source.index('\ncurl -fsSL'))
        self.assertLess(source.index('verify_repo_key "$repo_work/node.key"'), source.index('\napt-get update'))

    def test_actual_repository_entry_refuses_missing_pins_before_verifier(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            script = root / 'setup.sh'
            source = (ROOT / 'scripts/00-add-repos.sh').read_text().replace('[[ $EUID -eq 0 ]]', '[[ 1 -eq 1 ]]', 1)
            script.write_text(source)
            env = dict(os.environ, VRX_VPP_ARTIFACTS=str(root / 'missing-artifacts'))
            env.pop('VRX_FRR_KEY_FINGERPRINTS', None)
            env.pop('VRX_NODESOURCE_KEY_FINGERPRINTS', None)
            result = subprocess.run(['bash', str(script)], env=env, text=True, capture_output=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn('VRX_FRR_KEY_FINGERPRINTS', result.stderr)
            self.assertNotIn('realpath:', result.stderr)

    def test_revoked_or_expired_primary_refused(self):
        for validity in ('r', 'e', 'i', 'd', '?', 'n', 'w', 's', '', 'future'):
            result, _, output = self.run_gate(identities=primary(A).replace('pub:-:',f'pub:{validity}:'))
            self.assertNotEqual(result.returncode,0)
            self.assertFalse(output)

    def test_malformed_primary_fields_and_disabled_or_nonsigning_capabilities_refused(self):
        for index, value in ((2, ''), (2, 'bits'), (3, 'algorithm'), (4, 'short'), (5, 'created'), (6, 'expiry'), (11, 'scSCD'), (11, 'cC'), (11, '?')):
            with self.subTest(index=index, value=value):
                lines = primary(A).splitlines()
                fields = lines[0].split(':'); fields[index] = value
                lines[0] = ':'.join(fields)
                result, calls, output = self.run_gate(identities='\n'.join(lines))
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(output)
                self.assertFalse(any('--dearmor' in call for call in calls))

    def test_supported_unknown_and_valid_primary_states(self):
        for validity in ('-', 'o', 'q', 'm', 'f', 'u'):
            with self.subTest(validity=validity):
                result, _, output = self.run_gate(identities=primary(A).replace('pub:-:', f'pub:{validity}:'))
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertTrue(output)


if __name__ == '__main__':
    unittest.main(verbosity=2)
