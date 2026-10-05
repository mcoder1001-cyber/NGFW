#!/usr/bin/env python3
"""Package authenticated private engine for its explicit offline appliance ABI.

Never installs on the builder and never calls a daemon, ldconfig or systemctl.
"""
import importlib.util
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile

module = importlib.util.spec_from_file_location('ra_stage', Path(__file__).with_name('stage-engine.py'))
stage = importlib.util.module_from_spec(module)
module.loader.exec_module(stage)


def package(artifact, abi_root, output):
    output = Path(output).absolute()
    if output.exists() or output.is_symlink() or output.suffix != '.deb':
        raise stage.Refused('new .deb output required')
    prefix, _ = stage.validate(Path(artifact), Path(abi_root))
    build = (prefix / 'share/ngfw/engine-build.txt').read_text()
    expected = ('version=6.1.0\n'
                'source_sha256=fe6c97481298767213cfc2e9a1da29fdd8018d481ff4cb9cf0283099654f20d4\n'
                'release_fingerprint=948F158A4E76A27BF3D07532DF42C170B34DBA77\n')
    if build != expected:
        raise stage.Refused('pinned authenticated source receipt required')
    abi = json.loads((prefix / 'share/ngfw/engine-abi.json').read_text())
    dependencies = []
    for name in ('libc6', 'libssl3t64', 'libsystemd0'):
        version = abi['runtimePackages'][name]
        if not re.fullmatch(r'[0-9A-Za-z.+~:-]{1,96}', version):
            raise stage.Refused('bounded Debian dependency version required')
        dependencies.append(f'{name} (= {version})')
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='ngfw-ra-deb-', dir=output.parent) as temporary:
        root = Path(temporary)
        os.chmod(root, 0o700)
        (root / 'opt').mkdir()
        shutil.copytree(prefix, root / 'opt/ngfw-ra', symlinks=True)
        control = root / 'DEBIAN'
        control.mkdir()
        (control / 'control').write_text(
            'Package: ngfw-ra-engine\nVersion: 6.1.0+ngfw1\nArchitecture: amd64\n'
            'Maintainer: NGFW maintainers <maintainers@example.invalid>\n'
            'Section: net\nPriority: optional\nDepends: ' + ', '.join(dependencies) + '\n'
            'Description: NGFW isolated remote-access IKE engine\n'
            ' Private route-based strongSwan; starts only through the guarded NGFW helper.\n')
        subprocess.run(['dpkg-deb', '--root-owner-group', '--build', str(root), str(output)], check=True,
                       stdout=subprocess.DEVNULL, stderr=subprocess.PIPE)
    return output


if __name__ == '__main__':
    try:
        if len(sys.argv) != 4:
            raise stage.Refused('usage: package-engine.py ARTIFACT_ROOT OFFLINE_ABI_ROOT NEW_PACKAGE.deb')
        package(*sys.argv[1:])
    except (OSError, ValueError, KeyError, subprocess.CalledProcessError):
        print('RA engine packaging refused', file=sys.stderr)
        sys.exit(1)
