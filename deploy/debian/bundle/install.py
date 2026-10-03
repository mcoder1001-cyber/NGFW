#!/usr/bin/env python3
"""Trusted-manifest preflight and explicitly requested local Debian installation."""
import argparse
import contextlib
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import stat
import subprocess
import sys
import tempfile

SPEC = importlib.util.spec_from_file_location('bundle_verify', Path(__file__).with_name('verify.py'))
VERIFY = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(VERIFY)
InvalidBundle = VERIFY.InvalidBundle


@contextlib.contextmanager
def safe_environment():
    # CLI is single-threaded. All verification helpers, including VPP's fixed
    # Bash verifier, must inherit this boundary rather than the caller's PATH,
    # loader, Python/Bash startup, proxy, APT_CONFIG or temporary-directory values.
    previous = dict(os.environ)
    os.environ.clear()
    os.environ.update({'PATH': '/usr/sbin:/usr/bin:/sbin:/bin', 'LC_ALL': 'C'})
    try:
        yield
    finally:
        os.environ.clear()
        os.environ.update(previous)


def identity(st):
    return (st.st_dev, st.st_ino, st.st_size, st.st_mtime_ns, st.st_ctime_ns)


def copy_tree(source, destination):
    """Walk using pinned no-follow directory descriptors, never source path reopen."""
    budget = [0, 0]
    def walk(fd, target, relative):
        if len(relative.parts) > 32:
            raise InvalidBundle('delivery directory depth exceeds bound')
        before = os.fstat(fd)
        names = []
        with os.scandir(fd) as entries:
            for entry in entries:
                budget[0] += 1
                if budget[0] > VERIFY.MAX_PACKAGES * 4:
                    raise InvalidBundle('delivery entry count exceeds bound')
                names.append(entry.name)
        for name in sorted(names):
            child_relative = relative / name
            entry = os.stat(name, dir_fd=fd, follow_symlinks=False)
            if stat.S_ISDIR(entry.st_mode):
                child = os.open(name, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW, dir_fd=fd)
                try:
                    if identity(entry) != identity(os.fstat(child)):
                        raise InvalidBundle('source directory replaced')
                    child_target = target / name
                    child_target.mkdir(mode=0o700)
                    walk(child, child_target, child_relative)
                finally:
                    os.close(child)
            elif stat.S_ISREG(entry.st_mode):
                if not (name.endswith('.deb') or child_relative.as_posix() in (
                        'vpp/manifest.json', 'vpp/SHA256SUMS')):
                    continue
                limit = VERIFY.MAX_ARCHIVE if name.endswith('.deb') else VERIFY.MAX_MANIFEST
                descriptor = os.open(name, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK, dir_fd=fd)
                try:
                    opened = os.fstat(descriptor)
                    if identity(entry) != identity(opened) or opened.st_size > limit:
                        raise InvalidBundle('source file replaced or exceeds bound')
                    copied = 0
                    with (target / name).open('xb') as output:
                        os.chmod(target / name, 0o600)
                        while True:
                            chunk = os.read(descriptor, 1024 * 1024)
                            if not chunk:
                                break
                            copied += len(chunk)
                            budget[1] += len(chunk)
                            if copied > limit or budget[1] > VERIFY.MAX_TOTAL:
                                raise InvalidBundle('snapshot size exceeds bound')
                            output.write(chunk)
                    if identity(opened) != identity(os.fstat(descriptor)) or identity(opened) != identity(
                            os.stat(name, dir_fd=fd, follow_symlinks=False)):
                        raise InvalidBundle('source file changed during snapshot')
                finally:
                    os.close(descriptor)
            else:
                raise InvalidBundle('delivery contains symlink or special file')
        if identity(before) != identity(os.fstat(fd)):
            raise InvalidBundle('source directory changed during snapshot')
    descriptor = os.open(source, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
    try:
        walk(descriptor, destination, Path())
    finally:
        os.close(descriptor)


def expected_manifest(path, source):
    resolved = path.resolve(strict=True)
    if resolved.is_relative_to(source.resolve(strict=True)):
        raise InvalidBundle('trusted manifest must be outside delivery directory')
    return json.loads(VERIFY.manifest_bytes(resolved))


@contextlib.contextmanager
def prepared(source, manifest):
    expected = expected_manifest(manifest, source)
    with safe_environment(), tempfile.TemporaryDirectory(prefix='vrx-offline-', dir='/var/tmp') as directory:
        private = Path(directory)
        snapshot = private / 'delivery'
        snapshot.mkdir(mode=0o700)
        copy_tree(source, snapshot)
        actual = VERIFY.verify(snapshot)
        if actual != expected:
            raise InvalidBundle('snapshot differs from trusted expected manifest')
        yield private, snapshot, actual


def _target_check():
    if os.geteuid() != 0:
        raise InvalidBundle('--install requires root on the authorized target')
    fields = {}
    for line in Path('/etc/os-release').read_text().splitlines():
        if '=' in line:
            key, value = line.split('=', 1)
            fields[key] = value.strip('"')
    if fields.get('ID') != 'ubuntu' or fields.get('VERSION_ID') != '26.04':
        raise InvalidBundle('installation target must be Ubuntu 26.04')
    if VERIFY.run(['/usr/bin/dpkg', '--print-architecture']).strip() != 'amd64':
        raise InvalidBundle('installation target must be amd64')


def target_check():
    with safe_environment():
        _target_check()


def apt_arguments(private, snapshot, plan, simulate):
    etc = private / 'apt-etc'
    etc.mkdir(exist_ok=True, mode=0o700)
    for name in ('parts', 'sources'):
        (etc / name).mkdir(exist_ok=True, mode=0o700)
    (etc / 'empty.conf').write_text('')
    (etc / 'sources.list').write_text('')
    for name in ('lists', 'archives'):
        (private / name).mkdir(exist_ok=True, mode=0o700)
        (private / name / 'partial').mkdir(exist_ok=True, mode=0o700)
    options = {
        'Dir::Etc': str(etc), 'Dir::Etc::main': 'empty.conf',
        'Dir::Etc::parts': 'parts', 'Dir::Etc::sourcelist': 'sources.list',
        'Dir::Etc::sourceparts': 'sources', 'Dir::State::lists': str(private / 'lists'),
        'Dir::Cache::archives': str(private / 'archives'),
        'Dir::Cache::pkgcache': '', 'Dir::Cache::srcpkgcache': '',
        'Acquire::http::Proxy': 'false', 'Acquire::https::Proxy': 'false',
        'APT::Install-Recommends': 'false', 'APT::Install-Suggests': 'false',
        # Root-only archives cannot be opened by _apt; do not loosen their permissions.
        'APT::Sandbox::User': 'root',
        'Dpkg::Options::': '--force-confold',
    }
    argv = ['/usr/bin/apt-get']
    for key, value in options.items():
        argv += ['-o', key + '=' + value]
    argv += ['--no-download', '--no-remove', '--assume-yes']
    if simulate:
        argv += ['--simulate']
    argv += ['install', '--'] + [str(snapshot / path) for path in plan['install_files']]
    return argv


def check_archives(snapshot, plan):
    for entry in plan['artifacts']:
        path = snapshot / entry['file']
        if path.is_symlink() or path.stat().st_size != entry['size']:
            raise InvalidBundle('private archive changed before installation')
        with path.open('rb') as stream:
            digest = hashlib.file_digest(stream, 'sha256').hexdigest()
        if digest != entry['sha256']:
            raise InvalidBundle('private archive hash changed before installation')


def execute(private, snapshot, plan):
    # APT_CONFIG bypasses the target's apt.conf and hook fragments. Preserve no
    # caller proxy/config/loader variables. Maintainer scripts remain privileged.
    env = {'PATH': '/usr/sbin:/usr/bin:/sbin:/bin', 'LC_ALL': 'C',
           'DEBIAN_FRONTEND': 'noninteractive', 'APT_CONFIG': str(private / 'apt-bootstrap.conf')}
    bootstrap = private / 'apt-bootstrap.conf'
    bootstrap.write_text('Dir::Etc::main "-"; Dir::Etc::parts "-";\n')
    for simulate in (True, False):
        check_archives(snapshot, plan)
        subprocess.run(apt_arguments(private, snapshot, plan, simulate), env=env, check=True)


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('directory', type=Path)
    parser.add_argument('--manifest', type=Path, required=True,
                        help='trusted expected manifest obtained separately')
    parser.add_argument('--install', action='store_true', help='mutate this authorized fresh target')
    args = parser.parse_args(argv)
    try:
        if args.install:
            target_check()
        with prepared(args.directory, args.manifest) as (private, snapshot, plan):
            if args.install:
                execute(private, snapshot, plan)
            else:
                print(json.dumps({'mode': 'plan', 'manifest': plan}, sort_keys=True, indent=2))
        return 0
    except (InvalidBundle, OSError, ValueError, subprocess.SubprocessError) as error:
        print(f'offline installation failed: {error}', file=sys.stderr)
        return 1


if __name__ == '__main__':
    sys.exit(main())
