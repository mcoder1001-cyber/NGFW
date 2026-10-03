#!/usr/bin/env python3
"""Authenticated standalone helper bootstrap. Authenticate THIS file before use.

Only a separately authenticated helper report may authorize helper execution.
Runtime packages still need the separately trusted runtime manifest. No helper
code is imported from the delivery directory or an unchecked helper archive.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import subprocess
import sys
import tarfile
import tempfile

MAX_HELPER = 1024 * 1024
MAX_TOTAL = 8 * 1024 * 1024
MAX_TAR = 10 * 1024 * 1024
MAX_FILES = 128
MAX_REPORT = 1024 * 1024
ENV = {'PATH': '/usr/sbin:/usr/bin:/sbin:/bin', 'LC_ALL': 'C'}


def read_regular(path, limit):
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    try:
        before = os.fstat(fd)
        if not stat.S_ISREG(before.st_mode) or before.st_size > limit:
            raise ValueError('input is not a bounded regular file')
        data = bytearray()
        while True:
            chunk = os.read(fd, min(65536, limit + 1 - len(data)))
            if not chunk:
                break
            data.extend(chunk)
            if len(data) > limit:
                raise ValueError('input exceeds size bound')
        after = os.fstat(fd)
        identity = lambda st: (st.st_dev, st.st_ino, st.st_size, st.st_mtime_ns, st.st_ctime_ns)
        if identity(before) != identity(after) or len(data) != before.st_size:
            raise ValueError('input changed while reading')
        return bytes(data)
    finally:
        os.close(fd)


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError('duplicate trusted report key')
        result[key] = value
    return result


def record(value, mode=False):
    keys = {'size', 'sha256', 'mode'} if mode else {'size', 'sha256'}
    if (not isinstance(value, dict) or set(value) != keys
            or type(value['size']) is not int or not 0 <= value['size'] <= MAX_HELPER
            or not isinstance(value['sha256'], str)
            or not re.fullmatch('[0-9a-f]{64}', value['sha256'])
            or (mode and (type(value['mode']) is not int or value['mode'] not in (0o600, 0o700)))):
        raise ValueError('invalid trusted helper record')


def load_report(path, expected_sha256):
    if not isinstance(expected_sha256, str) or not re.fullmatch('[0-9a-f]{64}', expected_sha256):
        raise ValueError('expected report SHA256 must come from the trusted channel')
    content = read_regular(path, MAX_REPORT)
    if hashlib.sha256(content).hexdigest() != expected_sha256:
        raise ValueError('helper report differs from externally trusted SHA256')
    report = json.loads(content, object_pairs_hook=unique_object)
    if (not isinstance(report, dict)
            or set(report) != {'schema', 'source_commit', 'archive_bytes', 'sha256', 'launcher', 'files'}
            or report['schema'] != 'vrx.recipient-helpers/v1'
            or not isinstance(report['source_commit'], str)
            or not re.fullmatch('[0-9a-f]{40}', report['source_commit'])
            or type(report['archive_bytes']) is not int or not 0 < report['archive_bytes'] <= MAX_TAR
            or not isinstance(report['sha256'], str) or not re.fullmatch('[0-9a-f]{64}', report['sha256'])
            or not isinstance(report['files'], dict) or not 1 <= len(report['files']) <= MAX_FILES):
        raise ValueError('invalid trusted helper report')
    record(report['launcher'])
    total = 0
    for name, entry in report['files'].items():
        if (len(name) > 240 or not re.fullmatch(r'(?:[A-Za-z0-9_+.-]+/)*[A-Za-z0-9_+.-]+', name)
                or any(part in ('.', '..') for part in Path(name).parts)):
            raise ValueError('unsafe trusted helper path')
        record(entry, mode=True)
        total += entry['size']
    if total > MAX_TOTAL:
        raise ValueError('trusted helper payload exceeds bound')
    if not {'deploy/debian/bundle/install.py', 'deploy/debian/bundle/verify.py',
            'deploy/vpp/verify.sh', 'deploy/vpp/tests/run.sh', 'scripts/10-install-runtime.sh'} <= report['files'].keys():
        raise ValueError('trusted helper inventory omits required entrypoints')
    return report


def unpack_helpers(archive_path, report, destination):
    # The complete archive is bounded, copied once into memory, and authenticated
    # BEFORE tar parsing. Tar filenames/modes never authorize themselves.
    import io
    data = read_regular(archive_path, MAX_TAR)
    if len(data) != report['archive_bytes'] or hashlib.sha256(data).hexdigest() != report['sha256']:
        raise ValueError('helper archive differs from trusted report')
    seen = set()
    with tarfile.open(fileobj=io.BytesIO(data), mode='r:') as archive:
        for member in archive:
            entry = report['files'].get(member.name)
            if (entry is None or member.name in seen or member.type != tarfile.REGTYPE
                    or member.size != entry['size'] or member.mode != entry['mode']
                    or member.pax_headers or member.linkname):
                raise ValueError('helper archive member differs from trusted inventory')
            seen.add(member.name)
            with archive.extractfile(member) as stream:
                content = stream.read(MAX_HELPER + 1)
            if len(content) != entry['size'] or hashlib.sha256(content).hexdigest() != entry['sha256']:
                raise ValueError('helper member differs from trusted inventory')
            path = destination / member.name
            # Path.mkdir(parents=True) applies mode only to the leaf, leaving
            # implicit parents at the process umask's default permissions.
            # Create every canonical directory component explicitly instead.
            parent = destination
            for component in Path(member.name).parts[:-1]:
                parent = parent / component
                parent.mkdir(exist_ok=True, mode=0o700)
            with path.open('xb') as stream:
                stream.write(content)
            path.chmod(entry['mode'])
    if seen != report['files'].keys():
        raise ValueError('helper archive omits trusted dependency')


def launch(delivery, manifest, helpers, report_path, report_sha256, install=False):
    delivery = delivery.resolve(strict=True)
    manifest = manifest.resolve(strict=True)
    report_path = report_path.resolve(strict=True)
    if manifest.is_relative_to(delivery) or report_path.is_relative_to(delivery):
        raise ValueError('trusted runtime manifest and helper report must be outside delivery')
    report = load_report(report_path, report_sha256)
    # Detect accidental launcher mismatch. This cannot establish the launcher's
    # own trust: authentication must precede execution via the trusted channel.
    launcher = read_regular(Path(__file__), MAX_HELPER)
    if len(launcher) != report['launcher']['size'] or hashlib.sha256(launcher).hexdigest() != report['launcher']['sha256']:
        raise ValueError('launcher differs from separately trusted report')
    with tempfile.TemporaryDirectory(prefix='vrx-helpers-', dir='/var/tmp') as temporary:
        private = Path(temporary)
        unpack_helpers(helpers, report, private)
        command = ['/usr/bin/python3', '-I', str(private / 'deploy/debian/bundle/install.py'),
                   str(delivery), '--manifest', str(manifest)]
        if install:
            command.append('--install')
        subprocess.run(command, env=ENV, cwd=private, check=True)


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('directory', type=Path)
    parser.add_argument('--manifest', type=Path, required=True)
    parser.add_argument('--helpers', type=Path, required=True)
    parser.add_argument('--helper-report', type=Path, required=True)
    parser.add_argument('--helper-report-sha256', required=True,
                        help='expected report digest obtained through authenticated channel')
    parser.add_argument('--install', action='store_true')
    args = parser.parse_args(argv)
    try:
        launch(args.directory, args.manifest, args.helpers, args.helper_report,
               args.helper_report_sha256, args.install)
        return 0
    except (OSError, ValueError, subprocess.SubprocessError, tarfile.TarError) as error:
        print(f'recipient preflight failed: {error}', file=sys.stderr)
        return 1


if __name__ == '__main__':
    sys.exit(main())
