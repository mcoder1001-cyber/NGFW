#!/usr/bin/env python3
"""Signed-cache SMART download/extract into existing RAM; no install/device writes."""
import hashlib
import json
import os
import pathlib
import shutil
import subprocess

os.umask(0o077)
ROOT = pathlib.Path('/run/ngfwrescue')
PRIVATE = ROOT / 'root/recovery-private/smart'
REPORT = {'commands': []}
CHROOT = shutil.which('chroot')


def run(argv, **kwargs):
    p = subprocess.run(argv, capture_output=True, text=True, **kwargs)
    REPORT['commands'].append({'argv': argv, 'exit': p.returncode,
                               'stdout': p.stdout, 'stderr': p.stderr})
    return p


def sha256(path):
    h = hashlib.sha256()
    with path.open('rb') as f:
        for chunk in iter(lambda: f.read(1048576), b''):
            h.update(chunk)
    return h.hexdigest()


def main():
    assert not os.path.lexists('/run/nextroot')
    assert CHROOT in ['/usr/bin/chroot', '/usr/sbin/chroot']
    mount = run(['findmnt', '-J', '--target', str(ROOT)])
    fs = json.loads(mount.stdout)['filesystems'][0]
    assert fs['fstype'] == 'tmpfs' and fs['target'] == str(ROOT)
    assert not PRIVATE.exists()
    PRIVATE.mkdir(parents=True, mode=0o700)
    (ROOT / 'root/recovery-private').chmod(0o700)
    integrity = run(['dpkg', '-V', 'ubuntu-keyring', 'libstdc++6', 'libgcc-s1'])
    assert integrity.returncode == 0
    assert not [s for s in integrity.stdout.splitlines()
                if not any(p in s for p in ['/usr/share/doc/', '/usr/share/man/', '/usr/share/lintian/'])]
    release = pathlib.Path('/var/lib/apt/lists/repo.amnafzar.ir_ubuntu_dists_resolute_InRelease')
    index = pathlib.Path('/var/lib/apt/lists/repo.amnafzar.ir_ubuntu_dists_resolute_main_binary-amd64_Packages')
    signature = run(['gpgv', '--keyring', '/usr/share/keyrings/ubuntu-archive-keyring.gpg', str(release)])
    assert signature.returncode == 0
    section = None
    expected_index = None
    for line in release.read_text().splitlines():
        if line == 'SHA256:':
            section = 'SHA256'
        elif section and not line.startswith(' '):
            section = None
        elif section and line.split()[-1] == 'main/binary-amd64/Packages':
            expected_index = line.split()[:2]
    assert expected_index
    assert index.stat().st_size == int(expected_index[1])
    assert sha256(index) == expected_index[0]
    packages = []
    for stanza in index.read_text().split('\n\n'):
        values = {}
        for line in stanza.splitlines():
            if line and not line.startswith(' ') and ':' in line:
                key, value = line.split(':', 1)
                values[key] = value.strip()
        if values.get('Package') == 'smartmontools' and values.get('Architecture') == 'amd64':
            packages.append(values)
    assert len(packages) == 1
    package = packages[0]
    REPORT['package'] = {k: package[k] for k in ['Package', 'Version', 'Architecture', 'Filename', 'Size', 'SHA256']}
    options = ['-o', 'Debug::NoLocking=true', '-o', 'APT::Sandbox::User=root',
               '-o', 'Dir::Cache=' + str(PRIVATE / 'apt-cache'),
               '-o', 'Dir::State=' + str(PRIVATE / 'apt-state'),
               '-o', 'Dir::State::status=/var/lib/dpkg/status',
               '-o', 'Dir::State::lists=/var/lib/apt/lists',
               '-o', 'Dir::Log=' + str(PRIVATE / 'apt-log')]
    downloaded = run(['apt-get'] + options + ['download', 'smartmontools=' + package['Version']], cwd=PRIVATE)
    assert downloaded.returncode == 0
    debs = list(PRIVATE.glob('*.deb'))
    assert len(debs) == 1
    deb = debs[0]
    assert deb.stat().st_size == int(package['Size']) and sha256(deb) == package['SHA256']
    extract = PRIVATE / 'extracted'
    extract.mkdir(mode=0o700)
    assert run(['dpkg-deb', '-x', str(deb), str(extract)]).returncode == 0
    binary = extract / 'usr/sbin/smartctl'
    target = ROOT / 'usr/sbin/smartctl'
    assert not target.exists()
    shutil.copy2(binary, target)
    linked = run(['ldd', str(binary)])
    assert 'not found' not in linked.stdout
    for line in linked.stdout.splitlines():
        for token in line.split():
            if token.startswith('/') and pathlib.Path(token).is_file():
                destination = ROOT / token.lstrip('/')
                destination.parent.mkdir(parents=True, exist_ok=True)
                if not destination.exists():
                    shutil.copy2(token, destination, follow_symlinks=True)
    database = extract / 'var/lib/smartmontools/drivedb/drivedb.h'
    if database.exists():
        destination = ROOT / 'var/lib/smartmontools/drivedb/drivedb.h'
        destination.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(database, destination)
    REPORT['binary_sha256'] = sha256(target)
    # -x is a read-only information query; never enable or start SMART tests.
    result = run([CHROOT, str(ROOT), '/usr/sbin/smartctl', '-x', '-j', '/dev/sda'])
    REPORT['smartctl_exit'] = result.returncode
    REPORT['smartctl_json'] = json.loads(result.stdout)
    assert not os.path.lexists('/run/nextroot')


try:
    main()
except Exception as exc:
    REPORT['error'] = type(exc).__name__ + ': ' + str(exc)
    print(json.dumps(REPORT, indent=2))
    raise SystemExit(1)
print(json.dumps(REPORT, indent=2))
