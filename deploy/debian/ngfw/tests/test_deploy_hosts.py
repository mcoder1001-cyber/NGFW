#!/usr/bin/env python3
"""Run the deployment entry point with real .debs and disposable recording seams.

Only SSH/host commands and expensive builds are fixtures. The shipped shell,
package/checksum validation and remote control flow execute. Fixed host paths
are redirected in the SSH seam; production exposes no fixture/root override.
"""
import fcntl
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[4]
SCRIPT = ROOT / 'deploy/deploy-hosts.sh'
VERSION = '0.1.0~dev+git42.' + 'a' * 12
SOURCE = 'a' * 40
PACKAGES = {'ngfw-agent': 'amd64', 'ngfw-api': 'amd64', 'ngfw-meta': 'all', 'ngfw-web': 'all'}


def checksums(directory):
    (directory / 'SHA256SUMS').write_text(''.join(
        f'{hashlib.sha256(path.read_bytes()).hexdigest()}  {path.name}\n'
        for path in sorted(directory.iterdir()) if path.name != 'SHA256SUMS'))


def command(directory, name, body):
    path = directory / name
    path.write_text('#!/usr/bin/env python3\n' + body)
    path.chmod(0o755)


class DeployHosts(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.template = tempfile.TemporaryDirectory()
        cls.packages = Path(cls.template.name) / 'packages'
        cls.packages.mkdir()
        for package, arch in PACKAGES.items():
            directory = Path(cls.template.name) / package / 'DEBIAN'
            directory.mkdir(parents=True)
            (directory / 'control').write_text(
                f'Package: {package}\nVersion: {VERSION}\nArchitecture: {arch}\n'
                'Maintainer: Fixture <fixture@example.invalid>\nDescription: deployment fixture\n')
            subprocess.run(['dpkg-deb', '--build', str(directory.parent),
                            str(cls.packages / f'{package}_{VERSION}_{arch}.deb')],
                           check=True, capture_output=True)
        checksums(cls.packages)

    @classmethod
    def tearDownClass(cls):
        cls.template.cleanup()

    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.delivery = self.root / 'delivery with spaces'
        shutil.copytree(self.packages, self.delivery)
        self.bin = self.root / 'bin'
        self.bin.mkdir()
        self.host = self.root / 'host'
        for relative in ('etc/ngfw', 'etc/vpp', 'var/lib/ngfw', 'run/lock', 'root'):
            (self.host / relative).mkdir(parents=True)
        (self.host / 'etc/ngfw/api.env').write_text(
            'NGFW_DATABASE_URL=postgresql:///ngfw?host=/var/run/postgresql&user=ngfw\n'
            f'NGFW_SECRET_KEY_FILE={self.host}/var/lib/ngfw/secret.key\n')
        (self.host / 'etc/vpp/startup.conf').write_text('fixture startup\n')
        (self.host / 'var/lib/ngfw/secret.key').write_text('fixture-only-key\n')
        (self.host / 'var/lib/ngfw/firstboot-complete').write_text('complete\n')
        self.events = self.root / 'events'
        self.env = dict(os.environ, PATH=str(self.bin) + ':' + os.environ['PATH'],
                        FIXTURE_ROOT=str(self.root), FIXTURE_VERSION=VERSION)
        command(self.bin, 'ssh', r'''
import os, pathlib, subprocess, sys
root = pathlib.Path(os.environ['FIXTURE_ROOT'])
args = sys.argv[1:]
with (root / 'events').open('a') as log: log.write('ssh ' + repr(args) + '\n')
# OpenSSH concatenates remote command words into a shell command. Reproduce it.
while args and args[0] == '-o': args = args[2:]
assert len(args) >= 2
host, cmd = args[0], ' '.join(args[1:])
if os.environ.get('FAIL_STAGE') == 'ssh' and cmd.startswith('umask 077;'): sys.exit(1)
script = sys.stdin.read() if 'bash -s' in cmd else ''
def redirect(value):
    for prefix in ['/root/ngfw-deploy', '/run/lock', '/etc/ngfw', '/etc/vpp', '/var/lib/ngfw']:
        value = value.replace(prefix, str(root / 'host') + prefix)
    return value
cmd, script = redirect(cmd), redirect(script)
# Keep fixture PATH/environment only in this recorder, never on a real host.
cmd = cmd.replace('env -i PATH=/usr/sbin:/usr/bin:/sbin:/bin LC_ALL=C HOME=/root ', '')
result = subprocess.run(['bash', '-c', cmd], input=script, text=True, capture_output=True)
sys.stdout.write(result.stdout.replace(str(root / 'host'), ''))
sys.stderr.write(result.stderr)
sys.exit(result.returncode)
''')
        command(self.bin, 'scp', r'''
import os, pathlib, shutil, sys
root = pathlib.Path(os.environ['FIXTURE_ROOT'])
args = sys.argv[1:]
with (root / 'events').open('a') as log: log.write('scp\n')
while args and args[0].startswith('-'):
    args = args[2:] if args[0] == '-o' else args[1:]
destination = root / 'host' / args[-1].split(':', 1)[1].lstrip('/')
for source in args[:-1]: shutil.copy2(source, destination)
if os.environ.get('FAIL_STAGE') == 'transfer':
    with next(destination.glob('*.deb')).open('ab') as output: output.write(b'corrupt')
''')
        recorder = r'''
import os, pathlib, subprocess, sys
root = pathlib.Path(os.environ['FIXTURE_ROOT'])
name = pathlib.Path(sys.argv[0]).name
args = sys.argv[1:]
failure = os.environ.get('FAIL_STAGE', '')
with (root / 'events').open('a') as log: log.write(name + ' ' + repr(args) + '\n')
if name == 'id': print('1000' if failure == 'root' else '0')
elif name == 'dpkg-query':
    if '${Status}' in args[1]: print('install ok installed')
    elif '${Version}' in args[1]:
        print('99.0' if failure == 'downgrade' else (os.environ['FIXTURE_VERSION'] if (root / 'installed').exists() else '0.0.1'))
    else: print('ngfw-api 0.0.1\nvpp 26.06-release+ngfw3')
elif name == 'ip':
    if args[0] == '-4': print('default via 192.0.2.1 dev fixture' + (' changed' if failure == 'route' and (root / 'installed').exists() else ''))
elif name == 'apt-get':
    if '--simulate' in args:
        print('Inst vpp [old] (new)' if failure == 'dependency' else 'Inst ngfw-api [old] (new)')
        if failure == 'simulate': sys.exit(1)
    else:
        if failure == 'apt': sys.exit(1)
        (root / 'installed').write_text('yes')
elif name == 'runuser':
    if failure == 'dump': sys.exit(1)
    print('fixture database archive')
elif name == 'pg_restore':
    if failure == 'restore': sys.exit(1)
elif name == 'tar':
    if failure == 'tar' and '-czf' in args: sys.exit(1)
    if '-C' in args: args[args.index('-C') + 1] = str(root / 'host')
    sys.exit(subprocess.run(['/usr/bin/tar'] + args).returncode)
elif name == 'curl': sys.exit(1 if failure == 'health' else 0)
elif name == 'systemctl':
    # Match systemctl's ANY-active multi-unit exit semantics to catch regressions.
    if args[0] == 'is-active' and failure.startswith(('inactive-', 'preinactive-')):
        unit = failure.split('-', 1)[1] + '.service'
        if (root / 'installed').exists() or failure.startswith('preinactive-'):
            sys.exit(1 if args[-1:] == [unit] and len([a for a in args[1:] if not a.startswith('-')]) == 1 else 0)
    sys.exit(1 if failure == 'service' and (root / 'installed').exists() else 0)
elif name == 'stat':
    result = subprocess.run(['/usr/bin/stat'] + args, capture_output=True, text=True)
    # Model UID 0 only for this disposable host (works for non-root CI builders).
    output = result.stdout
    if args[:2] == ['-c', '%u:%a']: output = '0:' + output.split(':', 1)[1]
    print(output, end=''); sys.exit(result.returncode)
elif name in ('sleep', 'pg_dump'): pass
else: raise AssertionError(name)
'''
        for name in ('id', 'dpkg-query', 'ip', 'apt-get', 'runuser', 'pg_restore', 'tar', 'curl', 'systemctl', 'sleep', 'pg_dump', 'stat'):
            command(self.bin, name, recorder)

    def run_script(self, *args, failure='', script=SCRIPT):
        env = dict(self.env, FAIL_STAGE=failure)
        return subprocess.run(['bash', str(script), *map(str, args)], env=env,
                              text=True, capture_output=True, timeout=30)

    def deploy(self, *hosts, failure=''):
        return self.run_script('-p', self.delivery, *(hosts or ['appliance.example']), failure=failure)

    def log(self):
        return self.events.read_text() if self.events.exists() else ''

    def assert_rejected_without_ssh(self, result):
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertNotIn('ssh ', self.log())

    def test_success_uses_private_backup_and_verified_installs(self):
        result = self.deploy()
        self.assertEqual(result.returncode, 0, result.stderr)
        runs = list((self.host / 'root/ngfw-deploy').glob('run.*'))
        self.assertEqual(len(runs), 1)
        self.assertEqual(runs[0].stat().st_mode & 0o777, 0o700)
        for name in ('preupgrade-config.tar.gz', 'preupgrade-ngfw.dump', 'previous-versions.txt', 'upgrade-complete'):
            self.assertTrue((runs[0] / name).is_file(), name)
            self.assertEqual((runs[0] / name).stat().st_mode & 0o777, 0o600)
        log = self.log()
        self.assertLess(log.index('pg_restore '), log.rindex('apt-get '))
        self.assertIn('StrictHostKeyChecking=yes', log)
        self.assertNotIn('--allow-downgrades', log)
        self.assertIn("systemctl ['restart', 'ngfw-agent.service', 'ngfw-api.service']", log)

    def test_repeated_upgrade_keeps_previous_backup(self):
        for _ in range(2):
            result = self.deploy()
            self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(len(list((self.host / 'root/ngfw-deploy').glob('run.*'))), 2)

    def test_help_and_build_only_have_no_network_or_host_mutation(self):
        self.assertEqual(self.run_script('-h').returncode, 0)
        result = self.run_script('-p', self.delivery, '-b')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.strip(), str(self.delivery))
        self.assertNotIn('ssh ', self.log())
        self.assertNotIn('apt-get ', self.log())

    def test_unsafe_arguments_fail_before_ssh(self):
        for args in [('-p', self.delivery, 'bad;touch'), ('-u', 'root;true', '-p', self.delivery, 'host'),
                     ('-r', '--upload-pack=x', '-p', self.delivery, 'host'),
                     ('-p', self.delivery, '-w', self.root, 'host'), ('-p', self.delivery)]:
            with self.subTest(args=args): self.assert_rejected_without_ssh(self.run_script(*args))

    def test_missing_or_incomplete_or_traversing_manifest_fails_before_ssh(self):
        manifest = self.delivery / 'SHA256SUMS'
        original = manifest.read_text()
        for content in [None, original.splitlines()[0] + '\n', original + original.splitlines()[0] + '\n',
                        original.replace('  ngfw-agent_', '  ../ngfw-agent_')]:
            with self.subTest(content=content):
                if content is None: manifest.unlink()
                else: manifest.write_text(content)
                self.assert_rejected_without_ssh(self.deploy())

    def test_corrupt_package_fails_before_ssh(self):
        with next(self.delivery.glob('*.deb')).open('ab') as output: output.write(b'changed')
        self.assert_rejected_without_ssh(self.deploy())

    def test_symlink_and_extra_archive_fail_before_ssh(self):
        package = next(self.delivery.glob('*.deb'))
        saved = self.root / package.name
        package.rename(saved)
        package.symlink_to(saved)
        self.assert_rejected_without_ssh(self.deploy())
        package.unlink()
        saved.rename(package)
        shutil.copy2(package, self.delivery / 'extra.deb')
        checksums(self.delivery)
        self.assert_rejected_without_ssh(self.deploy())

    def test_metadata_identity_version_and_architecture_fail_closed(self):
        package = self.delivery / f'ngfw-agent_{VERSION}_amd64.deb'
        for field, replacement in [('Package', 'other-package'), ('Version', '0.2.0'), ('Architecture', 'arm64')]:
            with self.subTest(field=field):
                directory = self.root / ('metadata-' + field)
                subprocess.run(['dpkg-deb', '-R', str(self.packages / package.name), str(directory)], check=True, capture_output=True)
                control = directory / 'DEBIAN/control'
                lines = control.read_text().splitlines()
                control.write_text('\n'.join(f'{field}: {replacement}' if line.startswith(field + ':') else line for line in lines) + '\n')
                subprocess.run(['dpkg-deb', '--build', str(directory), str(package)], check=True, capture_output=True)
                checksums(self.delivery)
                self.assert_rejected_without_ssh(self.deploy())

    def test_preinstall_failures_do_not_install_or_continue_to_second_host(self):
        for failure in ('root', 'transfer', 'downgrade', 'dependency', 'simulate', 'tar', 'dump', 'restore'):
            with self.subTest(failure=failure):
                self.events.unlink(missing_ok=True)
                result = self.deploy('first.example', 'second.example', failure=failure)
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertFalse((self.root / 'installed').exists())
                self.assertNotIn('second.example', self.log())
                if failure in ('tar', 'dump', 'restore'):
                    self.assertIn('apt-get ', self.log())
                if failure in ('dump', 'restore'):
                    self.assertIn('runuser ', self.log())
                if failure == 'restore':
                    self.assertIn('pg_restore ', self.log())

    def test_install_and_postinstall_failures_are_reported_and_stop_fleet(self):
        for failure in ('apt', 'health', 'service', 'route', 'ssh', 'inactive-ngfw-agent', 'inactive-nginx', 'inactive-vpp'):
            with self.subTest(failure=failure):
                self.events.unlink(missing_ok=True)
                (self.root / 'installed').unlink(missing_ok=True)
                result = self.deploy('first.example', 'second.example', failure=failure)
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertNotIn('second.example', self.log())
                self.assertIn('inspect /root/ngfw-deploy/run.', result.stderr)

    def test_inactive_preflight_service_prevents_install(self):
        for unit in ('ngfw-agent', 'nginx', 'vpp'):
            with self.subTest(unit=unit):
                result = self.deploy(failure='preinactive-' + unit)
                self.assertNotEqual(result.returncode, 0)
                self.assertNotIn('apt-get ', self.log())

    def test_old_shared_lock_symlink_cannot_truncate_target(self):
        target = self.root / 'keep'
        target.write_text('must survive')
        (self.host / 'run/lock/ngfw-deploy.lock').symlink_to(target)
        result = self.deploy()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(target.read_text(), 'must survive')

    def test_remote_lock_symlink_is_refused_without_truncation(self):
        directory = self.host / 'root/ngfw-deploy'
        directory.mkdir(mode=0o700)
        target = self.root / 'keep'
        target.write_text('must survive')
        (directory / 'upgrade.lock').symlink_to(target)
        result = self.deploy()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(target.read_text(), 'must survive')
        self.assertNotIn('apt-get ', self.log())

    def test_existing_deployment_lock_prevents_install(self):
        directory = self.host / 'root/ngfw-deploy'
        directory.mkdir(mode=0o700)
        with (directory / 'upgrade.lock').open('w') as lock:
            os.chmod(lock.name, 0o600)
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            result = self.deploy()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('another NGFW deployment', result.stderr)
        self.assertNotIn('apt-get ', self.log())

    def test_noncanonical_database_fails_before_backup_or_install(self):
        (self.host / 'etc/ngfw/api.env').write_text('NGFW_DATABASE_URL=unsupported-fixture-db\n')
        result = self.deploy()
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn('runuser ', self.log())
        self.assertFalse((self.root / 'installed').exists())

    def test_external_secret_key_requires_manual_backup_review(self):
        path = self.host / 'etc/ngfw/api.env'
        path.write_text(path.read_text().replace('/var/lib/ngfw/secret.key', '/other/secret.key'))
        result = self.deploy()
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn('runuser ', self.log())
        self.assertFalse((self.root / 'installed').exists())

    def build_fixture(self):
        repo = self.root / 'repo'
        (repo / 'deploy/vpp').mkdir(parents=True)
        script = repo / 'deploy/deploy-hosts.sh'
        shutil.copy2(SCRIPT, script)
        command(repo / 'deploy/vpp', 'verify.sh', 'import sys\nsys.exit(0)\n')
        command(self.bin, 'git', r'''
import os, pathlib, shutil, sys
root = pathlib.Path(os.environ['FIXTURE_ROOT'])
args = sys.argv[1:]
with (root / 'events').open('a') as log: log.write('git ' + repr(args) + '\n')
if '--is-shallow-repository' in args: print('true' if os.environ.get('FAIL_STAGE') == 'shallow' else 'false')
elif 'rev-parse' in args: print('a' * 40)
elif 'rev-list' in args: print('42')
elif 'worktree' in args and 'add' in args:
    source = pathlib.Path(args[-2]); (source / 'tools').mkdir(parents=True)
    shutil.copy2(root / 'heavy.sh', source / 'tools/heavy.sh')
elif 'worktree' in args and 'remove' in args: shutil.rmtree(args[-1])
''')
        heavy = self.root / 'heavy.sh'
        heavy.write_text('''#!/usr/bin/env python3
import os, pathlib, shutil, sys
root = pathlib.Path(os.environ['FIXTURE_ROOT'])
if sys.argv[1] == 'deploy/debian/ngfw/prepare.sh': pathlib.Path(sys.argv[3]).mkdir()
if sys.argv[1] == 'dpkg-buildpackage':
    for path in (root / 'delivery with spaces').glob('*.deb'): shutil.copy2(path, pathlib.Path.cwd().parent)
''')
        heavy.chmod(0o755)
        vpp = self.root / 'vpp'
        vpp.mkdir()
        (vpp / 'manifest.json').write_text('fixture VPP provenance\n')
        return script, vpp

    def test_build_and_cache_reuse_are_bound_to_source_and_vpp(self):
        script, vpp = self.build_fixture()
        work = self.root / 'build'
        result = self.run_script('-b', '-w', work, '-v', vpp, script=script)
        self.assertEqual(result.returncode, 0, result.stderr)
        info = json.loads((work / 'packages/build-info.json').read_text())
        self.assertEqual(info['source_commit'], SOURCE)
        result = self.run_script('-b', '-w', work, '-v', vpp, script=script)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.log().count("'add'"), 1)
        (vpp / 'manifest.json').write_text('different VPP input\n')
        result = self.run_script('-b', '-w', work, '-v', vpp, script=script)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('cache mismatch', result.stderr)
        self.assertTrue((work / 'packages/build-info.json').exists())

    def test_shallow_build_refuses_incomplete_version_history(self):
        script, vpp = self.build_fixture()
        result = self.run_script('-b', '-v', vpp, script=script, failure='shallow')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('complete Git history', result.stderr)
        self.assertNotIn("'add'", self.log())

    def test_development_versions_increase_for_descendants_and_preserve_release_guard(self):
        for previous in ('0.1.0~dev+ffffffffffff', '0.1.0~dev+git41.ffffffffffff'):
            subprocess.run(['dpkg', '--compare-versions', VERSION, 'gt', previous], check=True)
        result = subprocess.run(['dpkg', '--compare-versions', VERSION, 'ge', '0.1.0'])
        self.assertNotEqual(result.returncode, 0)

    def test_stale_source_cache_and_preexisting_work_are_not_reused_or_deleted(self):
        script, vpp = self.build_fixture()
        work = self.root / 'build'
        result = self.run_script('-b', '-w', work, '-v', vpp, script=script)
        self.assertEqual(result.returncode, 0, result.stderr)
        path = work / 'packages/build-info.json'
        info = json.loads(path.read_text()); info['source_commit'] = 'b' * 40
        path.write_text(json.dumps(info)); checksums(path.parent)
        result = self.run_script('-b', '-w', work, '-v', vpp, script=script)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('cache mismatch', result.stderr)
        existing = self.root / 'existing'; existing.mkdir()
        (existing / 'keep').write_text('operator data')
        result = self.run_script('-b', '-w', existing, '-v', vpp, script=script)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual((existing / 'keep').read_text(), 'operator data')


if __name__ == '__main__':
    unittest.main()
