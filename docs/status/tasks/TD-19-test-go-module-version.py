#!/usr/bin/env python3
"""Actual script preflight in temporary repository shapes; no host mutations."""
import os
from pathlib import Path
import shutil
import shlex
import subprocess
import tempfile
import unittest

ROOT=Path(__file__).resolve().parents[3]


class ModuleVersion(unittest.TestCase):
    def invoke(self,module,args=(),foreign_cwd=False,symlink=False,reader_failure=False):
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory);repo=root/'repo';(repo/'scripts').mkdir(parents=True)
            target=repo/'scripts/20-install-build.sh';shutil.copyfile(ROOT/'scripts/20-install-build.sh',target)
            for helper in ('install-common.sh', 'install-recording-stub.py'):
                shutil.copyfile(ROOT/'scripts'/helper,repo/'scripts'/helper)
            parent=repo/'apps/agent';parent.mkdir(parents=True)
            if module is not None:
                path=parent/'go.mod'
                if symlink:
                    alternate=root/'alternate';alternate.write_text(module);path.symlink_to(alternate)
                else:path.write_text(module)
            fake=root/'bin';fake.mkdir();log=root/'calls'
            for name in ('apt-get','curl','go','tar','rm','corepack','npm'):
                stub=fake/name;stub.write_text('#!/bin/sh\nprintf "%s\\n" "$0" >> "$CALL_LOG"\nexit 91\n');stub.chmod(0o755)
            if reader_failure:
                stub=fake/'awk';stub.write_text('#!/bin/sh\necho "fixture module read failed" >&2\nexit 2\n');stub.chmod(0o755)
                # Trusted absolute readers intentionally ignore PATH. Fault-inject
                # this one subprocess in the disposable entry, retaining the
                # actual AWK program, failure branch and canonical module input.
                source=target.read_text();needle='GO_VER=$(/usr/bin/awk '
                self.assertEqual(source.count(needle),1)
                target.write_text(source.replace(needle,'GO_VER=$('+shlex.quote(str(stub))+' ',1))
            cwd=root/'elsewhere';(cwd/'apps/agent').mkdir(parents=True)
            (cwd/'apps/agent/go.mod').write_text('go 9.99.9\n')
            env=dict(os.environ,PATH=f'{fake}:/usr/bin:/bin',CALL_LOG=str(log),GO_MODULE=str(cwd/'apps/agent/go.mod'),GO_VER='9.99.9')
            env.pop('NGFW_GO_SHA256',None)
            result=subprocess.run(['bash',str(target),*args],cwd=cwd if foreign_cwd else repo,env=env,text=True,capture_output=True)
            return result,log.read_text() if log.exists() else ''

    def test_current_minor_and_explicit_patch_use_original_pin(self):
        for version in ('1.26','1.26.0'):
            with self.subTest(version=version):
                result,calls=self.invoke(f'module fixture\n\ngo {version}\n',('--check-config',))
                self.assertEqual(result.returncode,0,result.stderr);self.assertEqual(calls,'')
                self.assertEqual(result.stdout.strip(),'Go=1.26.0 protoc-gen-go=v1.36.12 protoc-gen-go-grpc=v1.6.2 govpp=v0.13.0')

    def test_unsupported_versions_refuse_before_any_mutation(self):
        for version in ('1.26.1','1.27','2.0','1.25.9'):
            with self.subTest(version=version):
                result,calls=self.invoke(f'go {version}\n')
                self.assertNotEqual(result.returncode,0);self.assertIn('no reviewed official',result.stderr);self.assertEqual(calls,'')

    def test_missing_duplicate_malformed_and_trailing_directives_refuse(self):
        for module in (None,'module fixture\n','go 1.26\ngo 1.26\n','go 1.26\n go 1.26.0\n',
                       'go\n','go 1.026\n','go 1.26.00\n','go 1.26 // ambiguous trailing\n',
                       'go 1.26 malicious\n','go $(touch unwanted)\n'):
            with self.subTest(module=module):
                result,calls=self.invoke(module)
                self.assertNotEqual(result.returncode,0);self.assertIn('REFUSED:',result.stderr);self.assertEqual(calls,'')

    def test_script_location_ignores_cwd_and_alternate_environment(self):
        result,calls=self.invoke('go 1.26\n',('--check-config',),foreign_cwd=True)
        self.assertEqual(result.returncode,0,result.stderr);self.assertIn('Go=1.26.0',result.stdout);self.assertEqual(calls,'')

    def test_nonregular_symlink_canonical_module_refused(self):
        result,calls=self.invoke('go 1.26\n',symlink=True)
        self.assertNotEqual(result.returncode,0);self.assertIn('regular file',result.stderr);self.assertEqual(calls,'')
    def test_module_reader_failure_refuses_before_mutation(self):
        # Model failed actual AWK read; chmod alone cannot model unreadability
        # when the fixture process happens to run with a root UID.
        result,calls=self.invoke('go 1.26\n',reader_failure=True)
        self.assertNotEqual(result.returncode,0);self.assertIn('fixture module read failed',result.stderr)
        self.assertIn('exactly one valid go directive',result.stderr);self.assertEqual(calls,'')


if __name__=='__main__':unittest.main(verbosity=2)
