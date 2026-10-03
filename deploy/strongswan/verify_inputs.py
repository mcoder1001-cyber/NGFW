#!/usr/bin/env python3
"""Read-only P11 build intake; no download, extraction, compilation or installation."""
import argparse
import bz2
import contextlib
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import stat
import subprocess
import sys
import tarfile
import tempfile

ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location('p11_bundle_install', ROOT / 'deploy/debian/bundle/install.py')
INSTALL = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(INSTALL)
VERIFY = INSTALL.VERIFY
InvalidInputs = VERIFY.InvalidBundle
REQUIRED = ('vpp-dev', 'libvppinfra-dev', 'vpp', 'libvppinfra')
MAX_SOURCE = 128 * 1024 * 1024
MAX_EXPANDED = 512 * 1024 * 1024


def source_snapshot(source, destination, digest=None, limit=MAX_SOURCE, dir_fd=None):
    if digest is not None and not re.fullmatch(r'[0-9a-f]{64}', digest):
        raise InvalidInputs('expected source SHA-256 must be 64 lowercase hex characters')
    fd = os.open(source, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK, dir_fd=dir_fd)
    try:
        before = os.fstat(fd)
        if not stat.S_ISREG(before.st_mode) or before.st_size > limit:
            raise InvalidInputs('source must be a regular bounded release archive')
        sha = hashlib.sha256()
        count = 0
        with destination.open('xb') as output:
            os.chmod(destination, 0o600)
            while True:
                chunk = os.read(fd, 1024 * 1024)
                if not chunk:
                    break
                count += len(chunk)
                if count > limit:
                    raise InvalidInputs('source archive exceeds bound')
                sha.update(chunk)
                output.write(chunk)
        if INSTALL.identity(before) != INSTALL.identity(os.fstat(fd)) or INSTALL.identity(
                before) != INSTALL.identity(os.stat(source, dir_fd=dir_fd, follow_symlinks=False)):
            raise InvalidInputs('source changed during snapshot')
        if digest is not None and sha.hexdigest() != digest:
            raise InvalidInputs('source archive differs from separately trusted digest')
        return {'sha256': sha.hexdigest(), 'size': count}
    finally:
        os.close(fd)


def release_contents(path, version):
    prefix = 'strongswan-' + version
    required = {prefix + '/configure', prefix + '/src/libstrongswan/settings/settings_parser.c'}
    # Bound actual decompressed bytes, including hidden tar metadata and padding,
    # before tarfile parses extended headers or advertised member sizes.
    with tempfile.TemporaryFile(dir='/var/tmp') as raw:
        expanded = 0
        with bz2.open(path, 'rb') as compressed:
            while True:
                chunk = compressed.read(1024 * 1024)
                if not chunk:
                    break
                expanded += len(chunk)
                if expanded > MAX_EXPANDED:
                    raise InvalidInputs('decompressed source archive exceeds bound')
                raw.write(chunk)
        raw.seek(0)
        _release_members(raw, prefix, required)


def _release_members(raw, prefix, required):
    seen = set()
    total = 0
    with tarfile.open(fileobj=raw, mode='r:') as archive:
        for index, member in enumerate(archive):
            name = member.name.rstrip('/')
            parts = name.split('/')
            if (index >= 20000 or len(name) > 1024 or name in seen or parts[0] != prefix
                    or any(part in ('', '.', '..') for part in parts)
                    or not (member.isfile() or member.isdir())):
                raise InvalidInputs('unsupported or unsafe source archive member')
            seen.add(name)
            total += member.size
            if member.size < 0 or total > MAX_EXPANDED:
                raise InvalidInputs('expanded source archive exceeds bound')
            if name in required:
                if not member.isfile() or member.size == 0:
                    raise InvalidInputs('release configure/parser must be nonempty regular files')
                if name.endswith('/configure') and not member.mode & 0o111:
                    raise InvalidInputs('release configure must be executable')
                required.remove(name)
    if required:
        raise InvalidInputs('release archive lacks pregenerated configure/parser')


@contextlib.contextmanager
def verified_snapshot(vpp_output, source, version, digest):
    """Yield the report and exact checked private paths, valid only within this scope."""
    if not isinstance(digest, str) or not re.fullmatch(r'[0-9a-f]{64}', digest):
        raise InvalidInputs('separately trusted source SHA-256 is required')
    # Keep the historical tested version; this is intake, never release approval.
    if version != '5.9.6':
        raise InvalidInputs('this intake profile requires historical strongSwan 5.9.6')
    with INSTALL.safe_environment(), tempfile.TemporaryDirectory(prefix='vrx-p11-', dir='/var/tmp') as temporary:
        private = Path(temporary)
        delivery = private / 'delivery'
        delivery.mkdir(mode=0o700)
        snapshot = delivery / 'vpp'
        snapshot.mkdir(mode=0o700)
        INSTALL.copy_tree(vpp_output, snapshot)
        # The shared delivery copier only keeps vpp/manifest.json and
        # vpp/SHA256SUMS relative to a delivery root; intake starts at VPP output.
        # Pin this output directory while copying those two bounded metadata files.
        directory_fd = os.open(vpp_output, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
        try:
            directory_before = os.fstat(directory_fd)
            for name in ('manifest.json', 'SHA256SUMS'):
                source_snapshot(name, snapshot / name, limit=VERIFY.MAX_MANIFEST, dir_fd=directory_fd)
            if INSTALL.identity(directory_before) != INSTALL.identity(os.fstat(directory_fd)):
                raise InvalidInputs('VPP output changed during metadata snapshot')
        finally:
            os.close(directory_fd)
        before = VERIFY.manifest_bytes(snapshot / 'manifest.json')
        VERIFY.run(['bash', str(ROOT / 'deploy/vpp/verify.sh'), '--require-files',
                    str(snapshot), '--install-gate'])
        if before != VERIFY.manifest_bytes(snapshot / 'manifest.json'):
            raise InvalidInputs('VPP manifest changed during verification')
        manifest = json.loads(before)
        selected = {}
        for entry in manifest['packages']:
            if entry['package'] not in REQUIRED:
                continue
            name = entry['package']
            if name in selected or not re.fullmatch(r'[A-Za-z0-9_.+-]+\.deb', entry['file']):
                raise InvalidInputs('duplicate staging package or unsafe filename')
            actual = VERIFY.metadata(snapshot / entry['file'])
            fields = actual['fields']
            if (fields['Package'], fields['Version'], fields['Architecture'], actual['sha256'], actual['size']) != (
                    name, manifest['version'], 'amd64', entry['sha256'], entry['size']):
                raise InvalidInputs('staging archive metadata/hash differs from verified build')
            if entry['version'] != manifest['version'] or entry['architecture'] != 'amd64':
                raise InvalidInputs('staging entry version/architecture differs from build')
            selected[name] = {'file': entry['file'], 'version': fields['Version'],
                              'sha256': actual['sha256'], 'size': actual['size']}
        if set(selected) != set(REQUIRED):
            raise InvalidInputs('complete VPP dev/runtime staging quartet required')
        tarball = private / 'source.tar.bz2'
        source_report = source_snapshot(source, tarball, digest, limit=MAX_SOURCE)
        release_contents(tarball, version)
        report = {'format': 1, 'mode': 'read-only-build-intake', 'vpp_version': manifest['version'],
                'staging_packages': selected, 'source': {'version': version,
                'expected_origin': 'https://download.strongswan.org/strongswan-' + version + '.tar.bz2',
                **source_report}, 'release_approved': False}
        yield report, snapshot, tarball


def verify(vpp_output, source, version, digest):
    with verified_snapshot(vpp_output, source, version, digest) as (report, _, __):
        return report


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--vpp-output', required=True, type=Path)
    parser.add_argument('--source', required=True, type=Path)
    parser.add_argument('--source-version', required=True)
    parser.add_argument('--source-sha256', required=True, help='expected digest obtained through a trusted channel')
    args = parser.parse_args(argv)
    try:
        print(json.dumps(verify(args.vpp_output, args.source, args.source_version,
                                args.source_sha256), sort_keys=True, indent=2))
        return 0
    except (InvalidInputs, OSError, EOFError, ValueError, KeyError, TypeError, tarfile.TarError, subprocess.SubprocessError) as error:
        print(f'P11 input verification failed: {error}', file=sys.stderr)
        return 1


if __name__ == '__main__':
    sys.exit(main())
