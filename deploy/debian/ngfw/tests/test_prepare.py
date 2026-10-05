#!/usr/bin/env python3
"""Run the actual staging driver with private source and harmless build tools."""
import json
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
            (driver / 'assets').mkdir()
            (driver / 'tests').mkdir()
            shutil.copytree(SOURCE.parents[1] / 'hardening', root / 'deploy/hardening',
                            ignore=shutil.ignore_patterns('__pycache__', '.scratch'))
            (root / 'deploy/vpp').mkdir()
            (root / 'deploy/vpp/verify.sh').write_text('#!/bin/sh\nexit 0\n')
            (root / 'deploy/vpp/verify.sh').chmod(0o755)
            (root / 'deploy/vpp/apply-startup.sh').write_text('#!/bin/sh\nexit 0\n')
            for name in ['apply-executor.py', 'apply-executor.service', 'apply-executor.socket']:
                shutil.copyfile(SOURCE.parents[1] / 'vpp' / name, root / 'deploy/vpp' / name)
            shutil.copytree(SOURCE.parents[1] / 'upgrade', root / 'deploy/upgrade', ignore=shutil.ignore_patterns('__pycache__'))
            (root / 'deploy/systemd').mkdir()
            for name in ['agent', 'api', 'firstboot', 'firewall-bootstrap']:
                (root / f'deploy/systemd/ngfw-{name}.service').write_text('[Unit]\n')
            for directory in ['apps/agent', 'apps/api/dist', 'apps/api/migrations', 'apps/web/dist', 'artifacts', 'stub']:
                (root / directory).mkdir(parents=True, exist_ok=True)
            (root / 'apps/api/dist/main.js').write_text('stale API')
            (root / 'apps/web/dist/index.html').write_text('stale web')
            (root / 'artifacts/manifest.json').write_text(json.dumps({'version': '26.06-release+ngfw3'}))
            (root / 'source.txt').write_text('reviewed source')
            (root / '.gitignore').write_text('apps/*/dist/\nartifacts/\noutput/\ncommands.log\n')
            commands = {
                'go': '''#!/bin/bash
set -eu
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
