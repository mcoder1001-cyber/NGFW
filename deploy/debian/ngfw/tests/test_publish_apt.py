#!/usr/bin/env python3
"""Package validation + real GPG signing, using fixture VPP/reprepro commands.

This is not proof of product VPP provenance or actual reprepro publication.
"""
import json
import os
import pathlib
import shutil
import subprocess
import tempfile
import unittest

SOURCE = pathlib.Path(__file__).resolve().parents[1]
ROOT = SOURCE.parents[2]


class Publish(unittest.TestCase):
    def fixture(self, directory, bad_pin=False, meta_depends=None):
        root = pathlib.Path(directory)
        for relative in ['source/scripts', 'source/deploy/apt', 'source/deploy/vpp', 'vpp', 'ngfw', 'bin']:
            (root / relative).mkdir(parents=True)
        shutil.copy(ROOT / 'scripts/publish-apt.sh', root / 'source/scripts/publish-apt.sh')
        shutil.copy(ROOT / 'deploy/apt/distributions.in', root / 'source/deploy/apt/distributions.in')
        def script(path, text):
            file = root / path
            file.write_text('#!/usr/bin/env bash\nset -eu\n' + text)
            file.chmod(0o755)
        script('source/deploy/vpp/verify.sh', '[[ "$1" == --require-files && "$3" == --install-gate ]]\n')
        script('bin/reprepro', '''directory=$2
mkdir -p "$directory/dists/resolute"
printf 'Codename: resolute\\n' > "$directory/dists/resolute/Release"
gpg --batch --yes --clearsign -o "$directory/dists/resolute/InRelease" "$directory/dists/resolute/Release" >/dev/null 2>&1
''')
        def package(name, output, version='0.1.0~dev+fixture', depends=None):
            folder = root / ('build-' + name)
            (folder / 'DEBIAN').mkdir(parents=True)
            fields = f'Package: {name}\nVersion: {version}\nArchitecture: amd64\nMaintainer: Fixture <test@example.invalid>\nDescription: Fixture package\n'
            if depends: fields += 'Depends: ' + depends + '\n'
            (folder / 'DEBIAN/control').write_text(fields)
            destination = output / (name + '.deb')
            subprocess.run(['dpkg-deb', '--build', '--root-owner-group', str(folder), str(destination)],
                           check=True, capture_output=True)
            return destination
        vpp_version = '26.06-release+ngfw1'
        entries = []
        for name in ['vpp', 'vpp-plugin-core', 'vpp-plugin-dpdk', 'vpp-drivers',
                     'vpp-crypto-engines', 'libvppinfra', 'python3-vpp-api']:
            destination = package(name, root / 'vpp', vpp_version)
            entries.append(dict(package=name, ship=True, file=destination.name))
        (root / 'vpp/manifest.json').write_text(json.dumps(dict(version=vpp_version, packages=entries)))
        for name in ['ngfw-agent', 'ngfw-api', 'ngfw-web', 'ngfw-meta']:
            dependency = 'vpp (= 0.0-wrong)' if bad_pin else 'vpp (= ' + vpp_version + ')'
            package(name, root / 'ngfw', depends=(meta_depends or dependency) if name == 'ngfw-meta' else None)
        environment = dict(os.environ, PATH=str(root / 'bin') + ':' + os.environ['PATH'],
                           XDG_CONFIG_HOME=str(root / 'config'))
        return root, environment

    def test_signing_home_overlap_refuses_before_any_mutation(self):
        for overlap in ['ancestor', 'equal', 'descendant']:
            with self.subTest(overlap=overlap), tempfile.TemporaryDirectory() as directory:
                root, env = self.fixture(directory)
                key_home = root / 'config/ngfw/apt-signing'
                output = {'ancestor': root / 'config', 'equal': key_home,
                          'descendant': key_home / 'repository'}[overlap]
                result = subprocess.run(['bash', str(root / 'source/scripts/publish-apt.sh'),
                                         str(root / 'vpp'), str(root / 'ngfw'), str(output)],
                                        env=env, capture_output=True)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn(b'overlap refused', result.stderr)
                self.assertFalse((root / 'config').exists())

    def test_vpp_alternatives_lookalikes_and_duplicates_refuse(self):
        correct = '26.06-release+ngfw1'
        for depends in [f'vpp (= {correct}) | vpp (= 0.0-wrong)',
                        f'not-vpp (= {correct})',
                        f'vpp (= {correct}), vpp (= {correct})',
                        f'vpp (>= {correct})']:
            with self.subTest(depends=depends), tempfile.TemporaryDirectory() as directory:
                root, env = self.fixture(directory, meta_depends=depends)
                result = subprocess.run(['bash', str(root / 'source/scripts/publish-apt.sh'),
                                         str(root / 'vpp'), str(root / 'ngfw'), str(root / 'repo')],
                                        env=env, capture_output=True)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn(b'exact standalone', result.stderr)
                self.assertFalse((root / 'config').exists())
                self.assertFalse((root / 'repo').exists())

    def test_bad_vpp_pin_refuses_before_signing_or_output(self):
        with tempfile.TemporaryDirectory() as directory:
            root, env = self.fixture(directory, bad_pin=True)
            result = subprocess.run(['bash', str(root / 'source/scripts/publish-apt.sh'),
                                     str(root / 'vpp'), str(root / 'ngfw'), str(root / 'repo')],
                                    env=env, capture_output=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn(b'VPP dependency', result.stderr)
            self.assertFalse((root / 'config').exists())
            self.assertFalse((root / 'repo').exists())

    def test_fixture_repository_is_signed_with_real_gpg_private_key_stays_outside(self):
        with tempfile.TemporaryDirectory() as probe:
            check = subprocess.run(['gpg', '--homedir', probe, '--batch', '--pinentry-mode',
                                    'loopback', '--passphrase', '', '--quick-generate-key',
                                    'NGFW fixture signing', 'rsa3072', 'sign', '1d'],
                                   capture_output=True)
            subprocess.run(['gpgconf', '--homedir', probe, '--kill', 'gpg-agent'], capture_output=True)
            if check.returncode != 0 and b"can't connect to the gpg-agent" in check.stderr:
                self.skipTest('environment cannot start isolated gpg-agent; signing NOT RUN')
            self.assertEqual(check.returncode, 0, 'isolated signing preflight failed')
        with tempfile.TemporaryDirectory() as directory:
            root, env = self.fixture(directory)
            subprocess.run(['bash', str(root / 'source/scripts/publish-apt.sh'), str(root / 'vpp'),
                            str(root / 'ngfw'), str(root / 'repo')], env=env, check=True,
                           capture_output=True)
            repo = root / 'repo'
            self.assertTrue((repo / 'RELEASE-READY').exists())
            self.assertFalse((repo / 'private-keys-v1.d').exists())
            subprocess.run(['gpgv', '--keyring', str(repo / 'ngfw-archive-keyring.gpg'),
                            str(repo / 'dists/resolute/InRelease')], check=True, capture_output=True)
            self.assertEqual((root / 'config/ngfw/apt-signing').stat().st_mode & 0o777, 0o700)
            subprocess.run(['gpgconf', '--homedir', str(root / 'config/ngfw/apt-signing'),
                            '--kill', 'gpg-agent'], capture_output=True)


if __name__ == '__main__':
    unittest.main()
