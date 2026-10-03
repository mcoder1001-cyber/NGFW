#!/usr/bin/env python3
"""Read-only validation of an offline amd64 appliance package set.

Package hashes establish consistency, not publisher authenticity. No packages,
maintainer scripts or APT operations are executed by this tool.
"""
import argparse
import hashlib
import json
import os
import re
import selectors
import shlex
import subprocess
import sys
import time
import stat
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
PRODUCT = {'vrx-agent', 'vrx-api', 'vrx-web', 'vrx-meta'}
# The portable full-runtime profile needs P11's product plugin package even
# while vrx-meta only Recommends it. Upstream strongSwan is not a substitute.
REQUIRED_VPN = {'vrx-strongswan'}
TERM = re.compile(r'([a-z0-9][a-z0-9+.-]+(?::any)?)'
                  r'(?:\s*\((<<|<=|=|>=|>>)\s*([^()\s]+)\))?')
MAX_PACKAGES = 4096
MAX_CONTROL = 65536
MAX_ARCHIVE = 2 * 1024 * 1024 * 1024
MAX_TOTAL = 16 * 1024 * 1024 * 1024
MAX_MANIFEST = 4 * 1024 * 1024


class InvalidBundle(ValueError):
    """A package set cannot support the declared installation plan."""


def run(argv, limit=1024 * 1024, pass_fds=()):
    # Read both output streams through one bounded pipe. Large control fields
    # never become an unbounded capture_output allocation.
    process = subprocess.Popen(argv, stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                               pass_fds=pass_fds)
    output = bytearray()
    deadline = time.monotonic() + 120
    try:
        with selectors.DefaultSelector() as selector:
            selector.register(process.stdout, selectors.EVENT_READ)
            while True:
                if time.monotonic() >= deadline:
                    raise InvalidBundle('package verification command timed out')
                if not selector.select(timeout=min(1, deadline - time.monotonic())):
                    continue
                chunk = os.read(process.stdout.fileno(), 4096)
                if not chunk:
                    break
                output.extend(chunk)
                if len(output) > limit:
                    raise InvalidBundle('package verification output exceeds bound')
        process.wait(timeout=max(0.01, deadline - time.monotonic()))
        if process.returncode:
            raise InvalidBundle(f'{argv[0]} failed')
        return output.decode('utf-8', errors='strict')
    finally:
        if process.poll() is None:
            process.kill()
        process.wait()
        process.stdout.close()


def relations(value, allow_any=False):
    if len(value) > MAX_CONTROL or len(value.split(',')) > 256:
        raise InvalidBundle('dependency relationship count exceeds bound')
    groups = []
    for group in value.split(','):
        if not group.strip():
            continue
        alternatives = []
        if len(group.split('|')) > 32:
            raise InvalidBundle('dependency alternatives exceed bound')
        for term in group.split('|'):
            match = TERM.fullmatch(term.strip())
            if not match or (match.group(1).endswith(':any') and not allow_any):
                raise InvalidBundle(f'unsupported Debian relationship: {term!r}')
            alternatives.append(match.groups())
            if match.group(3):
                validate_version(match.group(3))
        groups.append(alternatives)
    return groups


def validate_version(version):
    if len(version) > 128 or not re.fullmatch(r'[A-Za-z0-9.+:~\-]+', version):
        raise InvalidBundle('invalid Debian version')
    run(['dpkg', '--validate-version', version], MAX_CONTROL)


def parse_control(control):
    # Debian control field names are case-insensitive, including relationship
    # fields. Normalize fields consumed by this verifier; retain unknown names
    # for manifest compatibility while still rejecting duplicate spellings.
    known = {name.lower(): name for name in (
        'Package', 'Version', 'Architecture', 'Pre-Depends', 'Depends',
        'Conflicts', 'Breaks', 'Provides', 'Multi-Arch')}
    fields = {}
    seen = set()
    previous = None
    for line in control.splitlines():
        if line.startswith((' ', '\t')) and previous:
            fields[previous] += ' ' + line.strip()
        elif ':' in line:
            key, value = line.split(':', 1)
            folded = key.lower()
            if folded in seen:
                raise InvalidBundle(f'duplicate control field: {key}')
            seen.add(folded)
            key = known.get(folded, key)
            fields[key] = value.strip()
            previous = key
    return fields


def metadata(path):
    if path.is_symlink() or not path.is_file():
        raise InvalidBundle(f'archive must be a regular file: {path}')
    # Copy one securely opened archive into a private snapshot. Both control
    # inspection and hashing consume that identical snapshot, even if a source
    # pathname is replaced. No payload/maintainer scripts are executed.
    descriptor = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    try:
        before = os.fstat(descriptor)
        if not stat.S_ISREG(before.st_mode) or before.st_size > MAX_ARCHIVE:
            raise InvalidBundle('archive type or size exceeds bound')
        with tempfile.TemporaryFile() as snapshot:
            digest = hashlib.sha256()
            copied = 0
            while True:
                chunk = os.read(descriptor, 1024 * 1024)
                if not chunk:
                    break
                copied += len(chunk)
                if copied > MAX_ARCHIVE:
                    raise InvalidBundle('archive grew beyond size bound')
                digest.update(chunk)
                snapshot.write(chunk)
            after = os.fstat(descriptor)
            if (before.st_size, before.st_mtime_ns, before.st_ctime_ns) != (
                    after.st_size, after.st_mtime_ns, after.st_ctime_ns):
                raise InvalidBundle('archive changed while taking snapshot')
            snapshot.flush()
            snapshot.seek(0)
            control = run(['dpkg-deb', '--field', '/proc/self/fd/' + str(snapshot.fileno())],
                          MAX_CONTROL, pass_fds=(snapshot.fileno(),))
            current = path.stat(follow_symlinks=False)
            if (before.st_dev, before.st_ino, before.st_size, before.st_mtime_ns,
                    before.st_ctime_ns) != (current.st_dev, current.st_ino,
                    current.st_size, current.st_mtime_ns, current.st_ctime_ns):
                raise InvalidBundle('source archive changed during inspection')
    finally:
        os.close(descriptor)
    fields = parse_control(control)
    for key in ('Package', 'Version', 'Architecture'):
        if not fields.get(key):
            raise InvalidBundle(f'missing {key}: {path}')
    if not re.fullmatch(r'[a-z0-9][a-z0-9+.-]+', fields['Package']):
        raise InvalidBundle('invalid package name')
    if fields['Architecture'] not in ('amd64', 'all'):
        raise InvalidBundle(f'unsupported architecture: {path}')
    validate_version(fields['Version'])
    return {'fields': fields, 'sha256': digest.hexdigest(), 'size': copied}


def matches(version, operator, wanted):
    if not operator:
        return True
    result = subprocess.run(['dpkg', '--compare-versions', version, operator, wanted],
                            check=False, capture_output=True, timeout=10)
    if result.returncode not in (0, 1):
        raise InvalidBundle('invalid Debian version comparison')
    return result.returncode == 0


def validate_set(packages, required):
    missing = set(required) - packages.keys()
    if missing:
        raise InvalidBundle('missing required packages: ' + ', '.join(sorted(missing)))
    providers = {}
    for name, package in packages.items():
        fields = package['fields']
        providers.setdefault(name, []).append((name, fields['Version']))
        for group in relations(fields.get('Provides', '')):
            if len(group) != 1 or group[0][1] not in (None, '='):
                raise InvalidBundle('unsupported Provides relationship')
            provided, _, version = group[0]
            providers.setdefault(provided, []).append((name, version))
    def satisfied(term, excluding=None):
        name, operator, wanted = term
        if name.endswith(':any'):
            direct = packages.get(name[:-4])
            if direct is None:
                return False
            fields = direct['fields']
            return (name[:-4] != excluding and fields.get('Multi-Arch') == 'allowed'
                    and fields['Architecture'] in ('amd64', 'all') and (
                        not operator or matches(fields['Version'], operator, wanted)))
        return any(owner != excluding and (not operator or (
            version is not None and matches(version, operator, wanted)))
                   for owner, version in providers.get(name, []))
    for name, package in packages.items():
        fields = package['fields']
        for kind in ('Pre-Depends', 'Depends'):
            for group in relations(fields.get(kind, ''), allow_any=True):
                # Qualified virtual dependencies are deliberately unsupported.
                # Do not accidentally accept them via an earlier alternative.
                for term in group:
                    if (term[0].endswith(':any') and term[0][:-4] not in packages
                            and term[0][:-4] in providers):
                        raise InvalidBundle('qualified virtual dependency is unsupported')
                if not any(satisfied(term) for term in group):
                    raise InvalidBundle(f'{name}: unresolved {kind}: {group}')
        for kind in ('Conflicts', 'Breaks'):
            for group in relations(fields.get(kind, '')):
                for term in group:
                    if satisfied(term, excluding=name):
                        raise InvalidBundle(f'{name}: incompatible {kind}: {term[0]}')


def runtime_roots():
    """Read existing literal installer lists as data; never source a shell file."""
    script = (ROOT / 'scripts/10-install-runtime.sh').read_text()
    names = set(PRODUCT | REQUIRED_VPN)
    for array in ('NIC', 'ROUTING', 'VPN', 'SERVICES', 'CONTROL', 'OBSERV', 'TOOLS'):
        match = re.search(r'^' + array + r'=\(([^)]*)\)', script, re.MULTILINE)
        if not match:
            raise InvalidBundle(f'cannot read runtime list {array}')
        for name in shlex.split(match.group(1)):
            if not re.fullmatch(r'[a-z0-9][a-z0-9+.-]+', name):
                raise InvalidBundle('runtime list contains nonliteral package')
            names.add(name)
    return names


def manifest_bytes(path):
    with path.open('rb') as stream:
        content = stream.read(MAX_MANIFEST + 1)
    if len(content) > MAX_MANIFEST:
        raise InvalidBundle('manifest exceeds size bound')
    return content


def verify(directory):
    root = directory.resolve(strict=True)
    if not root.is_dir():
        raise InvalidBundle('bundle root must be a directory')
    before_manifest = manifest_bytes(root / 'vpp/manifest.json')
    # Reuse product VPP provenance/version/patch checks without skipping its tests.
    run(['bash', str(ROOT / 'deploy/vpp/verify.sh'), '--require-files',
         str(root / 'vpp'), '--install-gate'])
    if before_manifest != manifest_bytes(root / 'vpp/manifest.json'):
        raise InvalidBundle('VPP manifest changed during provenance verification')
    vpp = json.loads(before_manifest)
    shipping = {entry['file'] for entry in vpp['packages'] if entry['ship']}
    expected_vpp = {entry['file']: entry for entry in vpp['packages'] if entry['ship']}
    packages = {}
    artifacts = []
    total = 0
    paths = []
    for path in root.rglob('*.deb'):
        if len(paths) >= MAX_PACKAGES:
            raise InvalidBundle('bundle archive count exceeds bound')
        paths.append(path)
    for path in sorted(paths):
        if len(path.relative_to(root).as_posix()) > 240:
            raise InvalidBundle('artifact path exceeds length bound')
        if not re.fullmatch(r'[A-Za-z0-9_.+-]+\.deb', path.name):
            raise InvalidBundle('unsafe archive filename')
        if any(parent.is_symlink() for parent in path.parents if parent != root.parent):
            raise InvalidBundle('symlinked artifact directory')
        relative = path.relative_to(root).as_posix()
        package = metadata(path)
        total += package['size']
        if total > MAX_TOTAL:
            raise InvalidBundle('bundle total size exceeds bound')
        name = package['fields']['Package']
        if name in packages:
            raise InvalidBundle(f'duplicate package: {name}')
        if path.parent == root / 'vpp' and path.name not in shipping:
            # Verified VPP builds contain development/debug files, but those are
            # excluded from the transferable runtime artifact plan.
            continue
        if path.parent == root / 'vpp':
            expected = expected_vpp[path.name]
            fields = package['fields']
            if (name, fields['Version'], fields['Architecture'], package['sha256'], package['size']) != (
                    expected['package'], expected['version'], expected['architecture'],
                    expected['sha256'], expected['size']):
                raise InvalidBundle('VPP archive no longer matches verified provenance manifest')
        if name.startswith(('vpp', 'libvpp')) and relative != 'vpp/' + path.name:
            raise InvalidBundle('VPP package outside verified VPP output')
        package['file'] = relative
        packages[name] = package
        artifacts.append({'file': relative, **package})
    validate_set(packages, runtime_roots() | {
        entry['package'] for entry in vpp['packages'] if entry['ship']})
    versions = {packages[name]['fields']['Version'] for name in PRODUCT}
    if len(versions) != 1:
        raise InvalidBundle('product package versions differ')
    return {'format': 1, 'target': {'os': 'ubuntu', 'release': '26.04', 'architecture': 'amd64'},
            'product_version': versions.pop(), 'artifacts': artifacts,
            'install_files': [entry['file'] for entry in artifacts]}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('directory', type=Path)
    parser.add_argument('--manifest', type=Path,
                        help='compare against a previously trusted manifest; never install')
    args = parser.parse_args()
    try:
        manifest = verify(args.directory)
        if args.manifest and json.loads(manifest_bytes(args.manifest)) != manifest:
            raise InvalidBundle('bundle differs from expected manifest')
        print(json.dumps(manifest, sort_keys=True, indent=2))
    except (InvalidBundle, OSError, ValueError, subprocess.TimeoutExpired) as error:
        print(f'bundle verification failed: {error}', file=sys.stderr)
        return 1
    return 0


if __name__ == '__main__':
    sys.exit(main())
