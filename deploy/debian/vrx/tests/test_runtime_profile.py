#!/usr/bin/env python3
"""Runtime installer preflight refusal; never call package management."""
import os
import pathlib
import subprocess
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[4]


class RuntimeProfile(unittest.TestCase):
    def test_explicit_appliance_and_artifact_inputs_required_before_apt(self):
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            command = root / 'apt-get'
            command.write_text('#!/bin/sh\ntouch "$APT_FIXTURE_CALLED"\nexit 99\n')
            command.chmod(0o755)
            environment = dict(os.environ, PATH=str(root) + ':' + os.environ['PATH'],
                               APT_FIXTURE_CALLED=str(root / 'called'))
            environment.pop('VRX_VPP_ARTIFACTS', None)
            for explicit in ['0', '1']:
                environment['VRX_INSTALL_APPLIANCE'] = explicit
                result = subprocess.run(['bash', str(ROOT / 'scripts/10-install-runtime.sh')],
                                        env=environment, capture_output=True)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse((root / 'called').exists())


if __name__ == '__main__':
    unittest.main()
