#!/usr/bin/env python3
"""Private certificate fixtures and blocked repository entry; no host installation."""
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[3]
SOURCE = (ROOT / 'scripts/00-add-repos.sh').read_text()
FUNCTIONS = SOURCE[SOURCE.index('check_key_pins() {'):SOURCE.index('if [[ ${1:-} == --check-artifacts ]]')]
A, B = 'A' * 40, 'B' * 40


def primary(fingerprint):
    return 'pub:-:2048:1:0123456789ABCDEF:1:0:::::scSC:\nfpr:::::::::' + fingerprint + ':\nuid:::::::::Fixture:\n'


def select(key, expected, output, *, environment=None):
    return subprocess.run(['bash', '-c', 'set -euo pipefail\n' + FUNCTIONS +
        '\nselect_frr_certificates "$KEY" "$EXPECTED" "$OUTPUT"\n'], text=True, capture_output=True,
        env=dict(os.environ, KEY=str(key), EXPECTED=expected, OUTPUT=str(output), **(environment or {})))


class ControlledSelection(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name)
        self.bin = self.root / 'bin'; self.bin.mkdir()
        self.key = self.root / 'raw.key'; self.key.write_bytes(b'controlled public material')
        self.output = self.root / 'selected.gpg'; self.log = self.root / 'calls'
        stub = self.bin / 'gpg'
        stub.write_text('''#!/usr/bin/python3
import json,os,pathlib,sys
args=sys.argv[1:]
with open(os.environ['CALL_LOG'],'a') as f:f.write(json.dumps(args)+'\\n')
assert args[0]=='--no-options' and args[1]=='--homedir' and args[3]=='--batch'
if '--list-packets' in args:
 if os.environ.get('REPLACE_RAW'): pathlib.Path(os.environ['REPLACE_RAW']).write_bytes(b'replaced caller secret material')
 print(os.environ.get('PACKETS',':public key packet:'))
 raise SystemExit(int(os.environ.get('PACKET_RC','0')))
if '--show-keys' in args:
 print(os.environ.get('NODE_IDENTITIES',os.environ['IDENTITIES']) if args[-1].endswith('node.key') else os.environ['IDENTITIES'])
elif '--import' in args:
 if os.environ.get('REPLACE_RAW'): assert pathlib.Path(args[-1]).read_bytes()==b'controlled public material' and args[-1]!=os.environ['REPLACE_RAW']
 pathlib.Path(args[2],'partial-public-keyring').write_text('private fixture partial import')
 raise SystemExit(int(os.environ.get('IMPORT_RC','0')))
elif '--export' in args:
 assert args[args.index('--export')+1:]==['A'*40]
 assert not any('minimal' in a or 'clean' in a for a in args)
 sys.stdout.buffer.write(b'controlled selected certificate')
 raise SystemExit(int(os.environ.get('EXPORT_RC','0')))
elif '--dearmor' in args:
 pathlib.Path(args[args.index('--output')+1]).write_bytes(b'controlled verified public certificate')
else:raise SystemExit(95)
''')
        stub.chmod(0o755)
        self.env = dict(PATH=f'{self.bin}:/usr/bin:/bin', CALL_LOG=str(self.log), IDENTITIES=primary(A))

    def tearDown(self):
        self.temp.cleanup()

    def calls(self):
        return [json.loads(line) for line in self.log.read_text().splitlines()] if self.log.exists() else []

    def invoke(self, **options):
        result = select(self.key, A, self.output, environment=dict(self.env, **options))
        # Every actual transient home must have been removed on success/failure.
        for call in self.calls():
            self.assertFalse(Path(call[2]).parent.exists())
        return result

    def test_actual_gpg_malformed_raw_refuses_without_key_generation(self):
        self.key.write_bytes(b'malformed not OpenPGP certificate bytes')
        result = select(self.key, A, self.output)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('no valid OpenPGP data', result.stderr)
        self.assertFalse(self.output.exists())
        self.assertEqual(self.calls(), [])

    def test_partial_import_nonzero_never_exports_or_publishes(self):
        result = self.invoke(IMPORT_RC='2', REPLACE_RAW=str(self.key))
        self.assertNotEqual(result.returncode, 0)
        self.assertTrue(any('--import' in args for args in self.calls()))
        self.assertFalse(any('--export' in args for args in self.calls()))
        self.assertFalse(self.output.exists())

    def test_secret_or_parser_failure_refuses_before_import(self):
        for options in ({'PACKETS': ':secret key packet:'}, {'PACKETS': ':secret sub key packet:'},
                        {'IDENTITIES': 'sec:::::::::\n'}, {'PACKET_RC': '2'}):
            with self.subTest(options=options):
                self.log.unlink(missing_ok=True)
                result = self.invoke(**options)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(any('--import' in args for args in self.calls()))
                self.assertFalse(self.output.exists())

    def test_export_failure_and_existing_output_preserve_private_invariants(self):
        result = self.invoke(EXPORT_RC='2')
        self.assertNotEqual(result.returncode, 0); self.assertFalse(self.output.exists())
        self.log.unlink(); self.output.write_bytes(b'preserve existing')
        result = self.invoke()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.output.read_bytes(), b'preserve existing')

    def test_nonregular_size_and_nonfull40_pins_refuse_before_gpg(self):
        for kind in ('empty', 'oversize', 'symlink', 'fifo'):
            with self.subTest(kind=kind):
                self.key.unlink(missing_ok=True)
                if kind == 'symlink': self.key.symlink_to(self.root / 'foreign')
                elif kind == 'fifo': os.mkfifo(self.key)
                else: self.key.write_bytes(b'' if kind == 'empty' else b'x' * (1024 * 1024 + 1))
                result = select(self.key, A, self.output, environment=self.env)
                self.assertNotEqual(result.returncode, 0); self.assertEqual(self.calls(), [])
        self.key.unlink(); self.key.write_bytes(b'controlled public input')
        self.root.chmod(0o755)
        result = select(self.key, A, self.output, environment=self.env)
        self.assertNotEqual(result.returncode, 0); self.assertEqual(self.calls(), [])
        self.root.chmod(0o700)
        for pin in ('', 'A'*64, A+','+A, A+';false', A.lower()):
            result = select(self.key, pin, self.output, environment=self.env)
            self.assertNotEqual(result.returncode, 0); self.assertEqual(self.calls(), [])

    def test_repository_entry_failure_at_either_gate_never_mutates_host(self):
        # Run the actual entry with an explicit preflight fixture and harmless argv stubs.
        for failing in ('frr-import', 'node-identity'):
            with self.subTest(failing=failing):
                self.log.unlink(missing_ok=True)
                for name in ('apt-get', 'install', 'mv'):
                    stub = self.bin / name
                    stub.write_text('#!/bin/sh\necho "UNEXPECTED HOST MUTATION" >&2\nexit 97\n'); stub.chmod(0o755)
                curl = self.bin / 'curl'
                curl.write_text('#!/usr/bin/python3\nimport pathlib,sys\npathlib.Path(sys.argv[sys.argv.index("-o")+1]).write_bytes(b"controlled raw public bytes")\n')
                curl.chmod(0o755)
                source = SOURCE.replace('[[ $EUID -eq 0 ]]', '[[ 1 -eq 1 ]]', 1)
                source = source.replace('\ncheck_key_pins\n', '\npreflight_artifacts() { :; }\ncheck_key_pins\n', 1)
                # D-238: authorize the controlled fixture identities so the run reaches both gates.
                source, count = re.subn(r'(?m)^readonly NGFW_FRR_AUTHORIZED_PRIMARIES=\S+$', 'readonly NGFW_FRR_AUTHORIZED_PRIMARIES=' + A, source)
                self.assertEqual(count, 1)
                source, count = re.subn(r'(?m)^readonly NGFW_NODESOURCE_AUTHORIZED_PRIMARIES=\S+$', 'readonly NGFW_NODESOURCE_AUTHORIZED_PRIMARIES=' + B, source)
                self.assertEqual(count, 1)
                env = dict(os.environ, **self.env, NGFW_VPP_ARTIFACTS=str(self.root),
                    NGFW_FRR_KEY_FINGERPRINTS=A, NGFW_NODESOURCE_KEY_FINGERPRINTS=B,
                    IMPORT_RC='2' if failing == 'frr-import' else '0', NODE_IDENTITIES=primary(A))
                result = subprocess.run(['bash', '-c', source], env=env, text=True, capture_output=True)
                self.assertNotEqual(result.returncode, 0)
                self.assertNotIn('UNEXPECTED HOST MUTATION', result.stderr)
                self.assertNotIn('REFUSED: NGFW_', result.stderr)
                gate = '--import' if failing == 'frr-import' else '--show-keys'
                self.assertTrue(any(gate in args for args in self.calls()))
                for args in self.calls(): self.assertFalse(Path(args[2]).exists())


class RealCertificates(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        if not shutil.which('gpg') or not shutil.which('gpgv'):
            raise RuntimeError('actual GPG fixture prerequisite missing; NOT RUN')
        cls.temp = tempfile.TemporaryDirectory()
        cls.addClassCleanup(cls.temp.cleanup)
        cls.root = Path(cls.temp.name); cls.home = cls.root / 'builder'; cls.home.mkdir(mode=0o700)
        cls.fingerprints = []
        for label in ('Selected', 'Extra'):
            result = cls.gpg('--pinentry-mode', 'loopback', '--passphrase', '', '--quick-generate-key',
                             f'NGFWFRRSelectionFixture{label}', 'rsa2048', 'sign', '0')
            if result.returncode:
                raise RuntimeError('actual GPG fixture generation failed (not source acceptance): ' + result.stderr)
            keys = cls.gpg('--with-colons', '--with-fingerprint', '--list-keys', f'NGFWFRRSelectionFixture{label}')
            cls.fingerprints.append(next(line.split(':')[9] for line in keys.stdout.splitlines() if line.startswith('fpr:')))
        cls.a, cls.b = cls.fingerprints
        result = cls.gpg('--pinentry-mode', 'loopback', '--passphrase', '', '--quick-add-key', cls.a, 'rsa2048', 'encr', '0')
        if result.returncode: raise RuntimeError('actual public subkey fixture generation failed: '+result.stderr)
        cls.public_a = cls.public_export(cls.a)
        cls.public_b = cls.public_export(cls.b)

    @classmethod
    def gpg(cls, *arguments, binary=False):
        return subprocess.run(['gpg', '--no-options', '--homedir', str(cls.home), '--batch', *arguments],
                              capture_output=True, text=not binary)

    @classmethod
    def public_export(cls, fingerprint):
        result = cls.gpg('--export', fingerprint, binary=True)
        if result.returncode or not result.stdout:
            raise RuntimeError('actual public certificate export failed: '+str(result.stderr))
        return result.stdout

    def invoke(self, data, expected=None):
        temp = tempfile.TemporaryDirectory(dir=self.root); self.addCleanup(temp.cleanup)
        root = Path(temp.name); key = root / 'raw.key'; key.write_bytes(data); output = root / 'selected.gpg'
        result = select(key, expected or self.a, output)
        return result, output

    def test_real_duplicate_extra_selection_preserves_subkeys_and_exact_installed_set(self):
        result, output = self.invoke(self.public_a + self.public_b + self.public_a)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(output.stat().st_mode & 0o777, 0o600)
        show = subprocess.run(['gpg','--no-options','--homedir',str(self.home),'--batch','--with-colons','--with-fingerprint','--show-keys',str(output)], text=True, capture_output=True)
        self.assertEqual(show.returncode, 0, show.stderr)
        self.assertEqual(sum(line.startswith('pub:') for line in show.stdout.splitlines()), 1)
        self.assertIn(self.a, show.stdout); self.assertNotIn(self.b, show.stdout)
        self.assertTrue(any(line.startswith('sub:') for line in show.stdout.splitlines()))

    def test_unselected_signer_refused_until_fixture_owner_explicitly_authorizes_it(self):
        payload = self.root / 'repository-metadata'; payload.write_bytes(b'fixture repository metadata\n')
        signature = self.root / 'metadata.sig'
        signed = self.gpg('--pinentry-mode', 'loopback', '--passphrase', '',
                          '--local-user', self.b, '--output', str(signature), '--detach-sign', str(payload))
        self.assertEqual(signed.returncode, 0, signed.stderr)
        self.assertGreater(signature.stat().st_size, 0)
        for authorized, accepted in ((self.a, False), (self.b, True)):
            selected, output = self.invoke(self.public_a + self.public_b, authorized)
            self.assertEqual(selected.returncode, 0, selected.stderr)
            home = output.parent / 'signature-verifier'; home.mkdir(mode=0o700)
            checked = subprocess.run(['gpgv', '--homedir', str(home), '--keyring', str(output),
                                      '--status-fd', '1', str(signature), str(payload)],
                                     capture_output=True, text=True)
            if accepted:
                self.assertEqual(checked.returncode, 0, checked.stderr)
                self.assertIn('[GNUPG:] VALIDSIG ' + self.b + ' ', checked.stdout)
            else:
                self.assertNotEqual(checked.returncode, 0)
                self.assertIn('[GNUPG:] NO_PUBKEY ', checked.stdout)
                self.assertNotIn('[GNUPG:] VALIDSIG ', checked.stdout)

    def test_missing_authorized_primary_is_not_silently_omitted(self):
        result, output = self.invoke(self.public_a, self.a+','+self.b)
        self.assertNotEqual(result.returncode, 0); self.assertFalse(output.exists())

    def test_conflicting_duplicate_revocation_is_preserved_and_refused(self):
        revocation = (self.home / 'openpgp-revocs.d' / (self.a+'.rev')).read_text()
        revocation = revocation[revocation.index(':-----BEGIN PGP PUBLIC KEY BLOCK-----'):].replace(':-----BEGIN', '-----BEGIN', 1)
        key = self.root / 'revocation.asc'; key.write_text(revocation)
        result = self.gpg('--import', str(key)); self.assertEqual(result.returncode, 0, result.stderr)
        revoked = self.public_export(self.a)
        result, output = self.invoke(self.public_a + revoked + self.public_b)
        self.assertNotEqual(result.returncode, 0); self.assertFalse(output.exists())

    def test_secret_and_malformed_raw_material_are_rejected(self):
        secret = self.gpg('--pinentry-mode','loopback','--passphrase','','--export-secret-keys',self.b,binary=True)
        self.assertEqual(secret.returncode, 0, secret.stderr)
        for data in (secret.stdout, b'malformed not OpenPGP material'):
            result, output = self.invoke(data)
            self.assertNotEqual(result.returncode, 0); self.assertFalse(output.exists())

    def test_actual_non_signing_authorized_primary_is_refused(self):
        result = self.gpg('--pinentry-mode','loopback','--passphrase','',
                          '--quick-generate-key','NGFWFRRSelectionFixtureCertOnly','rsa2048','cert','0')
        self.assertEqual(result.returncode, 0, result.stderr)
        keys = self.gpg('--with-colons','--with-fingerprint','--list-keys','NGFWFRRSelectionFixtureCertOnly')
        fingerprint = next(line.split(':')[9] for line in keys.stdout.splitlines() if line.startswith('fpr:'))
        certificate = self.public_export(fingerprint)
        result, output = self.invoke(certificate, fingerprint)
        self.assertNotEqual(result.returncode, 0); self.assertFalse(output.exists())

    def test_actual_expired_primary_is_refused(self):
        result = self.gpg('--faked-system-time','1577836800','--pinentry-mode','loopback','--passphrase','',
                          '--quick-generate-key','NGFWFRRSelectionFixtureExpired','rsa2048','sign','1d')
        self.assertEqual(result.returncode, 0, result.stderr)
        keys = self.gpg('--with-colons','--with-fingerprint','--list-keys','NGFWFRRSelectionFixtureExpired')
        fingerprint = next(line.split(':')[9] for line in keys.stdout.splitlines() if line.startswith('fpr:'))
        expired = self.public_export(fingerprint)
        result, output = self.invoke(expired, fingerprint)
        self.assertNotEqual(result.returncode, 0); self.assertFalse(output.exists())


if __name__ == '__main__':
    result = unittest.TextTestRunner(verbosity=2).run(unittest.defaultTestLoader.loadTestsFromModule(sys.modules[__name__]))
    raise SystemExit(result.testsRun != 13 or not result.wasSuccessful() or bool(result.skipped or result.expectedFailures or result.unexpectedSuccesses))
