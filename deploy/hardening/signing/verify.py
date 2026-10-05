#!/usr/bin/env python3
"""Verify a pinned APT Release signature and every SHA256 member before use."""
import argparse
import hashlib
from pathlib import Path
import re
import subprocess
import tempfile



def bounded_read(path, limit):
    with path.open('rb') as source:
        data = source.read(limit + 1)
    if len(data) > limit:
        raise ValueError('signed metadata exceeds size budget')
    return data


def verify(repository, keyring, suite):
    repository = Path(repository).resolve(strict=True)
    keyring = Path(keyring).resolve(strict=True)
    if not re.fullmatch(r'[A-Za-z0-9_-]+', suite):
        raise ValueError('invalid suite')
    directory = repository / 'dists' / suite
    if not directory.resolve(strict=True).is_relative_to(repository):
        raise ValueError('escaping suite directory')
    release = directory / 'Release'
    signature = directory / 'Release.gpg'
    for file in [release, signature]:
        if file.is_symlink() or not file.is_file():
            raise ValueError('detached signed Release required')
    # Authenticate exactly the bytes subsequently parsed. The caller's repository
    # can be updated concurrently; never verify one pathname and parse it again.
    release_bytes = bounded_read(release, 16 * 1024 * 1024)
    signature_bytes = bounded_read(signature, 1024 * 1024)
    key_bytes = bounded_read(keyring, 64 * 1024 * 1024)
    with tempfile.TemporaryDirectory(prefix='ngfw-gpgv-') as home:
        snapshot = Path(home)
        (snapshot / 'Release').write_bytes(release_bytes)
        (snapshot / 'Release.gpg').write_bytes(signature_bytes)
        (snapshot / 'trusted.gpg').write_bytes(key_bytes)
        subprocess.run(['gpgv', '--homedir', home, '--keyring', str(snapshot / 'trusted.gpg'),
                        str(snapshot / 'Release.gpg'), str(snapshot / 'Release')],
                       check=True, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE)
    entries = []
    in_sha = False
    for line in release_bytes.decode('utf-8').splitlines():
        if line == 'SHA256:':
            in_sha = True
            continue
        if line and not line.startswith(' '):
            in_sha = False
        if not in_sha:
            continue
        match = re.fullmatch(r' ([a-fA-F0-9]{64})\s+([0-9]+)\s+(\S+)', line)
        if not match:
            raise ValueError('invalid SHA256 entry')
        digest, size, relative = match.groups()
        path = Path(relative)
        if path.is_absolute() or '..' in path.parts or relative in [e[2] for e in entries]:
            raise ValueError('unsafe or duplicate Release member')
        member = directory / path
        if member.is_symlink() or not member.is_file() or not member.resolve().is_relative_to(directory.resolve()):
            raise ValueError('missing or escaping Release member')
        member_bytes = member.read_bytes()
        if len(member_bytes) != int(size) or hashlib.sha256(member_bytes).hexdigest() != digest.lower():
            raise ValueError('Release member checksum mismatch')
        entries.append((digest, size, relative))
    if not entries or not any('/Packages' in entry[2] for entry in entries):
        raise ValueError('signed package index required')
    return len(entries)


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--repository', required=True)
    p.add_argument('--keyring', required=True)
    p.add_argument('--suite', default='resolute')
    args = p.parse_args()
    try:
        count = verify(args.repository, args.keyring, args.suite)
    except (OSError, ValueError, subprocess.CalledProcessError):
        p.exit(1, 'Repository verification failed\n')
    print('PASS pinned Release signature and ' + str(count) + ' SHA256 members')


if __name__ == '__main__':
    main()
