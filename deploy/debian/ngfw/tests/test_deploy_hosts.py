#!/usr/bin/env python3
"""Run the deployment entry point with real .debs and disposable recording seams.

Only SSH/host commands, expensive builds and the source history (a fake `git`
with a private lineage) are fixtures. The shipped shell, package/checksum
validation and remote control flow execute. Fixed host paths are redirected in
the SSH seam; production exposes no fixture/root override.
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
# Fixture source history: c < a < e on one line; b branched from c; d is unknown.
INSTALLED_VERSIONS = {
    'downgrade': '99.0',
    'unrelated-lower': '0.1.0~dev+git41.' + 'b' * 12,
    'unrelated-higher': '0.1.0~dev+git99.' + 'b' * 12,
    'ancestor': '0.1.0~dev+git40.' + 'c' * 12,
    'legacy-ancestor': '0.1.0~dev+' + 'c' * 12,
    'unresolved': '0.1.0~dev+git40.' + 'd' * 12,
    'unresolved-new': '0.1.0~dev+git40.' + 'c' * 12,
    'fetch-resolves': '0.1.0~dev+git39.' + 'd' * 12,
    'broken-repo': '0.1.0~dev+git40.' + 'c' * 12,
    'rollback': '0.1.0~dev+git50.' + 'e' * 12,
    'race': '0.1.0~dev+git40.' + 'c' * 12,
    'unparsed-dev': '0.1.0~dev+git40.' + 'c' * 13,
    'hostile-version': '1.0;id',
    'hostile-extra': '0.0.1\nextra line',
}
BUILD_INFO = {'source_commit': SOURCE, 'source_count': 42, 'vpp_manifest_sha256': 'f' * 64}
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
(root / 'staged').write_text('yes')
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
    elif args[1] == '-f=${Version}':
        if (root / 'installed').exists(): print(os.environ['FIXTURE_VERSION'])
        elif failure == 'mixed' and args[-1] == 'ngfw-web': print('0.0.2')
        elif failure == 'race' and (root / 'staged').exists(): print(os.environ['FIXTURE_INSTALLED_UNRELATED_LOWER'])
        else: print(os.environ.get('FIXTURE_INSTALLED_' + failure.replace('-', '_').upper(), '0.0.1'))
    else: print('ngfw-api 0.0.1\nvpp 26.06-release+ngfw3')
elif name == 'ip':
    installed = (root / 'installed').exists()
    if args[0] == '-4':
        gateway = '192.0.2.254' if failure == 'route' and installed else '192.0.2.1'
        metric = 200 if failure == 'route-metric' and installed else 100
        print(f'default via {gateway} dev fixture proto dhcp src 192.0.2.10 metric {metric}')
    elif args[0] == '-6':
        gateway = 'fe80::2' if failure == 'route6' and installed else 'fe80::1'
        device = 'other' if failure == 'route-dev' and installed else 'fixture'
        print(f'default via {gateway} dev {device} proto ra metric 1024 expires {1402 if installed else 1693}sec hoplimit 64 pref medium')
        hops = ('fe80::b', 'fe80::a') if failure == 'route-nexthop' and installed else ('fe80::a', 'fe80::b')
        print(f'default proto ra metric 2048 expires {1300 if installed else 1500}sec pref medium')
        print(f'\tnexthop via {hops[0]} dev fixture weight 1 \n\tnexthop via {hops[1]} dev fixture2 weight 1 ')
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
    if args[0] == 'show':
        assert args[1:4] == ['-p', 'NRestarts', '--value'] and len(args) == 5
        marker = root / ('shown-' + args[4])
        seen = marker.exists(); marker.write_text('seen')
        # The daemon is still 'active' at each probe but was restarted in between.
        looping = {'crashloop': 'ngfw-agent.service', 'crashloop-api': 'ngfw-api.service'}.get(failure)
        print('1' if seen and args[4] == looping else '0')
        sys.exit(0)
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
        # Private source history: the real repository is never read or fetched.
        command(self.bin, 'git', r'''
import os, pathlib, shutil, sys
root = pathlib.Path(os.environ['FIXTURE_ROOT'])
args = sys.argv[1:]
with (root / 'events').open('a') as log: log.write('git ' + repr(args) + '\n')
a, b, c, d, e = 'a' * 40, 'b' * 40, 'c' * 40, 'd' * 40, 'e' * 40
ancestors = {a: {a, c}, b: {b, c}, c: {c}, e: {e, a, c}}
mode = os.environ.get('FAIL_STAGE', '')
# d becomes known (as an ancestor of a) only after a fetch in the fetch-resolves mode.
if mode == 'fetch-resolves' and (root / 'fetched').exists():
    ancestors[d] = {d, c}; ancestors[a].add(d)
if '--is-shallow-repository' in args: print('true' if mode == 'shallow' else 'false')
elif '--is-inside-work-tree' in args: print('true')
elif 'config' in args: sys.exit(1)
elif 'rev-parse' in args:
    target = args[-1].removesuffix('^{commit}')
    if target == 'FETCH_HEAD': print(a)
    else:
        if mode == 'unresolved-new' and target.startswith('a'): sys.exit(1)
        found = [commit for commit in ancestors if commit.startswith(target)]
        if len(found) != 1: sys.exit(1)
        print(found[0])
elif 'merge-base' in args:
    if mode == 'broken-repo': sys.exit(128)
    if '--is-ancestor' in args: sys.exit(0 if args[-2] in ancestors[args[-1]] else 1)
    common = ancestors[args[-2]] & ancestors[args[-1]]
    if not common: sys.exit(1)
    print(max(common, key=lambda commit: len(ancestors[commit])))
elif 'rev-list' in args: print('3\t2' if '--left-right' in args else '42')
elif 'fetch' in args:
    if args[-1] != 'main':  # the lineage fetch; the build fetches origin REF
        assert args[-4:] == ['--quiet', '--no-tags', '--no-write-fetch-head', 'origin'], args
        assert os.environ.get('GIT_TERMINAL_PROMPT') == '0' and os.environ.get('GIT_SSH_COMMAND') == 'ssh -o BatchMode=yes'
    (root / 'fetched').write_text('yes')
elif 'worktree' in args and 'add' in args:
    source = pathlib.Path(args[-2]); (source / 'tools').mkdir(parents=True)
    shutil.copy2(root / 'heavy.sh', source / 'tools/heavy.sh')
elif 'worktree' in args and 'remove' in args: shutil.rmtree(args[-1])
else: raise AssertionError(args)
''')

    def run_script(self, *args, failure='', script=SCRIPT):
        env = dict(self.env, FAIL_STAGE=failure)
        for mode, installed in INSTALLED_VERSIONS.items():
            env['FIXTURE_INSTALLED_' + mode.replace('-', '_').upper()] = installed
        return subprocess.run(['bash', str(script), *map(str, args)], env=env,
                              text=True, capture_output=True, timeout=30)

    def deploy(self, *hosts, failure='', flags=()):
        return self.run_script('-p', self.delivery, *flags, *(hosts or ['appliance.example']), failure=failure)

    def reset_host(self):
        self.events.unlink(missing_ok=True)
        for marker in [self.root / 'installed', self.root / 'staged', self.root / 'fetched', *self.root.glob('shown-*')]:
            marker.unlink(missing_ok=True)
        shutil.rmtree(self.host / 'root/ngfw-deploy', ignore_errors=True)

    def add_build_info(self):
        (self.delivery / 'build-info.json').write_text(json.dumps(BUILD_INFO))
        checksums(self.delivery)

    def runs(self):
        return sorted((self.host / 'root/ngfw-deploy').glob('run.*'))

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
        # Restart counters are read after the manual restart and again after a stability window.
        self.assertEqual(log.count("systemctl ['show', '-p', 'NRestarts', '--value', 'ngfw-agent.service']"), 2)
        self.assertEqual(log.count("systemctl ['show', '-p', 'NRestarts', '--value', 'ngfw-api.service']"), 2)
        self.assertLess(log.index("systemctl ['restart'"), log.index("'NRestarts'"))
        self.assertLess(log.rindex('curl '), log.index("sleep ['10']"))
        self.assertLess(log.index("sleep ['10']"), log.rindex("'NRestarts'"))
        self.assertLess(log.rindex("'NRestarts'"), log.rindex("systemctl ['is-active'"))
        self.assertNotIn("sleep ['1']", log)
        # The RA-learned IPv6 route expiry differs before and after; only via/dev/metric count.
        self.assertIn('expires 1693sec', (runs[0] / 'default-route-v6.txt').read_text())
        self.assertIn('src 192.0.2.10', (runs[0] / 'default-route-v4.txt').read_text())
        self.assertEqual(log.count("ip ['-6', 'route', 'show', 'default']"), 3)
        self.assertFalse((runs[0] / 'default-route-v6-after.txt').exists())
        self.assertFalse((runs[0] / 'override.txt').exists())
        self.assertFalse((runs[0] / 'service-status.txt').exists())
        self.assertNotIn('git ', log)
        self.assertRegex(log, r"bash -s -- /root/ngfw-deploy/run\.[A-Za-z0-9]{8}/packages [a-f0-9]{64} 0\.1\.0~dev\+git42\.a{12} 0 0\.0\.1'")

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
                     ('-p', self.delivery, '-w', self.root, 'host'), ('-p', self.delivery),
                     ('--allow-divergent=1', '-p', self.delivery, 'host'), ('--allow', '-p', self.delivery, 'host'),
                     ('--allow-divergent', '-p', self.delivery, 'a.example', 'b.example'),
                     ('-p', self.delivery, 'a.example', '--allow-divergent')]:
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
        for failure in ('root', 'transfer', 'downgrade', 'mixed', 'dependency', 'simulate', 'tar', 'dump', 'restore'):
            with self.subTest(failure=failure):
                self.events.unlink(missing_ok=True)
                result = self.deploy('first.example', 'second.example', failure=failure)
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertFalse((self.root / 'installed').exists())
                self.assertNotIn('second.example', self.log())
                self.assertNotIn('--allow-downgrades', self.log())
                if failure == 'downgrade':
                    self.assertIn('downgrade refused: ngfw-agent 99.0', result.stderr)
                if failure == 'mixed':
                    self.assertIn('mixed installed package versions', result.stderr)
                    self.assertNotIn('scp', self.log())
                if failure in ('tar', 'dump', 'restore'):
                    self.assertIn('apt-get ', self.log())
                if failure in ('dump', 'restore'):
                    self.assertIn('runuser ', self.log())
                if failure == 'restore':
                    self.assertIn('pg_restore ', self.log())

    def test_install_and_postinstall_failures_are_reported_and_stop_fleet(self):
        for failure in ('apt', 'health', 'service', 'route', 'route6', 'route-dev', 'route-metric', 'route-nexthop',
                        'crashloop', 'crashloop-api', 'ssh', 'inactive-ngfw-agent', 'inactive-nginx', 'inactive-vpp'):
            with self.subTest(failure=failure):
                self.reset_host()
                result = self.deploy('first.example', 'second.example', failure=failure)
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertNotIn('second.example', self.log())
                self.assertIn('inspect /root/ngfw-deploy/run.', result.stderr)
                if failure != 'ssh':
                    self.assertTrue((self.runs()[0] / 'service-status.txt').is_file())
                if failure.startswith('route'):
                    self.assertIn('management default route changed', result.stderr)
                    self.assertIn('after v6:', result.stderr)
                    self.assertTrue((self.runs()[0] / 'default-route-v6-after.txt').is_file())
                if failure.startswith('crashloop'):
                    unit = 'ngfw-api.service' if failure == 'crashloop-api' else 'ngfw-agent.service'
                    self.assertIn(f'after: {unit} NRestarts=1', result.stderr.replace('ngfw-agent.service NRestarts=0, ', ''))
                    self.assertTrue((self.root / 'installed').exists())

    def test_stale_branch_with_higher_commit_count_is_refused_before_transfer(self):
        # dpkg orders git41.b below git42.a, so the old count-only guard accepted this older tree.
        subprocess.run(['dpkg', '--compare-versions', VERSION, 'gt', INSTALLED_VERSIONS['unrelated-lower']], check=True)
        self.add_build_info()
        result = self.deploy('first.example', 'second.example', failure='unrelated-lower')
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertIn('divergent histories: installed bbbbbbbbbbbb, new aaaaaaaaaaaa, merge-base cccccccccccc, commits only in installed/new: 3/2', result.stderr)
        self.assertIn('review: git -C ' + str(ROOT) + ' log --oneline bbbbbbbbbbbb..aaaaaaaaaaaa', result.stderr)
        self.assertIn('nothing was transferred', result.stderr)
        log = self.log()
        self.assertIn("'" + 'a' * 40 + "^{commit}'", log)
        self.assertIn("git ['-C', '" + str(ROOT) + "', 'merge-base', '--is-ancestor', '" + 'b' * 40 + "', '" + 'a' * 40 + "']", log)
        self.assertNotIn('scp', log)
        self.assertNotIn('apt-get ', log)
        self.assertNotIn('second.example', log)
        self.assertRegex(log, r"'rmdir -- /root/ngfw-deploy/run\.[A-Za-z0-9]{8}'")
        self.assertEqual(self.runs(), [])

    def test_rollback_within_one_history_is_refused_even_with_override(self):
        for flags in ((), ('--allow-divergent',)):
            with self.subTest(flags=flags):
                self.reset_host()
                result = self.deploy(failure='rollback', flags=flags)
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertIn('is older than installed eeeeeeeeeeee in the same history', result.stderr)
                self.assertNotIn('scp', self.log())
                self.assertNotIn('--allow-downgrades', self.log())
                self.assertEqual(self.runs(), [])

    def test_installed_version_changed_between_preflight_and_lock_is_refused(self):
        result = self.deploy(failure='race')
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertIn('installed version changed since preflight: ngfw-agent 0.1.0~dev+git41.bbbbbbbbbbbb, expected 0.1.0~dev+git40.cccccccccccc', result.stderr)
        self.assertIn('is an ancestor of', result.stderr)
        self.assertNotIn('--simulate', self.log())
        self.assertFalse((self.root / 'installed').exists())

    def test_hostile_preflight_output_is_rejected_before_git_or_transfer(self):
        for failure, message in (('hostile-version', 'invalid installed package version'),
                                 ('hostile-extra', 'unexpected preflight output'),
                                 ('unparsed-dev', 'carries no recognisable source commit')):
            with self.subTest(failure=failure):
                self.reset_host()
                result = self.deploy(failure=failure)
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertIn(message, result.stderr)
                self.assertNotIn('scp', self.log())
                self.assertNotIn("'merge-base'", self.log())

    def test_newer_main_build_over_divergent_branch_needs_explicit_override(self):
        # dpkg orders git99.b above git42.a: without the override this is a downgrade with no path forward.
        subprocess.run(['dpkg', '--compare-versions', INSTALLED_VERSIONS['unrelated-higher'], 'gt', VERSION], check=True)
        result = self.deploy(failure='unrelated-higher')
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertIn('commit counts do not order divergent histories', result.stderr)
        self.assertNotIn('scp', self.log())
        self.assertEqual(self.runs(), [])
        self.reset_host()
        result = self.deploy(failure='unrelated-higher', flags=('--allow-divergent',))
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('WARNING', result.stderr)
        self.assertIn('divergent development build accepted over ngfw-agent', result.stderr)
        log = self.log()
        self.assertEqual(log.count("'--allow-downgrades'"), 2)
        self.assertLess(log.index("'--allow-downgrades'"), log.index("'--simulate'"))
        self.assertRegex(log, r"bash -s -- /root/ngfw-deploy/run\.[A-Za-z0-9]{8}/packages [a-f0-9]{64} 0\.1\.0~dev\+git42\.a{12} 1 0\.1\.0~dev\+git99\.b{12}'")
        self.assertTrue((self.root / 'installed').exists())
        self.assertEqual((self.runs()[0] / 'override.txt').read_text(), 'allow-divergent accepted by operator; apt --allow-downgrades\n')

    def test_override_never_accepts_release_downgrade_or_mixed_versions(self):
        for failure in ('downgrade', 'mixed'):
            with self.subTest(failure=failure):
                self.reset_host()
                result = self.deploy(failure=failure, flags=('--allow-divergent',))
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertFalse((self.root / 'installed').exists())
                self.assertNotIn('--allow-downgrades', self.log())
                if failure == 'mixed':
                    self.assertIn('ngfw-api 0.0.1', result.stderr)
        self.reset_host()
        # A host whose lineage needs no override gets the remote literal 0 even with the flag.
        self.assertEqual(self.deploy(flags=('--allow-divergent',)).returncode, 0)
        self.assertNotIn('--allow-downgrades', self.log())
        self.assertRegex(self.log(), r"bash -s -- /root/ngfw-deploy/run\.[A-Za-z0-9]{8}/packages [a-f0-9]{64} 0\.1\.0~dev\+git42\.a{12} 0 0\.0\.1'")

    def test_descendant_sources_install_including_legacy_hash_versions(self):
        for failure in ('ancestor', 'legacy-ancestor'):
            with self.subTest(failure=failure):
                self.reset_host()
                if failure == 'ancestor': self.add_build_info()
                result = self.deploy(failure=failure)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn('is an ancestor of', result.stderr)
                self.assertNotIn('--allow-downgrades', self.log())
                self.assertNotIn("'fetch'", self.log())
                self.assertTrue((self.root / 'installed').exists())

    def test_unknown_source_is_fetched_once_then_refused_without_override(self):
        for failure, missing in (('unresolved', 'installed source dddddddddddd is not in'), ('unresolved-new', 'new source aaaaaaaaaaaa is not in')):
            with self.subTest(failure=failure):
                self.reset_host()
                result = self.deploy(failure=failure)
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertIn(missing, result.stderr)
                self.assertIn('fetching origin branches', result.stderr)
                self.assertEqual(self.log().count("'fetch'"), 1)
                self.assertIn("'--no-tags', '--no-write-fetch-head', 'origin'", self.log())
                self.assertNotIn('scp', self.log())
                self.assertEqual(self.runs(), [])
                self.reset_host()
                result = self.deploy(failure=failure, flags=('--allow-divergent',))
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn('lineage unverifiable', result.stderr)
                self.assertNotIn('--allow-downgrades', self.log())
                self.assertIn('without --allow-downgrades', (self.runs()[0] / 'override.txt').read_text())
        # A commit that the fetch brings in resolves and installs without any override.
        self.reset_host()
        result = self.deploy(failure='fetch-resolves')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('fetching origin branches', result.stderr)
        self.assertIn('installed source dddddddddddd is an ancestor of aaaaaaaaaaaa', result.stderr)
        self.assertEqual(self.log().count("'fetch'"), 1)
        self.assertTrue((self.root / 'installed').exists())
        # git failing to decide ancestry is refused, not read as divergence, even with the flag.
        self.reset_host()
        result = self.deploy(failure='broken-repo', flags=('--allow-divergent',))
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertIn('could not decide the ancestry', result.stderr)
        self.assertNotIn('scp', self.log())
        self.assertEqual(self.runs(), [])

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
        # Counts say nothing about unrelated histories; the helper checks ancestry separately.
        subprocess.run(['dpkg', '--compare-versions', '0.1.0~dev+git99.' + 'b' * 12, 'gt', VERSION], check=True)
        # The lineage parser must keep matching prepare.sh's version format.
        self.assertIn('VERSION=0.1.0~dev+git$SOURCE_COUNT.$COMMIT\n', (ROOT / 'deploy/debian/ngfw/prepare.sh').read_text())
        self.assertIn("'^0\\.1\\.0~dev\\+(git[1-9][0-9]*\\.)?[a-f0-9]{12}$'", SCRIPT.read_text())

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
