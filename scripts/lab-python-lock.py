#!/usr/bin/env python3
"""Offline lab lock candidate builder. Run with the intended target's trusted Python -I.

Local wheel bytes/hashes prove consistency, not upstream authenticity. Review wheel
provenance separately before giving the resulting lock to 40-install-lab.sh.
"""
import argparse
import email.parser
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import shutil
import stat
import subprocess
import sys
import tempfile
from urllib.parse import unquote, urlparse
import zipfile

REQUIRED = {'pip', 'robotframework', 'robotframework-sshlibrary', 'scapy', 'pytest', 'requests'}
LIMIT = 1024 * 1024


def refuse(message):
    raise ValueError(message)


def canonical(name):
    return re.sub(r'[-_.]+', '-', name).lower()


def regular_bytes(path, limit):
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    with os.fdopen(fd, 'rb') as stream:
        info = os.fstat(stream.fileno())
        if not stat.S_ISREG(info.st_mode) or not 0 < info.st_size <= limit:
            refuse('bounded nonempty regular input required')
        data = stream.read(limit + 1)
    if len(data) > limit:
        refuse('input exceeds bound')
    return data


def direct_pins(data):
    pins = {}
    for line in data.decode('utf-8').splitlines():
        line = line.strip()
        if not line or line.startswith('#'):
            continue
        match = re.fullmatch(r'([A-Za-z0-9][A-Za-z0-9_.-]*)==([A-Za-z0-9.!+_-]+)', line)
        if not match or canonical(match[1]) in pins:
            refuse('direct pins require unique exact versions, without URLs/options/markers')
        pins[canonical(match[1])] = match[2]
    if set(pins) != REQUIRED:
        refuse('direct pins must contain exactly the six installer packages')
    return pins


def inspect_wheel(data):
    # Reading metadata never imports/executes wheel content. Bound decompression.
    import io
    with zipfile.ZipFile(io.BytesIO(data)) as archive:
        entries = archive.infolist()
        if len(entries) > 10000:
            refuse('too many wheel members')
        names = [entry.filename for entry in entries]
        if len(names) != len(set(names)):
            refuse('duplicate wheel members')
        for name in names:
            if '\\' in name or name.startswith('/') or '..' in name.split('/'):
                refuse('unsafe wheel member path')
        metadata = [entry for entry in entries if entry.filename.endswith('.dist-info/METADATA')]
        if len(metadata) != 1 or not 0 < metadata[0].file_size <= LIMIT:
            refuse('one bounded wheel METADATA required')
        text = archive.read(metadata[0]).decode('utf-8')
        message = email.parser.Parser().parsestr(text)
        for dependency in message.get_all('Requires-Dist', []):
            # no-index does not block direct dependency URLs; refuse before pip.
            if '@' in dependency or '://' in dependency:
                refuse('direct URL wheel dependency forbidden')


def resolve(wheelhouse, requirements, report, work):
    env = {'PATH': '/usr/bin:/bin', 'PIP_CONFIG_FILE': os.devnull,
           'HOME': str(work), 'TMPDIR': str(work), 'LANG': 'C.UTF-8'}
    command = [sys.executable, '-I', '-m', 'pip', '--isolated', '--disable-pip-version-check',
               '--no-cache-dir', 'install', '--dry-run', '--ignore-installed', '--no-index',
               '--only-binary=:all:', '--find-links', str(wheelhouse), '--report', str(report),
               '-r', str(requirements)]
    if requirements.name == 'requirements.lock':
        command.append('--require-hashes')
    result = subprocess.run(command, env=env, capture_output=True, text=True, timeout=180)
    if result.returncode:
        refuse('offline wheel resolution failed:\n' + result.stderr[-6000:])
    return json.loads(regular_bytes(report, 16 * LIMIT))


def generate(args):
    runtime = {'python': f'{sys.version_info.major}.{sys.version_info.minor}',
               'platform': f'{sys.platform}-{platform.machine()}',
               'os': ':'.join(platform.freedesktop_os_release().get(key, '') for key in ('ID', 'VERSION_ID'))}
    for key in runtime:
        if runtime[key] != getattr(args, 'target_' + key):
            refuse(f'target {key} differs from resolver runtime: {runtime[key]}')
    seed_bytes = regular_bytes(args.direct, LIMIT)
    pins = direct_pins(seed_bytes)
    source = args.wheelhouse.absolute()
    if source.is_symlink() or not source.is_dir():
        refuse('regular wheelhouse directory required')
    if args.output.exists() or args.output.is_symlink():
        refuse('output must be a new directory')
    with tempfile.TemporaryDirectory(prefix='ngfw-lab-lock-') as temporary:
        work = Path(temporary)
        wheels = work / 'wheels'
        wheels.mkdir()
        artifacts = {}
        total = 0
        for path in sorted(source.iterdir()):
            if not re.fullmatch(r'[A-Za-z0-9_.+-]+\.whl', path.name):
                refuse('wheelhouse may contain only wheel files with safe names')
            data = regular_bytes(path, 100 * LIMIT)
            total += len(data)
            if total > 1024 * LIMIT:
                refuse('wheelhouse exceeds 1 GiB')
            inspect_wheel(data)
            (wheels / path.name).write_bytes(data)
            artifacts[path.name] = hashlib.sha256(data).hexdigest()
        direct = work / 'direct.txt'
        direct.write_bytes(seed_bytes)
        report = resolve(wheels, direct, work / 'resolve.json', work)
        entries = {}
        selected = []
        for item in report['install']:
            metadata = item['metadata']
            name, version = canonical(metadata['name']), metadata['version']
            if name in entries or not re.fullmatch(r'[A-Za-z0-9.!+_-]+', version):
                refuse('duplicate or invalid resolved version')
            url = urlparse(item['download_info']['url'])
            artifact = Path(unquote(url.path))
            digest = artifacts.get(artifact.name)
            if url.scheme != 'file' or url.netloc or artifact.parent != wheels or digest is None:
                refuse('resolver selected an artifact outside snapshot')
            if item['download_info']['archive_info']['hashes'].get('sha256') != digest:
                refuse('resolver artifact digest differs from snapshot')
            entries[name] = version
            selected.append({'name': name, 'version': version, 'wheel': artifact.name, 'sha256': digest})
        if not REQUIRED <= entries.keys() or any(entries[name] != version for name, version in pins.items()):
            refuse('resolved closure differs from direct pins')
        lock_bytes = ''.join(f"{item['name']}=={item['version']} --hash=sha256:{item['sha256']}\n"
                             for item in sorted(selected, key=lambda item: item['name'])).encode()
        lock = work / 'requirements.lock'
        lock.write_bytes(lock_bytes)
        verification = resolve(wheels, lock, work / 'verify.json', work)
        if {canonical(item['metadata']['name']): item['metadata']['version']
                for item in verification['install']} != entries:
            refuse('hash-verified closure differs from resolution')
        receipt = {'schema': 1, 'status': 'offline-candidate; upstream provenance and target installation unverified',
                   'runtime': runtime, 'python_version': sys.version, 'libc': platform.libc_ver(),
                   'resolver': report.get('pip_version'), 'environment': report.get('environment'),
                   'direct_sha256': hashlib.sha256(seed_bytes).hexdigest(),
                   'lock_sha256': hashlib.sha256(lock_bytes).hexdigest(),
                   'artifacts': sorted(selected, key=lambda item: item['name'])}
        args.output.mkdir(mode=0o700)
        try:
            (args.output / 'requirements.lock').write_bytes(lock_bytes)
            (args.output / 'provenance.json').write_text(json.dumps(receipt, indent=2, sort_keys=True) + '\n')
        except BaseException:
            shutil.rmtree(args.output)
            raise
    print(f'Offline candidate generated: {args.output}; upstream authenticity and installation remain unverified')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--direct', type=Path, required=True)
    parser.add_argument('--wheelhouse', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--target-python', required=True, help='major.minor of actual resolver interpreter')
    parser.add_argument('--target-platform', required=True, help='e.g. linux-x86_64')
    parser.add_argument('--target-os', required=True, help='e.g. ubuntu:26.04')
    try:
        generate(parser.parse_args())
    except (ValueError, OSError, KeyError, zipfile.BadZipFile, subprocess.TimeoutExpired) as error:
        print(f'REFUSED: {error}', file=sys.stderr)
        return 1
    return 0


if __name__ == '__main__':
    sys.exit(main())
