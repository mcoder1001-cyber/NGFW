#!/usr/bin/env python3
"""Export an already complete trusted delivery set; never build, fetch or install."""
import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import secrets
import stat
import subprocess
import sys
import tarfile

SPEC = importlib.util.spec_from_file_location('bundle_install', Path(__file__).with_name('install.py'))
INSTALL = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(INSTALL)
VERIFY = INSTALL.VERIFY
InvalidBundle = INSTALL.InvalidBundle


def directory_fd(path):
    """Pin each existing parent component; reject symlinked destination parents."""
    absolute = Path(os.path.abspath(path))
    descriptor = os.open('/', os.O_RDONLY | os.O_DIRECTORY)
    try:
        for name in absolute.parts[1:]:
            child = os.open(name, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW, dir_fd=descriptor)
            os.close(descriptor)
            descriptor = child
        return descriptor
    except BaseException:
        os.close(descriptor)
        raise


def destination(source, manifest, output):
    if '..' in output.parts or not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_.+-]*\.tar', output.name):
        raise InvalidBundle('output must have a safe .tar filename without traversal')
    source_root = source.resolve(strict=True)
    parent = output.parent.resolve(strict=True)
    result = parent / output.name
    if result.is_relative_to(source_root) or result == manifest.resolve(strict=True):
        raise InvalidBundle('output overlaps source or trusted manifest')
    # Secure traversal below additionally rejects symlinked parent spellings.
    return Path(os.path.abspath(output))


def member_name(path, snapshot):
    name = path.relative_to(snapshot).as_posix()
    if (len(name) > 240 or any(not re.fullmatch(r'[A-Za-z0-9_.+-]+', part)
                              or part in ('.', '..') for part in Path(name).parts)):
        raise InvalidBundle('unsafe tar member name')
    return name


def digest_file(path, limit):
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    try:
        before = os.fstat(fd)
        if not stat.S_ISREG(before.st_mode) or before.st_size > limit:
            raise InvalidBundle('snapshot file type or size exceeds bound')
        digest = hashlib.sha256()
        copied = 0
        while True:
            chunk = os.read(fd, 1024 * 1024)
            if not chunk:
                break
            copied += len(chunk)
            if copied > limit:
                raise InvalidBundle('snapshot grew beyond bound')
            digest.update(chunk)
        if INSTALL.identity(before) != INSTALL.identity(os.fstat(fd)):
            raise InvalidBundle('snapshot changed during hashing')
        return copied, digest.hexdigest()
    finally:
        os.close(fd)


def inventory(snapshot, plan):
    INSTALL.check_archives(snapshot, plan)
    expected = {item['file']: (item['size'], item['sha256']) for item in plan['artifacts']}
    vpp = json.loads(VERIFY.manifest_bytes(snapshot / 'vpp/manifest.json'))
    # The full existing VPP gate verified shipping AND nonshipping archives.
    # Retain them for re-verification; only plan.install_files may be installed.
    for item in vpp['packages']:
        name = 'vpp/' + item['file']
        if name in expected and expected[name] != (item['size'], item['sha256']):
            raise InvalidBundle('VPP and runtime manifests disagree')
        expected[name] = (item['size'], item['sha256'])
    for name in ('vpp/manifest.json', 'vpp/SHA256SUMS'):
        expected[name] = digest_file(snapshot / name, VERIFY.MAX_MANIFEST)
    actual = {}
    total = 0
    for path in snapshot.rglob('*'):
        if path.is_symlink():
            raise InvalidBundle('snapshot symlink is forbidden')
        if path.is_dir():
            continue
        name = member_name(path, snapshot)
        if name not in expected:
            raise InvalidBundle('unplanned file in private snapshot')
        actual[name] = digest_file(path, VERIFY.MAX_ARCHIVE if name.endswith('.deb') else VERIFY.MAX_MANIFEST)
        total += actual[name][0]
        if len(actual) > VERIFY.MAX_PACKAGES + 2 or total > VERIFY.MAX_TOTAL:
            raise InvalidBundle('export inventory exceeds bound')
    if actual != expected:
        raise InvalidBundle('private snapshot differs from verified export inventory')
    return actual


class CheckedReader:
    def __init__(self, stream):
        self.stream = stream
        self.digest = hashlib.sha256()
        self.count = 0

    def read(self, size):
        chunk = self.stream.read(size)
        self.digest.update(chunk)
        self.count += len(chunk)
        return chunk


def write_tar(stream, snapshot, files):
    with tarfile.open(fileobj=stream, mode='w|', format=tarfile.PAX_FORMAT) as archive:
        for name in sorted(files):
            path = snapshot / name
            fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
            with os.fdopen(fd, 'rb') as source:
                before = os.fstat(source.fileno())
                if not stat.S_ISREG(before.st_mode) or before.st_size != files[name][0]:
                    raise InvalidBundle('private snapshot changed before export')
                info = tarfile.TarInfo(name)
                info.size = before.st_size
                info.mode = 0o600
                info.mtime = 0
                info.uid = info.gid = 0
                info.uname = info.gname = ''
                checked = CheckedReader(source)
                archive.addfile(info, checked)
                if ((checked.count, checked.digest.hexdigest()) != files[name]
                        or INSTALL.identity(before) != INSTALL.identity(os.fstat(source.fileno()))):
                    raise InvalidBundle('private snapshot changed during export')


def export(source, manifest, output):
    output = destination(source, manifest, output)
    parent_fd = directory_fd(output.parent)
    parent_stat = os.fstat(parent_fd)
    if parent_stat.st_uid != os.geteuid() or parent_stat.st_mode & 0o022:
        os.close(parent_fd)
        raise InvalidBundle('output directory must be owned by caller and not group/other writable')
    temporary = '.vrx-export-' + secrets.token_hex(16)
    created = False
    published = False
    try:
        try:
            os.stat(output.name, dir_fd=parent_fd, follow_symlinks=False)
        except FileNotFoundError:
            pass
        else:
            raise InvalidBundle('output already exists; refusing overwrite')
        with INSTALL.prepared(source, manifest) as (_, snapshot, plan):
            files = inventory(snapshot, plan)
            descriptor = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW,
                                 0o600, dir_fd=parent_fd)
            created = True
            with os.fdopen(descriptor, 'wb') as stream:
                write_tar(stream, snapshot, files)
                stream.flush()
                os.fsync(stream.fileno())
            if inventory(snapshot, plan) != files:
                raise InvalidBundle('snapshot inventory changed before publication')
            # Recheck the destination path resolves to our pinned parent.
            current = directory_fd(output.parent)
            try:
                if INSTALL.identity(os.fstat(current))[:2] != INSTALL.identity(os.fstat(parent_fd))[:2]:
                    raise InvalidBundle('destination directory changed before publication')
            finally:
                os.close(current)
            # Atomic no-replace publication: a concurrent destination creator
            # wins safely, rather than being overwritten by rename().
            os.link(temporary, output.name, src_dir_fd=parent_fd, dst_dir_fd=parent_fd,
                    follow_symlinks=False)
            published = True
            current = directory_fd(output.parent)
            try:
                if os.fstat(current).st_ino != parent_stat.st_ino or os.fstat(current).st_dev != parent_stat.st_dev:
                    raise InvalidBundle('destination directory changed during publication')
            finally:
                os.close(current)
            os.fsync(parent_fd)
        return {'file': str(output), 'members': len(files), 'bytes': sum(size for size, _ in files.values())}
    except BaseException:
        if published:
            os.unlink(output.name, dir_fd=parent_fd)
        raise
    finally:
        if created:
            os.unlink(temporary, dir_fd=parent_fd)
        os.close(parent_fd)


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('directory', type=Path)
    parser.add_argument('--manifest', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args(argv)
    try:
        print(json.dumps(export(args.directory, args.manifest, args.output), sort_keys=True))
        return 0
    except (InvalidBundle, OSError, ValueError, tarfile.TarError, subprocess.SubprocessError) as error:
        print(f'export failed: {error}', file=sys.stderr)
        return 1


if __name__ == '__main__':
    sys.exit(main())
