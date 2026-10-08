#!/usr/bin/env python3
"""Materialize the reviewed CPython 3.14 Linux x86_64 lab wheelhouse.

Downloads are accepted only when their bytes match the committed PyPI receipts.
The one source-only upstream package is built offline using hash-locked tools;
its output must match the separately recorded reproducible derived-wheel hash.
This executes reviewed upstream build code in a disposable venv, not a sandbox.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import stat
import subprocess
import sys
import tempfile
import urllib.parse
import urllib.request
import venv

RELEASE = Path(__file__).resolve().parent.parent / 'deploy' / 'lab-python'
LIMIT = 100 * 1024 * 1024


def checked_bytes(data, artifact):
    if len(data) != artifact['size'] or hashlib.sha256(data).hexdigest() != artifact['sha256']:
        raise ValueError('artifact digest/size mismatch: ' + artifact['filename'])
    return data


def obtain(artifact, cache):
    name = artifact['filename']
    if Path(name).name != name or name in {'.', '..'} or not 0 < artifact['size'] <= LIMIT:
        raise ValueError('unsafe artifact entry')
    if cache is not None:
        fd = os.open(cache / name, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
        with os.fdopen(fd, 'rb') as stream:
            if not stat.S_ISREG(os.fstat(stream.fileno()).st_mode):
                raise ValueError('regular cached artifact required')
            return checked_bytes(stream.read(LIMIT + 1), artifact)
    url = urllib.parse.urlsplit(artifact['url'])
    if url.scheme != 'https' or url.netloc != 'files.pythonhosted.org':
        raise ValueError('official PyPI artifact URL required')
    with urllib.request.urlopen(artifact['url'], timeout=60) as response:
        final = urllib.parse.urlsplit(response.geturl())
        if final.scheme != 'https' or final.netloc != 'files.pythonhosted.org':
            raise ValueError('artifact redirect outside PyPI refused')
        return checked_bytes(response.read(LIMIT + 1), artifact)


def publish(output, artifacts):
    # Directory-fd anchored exclusive output: do not follow replaced child paths.
    parent_fd = os.open(output.parent, os.O_RDONLY | os.O_DIRECTORY)
    directory_fd = None
    created = []
    try:
        os.mkdir(output.name, mode=0o700, dir_fd=parent_fd)
        directory_fd = os.open(output.name, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW,
                               dir_fd=parent_fd)
        for name, data in artifacts.items():
            fd = os.open(name, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW,
                         0o644, dir_fd=directory_fd)
            created.append(name)
            with os.fdopen(fd, 'wb') as stream:
                stream.write(data)
    except BaseException:
        if directory_fd is not None:
            for name in created:
                os.unlink(name, dir_fd=directory_fd)
            info = os.fstat(directory_fd)
            current = os.stat(output.name, dir_fd=parent_fd, follow_symlinks=False)
            if (info.st_dev, info.st_ino) == (current.st_dev, current.st_ino):
                os.rmdir(output.name, dir_fd=parent_fd)
        raise
    finally:
        if directory_fd is not None:
            os.close(directory_fd)
        os.close(parent_fd)


def materialize(output, cache):
    if platform.python_implementation() != 'CPython' or sys.version_info[:2] != (3, 14) or sys.platform != 'linux' or platform.machine() != 'x86_64':
        raise ValueError('CPython 3.14 on Linux x86_64 required')
    upstream = json.loads((RELEASE / 'upstream.json').read_text())
    resolution = json.loads((RELEASE / 'resolution.json').read_text())
    if hashlib.sha256((RELEASE / 'requirements.lock').read_bytes()).hexdigest() != resolution['lock_sha256']:
        raise ValueError('release lock differs from reviewed resolution')
    with tempfile.TemporaryDirectory(prefix='ngfw-lab-release-') as temporary:
        work = Path(temporary)
        inputs = work / 'inputs'
        inputs.mkdir()
        for artifact in upstream['artifacts']:
            (inputs / artifact['filename']).write_bytes(obtain(artifact, cache))
        environment = work / 'build'
        venv.EnvBuilder(with_pip=True, symlinks=True).create(environment)
        python = str(environment / 'bin' / 'python')
        env = {'PATH': '/usr/bin:/bin', 'HOME': str(work), 'TMPDIR': str(work),
               'LANG': 'C.UTF-8', 'PIP_CONFIG_FILE': os.devnull,
               'SOURCE_DATE_EPOCH': upstream['build']['source_date_epoch'],
               'PYTHONHASHSEED': upstream['build']['pythonhashseed']}
        prefix = [python, '-I', '-m', 'pip', '--isolated', '--disable-pip-version-check', '--no-cache-dir']
        subprocess.run(prefix + ['install', '--force-reinstall', '--no-index', '--only-binary=:all:', '--find-links',
                       str(inputs), '--require-hashes', '-r', str(RELEASE / 'build.lock')],
                       env=env, check=True)
        built = work / 'built'
        subprocess.run(prefix + ['wheel', '--no-index', '--no-deps', '--no-build-isolation',
                       '--wheel-dir', str(built), str(inputs / upstream['build']['source'])],
                       env=env, check=True)
        files = {}
        for item in resolution['artifacts']:
            name = item['wheel']
            if Path(name).name != name or not name.endswith('.whl'):
                raise ValueError('unsafe release wheel name')
            path = (built if item['name'] == 'robotframework-sshlibrary' else inputs) / name
            data = path.read_bytes()
            if hashlib.sha256(data).hexdigest() != item['sha256']:
                raise ValueError('release wheel mismatch: ' + name)
            files[name] = data
        publish(output, files)
    print(f'Verified {len(files)} release wheels: {output}')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, required=True, help='new wheelhouse directory')
    parser.add_argument('--artifact-cache', type=Path, help='offline directory containing all upstream artifacts')
    args = parser.parse_args()
    try:
        materialize(args.output.absolute(), args.artifact_cache)
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        raise SystemExit(f'REFUSED: {error}') from error


if __name__ == '__main__':
    main()
