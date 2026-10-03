#!/usr/bin/env python3
"""Check producer v2 package contract against the ISO repository, without installing."""
import hashlib
import json
from pathlib import Path
import re
import shlex
import subprocess
import sys


def require(condition, message):
    if not condition:
        raise ValueError(message)


def verify(manifest, repo, version, contract):
    values = {}
    for line in Path(contract).read_text().splitlines():
        if line.startswith(('VPP_PACKAGES=', 'VPP_PACKAGES_SHIP=')):
            key, value = line.split('=', 1)
            require(key not in values, 'duplicate contract key')
            tokens = shlex.split(value)
            require(len(tokens) == 1, 'invalid package contract')
            values[key] = tokens[0].split()
    expected = set(values['VPP_PACKAGES'])
    shipped = set(values['VPP_PACKAGES_SHIP'])
    require('vpp' in shipped and shipped <= expected, 'invalid shipped package contract')
    m = json.loads(Path(manifest).read_text())
    require(m['schema'] == 'ngfw.vpp-debs.manifest/v2', 'unsupported manifest schema')
    require(m['version'] == version, 'manifest/repository version mismatch')
    packages = m['packages']
    require(isinstance(packages, list), 'packages must be an array')
    names = [p['package'] for p in packages]
    require(len(names) == len(set(names)) and set(names) == expected,
            'manifest package set differs from VERSION VPP_PACKAGES')
    files = {}
    for p in packages:
        filename = p['file']
        require(isinstance(filename, str) and
                re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_.+~%-]*\.deb', filename),
                'unsafe package filename')
        require(filename not in files, 'duplicate package filename')
        files[filename] = p
        require(p['version'] == version, 'package version mismatch')
        require(type(p['ship']) is bool and p['ship'] == (p['package'] in shipped),
                'ship flag differs from VERSION VPP_PACKAGES_SHIP')
        require(isinstance(p['sha256'], str) and
                re.fullmatch(r'[0-9a-f]{64}', p['sha256']), 'invalid package sha256')
    root = (Path(repo) / 'pool').resolve(strict=True)
    found = set()
    for path in root.rglob('*.deb'):
        require(path.resolve(strict=True).is_relative_to(root), 'package escapes repository pool')
        result = subprocess.run(['dpkg-deb', '-f', str(path), 'Package', 'Version', 'Architecture'],
                                check=True, capture_output=True, text=True)
        fields = dict(line.split(': ', 1) for line in result.stdout.splitlines())
        name = fields['Package']
        if name not in expected:
            require(path.name not in files, 'manifest filename has unrelated package identity')
            continue
        require(name in shipped, 'non-shipped VPP package is in repository')
        require(name not in found, 'duplicate repository VPP package')
        found.add(name)
        entry = next(p for p in packages if p['package'] == name)
        require(path.name == entry['file'], 'repository filename differs from manifest')
        require(fields['Version'] == entry['version'] and
                fields['Architecture'] == entry['architecture'], 'Debian control identity mismatch')
        with path.open('rb') as artifact:
            digest = hashlib.file_digest(artifact, 'sha256').hexdigest()
        require(digest == entry['sha256'], 'repository package sha256 mismatch')
        print('ok', name, version, digest)
    require(found == shipped, 'repository missing shipped VPP packages')
    print('VPP %s: %d shipped packages match the manifest; upstream commit %s' %
          (version, len(found), m['upstream']['commit']))


if __name__ == '__main__':
    try:
        verify(*sys.argv[1:])
    except (ValueError, KeyError, TypeError, OSError, subprocess.CalledProcessError) as error:
        print('VPP manifest verification failed:', error, file=sys.stderr)
        sys.exit(1)
