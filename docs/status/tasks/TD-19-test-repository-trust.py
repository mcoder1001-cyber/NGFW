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
RESOLUTION = SOURCE[SOURCE.index('NGFW_FRR_KEY_FINGERPRINTS=${'):SOURCE.index('if [[ $NGFW_INSTALL_DRY_RUN == 1 ]]')]

# Exact identities from docs/status/tasks/TD-19-trust-material-20261004.md (D-238).
FRR = ('4A56C7738BB3F81595A805D2A832769908F13ED1', '3D9968AC9AE7BE1169288DDB1FD5839895F57FDA',
       'BBC9ACA9D13025A2C186FF7F741E92A1F6E3975B', 'A90FC36D9429409798E9C2D874DEED43AB194DBF')
NODE = ('6F71F525282841EEDAF851B42F59B5F99B1BE0B4',)
FOREIGN = 'C' * 40
FRR_URL = 'https://deb.frrouting.org/frr/keys.gpg'
NODE_URL = 'https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key'
CONSTANT = re.compile(r'^readonly (NGFW_(?:FRR|NODESOURCE)_AUTHORIZED_PRIMARIES)=(\S+)$', re.M)


def packets(data):
    """Split an OpenPGP binary stream into (tag, raw packet bytes)."""
    result, index = [], 0
    while index < len(data):
        start, header = index, data[index]; index += 1
        if not header & 0x80:
            raise ValueError('not an OpenPGP packet')
        if header & 0x40:
            tag, first = header & 0x3f, data[index]; index += 1
            if first < 192:
                length = first
            elif first < 224:
                length = ((first - 192) << 8) + data[index] + 192; index += 1
            elif first == 255:
                length = int.from_bytes(data[index:index + 4], 'big'); index += 4
            else:
                raise ValueError('partial packet lengths unsupported in fixtures')
        else:
            tag, size = (header >> 2) & 0x0f, header & 0x03
            if size == 3:
                raise ValueError('indeterminate packet length unsupported in fixtures')
            width = (1, 2, 4)[size]
            length = int.from_bytes(data[index:index + width], 'big'); index += width
        index += length
        result.append((tag, data[start:index]))
    return result


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
        for name in ('frr', 'node'):
            raw = SOURCE.index(f'\ncheck_raw_primaries "$repo_work/{name}.key"')
            selected = SOURCE.index(f'\nselect_pinned_certificates "$repo_work/{name}.key"')
            self.assertLess(raw, selected)
            self.assertLess(selected, SOURCE.index('\napt-get update'))
        # One shared parser decides every primary set (raw and canonical export).
        self.assertEqual(SOURCE.count("<<'PYIDENTITY'"), 1)
        self.assertEqual(SOURCE.count('check_primary_set "'), 2)
        # NodeSource no longer installs a dearmored raw download.
        self.assertNotIn('verify_repo_key "$repo_work/node.key"', SOURCE)

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
        # Expired pinned primaries (generated in the past with a one-day lifetime).
        for label in ('F4E', 'NE'):
            uid = f'NGFWTrustFixture{label}'
            result = cls.gpg('--faked-system-time', '1577836800', '--pinentry-mode', 'loopback', '--passphrase', '',
                             '--quick-generate-key', uid, 'ed25519', 'sign', '1d')
            if result.returncode:
                raise RuntimeError('actual expired fixture generation failed: ' + result.stderr)
            keys = cls.gpg('--with-colons', '--with-fingerprint', '--list-keys', uid)
            cls.fpr[label] = next(line.split(':')[9] for line in keys.stdout.splitlines() if line.startswith('fpr:'))
            cls.public[label] = cls.gpg('--export', cls.fpr[label], binary=True).stdout
        # Attacker signing subkey (bound by the attacker primary X) spliced onto pinned primaries.
        result = cls.gpg('--pinentry-mode', 'loopback', '--passphrase', '', '--quick-add-key', cls.fpr['X'], 'ed25519', 'sign', '0')
        if result.returncode:
            raise RuntimeError('actual attacker subkey generation failed: ' + result.stderr)
        keys = cls.gpg('--with-colons', '--with-fingerprint', '--list-keys', cls.fpr['X'])
        cls.attacker_subkey = [line.split(':')[9] for line in keys.stdout.splitlines() if line.startswith('fpr:')][1]
        exported = packets(cls.gpg('--export', cls.fpr['X'], binary=True).stdout)
        index = next(i for i, (tag, _) in enumerate(exported) if tag == 14)
        cls.attacker_packets = b''.join(raw for _, raw in exported[index:index + 2])
        if [tag for tag, _ in exported[index:index + 2]] != [14, 2]:
            raise RuntimeError('attacker subkey/binding packets not found')
        signature = cls.root / 'Xsub.sig'
        result = cls.gpg('--pinentry-mode', 'loopback', '--passphrase', '', '--local-user', cls.attacker_subkey + '!',
                         '--output', str(signature), '--detach-sign', str(cls.payload))
        if result.returncode:
            raise RuntimeError('actual attacker subkey signature failed: ' + result.stderr)
        cls.signatures['Xsub'] = signature
        cls.public['F4+Xsub'] = cls.public['F4'] + cls.attacker_packets
        cls.public['N+Xsub'] = cls.public['N'] + cls.attacker_packets
        # Revoked pinned primaries: import GnuPG's own revocation certificate last.
        for label in ('F4', 'N'):
            revocation = (cls.home / 'openpgp-revocs.d' / (cls.fpr[label] + '.rev')).read_text()
            revocation = revocation[revocation.index(':-----BEGIN PGP PUBLIC KEY BLOCK-----'):].replace(':-----BEGIN', '-----BEGIN', 1)
            certificate = cls.root / f'{label}.rev.asc'; certificate.write_text(revocation)
            result = cls.gpg('--import', str(certificate))
            if result.returncode:
                raise RuntimeError('actual revocation import failed: ' + result.stderr)
            cls.public[label + 'R'] = cls.gpg('--export', cls.fpr[label], binary=True).stdout

    @classmethod
    def gpg(cls, *arguments, binary=False):
        return subprocess.run(['gpg', '--no-options', '--homedir', str(cls.home), '--batch', *arguments],
                              capture_output=True, text=not binary)

    def bundle(self, *labels):
        return b''.join(self.public[label] for label in labels)

    def run_entry(self, frr_labels, node_labels, frr_pins=('F1', 'F2', 'F3', 'F4'), node_pin='N'):
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
        for name in ('apt-get', 'install', 'mv', 'go', 'npm', 'corepack', 'tar'):
            stub = stubs / name
            stub.write_text('#!/bin/sh\necho "UNEXPECTED HOST MUTATION $0" >&2\nexit 97\n'); stub.chmod(0o755)
        # Shared seam uses a disposable root; private crypto operations alone
        # delegate to GnuPG, preserving the adversarial real certificate checks.
        for name, binary in (('gpg', '/usr/bin/gpg'), ('python3', '/usr/bin/python3')):
            stub = stubs / name
            stub.write_text('#!/bin/sh\nexec ' + binary + ' "$@"\n'); stub.chmod(0o755)
        install_root = work / 'install-root'; install_root.mkdir()
        (install_root / 'etc').mkdir()
        (install_root / 'etc/os-release').write_text('VERSION_CODENAME=resolute\n')
        source = SOURCE.replace('source "$ROOT/scripts/install-common.sh"',
                                'source ' + json.dumps(str(ROOT / 'scripts/install-common.sh')), 1)
        source = source.replace('\ncheck_key_pins\n', '\npreflight_artifacts() { :; }\ncheck_key_pins\n', 1)
        frr_pins = ','.join(self.fpr[label] for label in frr_pins)
        values = constants()
        source = source.replace('readonly NGFW_FRR_AUTHORIZED_PRIMARIES=' + values['NGFW_FRR_AUTHORIZED_PRIMARIES'],
                                'readonly NGFW_FRR_AUTHORIZED_PRIMARIES=' + frr_pins, 1)
        source = source.replace('readonly NGFW_NODESOURCE_AUTHORIZED_PRIMARIES=' + values['NGFW_NODESOURCE_AUTHORIZED_PRIMARIES'],
                                'readonly NGFW_NODESOURCE_AUTHORIZED_PRIMARIES=' + self.fpr[node_pin], 1)
        # Stop at the first APT boundary: capture the verified private keyrings only.
        boundary = '\ncp -- "$repo_work/frr.gpg" "$repo_work/node.gpg" "$CAPTURE/"\necho REACHED_APT_BOUNDARY\nexit 0\n'
        self.assertEqual(source.count('\napt-get update\n'), 2)
        source = source.replace('\napt-get update\n', boundary, 1)
        self.assertNotIn(values['NGFW_FRR_AUTHORIZED_PRIMARIES'], source)
        env = dict(os.environ, PATH=f'{stubs}:/usr/bin:/bin', CALL_LOG=str(log), WORK=str(work),
                   CAPTURE=str(capture), FRR_URL=FRR_URL, NODE_URL=NODE_URL, NGFW_VPP_ARTIFACTS=str(work),
                   NGFW_INSTALL_ROOT=str(install_root), NGFW_INSTALL_STUBS='1',
                   NGFW_INSTALL_STUB_DIR=str(stubs))
        env.pop('NGFW_FRR_KEY_FINGERPRINTS', None)
        env.pop('NGFW_NODESOURCE_KEY_FINGERPRINTS', None)
        result = subprocess.run(['bash', '-c', source], env=env, text=True, capture_output=True)
        calls = [json.loads(line) for line in log.read_text().splitlines()] if log.exists() else []
        self.assertNotIn('UNEXPECTED HOST MUTATION', result.stderr)
        return result, calls, capture

    def show(self, keyring):
        show = self.gpg('--with-colons', '--with-fingerprint', '--show-keys', str(keyring))
        self.assertEqual(show.returncode, 0, show.stderr)
        return show.stdout

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
                self.assertIn('REFUSED: downloaded FRR primary key set differs from the owner-authorized D-238 pins', result.stderr)
                self.assertEqual(list(capture.iterdir()), [])

    def test_extra_missing_or_changed_node_identity_refused_before_apt(self):
        for label, node in (('extra', ('N', 'X')), ('changed', ('X',)), ('duplicate', ('N', 'N'))):
            with self.subTest(case=label):
                result, _, capture = self.run_entry(('F1', 'F2', 'F3', 'F4'), node)
                self.assertNotEqual(result.returncode, 0)
                self.assertNotIn('REACHED_APT_BOUNDARY', result.stdout)
                self.assertIn('REFUSED: downloaded NodeSource primary key set differs from the owner-authorized D-238 pins', result.stderr)
                self.assertEqual(list(capture.iterdir()), [])

    def test_attacker_subkey_on_pinned_primary_never_reaches_installed_keyring(self):
        for frr, node in ((('F1', 'F2', 'F3', 'F4+Xsub'), ('N',)),
                          (('F1', 'F2', 'F3', 'F4'), ('N+Xsub',))):
            with self.subTest(frr=frr, node=node):
                result, _, capture = self.run_entry(frr, node)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn('REACHED_APT_BOUNDARY', result.stdout)
                for keyring in ('frr.gpg', 'node.gpg'):
                    shown = self.show(capture / keyring)
                    # Only canonical GnuPG-validated material is exported: no subkey at all.
                    self.assertNotIn(self.attacker_subkey, shown)
                    self.assertFalse(any(line.startswith('sub:') for line in shown.splitlines()))
                    checked = self.verify(capture / keyring, 'Xsub')
                    self.assertNotEqual(checked.returncode, 0)
                    self.assertNotIn('[GNUPG:] VALIDSIG ', checked.stdout)
                self.assertEqual(set(self.primaries(capture / 'frr.gpg')), {self.fpr[label] for label in ('F1', 'F2', 'F3', 'F4')})
                self.assertEqual(self.primaries(capture / 'node.gpg'), [self.fpr['N']])

    def test_revoked_or_expired_pinned_primary_refused_before_apt(self):
        cases = (
            ('frr-revoked', ('F1', 'F2', 'F3', 'F4R'), ('N',), ('F1', 'F2', 'F3', 'F4'), 'N'),
            ('frr-revoked-duplicate-copy', ('F1', 'F2', 'F3', 'F4', 'F4R'), ('N',), ('F1', 'F2', 'F3', 'F4'), 'N'),
            ('frr-expired', ('F1', 'F2', 'F3', 'F4E'), ('N',), ('F1', 'F2', 'F3', 'F4E'), 'N'),
            ('node-revoked', ('F1', 'F2', 'F3', 'F4'), ('NR',), ('F1', 'F2', 'F3', 'F4'), 'N'),
            ('node-expired', ('F1', 'F2', 'F3', 'F4'), ('NE',), ('F1', 'F2', 'F3', 'F4'), 'NE'),
        )
        for label, frr, node, frr_pins, node_pin in cases:
            with self.subTest(case=label):
                result, _, capture = self.run_entry(frr, node, frr_pins, node_pin)
                self.assertNotEqual(result.returncode, 0)
                self.assertNotIn('REACHED_APT_BOUNDARY', result.stdout)
                self.assertIn('unsupported validity', result.stderr)
                self.assertEqual(list(capture.iterdir()), [])


if __name__ == '__main__':
    unittest.main(verbosity=2)
