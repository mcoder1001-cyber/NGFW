#!/usr/bin/env python3
"""Export byte-identical canonical recipient helpers from a trusted checkout.

The printed inventory must be authenticated separately; this is not signing.
"""
import argparse
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tarfile

SPEC = importlib.util.spec_from_file_location('bundle_export', Path(__file__).with_name('export.py'))
EXPORT = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(EXPORT)
ROOT = Path(__file__).resolve().parents[3]
FIXED = (
    'deploy/debian/bundle/install.py', 'deploy/debian/bundle/verify.py',
    'scripts/10-install-runtime.sh', 'deploy/vpp/verify.sh', 'deploy/vpp/lib.sh',
    'deploy/vpp/build.sh', 'deploy/vpp/tests/run.sh', 'deploy/vpp/VERSION',
    'deploy/vpp/pydeps.lock', 'deploy/vpp/patches/series', 'deploy/vpp/build-patches/series',
)
LAUNCHER = 'deploy/debian/bundle/recipient.py'
MAX_HELPER = 1024 * 1024
MAX_TOTAL = 8 * 1024 * 1024
MAX_FILES = 128


def source_files():
    # Shell verifier reads both series and all optional patches; include the
    # exact canonical files, not a reconstructed copy of their interpretation.
    names = set(FIXED)
    for directory in ('patches', 'build-patches'):
        for path in (ROOT / 'deploy/vpp' / directory).rglob('*.patch'):
            names.add(path.relative_to(ROOT).as_posix())
    return sorted(names)


def canonical(name, commit):
    path = ROOT / name
    if path.is_symlink():
        raise EXPORT.InvalidBundle('canonical helper must not be a symlink')
    data = path.read_bytes()
    if len(data) > MAX_HELPER:
        raise EXPORT.InvalidBundle('canonical helper exceeds bound')
    recorded = subprocess.run(['git', '-C', str(ROOT), 'show', commit + ':' + name],
                              check=True, stdout=subprocess.PIPE).stdout
    if data != recorded:
        raise EXPORT.InvalidBundle('helper differs from committed source: ' + name)
    mode = subprocess.run(['git', '-C', str(ROOT), 'ls-tree', commit, '--', name],
                          check=True, capture_output=True, text=True).stdout.split()[0]
    if mode not in ('100644', '100755'):
        raise EXPORT.InvalidBundle('unsupported canonical helper mode')
    return data, 0o700 if mode == '100755' else 0o600


def export_helpers(output):
    commit = subprocess.run(['git', '-C', str(ROOT), 'rev-parse', 'HEAD'],
                            check=True, capture_output=True, text=True).stdout.strip()
    names = source_files()
    if len(names) > MAX_FILES:
        raise EXPORT.InvalidBundle('helper inventory exceeds bound')
    files = {name: canonical(name, commit) for name in names}
    launcher, _ = canonical(LAUNCHER, commit)
    if sum(len(data) for data, _ in files.values()) > MAX_TOTAL:
        raise EXPORT.InvalidBundle('helper payload exceeds bound')
    # Reuse the transport exporter's pinned destination and no-replace policy.
    output = EXPORT.destination(ROOT, ROOT / LAUNCHER, output)
    parent = EXPORT.directory_fd(output.parent)
    temporary = '.ngfw-helpers-' + EXPORT.secrets.token_hex(16)
    created = False
    published = False
    try:
        info = os.fstat(parent)
        if info.st_uid != os.geteuid() or info.st_mode & 0o022:
            raise EXPORT.InvalidBundle('output directory must be caller-owned and not writable by others')
        fd = os.open(temporary, os.O_CREAT | os.O_EXCL | os.O_RDWR | os.O_NOFOLLOW, 0o600, dir_fd=parent)
        created = True
        with os.fdopen(fd, 'w+b') as stream:
            with tarfile.open(fileobj=stream, mode='w|', format=tarfile.USTAR_FORMAT) as archive:
                for name, (data, mode) in files.items():
                    member = tarfile.TarInfo(name)
                    member.size = len(data)
                    member.mode = mode
                    archive.addfile(member, io.BytesIO(data))
            stream.flush()
            os.fsync(stream.fileno())
            size = stream.tell()
            stream.seek(0)
            digest = hashlib.file_digest(stream, 'sha256').hexdigest()
        current = EXPORT.directory_fd(output.parent)
        try:
            if EXPORT.INSTALL.identity(os.fstat(current))[:2] != EXPORT.INSTALL.identity(info)[:2]:
                raise EXPORT.InvalidBundle('helper destination changed')
        finally:
            os.close(current)
        os.link(temporary, output.name, src_dir_fd=parent, dst_dir_fd=parent, follow_symlinks=False)
        published = True
        current = EXPORT.directory_fd(output.parent)
        try:
            if EXPORT.INSTALL.identity(os.fstat(current))[:2] != EXPORT.INSTALL.identity(info)[:2]:
                raise EXPORT.InvalidBundle('helper destination changed during publication')
        finally:
            os.close(current)
        os.fsync(parent)
    except BaseException:
        if published:
            os.unlink(output.name, dir_fd=parent)
        raise
    finally:
        if created:
            os.unlink(temporary, dir_fd=parent)
        os.close(parent)
    return {'schema': 'ngfw.recipient-helpers/v1', 'source_commit': commit,
            'archive_bytes': size, 'sha256': digest,
            'launcher': {'size': len(launcher), 'sha256': hashlib.sha256(launcher).hexdigest()},
            'files': {name: {'size': len(data), 'sha256': hashlib.sha256(data).hexdigest(), 'mode': mode}
                      for name, (data, mode) in files.items()}}


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args(argv)
    try:
        print(json.dumps(export_helpers(args.output), sort_keys=True, indent=2))
        return 0
    except (OSError, ValueError, subprocess.SubprocessError, tarfile.TarError) as error:
        print(f'helper export failed: {error}', file=sys.stderr)
        return 1


if __name__ == '__main__':
    sys.exit(main())
