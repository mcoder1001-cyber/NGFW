#!/usr/bin/env python3
"""Materialise verified build prerequisites; never execute source or install packages."""
import argparse
import bz2
import ctypes
import errno
import importlib.util
import json
import os
from pathlib import Path
import re
import selectors
import subprocess
import sys
import tarfile
import tempfile
import time

SPEC = importlib.util.spec_from_file_location('p11_intake', Path(__file__).with_name('verify_inputs.py'))
INTAKE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(INTAKE)
InvalidInputs = INTAKE.InvalidInputs
MAX_FILE = 128 * 1024 * 1024
MAX_TOTAL = 1024 * 1024 * 1024
MAX_TAR = 1024 * 1024 * 1024
MAX_MEMBERS = 50000


def extract(raw, target, budget, source=False):
    """Only regular files/directories; do not use tarfile's extraction helpers."""
    seen = set()
    target.mkdir(mode=0o700)
    with tarfile.open(fileobj=raw, mode='r:') as archive:
        for index, member in enumerate(archive):
            name = member.name
            if not source and name.startswith('./'):
                name = name[2:]
            if not source and name in ('', '.') and member.isdir():
                name = '.'
            else:
                name = name.rstrip('/')
                parts = name.split('/')
                if (not name or len(name) > 1024 or len(parts) > 32
                        or any(p in ('', '.', '..') for p in parts)
                        or (source and parts[0] != 'strongswan-5.9.6')):
                    raise InvalidInputs('unsafe stage archive path')
            if index >= MAX_MEMBERS or name in seen or not (member.isfile() or member.isdir()):
                raise InvalidInputs('duplicate, special or linked stage member')
            seen.add(name)
            if member.size < 0 or member.size > MAX_FILE or (member.isdir() and member.size):
                raise InvalidInputs('stage member exceeds per-file bound')
            budget[0] += member.size
            if budget[0] > MAX_TOTAL:
                raise InvalidInputs('stage total expanded budget exceeded')
            if name == '.':
                continue
            path = target / name
            parent = target
            for component in path.relative_to(target).parts[:-1]:
                parent = parent / component
                parent.mkdir(mode=0o700, exist_ok=True)
            if member.isdir():
                # An implicit parent may already exist; explicit duplicates still fail above.
                path.mkdir(mode=0o700, exist_ok=True)
                continue
            with archive.extractfile(member) as contents, path.open('xb') as output:
                remaining = member.size
                while remaining:
                    chunk = contents.read(min(1024 * 1024, remaining))
                    if not chunk:
                        raise InvalidInputs('truncated stage member')
                    output.write(chunk)
                    remaining -= len(chunk)
            path.chmod(0o700 if member.mode & 0o111 else 0o600)


def deb_tar(package, raw, decoded):
    # dpkg-deb only decodes the data archive; maintainer scripts are never executed.
    process = subprocess.Popen(['/usr/bin/dpkg-deb', '--fsys-tarfile', str(package)],
                               stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)
    deadline = time.monotonic() + 120
    total = 0
    try:
        with selectors.DefaultSelector() as selector:
            selector.register(process.stdout, selectors.EVENT_READ)
            while True:
                if time.monotonic() >= deadline:
                    raise InvalidInputs('dev archive decoding timed out')
                if not selector.select(timeout=1):
                    continue
                chunk = os.read(process.stdout.fileno(), 1024 * 1024)
                if not chunk:
                    break
                total += len(chunk)
                decoded[0] += len(chunk)
                if decoded[0] > MAX_TOTAL:
                    raise InvalidInputs('total decompressed stage budget exceeded')
                if total > MAX_TAR:
                    raise InvalidInputs('decompressed dev archive exceeds bound')
                raw.write(chunk)
        process.wait(timeout=max(0.01, deadline - time.monotonic()))
        if process.returncode:
            raise InvalidInputs('dev archive decoding failed')
    finally:
        if process.poll() is None:
            process.kill()
        process.wait()
        process.stdout.close()
    raw.seek(0)


def publish(parent_fd, staging, name):
    # Linux renameat2 NOREPLACE closes the existence-check/rename overwrite race.
    libc = ctypes.CDLL(None, use_errno=True)
    rename = libc.renameat2
    rename.argtypes = [ctypes.c_int, ctypes.c_char_p, ctypes.c_int, ctypes.c_char_p, ctypes.c_uint]
    rename.restype = ctypes.c_int
    if rename(parent_fd, os.fsencode(staging), parent_fd, os.fsencode(name), 1):
        code = ctypes.get_errno()
        if code == errno.EEXIST:
            raise InvalidInputs('stage output already exists')
        raise OSError(code, os.strerror(code))


def directory_fd(path):
    """Pin every component and reject ancestor symlinks as well as the leaf."""
    absolute = Path(os.path.abspath(path))
    descriptor = os.open('/', os.O_RDONLY | os.O_DIRECTORY)
    try:
        for name in absolute.parts[1:]:
            child = os.open(name, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW,
                            dir_fd=descriptor)
            os.close(descriptor)
            descriptor = child
        return descriptor
    except BaseException:
        os.close(descriptor)
        raise


def check_parent(path, pinned_fd):
    current = directory_fd(path)
    try:
        actual, expected = os.fstat(current), os.fstat(pinned_fd)
        if (actual.st_dev, actual.st_ino) != (expected.st_dev, expected.st_ino):
            raise InvalidInputs('stage destination directory changed')
    finally:
        os.close(current)


def prepare(vpp_output, source, version, digest, output):
    # Validate trust at the public API, before output creation or helper invocation.
    if not isinstance(digest, str) or not re.fullmatch(r'[0-9a-f]{64}', digest):
        raise InvalidInputs('separately trusted source SHA-256 is required')
    output = Path(output)
    if output.name in ('', '.', '..') or '..' in output.parts:
        raise InvalidInputs('new named stage output required')
    destination = output.parent.resolve(strict=True) / output.name
    if (destination.is_relative_to(Path(vpp_output).resolve(strict=True))
            or destination == Path(source).resolve(strict=True)):
        raise InvalidInputs('stage output overlaps read-only inputs')
    parent_fd = directory_fd(output.parent)
    try:
        parent = os.fstat(parent_fd)
        if parent.st_uid != os.geteuid() or parent.st_mode & 0o022:
            raise InvalidInputs('stage parent must be owned and not group/world writable')
        try:
            os.stat(output.name, dir_fd=parent_fd, follow_symlinks=False)
        except FileNotFoundError:
            pass
        else:
            raise InvalidInputs('stage output already exists')
        # All writes and publication are anchored to this pinned owned parent.
        pinned = Path('/proc/self/fd') / str(parent_fd)
        with tempfile.TemporaryDirectory(prefix='.p11-stage-', dir=pinned) as temporary:
            tree = Path(temporary) / 'materialised'
            tree.mkdir(mode=0o700)
            with INTAKE.verified_snapshot(vpp_output, source, version, digest) as (report, snapshot, tarball):
                budget = [0]
                decoded = [0]
                with tempfile.TemporaryFile(dir=temporary) as raw:
                    expanded = 0
                    with bz2.open(tarball, 'rb') as compressed:
                        while True:
                            chunk = compressed.read(1024 * 1024)
                            if not chunk:
                                break
                            expanded += len(chunk)
                            decoded[0] += len(chunk)
                            if decoded[0] > MAX_TOTAL:
                                raise InvalidInputs('total decompressed stage budget exceeded')
                            if expanded > MAX_TAR:
                                raise InvalidInputs('decompressed source stage exceeds bound')
                            raw.write(chunk)
                    raw.seek(0)
                    extract(raw, tree / 'source', budget, source=True)
                dev = tree / 'vpp-dev'
                dev.mkdir(mode=0o700)
                for name in ('vpp-dev', 'libvppinfra-dev'):
                    with tempfile.TemporaryFile(dir=temporary) as raw:
                        deb_tar(snapshot / report['staging_packages'][name]['file'], raw, decoded)
                        extract(raw, dev / name, budget)
                report = {**report, 'mode': 'materialised-build-prerequisites',
                          'expanded_bytes': budget[0], 'builder_ready': False}
                (tree / 'intake.json').write_text(json.dumps(report, indent=2, sort_keys=True) + '\n')
                (tree / 'intake.json').chmod(0o600)
            relative = str(tree.relative_to(pinned))
            staged = tree.stat()
            try:
                check_parent(output.parent, parent_fd)
                publish(parent_fd, relative, output.name)
                check_parent(output.parent, parent_fd)
            except BaseException:
                # Undo only our inode; never remove a concurrent caller's output.
                try:
                    current = os.stat(output.name, dir_fd=parent_fd, follow_symlinks=False)
                except FileNotFoundError:
                    pass
                else:
                    if (current.st_dev, current.st_ino) == (staged.st_dev, staged.st_ino):
                        publish(parent_fd, output.name, relative)
                raise
            return report
    finally:
        os.close(parent_fd)


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--vpp-output', required=True, type=Path)
    parser.add_argument('--source', required=True, type=Path)
    parser.add_argument('--source-version', required=True)
    parser.add_argument('--source-sha256', required=True)
    parser.add_argument('--output', required=True, type=Path)
    args = parser.parse_args(argv)
    try:
        print(json.dumps(prepare(args.vpp_output, args.source, args.source_version,
                                 args.source_sha256, args.output), sort_keys=True, indent=2))
        return 0
    except (InvalidInputs, OSError, EOFError, ValueError, KeyError, TypeError,
            tarfile.TarError, subprocess.SubprocessError, AttributeError) as error:
        print(f'P11 stage preparation failed: {error}', file=sys.stderr)
        return 1


if __name__ == '__main__':
    sys.exit(main())
