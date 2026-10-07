#!/usr/bin/env python3
"""D-238 owner-authorized repository trust anchors; no network, APT or host mutation.

Exercises the actual scripts/00-add-repos.sh entry with recorder stubs for curl
and host-mutating commands, and real private GnuPG certificates for the
selection/verification path. The run stops at the first APT boundary.
"""
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[3]
SOURCE = (ROOT / 'scripts/00-add-repos.sh').read_text()
FUNCTIONS = SOURCE[SOURCE.index('check_key_pins() {'):SOURCE.index('if [[ ${1:-} == --check-artifacts ]]')]
RESOLUTION = SOURCE[SOURCE.index('NGFW_FRR_KEY_FINGERPRINTS=${'):SOURCE.index('preflight_artifacts "$NGFW_VPP_ARTIFACTS"')]

# Exact identities from docs/status/tasks/TD-19-trust-material-20261004.md (D-238).
FRR = ('4A56C7738BB3F81595A805D2A832769908F13ED1', '3D9968AC9AE7BE1169288DDB1FD5839895F57FDA',
       'BBC9ACA9D13025A2C186FF7F741E92A1F6E3975B', 'A90FC36D9429409798E9C2D874DEED43AB194DBF')
NODE = ('6F71F525282841EEDAF851B42F59B5F99B1BE0B4',)
FOREIGN = 'C' * 40
FRR_URL = 'https://deb.frrouting.org/frr/keys.gpg'
NODE_URL = 'https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key'
CONSTANT = re.compile(r'^readonly (NGFW_(?:FRR|NODESOURCE)_AUTHORIZED_PRIMARIES)=(\S+)$', re.M)


def constants(source=SOURCE):
    return {name: value for name, value in CONSTANT.findall(source)}


class AuthorizedPins(unittest.TestCase):
    def resolve(self, **overrides):
        env = dict(os.environ)
        env.pop('NGFW_FRR_KEY_FINGERPRINTS', None)
        env.pop('NGFW_NODESOURCE_KEY_FINGERPRINTS', None)
        env.update(overrides)
        script = ('set -euo pipefail\n' + FUNCTIONS + RESOLUTION +
                  'printf "%s\\n%s\\n" "$NGFW_FRR_KEY_FINGERPRINTS" "$NGFW_NODESOURCE_KEY_FINGERPRINTS"\n')
        return subprocess.run(['bash', '-c', script], env=env, text=True, capture_output=True)

    def test_source_pins_are_exactly_the_owner_authorized_identities(self):
        values = constants()
        self.assertEqual(set(values), {'NGFW_FRR_AUTHORIZED_PRIMARIES', 'NGFW_NODESOURCE_AUTHORIZED_PRIMARIES'})
        frr = values['NGFW_FRR_AUTHORIZED_PRIMARIES'].split(',')
        self.assertEqual(len(frr), 4)
        self.assertEqual(set(frr), set(FRR))
        self.assertIn('A90FC36D9429409798E9C2D874DEED43AB194DBF', frr)
        self.assertEqual(values['NGFW_NODESOURCE_AUTHORIZED_PRIMARIES'].split(','), list(NODE))
        # Pins are resolved before artifact preflight, network and APT.
        self.assertLess(SOURCE.index('\nrequire_authorized_pins\n'), SOURCE.index('\npreflight_artifacts "$NGFW_VPP_ARTIFACTS"'))
        self.assertLess(SOURCE.index('\ncheck_frr_raw_primaries "$repo_work/frr.key"'), SOURCE.index('\nselect_frr_certificates "$repo_work/frr.key"'))
        self.assertLess(SOURCE.index('\nselect_frr_certificates "$repo_work/frr.key"'), SOURCE.index('\napt-get update'))

    def test_unset_and_exact_reordered_overrides_accepted(self):
        for overrides in ({}, {'NGFW_FRR_KEY_FINGERPRINTS': ','.join(reversed(FRR)),
                               'NGFW_NODESOURCE_KEY_FINGERPRINTS': NODE[0]}):
            with self.subTest(overrides=overrides):
                result = self.resolve(**overrides)
                self.assertEqual(result.returncode, 0, result.stderr)
                frr, node = result.stdout.splitlines()
                self.assertEqual(set(frr.split(',')), set(FRR))
                self.assertEqual(node, NODE[0])

    def test_wrong_extra_missing_changed_or_malformed_overrides_refused(self):
        frr_cases = {
            'wrong': ','.join(FRR[:3] + (FOREIGN,)),
            'extra': ','.join(FRR + (FOREIGN,)),
            'missing-fourth-signer': ','.join(FRR[:3]),
            'missing-first': ','.join(FRR[1:]),
            'duplicate': ','.join(FRR + (FRR[0],)),
            'empty': '',
            'lowercase': ','.join(item.lower() for item in FRR),
        }
        node_cases = {'wrong': FOREIGN, 'extra': NODE[0] + ',' + FOREIGN, 'empty': '',
                      'lowercase': NODE[0].lower()}
        for variable, cases in (('NGFW_FRR_KEY_FINGERPRINTS', frr_cases),
                                ('NGFW_NODESOURCE_KEY_FINGERPRINTS', node_cases)):
            for label, value in cases.items():
                with self.subTest(variable=variable, label=label):
                    result = self.resolve(**{variable: value})
                    self.assertNotEqual(result.returncode, 0)
                    self.assertEqual(result.stdout, '')
                    self.assertTrue(variable in result.stderr or 'FRR selection' in result.stderr, result.stderr)


class RealRepositoryEntry(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        if not shutil.which('gpg') or not shutil.which('gpgv'):
            raise RuntimeError('actual GPG fixture prerequisite missing; NOT RUN')
        cls.temp = tempfile.TemporaryDirectory()
        cls.addClassCleanup(cls.temp.cleanup)
        cls.root = Path(cls.temp.name)
        cls.home = cls.root / 'builder'; cls.home.mkdir(mode=0o700)
        cls.fpr = {}
        for label in ('F1', 'F2', 'F3', 'F4', 'N', 'X'):
            uid = f'NGFWTrustFixture{label}'
            result = cls.gpg('--pinentry-mode', 'loopback', '--passphrase', '', '--quick-generate-key',
                             uid, 'ed25519', 'sign', '0')
            if result.returncode:
                raise RuntimeError('actual GPG fixture generation failed (not source acceptance): ' + result.stderr)
            keys = cls.gpg('--with-colons', '--with-fingerprint', '--list-keys', uid)
            cls.fpr[label] = next(line.split(':')[9] for line in keys.stdout.splitlines() if line.startswith('fpr:'))
        cls.public = {label: cls.gpg('--export', fingerprint, binary=True).stdout for label, fingerprint in cls.fpr.items()}
        if not all(cls.public.values()):
            raise RuntimeError('actual public certificate export failed')
        cls.payload = cls.root / 'InRelease-fixture'
        cls.payload.write_bytes(b'fixture repository metadata\n')
        cls.signatures = {}
        for label in ('F4', 'N', 'X'):
            signature = cls.root / f'{label}.sig'
            result = cls.gpg('--pinentry-mode', 'loopback', '--passphrase', '', '--local-user', cls.fpr[label],
                             '--output', str(signature), '--detach-sign', str(cls.payload))
            if result.returncode:
                raise RuntimeError('actual fixture signature failed: ' + result.stderr)
            cls.signatures[label] = signature

    @classmethod
    def gpg(cls, *arguments, binary=False):
        return subprocess.run(['gpg', '--no-options', '--homedir', str(cls.home), '--batch', *arguments],
                              capture_output=True, text=not binary)

    def bundle(self, *labels):
        return b''.join(self.public[label] for label in labels)

    def run_entry(self, frr_labels, node_labels):
        temp = tempfile.TemporaryDirectory(dir=self.root); self.addCleanup(temp.cleanup)
        work = Path(temp.name)
        stubs = work / 'bin'; stubs.mkdir()
        capture = work / 'capture'; capture.mkdir()
        (work / 'frr.bundle').write_bytes(self.bundle(*frr_labels))
        (work / 'node.bundle').write_bytes(self.bundle(*node_labels))
        log = work / 'calls'
        curl = stubs / 'curl'
        curl.write_text('''#!/usr/bin/python3
import json,os,pathlib,shutil,sys
args=sys.argv[1:]
with open(os.environ['CALL_LOG'],'a') as f: f.write(json.dumps(['curl']+args)+'\\n')
url=[a for a in args if a.startswith('https://')]
source={os.environ['FRR_URL']:'frr.bundle',os.environ['NODE_URL']:'node.bundle'}[url[0]]
shutil.copyfile(pathlib.Path(os.environ['WORK'])/source, args[args.index('-o')+1])
''')
        curl.chmod(0o755)
        for name in ('apt-get', 'install', 'mv'):
            stub = stubs / name
            stub.write_text('#!/bin/sh\necho "UNEXPECTED HOST MUTATION $0" >&2\nexit 97\n'); stub.chmod(0o755)
        source = SOURCE.replace('[[ $EUID -eq 0 ]]', '[[ 1 -eq 1 ]]', 1)
        source = source.replace('\ncheck_key_pins\n', '\npreflight_artifacts() { :; }\ncheck_key_pins\n', 1)
        frr_pins = ','.join(self.fpr[label] for label in ('F1', 'F2', 'F3', 'F4'))
        values = constants()
        source = source.replace('readonly NGFW_FRR_AUTHORIZED_PRIMARIES=' + values['NGFW_FRR_AUTHORIZED_PRIMARIES'],
                                'readonly NGFW_FRR_AUTHORIZED_PRIMARIES=' + frr_pins, 1)
        source = source.replace('readonly NGFW_NODESOURCE_AUTHORIZED_PRIMARIES=' + values['NGFW_NODESOURCE_AUTHORIZED_PRIMARIES'],
                                'readonly NGFW_NODESOURCE_AUTHORIZED_PRIMARIES=' + self.fpr['N'], 1)
        # Stop at the first APT boundary: capture the verified private keyrings only.
        boundary = '\ncp -- "$repo_work/frr.gpg" "$repo_work/node.gpg" "$CAPTURE/"\necho REACHED_APT_BOUNDARY\nexit 0\n'
        self.assertEqual(source.count('\napt-get update\n'), 2)
        source = source.replace('\napt-get update\n', boundary, 1)
        self.assertNotIn(values['NGFW_FRR_AUTHORIZED_PRIMARIES'], source)
        env = dict(os.environ, PATH=f'{stubs}:/usr/bin:/bin', CALL_LOG=str(log), WORK=str(work),
                   CAPTURE=str(capture), FRR_URL=FRR_URL, NODE_URL=NODE_URL, NGFW_VPP_ARTIFACTS=str(work))
        env.pop('NGFW_FRR_KEY_FINGERPRINTS', None)
        env.pop('NGFW_NODESOURCE_KEY_FINGERPRINTS', None)
        result = subprocess.run(['bash', '-c', source], env=env, text=True, capture_output=True)
        calls = [json.loads(line) for line in log.read_text().splitlines()] if log.exists() else []
        self.assertNotIn('UNEXPECTED HOST MUTATION', result.stderr)
        return result, calls, capture

    def primaries(self, keyring):
        show = self.gpg('--with-colons', '--with-fingerprint', '--show-keys', str(keyring))
        self.assertEqual(show.returncode, 0, show.stderr)
        result, pending = [], False
        for line in show.stdout.splitlines():
            fields = line.split(':')
            if fields[0] == 'pub': pending = True
            elif fields[0] == 'fpr' and pending: result.append(fields[9]); pending = False
        return result

    def verify(self, keyring, label):
        home = Path(tempfile.mkdtemp(dir=self.root)); home.chmod(0o700)
        return subprocess.run(['gpgv', '--homedir', str(home), '--keyring', str(keyring), '--status-fd', '1',
                               str(self.signatures[label]), str(self.payload)], capture_output=True, text=True)

    def test_exact_pins_with_duplicate_certificate_are_canonicalized_and_verify_signatures(self):
        result, calls, capture = self.run_entry(('F1', 'F2', 'F3', 'F4', 'F3'), ('N',))
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('REACHED_APT_BOUNDARY', result.stdout)
        self.assertEqual([call[0] for call in calls], ['curl', 'curl'])
        self.assertEqual({next(a for a in call if a.startswith('https://')) for call in calls}, {FRR_URL, NODE_URL})
        frr = self.primaries(capture / 'frr.gpg')
        self.assertEqual(len(frr), 4)
        self.assertEqual(set(frr), {self.fpr[label] for label in ('F1', 'F2', 'F3', 'F4')})
        self.assertEqual(self.primaries(capture / 'node.gpg'), [self.fpr['N']])
        # The fourth FRR signer and the Node primary validate repository signatures.
        for keyring, label in (('frr.gpg', 'F4'), ('node.gpg', 'N')):
            checked = self.verify(capture / keyring, label)
            self.assertEqual(checked.returncode, 0, checked.stderr)
            self.assertIn('[GNUPG:] VALIDSIG ' + self.fpr[label] + ' ', checked.stdout)
        # An unauthorized signer never validates against the installed sets.
        for keyring in ('frr.gpg', 'node.gpg'):
            checked = self.verify(capture / keyring, 'X')
            self.assertNotEqual(checked.returncode, 0)
            self.assertNotIn('[GNUPG:] VALIDSIG ', checked.stdout)

    def test_extra_missing_or_changed_frr_identity_refused_before_apt(self):
        for label, frr in (('extra', ('F1', 'F2', 'F3', 'F4', 'X')),
                           ('missing', ('F1', 'F2', 'F3')),
                           ('changed', ('F1', 'F2', 'F3', 'X')),
                           ('foreign-only', ('X',))):
            with self.subTest(case=label):
                result, _, capture = self.run_entry(frr, ('N',))
                self.assertNotEqual(result.returncode, 0)
                self.assertNotIn('REACHED_APT_BOUNDARY', result.stdout)
                self.assertIn('downloaded FRR primary set differs from the owner-authorized D-238 pins', result.stderr)
                self.assertEqual(list(capture.iterdir()), [])

    def test_extra_missing_or_changed_node_identity_refused_before_apt(self):
        for label, node in (('extra', ('N', 'X')), ('changed', ('X',)), ('duplicate', ('N', 'N'))):
            with self.subTest(case=label):
                result, _, capture = self.run_entry(('F1', 'F2', 'F3', 'F4'), node)
                self.assertNotEqual(result.returncode, 0)
                self.assertNotIn('REACHED_APT_BOUNDARY', result.stdout)
                self.assertIn('downloaded primary key set differs from trusted pins', result.stderr)
                self.assertEqual(list(capture.iterdir()), [])


if __name__ == '__main__':
    unittest.main(verbosity=2)
