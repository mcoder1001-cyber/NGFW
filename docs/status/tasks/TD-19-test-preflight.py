#!/usr/bin/env python3
"""Artifact consumer control-flow fixtures; never install or download anything."""
import copy
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[3]
SHIP = ['libvppinfra', 'python3-vpp-api', 'vpp', 'vpp-crypto-engines',
        'vpp-drivers', 'vpp-plugin-core', 'vpp-plugin-dpdk']


class ArtifactPreflight(unittest.TestCase):
    def fixture(self, directory):
        root = Path(directory)
        for name in ['source/scripts', 'source/deploy/vpp', 'artifacts', 'bin']:
            (root / name).mkdir(parents=True)
        script = (ROOT / 'scripts/00-add-repos.sh').read_text()
        # Only the root precondition is modeled for the no-mutation refusal case.
        script = script.replace('[[ $EUID -eq 0 ]]', '[[ 1 -eq 1 ]]', 1)
        entry = root / 'source/scripts/00-add-repos.sh'
        entry.write_text(script)
        for name in ['VERSION', 'lib.sh']:
            shutil.copy2(ROOT / 'deploy/vpp' / name, root / 'source/deploy/vpp' / name)
        def executable(path, body):
            target = root / path
            target.write_text('#!/usr/bin/env bash\nset -eu\n' + body)
            target.chmod(0o755)
        executable('source/deploy/vpp/verify.sh', '''[[ "$#" == 3 && "$1" == --require-files && "$3" == --install-gate ]]
printf 'original verifier invoked\\n' >> "$VERIFY_LOG"
exit "${VERIFY_FAILURE:-0}"
''')
        for command in ['apt-get', 'curl', 'ssh', 'scp', 'systemctl']:
            executable('bin/' + command, 'printf "forbidden command\\n" >> "$HOST_LOG"\nexit 99\n')
        version = '26.06-release+vrx1'
        packages = [dict(package=name, file=f'{name}_{version}_amd64.deb', version=version,
                         architecture='amd64', ship=True, sha256='a' * 64) for name in SHIP]
        packages.append(dict(package='vpp-dev', ship=False, file='vpp-dev.deb'))
        for package in packages:
            (root / 'artifacts' / package['file']).write_bytes(b'nonrelease selection fixture')
        (root / 'artifacts/SHA256SUMS').write_text('fixture metadata; original verifier is modeled')
        manifest = dict(schema='vrx.vpp-debs.manifest/v2', version=version, packages=packages)
        env = dict(os.environ, PATH=str(root / 'bin') + ':' + os.environ['PATH'],
                   VERIFY_LOG=str(root / 'verify-log'), HOST_LOG=str(root / 'host-log'))
        return root, entry, manifest, env

    def test_nonroot_preflight_selects_only_seven_runtimes_without_mutation(self):
        with tempfile.TemporaryDirectory() as directory:
            root, entry, manifest, env = self.fixture(directory)
            (root / 'artifacts/manifest.json').write_text(json.dumps(manifest))
            result = subprocess.run(['bash', str(entry), '--check-artifacts', str(root / 'artifacts')],
                                    env=env, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stderr.decode())
            selection = json.loads(result.stdout)
            self.assertEqual({p['package'] for p in selection['packages']}, set(SHIP))
            self.assertEqual(len(selection['packages']), 7)
            self.assertEqual(selection['version'], '26.06-release+vrx1')
            self.assertTrue((root / 'verify-log').exists())
            self.assertFalse((root / 'host-log').exists())

    def test_verifier_failure_blocks_repo_setup_before_host_commands(self):
        with tempfile.TemporaryDirectory() as directory:
            root, entry, manifest, env = self.fixture(directory)
            (root / 'artifacts/manifest.json').write_text(json.dumps(manifest))
            env.update(VRX_VPP_ARTIFACTS=str(root / 'artifacts'), VERIFY_FAILURE='17')
            result = subprocess.run(['bash', str(entry)], env=env, capture_output=True)
            self.assertEqual(result.returncode, 17)
            self.assertTrue((root / 'verify-log').exists())
            self.assertFalse((root / 'host-log').exists())

    def test_missing_artifacts_blocks_repo_setup_before_host_commands(self):
        with tempfile.TemporaryDirectory() as directory:
            root, entry, _, env = self.fixture(directory)
            env.pop('VRX_VPP_ARTIFACTS', None)
            result = subprocess.run(['bash', str(entry)], env=env, capture_output=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn(b'VRX_VPP_ARTIFACTS required', result.stderr)
            self.assertFalse((root / 'host-log').exists())

    def test_unsafe_or_nonproduct_selection_refuses(self):
        with tempfile.TemporaryDirectory() as directory:
            root, entry, original, env = self.fixture(directory)
            mutations = [('file', '../escape.deb'), ('version', '26.06-release'),
                         ('architecture', 'arm64'), ('sha256', ''), ('package', 'vpp-dev')]
            for field, value in mutations:
                with self.subTest(field=field):
                    manifest = copy.deepcopy(original)
                    manifest['packages'][0][field] = value
                    (root / 'artifacts/manifest.json').write_text(json.dumps(manifest))
                    result = subprocess.run(['bash', str(entry), '--check-artifacts', str(root / 'artifacts')],
                                            env=env, capture_output=True)
                    self.assertNotEqual(result.returncode, 0)
                    self.assertFalse((root / 'host-log').exists())

    def test_original_verifier_rejects_missing_release_manifest(self):
        with tempfile.TemporaryDirectory() as directory:
            result = subprocess.run(['bash', str(ROOT / 'deploy/vpp/verify.sh'),
                                     '--require-files', directory, '--install-gate'], capture_output=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn(b'cannot read', result.stdout + result.stderr)

    def test_foreign_symlink_inputs_refuse_before_original_verifier(self):
        for name in ['manifest.json', 'SHA256SUMS', 'vpp_26.06-release+vrx1_amd64.deb']:
            with self.subTest(name=name), tempfile.TemporaryDirectory() as directory:
                root, entry, manifest, env = self.fixture(directory)
                (root / 'artifacts/manifest.json').write_text(json.dumps(manifest))
                target = root / 'artifacts' / name
                foreign = root / ('foreign-' + name)
                foreign.write_bytes(target.read_bytes())
                target.unlink()
                target.symlink_to(foreign)
                result = subprocess.run(['bash', str(entry), '--check-artifacts', str(root / 'artifacts')],
                                        env=env, capture_output=True)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn(b'symlink artifact refused', result.stderr)
                self.assertFalse((root / 'verify-log').exists())
                self.assertFalse((root / 'host-log').exists())


if __name__ == '__main__':
    unittest.main(verbosity=2)
