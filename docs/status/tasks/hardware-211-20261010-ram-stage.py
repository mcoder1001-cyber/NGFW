#!/usr/bin/env python3
"""Authorized reversible .211 RAM staging only; never transition or repair."""
import glob
import hashlib
import json
import os
import pathlib
import shutil
import socket
import stat
import subprocess

STAGE = pathlib.Path('/run/ngfwrescue')
REPORT = {'phase': 'RAM staging', 'commands': []}
os.umask(0o077)


def command(argv, check=True, **kwargs):
    p = subprocess.run(argv, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                       text=True, **kwargs)
    REPORT['commands'].append({'argv': argv, 'exit': p.returncode,
                               'stdout': p.stdout, 'stderr': p.stderr})
    if check and p.returncode:
        raise RuntimeError('command failed: ' + argv[0])
    return p


def write(relative, data, mode=0o644):
    path = STAGE / relative.lstrip('/')
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(data)
    path.chmod(mode)


def digest(path):
    h = hashlib.sha256()
    with open(path, 'rb') as f:
        for chunk in iter(lambda: f.read(1048576), b''):
            h.update(chunk)
    return h.hexdigest()


def network_snapshot():
    result = {}
    for argv in [['ip', '-j', 'address'], ['ip', '-j', 'link'],
                 ['ip', '-j', '-4', 'route', 'show', 'table', 'all'],
                 ['ip', '-j', '-6', 'route', 'show', 'table', 'all'],
                 ['ip', '-j', '-4', 'rule'], ['ip', '-j', '-6', 'rule']]:
        p = subprocess.run(argv, capture_output=True, check=True)
        # Link statistics may advance; compare only persistent topology fields.
        value = json.loads(p.stdout)
        if argv[2] == 'address':
            for item in value:
                for address in item.get('addr_info', []):
                    address.pop('valid_life_time', None)
                    address.pop('preferred_life_time', None)
        if argv[2] == 'link':
            for item in value:
                item.pop('stats64', None)
                item.pop('stats', None)
        result[' '.join(argv)] = value
    return hashlib.sha256(json.dumps(result, sort_keys=True).encode()).hexdigest()


def main():
    assert not os.path.lexists('/run/nextroot'), 'nextroot must remain absent'
    assert not STAGE.exists(), 'refusing to overwrite an existing candidate'
    assert os.geteuid() == 0
    assert '259.5-0ubuntu3' in command(['dpkg-query', '-W', '-f=${Version}', 'systemd']).stdout
    assert command(['findmnt', '-no', 'SOURCE,FSTYPE', '/']).stdout.strip() == '/dev/sda2 ext4'
    assert command(['ip', '-j', 'address', 'show', 'enp4s0']).stdout.find('172.30.110.211') >= 0
    assert not pathlib.Path('/run/systemd/system/ngfw-rescue.service').exists()
    assert not pathlib.Path('/run/systemd/transient/ngfw-rescue.service').exists()
    assert not pathlib.Path('/run/systemd/system.control/ngfw-rescue.service').exists()
    assert not pathlib.Path('/etc/systemd/system.control/ngfw-rescue.service').exists()
    assert not glob.glob('/run/systemd/system/ngfw-rescue.service.d/*')
    assert not glob.glob('/etc/systemd/system/ngfw-rescue.service.d/*')
    with socket.socket() as s:
        s.bind(('172.30.110.211', 2222))
    REPORT['network_before_hash'] = network_snapshot()

    mount_unit = '''[Unit]
Description=NGFW reviewed RAM rescue staging
DefaultDependencies=no

[Mount]
What=tmpfs
Where=/run/ngfwrescue
Type=tmpfs
Options=size=8G,mode=0755,nosuid,nodev,exec
'''
    unit = pathlib.Path('/run/systemd/system/run-ngfwrescue.mount')
    assert not unit.exists()
    STAGE.mkdir(mode=0o755)
    unit.write_text(mount_unit)
    unit.chmod(0o644)
    command(['systemctl', 'daemon-reload'])
    command(['systemctl', 'start', 'run-ngfwrescue.mount'])
    fs = json.loads(command(['findmnt', '-J', '--target', str(STAGE)]).stdout)['filesystems'][0]
    assert fs['target'] == str(STAGE) and fs['fstype'] == 'tmpfs'
    assert 'noexec' not in fs['options'].split(',')
    REPORT['mount'] = fs
    for relative, mode in [('root', 0o700), ('root/.ssh', 0o700),
                           ('etc/ssh', 0o700), ('run/sshd', 0o755),
                           ('var/log', 0o700), ('dev/pts', 0o755),
                           ('proc', 0o755), ('sys', 0o755), ('tmp', 0o1777)]:
        (STAGE / relative).mkdir(parents=True, exist_ok=True, mode=mode)
        (STAGE / relative).chmod(mode)

    copied = set()
    def copy_runtime(source):
        if source in copied:
            return
        assert pathlib.Path(source).is_file(), 'missing runtime ' + source
        target = STAGE / source.lstrip('/')
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(source, target, follow_symlinks=True)
        copied.add(source)
        p = subprocess.run(['ldd', source], capture_output=True, text=True)
        assert 'not found' not in p.stdout, 'unresolved dependency ' + source
        for line in p.stdout.splitlines():
            for token in line.split():
                if token.startswith('/') and pathlib.Path(token).is_file():
                    copy_runtime(token)

    binaries = ['/usr/lib/systemd/systemd', '/usr/lib/systemd/systemd-executor',
                '/usr/lib/systemd/systemd-shutdown', '/usr/sbin/sshd',
                '/usr/lib/openssh/sshd-session', '/usr/lib/openssh/sshd-auth',
                '/usr/lib/openssh/sftp-server', '/usr/lib/initramfs-tools/bin/busybox']
    for name in ['bash', 'sh', 'systemctl', 'journalctl', 'ip', 'mount', 'umount',
                 'findmnt', 'lsblk', 'blkid', 'blockdev', 'e2fsck', 'e2image',
                 'e2undo', 'dumpe2fs', 'gzip', 'sha256sum', 'ps', 'stat', 'sync']:
        path = shutil.which(name)
        assert path, 'missing executable ' + name
        binaries.append(path)
    for source in binaries:
        copy_runtime(source)
    # OpenSSH uses these absolute shell paths; preserve their runtime closure.
    copy_runtime('/bin/bash')
    copy_runtime('/bin/sh')
    for name in glob.glob('/usr/lib/x86_64-linux-gnu/libnss_files.so*'):
        copy_runtime(name)
    (STAGE / 'sbin').mkdir(exist_ok=True)
    (STAGE / 'sbin/init').symlink_to('/usr/lib/systemd/systemd')
    (STAGE / 'usr/bin/busybox').symlink_to('/usr/lib/initramfs-tools/bin/busybox')
    applets = subprocess.run(['/usr/lib/initramfs-tools/bin/busybox', '--list'],
                             capture_output=True, text=True, check=True).stdout.splitlines()
    for name in ['cat', 'ls', 'cp', 'mv', 'rm', 'mkdir', 'rmdir', 'readlink',
                 'find', 'du', 'df', 'sleep', 'kill', 'awk', 'sed', 'grep',
                 'head', 'tail', 'wc', 'touch', 'chmod', 'chown', 'tee', 'tty', 'stty']:
        if name in applets and not (STAGE / 'usr/bin' / name).exists():
            (STAGE / 'usr/bin' / name).symlink_to('/usr/bin/busybox')
    for name, major, minor in [('null', 1, 3), ('zero', 1, 5),
                               ('random', 1, 8), ('urandom', 1, 9), ('tty', 5, 0)]:
        os.mknod(STAGE / 'dev' / name, stat.S_IFCHR | 0o666, os.makedev(major, minor))
    # nodev tmpfs prevents these nodes from becoming usable during staging;
    # use only a devtmpfs bind, never old-root files, for authenticated SSH.
    command(['mount', '--bind', '/dev', str(STAGE / 'dev')])
    command(['mount', '--make-private', str(STAGE / 'dev')])
    command(['mount', '--bind', '/dev/pts', str(STAGE / 'dev/pts')])
    command(['mount', '--make-private', str(STAGE / 'dev/pts')])
    command(['mount', '-t', 'proc', '-o', 'nosuid,nodev,noexec', 'proc', str(STAGE / 'proc')])
    command(['mount', '--make-private', str(STAGE / 'proc')])

    # Copy key material entirely on-target into RAM; never return key contents.
    credential_count = 0
    for source in ['/etc/ssh/ssh_host_rsa_key', '/etc/ssh/ssh_host_ecdsa_key',
                   '/etc/ssh/ssh_host_ed25519_key', '/root/.ssh/authorized_keys',
                   '/root/.ssh/authorized_keys2']:
        if pathlib.Path(source).is_file():
            target = STAGE / source.lstrip('/')
            shutil.copyfile(source, target)
            target.chmod(0o600)
            credential_count += 1
    assert (STAGE / 'root/.ssh/authorized_keys').stat().st_size > 0
    passwd = ['root:x:0:0:RAM rescue:/root:/bin/bash']
    group = ['root:x:0:']
    for line in pathlib.Path('/etc/passwd').read_text().splitlines():
        if line.split(':')[0] in ['sshd', '_sshd']:
            passwd.append(line)
    for line in pathlib.Path('/etc/group').read_text().splitlines():
        if line.split(':')[0] in ['sshd', '_sshd']:
            group.append(line)
    assert len(passwd) > 1, 'missing OpenSSH privilege-separation account'
    write('/etc/passwd', '\n'.join(passwd) + '\n')
    write('/etc/group', '\n'.join(group) + '\n')
    # Synthetic nonpassword shadow value; key-only sshd disallows passwords.
    write('/etc/shadow', 'root:x:0:0:99999:7:::\n', 0o600)
    write('/etc/nsswitch.conf', 'passwd: files\ngroup: files\nshadow: files\nhosts: files\n')
    write('/etc/hosts', '127.0.0.1 localhost\n::1 localhost\n')
    write('/etc/machine-id', '')
    shutil.copyfile('/etc/os-release', STAGE / 'etc/os-release')
    if pathlib.Path('/etc/ssh/moduli').is_file():
        shutil.copyfile('/etc/ssh/moduli', STAGE / 'etc/ssh/moduli')
    ssh_config = '''Port 2222
ListenAddress 172.30.110.211
HostKey /etc/ssh/ssh_host_ed25519_key
HostKey /etc/ssh/ssh_host_rsa_key
HostKey /etc/ssh/ssh_host_ecdsa_key
PidFile /run/sshd.pid
AuthorizedKeysFile /root/.ssh/authorized_keys /root/.ssh/authorized_keys2
PermitRootLogin prohibit-password
PubkeyAuthentication yes
PasswordAuthentication no
KbdInteractiveAuthentication no
AuthenticationMethods publickey
UsePAM no
StrictModes yes
PermitEmptyPasswords no
UseDNS no
PrintMotd no
PrintLastLog no
AllowUsers root
LogLevel VERBOSE
Subsystem sftp /usr/lib/openssh/sftp-server
'''
    write('/etc/ssh/sshd_config', ssh_config, 0o600)
    service_common = '''[Unit]
Description=NGFW RAM key-only management rescue SSH
DefaultDependencies=no
IgnoreOnIsolate=yes
SurviveFinalKillSignal=yes
After=basic.target
Before=shutdown.target rescue.target emergency.target
Conflicts=reboot.target kexec.target poweroff.target halt.target rescue.target emergency.target

[Service]
Type=exec
KillMode=control-group
Restart=on-failure
RestartSec=1s
'''
    # chroot itself changes cwd to RAM /. WorkingDirectory adds unwanted
    # implicit mount dependencies to the surviving bootstrap unit.
    runtime_unit = service_common + '''ExecStart=/usr/sbin/chroot /run/ngfwrescue /usr/sbin/sshd -D -e -f /etc/ssh/sshd_config
StandardOutput=append:/run/ngfwrescue/var/log/rescue-ssh.log
StandardError=inherit
'''
    candidate_unit = service_common + '''WorkingDirectory=/
RuntimeDirectory=sshd
RuntimeDirectoryMode=0755
ExecStart=/usr/sbin/sshd -D -e -f /etc/ssh/sshd_config
StandardOutput=append:/var/log/rescue-ssh.log
StandardError=inherit
'''
    write('/etc/systemd/system/ngfw-rescue.service', candidate_unit)
    write('/etc/systemd/system/basic.target', '''[Unit]
Description=Minimal inert RAM basic target
DefaultDependencies=no
''')
    write('/etc/systemd/system/ngfw-rescue.target', '''[Unit]
Description=NGFW RAM maintenance target
DefaultDependencies=no
Requires=ngfw-rescue.service
After=ngfw-rescue.service
AllowIsolate=yes
''')
    (STAGE / 'etc/systemd/system/default.target').symlink_to('ngfw-rescue.target')
    write('/var/log/rescue-ssh.log', '', 0o600)
    command(['/usr/sbin/chroot', str(STAGE), '/usr/sbin/sshd', '-t', '-f', '/etc/ssh/sshd_config'])
    command(['/usr/sbin/chroot', str(STAGE), '/usr/lib/systemd/systemd', '--version'])
    command(['systemd-analyze', 'verify', '--man=no', '--root=' + str(STAGE),
             'ngfw-rescue.target', 'ngfw-rescue.service'])
    path = pathlib.Path('/run/systemd/system/ngfw-rescue.service')
    path.write_text(runtime_unit)
    path.chmod(0o644)
    command(['systemd-analyze', 'verify', '--man=no', str(path)])
    command(['systemctl', 'daemon-reload'])
    command(['systemctl', 'start', 'ngfw-rescue.service'])
    REPORT['service_show'] = command(['systemctl', 'show', 'ngfw-rescue.service',
        '-p', 'MainPID', '-p', 'ActiveState', '-p', 'SubState', '-p', 'FragmentPath',
        '-p', 'DropInPaths', '-p', 'PrivateMounts', '-p', 'PrivateTmp', '-p', 'RootDirectory',
        '-p', 'ProtectSystem', '-p', 'SurviveFinalKillSignal', '-p', 'DefaultDependencies',
        '-p', 'Conflicts', '-p', 'Before', '-p', 'After']).stdout
    REPORT['units'] = {'mount': mount_unit, 'runtime': runtime_unit, 'candidate': candidate_unit}
    REPORT['credential_files_copied_on_target_only'] = credential_count
    REPORT['default_target'] = os.readlink(STAGE / 'etc/systemd/system/default.target')
    REPORT['generator_paths'] = [str(p) for pattern in ['usr/lib/systemd/system-generators/*',
        'etc/systemd/system-generators/*', 'run/systemd/system-generators/*',
        'usr/lib/systemd/system/*.service', 'etc/systemd/system/*.service']
        for p in STAGE.glob(pattern)]
    manifest = []
    for base, dirs, files in os.walk(STAGE):
        if pathlib.Path(base) in [STAGE / 'dev', STAGE / 'proc', STAGE / 'sys']:
            dirs.clear()
            continue
        for name in files:
            file = pathlib.Path(base) / name
            st = file.lstat()
            item = {'path': str(file.relative_to(STAGE)), 'mode': oct(stat.S_IMODE(st.st_mode)),
                    'bytes': st.st_size}
            if file.is_symlink():
                item['symlink'] = os.readlink(file)
            elif file.is_file():
                item['sha256'] = digest(file)
            manifest.append(item)
    REPORT['tree_manifest'] = sorted(manifest, key=lambda v: v['path'])
    REPORT['tree_manifest_hash'] = hashlib.sha256(json.dumps(REPORT['tree_manifest'],
        sort_keys=True).encode()).hexdigest()
    REPORT['network_after_hash'] = network_snapshot()
    assert REPORT['network_after_hash'] == REPORT['network_before_hash'], 'network differs'
    assert not os.path.lexists('/run/nextroot')
    REPORT['nextroot_absent'] = True
    REPORT['budget'] = command(['du', '-sx', '-B1', str(STAGE)]).stdout
    REPORT['phase'] = 'staged; authenticated controller SSH test pending'


try:
    main()
except Exception as exc:
    REPORT['error'] = type(exc).__name__ + ': ' + str(exc)
    print(json.dumps(REPORT, indent=2))
    raise SystemExit(1)
print(json.dumps(REPORT, indent=2))
