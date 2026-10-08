#!/usr/bin/env python3
"""Run the actual staging driver with private source and harmless build tools."""
import json
import hashlib
import os
import pathlib
import shutil
import subprocess
import tempfile
import unittest

SOURCE = pathlib.Path(__file__).resolve().parents[1]


class Preparation(unittest.TestCase):
    def run_prepare(self, mode):
        with tempfile.TemporaryDirectory() as temporary:
            root = pathlib.Path(temporary)
            driver = root / 'deploy/debian/ngfw'
            driver.mkdir(parents=True)
            shutil.copyfile(SOURCE / 'prepare.sh', driver / 'prepare.sh')
            shutil.copytree(SOURCE / 'debian', driver / 'debian')
            shutil.copytree(SOURCE / 'assets', driver / 'assets', ignore=shutil.ignore_patterns('__pycache__'))
            (driver / 'tests').mkdir()
            shutil.copyfile(SOURCE / 'tests/test_pppoe_assets.py', driver / 'tests/test_pppoe_assets.py')
            shutil.copytree(SOURCE.parents[1] / 'hardening', root / 'deploy/hardening',
                            ignore=shutil.ignore_patterns('__pycache__', '.scratch'))
            (root / 'deploy/vpp').mkdir()
            (root / 'deploy/vpp/verify.sh').write_text('#!/bin/sh\nexit 0\n')
            (root / 'deploy/vpp/verify.sh').chmod(0o755)
            (root / 'deploy/vpp/apply-startup.sh').write_text('#!/bin/sh\nexit 0\n')
            for name in ['apply-executor.py', 'apply-executor.service', 'apply-executor.socket']:
                shutil.copyfile(SOURCE.parents[1] / 'vpp' / name, root / 'deploy/vpp' / name)
            shutil.copytree(SOURCE.parents[1] / 'upgrade', root / 'deploy/upgrade', ignore=shutil.ignore_patterns('__pycache__'))
            shutil.copytree(SOURCE.parents[1] / 'support-bundle', root / 'deploy/support-bundle',
                            ignore=shutil.ignore_patterns('__pycache__'))
            (root / 'scripts').mkdir()
            shutil.copyfile(SOURCE.parents[2] / 'scripts/pppoe-kernel-carrier.py', root / 'scripts/pppoe-kernel-carrier.py')
            shutil.copytree(SOURCE.parents[2] / 'scripts/pppoe-carrier-assets', root / 'scripts/pppoe-carrier-assets')
            (root / 'deploy/systemd').mkdir()
            for name in ['agent', 'api', 'firstboot', 'firewall-bootstrap']:
                (root / f'deploy/systemd/ngfw-{name}.service').write_text('[Unit]\n')
            shutil.copyfile(SOURCE.parents[1] / 'systemd/ngfw-agent.service', root / 'deploy/systemd/ngfw-agent.service')
            for name in ['ngfw-ra@.service', 'ngfw-ra-openfile.service', 'ngfw-ra-openfile.socket', 'ngfw-ra-targets@.service', 'ngfw-ra-targets@.socket', 'ngfw-ra-observer@.service', 'ngfw-ra-observer@.socket', 'ngfw-ra-namespace-broker.socket', 'ngfw-ra-namespace-broker@.service']:
                shutil.copyfile(SOURCE.parents[1] / 'systemd' / name, root / 'deploy/systemd' / name)
            for directory in ['apps/agent', 'apps/api/dist', 'apps/api/migrations', 'apps/web/dist', 'artifacts', 'stub']:
                (root / directory).mkdir(parents=True, exist_ok=True)
            (root / 'apps/api/dist/main.js').write_text('stale API')
            (root / 'apps/web/dist/index.html').write_text('stale web')
            (root / 'artifacts/manifest.json').write_text(json.dumps({'version': '26.06-release+ngfw3'}))
            (root / 'source.txt').write_text('reviewed source')
            (root / '.gitignore').write_text('apps/*/dist/\nartifacts/\noutput/\ncommands.log\ngo-commands.jsonl\n')
            commands = {
                'go': '''#!/bin/bash
set -eu
python3 -c 'import json, os, sys; open(os.environ["FIXTURE_ROOT"] + "/go-commands.jsonl", "a").write(json.dumps({"args": sys.argv[1:], "cgo": os.environ.get("CGO_ENABLED")}) + "\\n")' "$@"
while [[ "$1" != -o ]]; do shift; done
printf binary > "$2"
chmod 755 "$2"
''',
                'pnpm': '''#!/bin/bash
set -eu
printf '%s\\n' "$*" >> "$FIXTURE_ROOT/commands.log"
if [[ "$1" == exec ]]; then
  [[ ! -e "$FIXTURE_ROOT/output" ]] || exit 90
  [[ "$FIXTURE_MODE" != failure ]] || exit 43
  printf 'fresh API' > "$FIXTURE_ROOT/apps/api/dist/main.js"
  printf 'fresh web' > "$FIXTURE_ROOT/apps/web/dist/index.html"
  if [[ "$FIXTURE_MODE" == dirty ]]; then printf changed > "$FIXTURE_ROOT/source.txt"; fi
else
  destination="${@: -1}"
  mkdir -p "$destination/dist"
  cp "$FIXTURE_ROOT/apps/api/dist/main.js" "$destination/dist/main.js"
fi
''',
            }
            for command in ['systemctl', 'systemd-tmpfiles', 'ip', 'nft', 'pppd', 'nsenter']:
                commands[command] = '#!/bin/sh\nprintf forbidden-host-action >&2\nexit 99\n'
            for name, content in commands.items():
                (root / 'stub' / name).write_text(content)
                (root / 'stub' / name).chmod(0o755)
            for args in [['init', '-q'], ['add', '.'], ['-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid', 'commit', '-qm', 'reviewed source']]:
                subprocess.run(['git', '-C', str(root), *args], check=True, capture_output=True)
            env = {**os.environ, 'PATH': str(root / 'stub') + os.pathsep + os.environ['PATH'],
                   'FIXTURE_ROOT': str(root), 'FIXTURE_MODE': mode}
            result = subprocess.run(['bash', str(driver / 'prepare.sh'), str(root / 'artifacts'), str(root / 'output')],
                                    env=env, capture_output=True, text=True)
            if mode == 'success':
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual((root / 'output/stage/usr/lib/ngfw/api/dist/main.js').read_text(), 'fresh API')
                self.assertEqual((root / 'output/stage/usr/share/ngfw/web/index.html').read_text(), 'fresh web')
                for name in ['service', 'socket']:
                    self.assertTrue((root / f'output/stage/usr/lib/systemd/system/apply-executor.{name}').is_file())
                self.assertTrue((root / 'output/stage/usr/lib/ngfw/apply-executor.py').is_file())
                self.assertTrue((root / 'output/stage/usr/lib/ngfw/ngfw-ra-daemon').is_file())
                self.assertEqual((root / 'output/stage/usr/lib/ngfw/ngfw-ra-daemon.sha256').read_text().strip(), hashlib.sha256(b'binary').hexdigest())
                self.assertEqual((root / 'output/stage/usr/lib/systemd/system/ngfw-ra@.service').read_bytes(), (root / 'deploy/systemd/ngfw-ra@.service').read_bytes())
                carrier_files = {
                    'scripts/pppoe-kernel-carrier.py': ('usr/lib/ngfw/pppoe-carrier.py', 0o644),
                    **{f'scripts/pppoe-carrier-assets/{name}': (f'usr/lib/systemd/system/{name}', 0o644)
                       for name in ['ngfw-pppoe-carrier@.service', 'ngfw-pppoe-broker@.service']},
                    **{f'scripts/pppoe-carrier-assets/{name}': (f'usr/lib/ngfw/pppoe-carrier-hooks/{name}', 0o755)
                       for name in ['ip-up', 'ip-down', 'ipv6-up', 'ipv6-down']},
                    'scripts/pppoe-carrier-assets/ngfw-pppoe-carrier.conf': ('usr/lib/tmpfiles.d/ngfw-pppoe-carrier.conf', 0o644),
                }
                for source, (destination, mode_bits) in carrier_files.items():
                    staged = root / 'output/stage' / destination
                    self.assertEqual(staged.read_bytes(), (root / source).read_bytes())
                    self.assertEqual(staged.stat().st_mode & 0o777, mode_bits)
                self.assertEqual((root / 'output/assets/provision-system-identity.py').read_bytes(),
                                 (driver / 'assets/provision-system-identity.py').read_bytes())
                hardening = root / 'output/stage/usr/lib/ngfw/hardening'
                self.assertTrue((hardening / 'stage.py').is_file())
                self.assertTrue((hardening / 'signing/verify.py').is_file())
                self.assertFalse(list(hardening.rglob('__pycache__')))
                self.assertFalse((root / 'output/stage/etc/sysctl.d/60-ngfw.conf').exists())
                self.assertTrue((root / 'output/stage/usr/sbin/ngfw-upgrade').is_file())
                for helper in ['prepare', 'health']:
                    self.assertTrue((root / f'output/stage/usr/lib/ngfw/ngfw-upgrade-{helper}').is_file())
                    self.assertTrue((root / f'output/stage/usr/lib/systemd/system/ngfw-upgrade-{helper}.service').is_file())
                self.assertTrue((root / 'output/stage/usr/lib/ngfw/ngfw-upgrade-probe').is_file())
                install_manifest = {
                    tuple(line.split()) for line in (driver / 'debian/ngfw-agent.install').read_text().splitlines()
                    if line.strip() and not line.lstrip().startswith('#')
                }
                go_commands = [json.loads(line) for line in (root / 'go-commands.jsonl').read_text().splitlines()]
                self.assertEqual(len(go_commands), 7)
                for command in go_commands:
                    args = command['args']
                    self.assertEqual(command['cgo'], '0')
                    self.assertEqual(args[:2], ['build', '-trimpath'])
                    if args[-1] in ['./cmd/ngfw-ra-daemon', './cmd/ngfw-ra-namespace-broker', './cmd/ngfw-wan-probe']:
                        self.assertEqual(args[2:5], ['-ldflags=-s -w', '-o', str(root / 'output/stage/usr/lib/ngfw' / args[-1].split('/')[-1])])
                    else:
                        self.assertEqual(args[2], '-o')
                        self.assertFalse(any(arg.startswith('-ldflags') for arg in args))
                for helper in ['ngfw-ra-daemon', 'ngfw-ra-namespace-broker', 'ngfw-wan-probe']:
                    staged = root / 'output/stage/usr/lib/ngfw' / helper
                    self.assertEqual(staged.read_bytes(), b'binary')
                    self.assertEqual(staged.stat().st_mode & 0o777, 0o755)
                    digest = root / 'output/stage/usr/lib/ngfw' / (helper + '.sha256')
                    self.assertEqual(digest.read_text(), hashlib.sha256(staged.read_bytes()).hexdigest() + '\n')
                    self.assertIn(('stage/usr/lib/ngfw/' + helper, 'usr/lib/ngfw/'), install_manifest)
                    self.assertIn(('stage/usr/lib/ngfw/' + helper + '.sha256', 'usr/lib/ngfw/'), install_manifest)
                for name in ['ngfw-ra@.service', 'ngfw-ra-openfile.service', 'ngfw-ra-openfile.socket', 'ngfw-ra-targets@.service', 'ngfw-ra-targets@.socket', 'ngfw-ra-observer@.service', 'ngfw-ra-observer@.socket', 'ngfw-ra-namespace-broker.socket', 'ngfw-ra-namespace-broker@.service']:
                    staged = root / 'output/stage/usr/lib/systemd/system' / name
                    self.assertEqual(staged.read_bytes(), (root / 'deploy/systemd' / name).read_bytes())
                    self.assertEqual(staged.stat().st_mode & 0o777, 0o644)
                    self.assertIn(('stage/usr/lib/systemd/system/' + name, 'usr/lib/systemd/system/'), install_manifest)
                agent_control = (driver / 'debian/control').read_text().split('Package: ngfw-agent\n', 1)[1].split('\nPackage:', 1)[0]
                self.assertIn('lsb-release', agent_control.split('Depends:', 1)[1].split('\n', 1)[0])
                dependencies = agent_control.split('Depends:', 1)[1].split('\n', 1)[0]
                for dependency in ['python3', 'iproute2', 'nftables', 'util-linux', 'procps', 'ppp', 'debianutils', 'systemd']:
                    self.assertIn(dependency, [item.strip() for item in dependencies.split(',')])
                for source, (destination, _) in carrier_files.items():
                    installed_source = 'stage/' + destination
                    if '/pppoe-carrier-hooks/' in destination:
                        installed_source = 'stage/usr/lib/ngfw/pppoe-carrier-hooks/*'
                    self.assertIn((installed_source, str(pathlib.PurePosixPath(destination).parent) + '/'), install_manifest)
                for helper in ['ngfw-support-collect', 'ngfw-upgrade-dispatch']:
                    staged = root / f'output/stage/usr/lib/ngfw/{helper}'
                    self.assertEqual(staged.read_bytes(), (root / f'deploy/support-bundle/{helper}').read_bytes())
                    self.assertEqual(staged.stat().st_mode & 0o777, 0o755)
                    self.assertIn((f'stage/usr/lib/ngfw/{helper}', 'usr/lib/ngfw/'), install_manifest)
                staged_unit = root / 'output/stage/usr/lib/systemd/system/ngfw-upgrade@.service'
                self.assertEqual(staged_unit.read_bytes(), (root / 'deploy/support-bundle/ngfw-upgrade@.service').read_bytes())
                self.assertEqual(staged_unit.stat().st_mode & 0o777, 0o644)
                self.assertIn(('stage/usr/lib/systemd/system/ngfw-upgrade@.service', 'usr/lib/systemd/system/'), install_manifest)
                packaged_checks = subprocess.run(['python3', str(root / 'output/tests/test_pppoe_assets.py')],
                                                 env=env, capture_output=True, text=True)
                self.assertEqual(packaged_checks.returncode, 0, packaged_checks.stderr)
                self.assertEqual((root / 'commands.log').read_text().splitlines()[0],
                                 'exec turbo run build --filter=@ngfw/api --filter=@ngfw/web --concurrency=2')
            else:
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse((root / 'output').exists())
                self.assertEqual(len((root / 'commands.log').read_text().splitlines()), 1)
                if mode == 'dirty':
                    self.assertIn('build changed tracked source', result.stderr)

    def test_stale_dist_rebuilt_before_staging(self):
        self.run_prepare('success')

    def test_failed_build_does_not_stage_stale_dist(self):
        self.run_prepare('failure')

    def test_build_mutation_refuses_release_staging(self):
        self.run_prepare('dirty')


if __name__ == '__main__':
    unittest.main()
