#!/usr/bin/env python3
"""Host-independent image plan, target configuration and release metadata."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import subprocess
import xml.etree.ElementTree as ET

HERE = Path(__file__).resolve().parent
COMMON = HERE.parent / 'iso/common'
PROFILES = json.loads((HERE.parent / 'cloud/profiles.json').read_text())
GIB = 1024 ** 3


def layout(size_gib):
    if size_gib < 96 or size_gib > 4096:
        raise ValueError('disk size must be 96..4096 GiB')
    rows = []
    for line in (COMMON / 'layout.tsv').read_text().splitlines():
        if line.strip() and not line.startswith('#'):
            label, size, fs, mount, role, _ = line.split()
            if not re.fullmatch(r'[A-Za-z0-9-]+', label):
                raise ValueError('invalid shared label')
            rows.append((label, size, fs, mount, role))
    start = 2048  # 1 MiB alignment; GPT BIOS boot partition first
    partitions = [{'start': start, 'size': 2048, 'type': '21686148-6449-6E6F-744E-656564454649', 'name': 'bios-boot'}]
    start += 2048
    for label, size, fs, mount, role in rows:
        sectors = (int(size[:-1]) * GIB // 512 if size != 'rest'
                   else size_gib * GIB // 512 - start - 2048)
        partitions.append({'start': start, 'size': sectors,
                           'type': 'U' if role == 'esp' else 'L',
                           'name': label, 'fstype': fs, 'mount': mount, 'role': role})
        start += sectors
    if [p.get('role') for p in partitions[1:]] != ['esp', 'root-active', 'root-reserved', 'log', 'pg', 'data']:
        raise ValueError('unexpected shared partition contract')
    return partitions


def partition_script(size):
    return 'label: gpt\nunit: sectors\n\n' + ''.join(
        f'start={p["start"]}, size={p["size"]}, type={p["type"]}, name="{p["name"]}"\n'
        for p in layout(size))


def mutation_root(root, paths=()):
    """Reject the host root and every existing symlink before image mutations."""
    root = Path(root).absolute()
    if root == Path('/') or root.resolve(strict=True) != root or not root.is_dir():
        raise ValueError('canonical offline root required')
    for relative in paths:
        path = Path(relative)
        if path.is_absolute() or '..' in path.parts:
            raise ValueError('relative image mutation path required')
        current = root
        for component in path.parts:
            current /= component
            if current.is_symlink():
                raise ValueError('symlink image mutation path refused: ' + str(relative))
    return root


def put(root, path, text):
    root = mutation_root(root, [path])
    target = root / path
    if target.exists() and not target.is_file():
        raise ValueError('regular image output file required')
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(text)


def configure(root, profile, management="eth0"):
    if not re.fullmatch(r"[A-Za-z0-9_][A-Za-z0-9_.-]{0,14}", management) or management == "lo":
        raise ValueError("invalid explicit management interface")
    # Preflight ALL write/unlink/symlink parents before any mutation. put() alone
    # cannot protect glob unlink operations or symlink creation in aliased dirs.
    writes = [
        'etc/ngfw/appliance', 'etc/ngfw/base-policy.env', 'etc/ngfw/image-profile',
        'etc/netplan/90-ngfw-management.yaml', 'var/lib/ngfw/firstboot.done',
        'etc/fstab', 'etc/cloud/cloud.cfg.d/90-ngfw-image.cfg',
        'etc/initramfs-tools/modules', 'etc/modules-load.d/ngfw-cloud.conf',
        'etc/machine-id', 'etc/systemd/system/ngfw-image-ssh-keys.service',
        'etc/systemd/system/multi-user.target.wants/ngfw-image-ssh-keys.service',
        'etc/ssh', 'var/lib/dbus',
    ]
    root = mutation_root(root, writes)
    p = PROFILES[profile]
    put(root, 'etc/ngfw/appliance', 'NGFW image appliance\n')
    put(root, 'etc/ngfw/base-policy.env', f'NGFW_BOOTSTRAP_MGMT_IF={management}\nNGFW_BOOTSTRAP_PUNT_IFS=\n')
    (root / 'etc/ngfw/base-policy.env').chmod(0o600)
    put(root, 'etc/netplan/90-ngfw-management.yaml', f'network:\n  version: 2\n  renderer: networkd\n  ethernets:\n    mgmt:\n      match:\n        name: {json.dumps(management)}\n      dhcp4: true\n      dhcp6: true\n')
    (root / 'etc/netplan/90-ngfw-management.yaml').chmod(0o600)
    # P14 checks firstboot.done, current P10 publishes firstboot-complete atomically.
    marker = root / 'var/lib/ngfw/firstboot.done'
    marker.parent.mkdir(parents=True, exist_ok=True)
    marker.symlink_to('firstboot-complete')
    fstab = '# Shared P14 partition labels; rootB is reserved for A/B upgrades.\n'
    for part in layout(96)[1:]:
        if part['mount'] != '-':
            fs = 'vfat' if part['fstype'] == 'fat32' else 'ext4'
            options = 'umask=0077' if fs == 'vfat' else 'defaults'
            check = 1 if part['mount'] == '/' else 2
            fstab += f'LABEL={part["name"]} {part["mount"]} {fs} {options} 0 {check}\n'
    put(root, 'etc/fstab', fstab)
    put(root, 'etc/cloud/cloud.cfg.d/90-ngfw-image.cfg',
        '# Credentials are generated by P14 firstboot; root SSH and password SSH stay disabled.\n'
        + 'datasource_list: ' + json.dumps(p['datasources']) + '\n'
        + 'users: []\ndisable_root: true\nssh_pwauth: false\n'
        + 'preserve_hostname: false\nssh_deletekeys: true\n'
        + 'network: {config: disabled}\n')
    # Load modules early without binding dataplane NICs. Never enable cloud DPDK here.
    put(root, 'etc/initramfs-tools/modules', '\n'.join(p['modules']) + '\n')
    put(root, 'etc/modules-load.d/ngfw-cloud.conf', '\n'.join(p['modules']) + '\n')
    put(root, 'etc/machine-id', '')
    (root / 'var/lib/dbus/machine-id').unlink(missing_ok=True)
    for path in (root / 'etc/ssh').glob('ssh_host_*'):
        path.unlink()
    # Dedicated regeneration ensures this does not depend on cloud-init's optional SSH module.
    put(root, 'etc/systemd/system/ngfw-image-ssh-keys.service', '''[Unit]
Description=Generate unique NGFW image SSH host keys
After=systemd-random-seed.service
Before=ssh.service ssh.socket
ConditionPathExists=!/etc/ssh/ssh_host_ed25519_key
[Service]
Type=oneshot
ExecStart=/usr/bin/ssh-keygen -A
[Install]
WantedBy=multi-user.target
''')
    wants = root / 'etc/systemd/system/multi-user.target.wants'
    wants.mkdir(parents=True, exist_ok=True)
    (wants / 'ngfw-image-ssh-keys.service').symlink_to('/etc/systemd/system/ngfw-image-ssh-keys.service')
    put(root, 'etc/ngfw/image-profile', profile + '\n')


def grub_config(root):
    root = mutation_root(root, ['boot', 'boot/grub/grub.cfg'])
    kernels = sorted((root / 'boot').glob('vmlinuz-*'))
    if len(kernels) != 1:
        raise ValueError('expected exactly one release kernel')
    kernel = kernels[0].name
    initrd = 'initrd.img-' + kernel.removeprefix('vmlinuz-')
    if not (root / 'boot' / initrd).is_file():
        raise ValueError('release initramfs absent')
    args = [l.strip() for l in (COMMON / 'kernel/cmdline').read_text().splitlines()
            if l.strip() and not l.startswith('#')]
    if any(not re.fullmatch(r'[A-Za-z0-9_.,:=/-]+', x) for x in args):
        raise ValueError('unsafe shared kernel argument')
    if not re.fullmatch(r'vmlinuz-[A-Za-z0-9.+~-]+', kernel):
        raise ValueError('unsafe kernel filename')
    put(root, 'boot/grub/grub.cfg', f'''set timeout=3
serial --unit=0 --speed=115200
terminal_input console serial
terminal_output console serial
menuentry 'NGFW rootA' {{
  search --no-floppy --label ngfw-rootA --set=root
  linux /boot/{kernel} root=LABEL=ngfw-rootA ro {' '.join(args)} console=tty0 console=ttyS0,115200n8
  initrd /boot/{initrd}
}}
''')


def validate(root, profile):
    root = root.resolve(strict=True)
    errors = []
    def need(condition, message):
        if not condition:
            errors.append(message)
    for part in layout(96)[1:]:
        line = f'LABEL={part["name"]} {part["mount"]} '
        fstab = (root / 'etc/fstab').read_text()
        need(line in fstab if part['mount'] != '-' else line not in fstab, 'fstab ' + part['name'])
    need((root / 'etc/machine-id').read_bytes() == b'', 'machine-id must be empty')
    need(not list((root / 'etc/ssh').glob('ssh_host_*')), 'SSH host keys present')
    for path in ['etc/ngfw/bootstrap.env', 'var/lib/ngfw/firstboot.done',
                 'var/lib/ngfw-image/console/bootstrap-password', 'var/lib/ngfw/firstboot-complete', 'etc/ngfw/api.env', 'var/lib/ngfw/secret.key',
                 'root/.ssh', 'root/.gnupg']:
        need(not (root / path).exists(), 'build credential/state present: ' + path)
    marker = root / 'var/lib/ngfw/firstboot.done'
    need(marker.is_symlink() and marker.readlink() == Path('firstboot-complete'), 'P14/P10 completion bridge missing')
    policy = (root / 'etc/ngfw/base-policy.env').read_text()
    need(bool(re.fullmatch(r'NGFW_BOOTSTRAP_MGMT_IF=[A-Za-z0-9_][A-Za-z0-9_.-]{0,14}\nNGFW_BOOTSTRAP_PUNT_IFS=\n', policy)), 'explicit management policy absent')
    need((root / 'etc/ngfw/base-policy.env').stat().st_mode & 0o777 == 0o600, 'management policy permissions')
    cfg = (root / 'etc/cloud/cloud.cfg.d/90-ngfw-image.cfg').read_text()
    need('datasource_list: ' + json.dumps(PROFILES[profile]['datasources']) in cfg, 'datasource mismatch')
    need('ssh_pwauth: false' in cfg and 'users: []' in cfg, 'unsafe cloud authentication defaults')
    for path in ['boot/grub/grub.cfg', 'boot/efi/EFI/BOOT/BOOTX64.EFI', 'etc/ngfw/appliance',
                 'usr/lib/ngfw-image/ngfw-bootstrap-password', 'usr/sbin/nft',
                 'etc/systemd/system/ngfw-bootstrap-password.service',
                 'etc/systemd/system/ngfw-image-ssh-keys.service',
                 'usr/lib/systemd/system/ngfw-firstboot.service', 'usr/lib/systemd/system/ngfw-firewall-bootstrap.service',
                 'usr/lib/systemd/system/ngfw-agent.service', 'usr/lib/systemd/system/ngfw-api.service',
                 'usr/lib/systemd/system/nftables.service']:
        need((root / path).is_file(), 'required image file absent: ' + path)
    grub = (root / 'boot/grub/grub.cfg').read_text()
    need('root=LABEL=ngfw-rootA' in grub and 'console=ttyS0,115200n8' in grub, 'invalid GRUB root/console')
    result = subprocess.run(['dpkg-query', '--admindir=' + str(root / 'var/lib/dpkg'),
                             '-W', '-f=${Package}\t${Version}\t${db:Status-Status}\n'],
                            check=True, capture_output=True, text=True)
    packages = dict((row[0], row[1]) for line in result.stdout.splitlines()
                    if len(row := line.split('\t')) == 3 and row[2] == 'installed')
    for name in ['ngfw-meta', 'cloud-init', 'openssh-server', 'nftables', 'grub-pc-bin', 'grub-efi-amd64-bin']:
        need(name in packages, 'package missing: ' + name)
    need(packages.get('vpp', '').startswith('26.06'), 'VPP 26.06 missing')
    # No active passwords, only system accounts locked with !/*; no default login.
    for line in (root / 'etc/shadow').read_text().splitlines():
        fields = line.split(':')
        need(len(fields) > 1 and fields[1].startswith(('!', '*')), 'unlocked password: ' + fields[0])
    if errors:
        raise ValueError('; '.join(errors))
    return {'profile': profile, 'packages': packages, 'offline_inspection': 'passed'}


def digest(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def ovf(output, disk, size):
    ns = {'ovf': 'http://schemas.dmtf.org/ovf/envelope/1',
          'rasd': 'http://schemas.dmtf.org/wbem/wscim/1/cim-schema/2/CIM_ResourceAllocationSettingData',
          'vssd': 'http://schemas.dmtf.org/wbem/wscim/1/cim-schema/2/CIM_VirtualSystemSettingData',
          'vmw': 'http://www.vmware.com/schema/ovf'}
    for key, value in ns.items():
        ET.register_namespace(key, value)
    def node(parent, name, attrs=None, text=None):
        tag = name.split(':'); qualified = '{' + ns[tag[0]] + '}' + tag[1]
        attributes = {('{' + ns[k.split(':')[0]] + '}' + k.split(':')[1] if ':' in k else k): str(v)
                      for k, v in (attrs or {}).items()}
        e = ET.SubElement(parent, qualified, attributes); e.text = text; return e
    env = ET.Element('{' + ns['ovf'] + '}Envelope')
    refs = node(env, 'ovf:References')
    node(refs, 'ovf:File', {'ovf:id': 'diskfile', 'ovf:href': disk.name, 'ovf:size': disk.stat().st_size})
    disks = node(env, 'ovf:DiskSection'); node(disks, 'ovf:Info', text='NGFW disk')
    node(disks, 'ovf:Disk', {'ovf:diskId': 'disk1', 'ovf:fileRef': 'diskfile', 'ovf:capacity': size,
                            'ovf:capacityAllocationUnits': 'byte',
                            'ovf:format': 'http://www.vmware.com/interfaces/specifications/vmdk.html#streamOptimized'})
    net = node(env, 'ovf:NetworkSection'); node(net, 'ovf:Info', text='Management network')
    node(net, 'ovf:Network', {'ovf:name': 'Management'})
    vm = node(env, 'ovf:VirtualSystem', {'ovf:id': 'NGFW'}); node(vm, 'ovf:Info', text='NGFW Ubuntu 26.04')
    hw = node(vm, 'ovf:VirtualHardwareSection'); node(hw, 'ovf:Info', text='4 CPUs, 16 GiB RAM')
    system = node(hw, 'ovf:System'); node(system, 'vssd:VirtualSystemIdentifier', text='NGFW')
    node(system, 'vssd:VirtualSystemType', text='vmx-17')
    def item(instance, resource, values):
        obj = node(hw, 'ovf:Item'); node(obj, 'rasd:InstanceID', text=str(instance)); node(obj, 'rasd:ResourceType', text=str(resource))
        for name, text in values.items():
            node(obj, 'rasd:' + name, text=str(text))
    item(1, 3, {'VirtualQuantity': 4})
    item(2, 4, {'AllocationUnits': 'byte * 2^20', 'VirtualQuantity': 16384})
    item(3, 6, {'ResourceSubType': 'VirtualSCSI', 'Address': 0})
    item(4, 17, {'Parent': 3, 'AddressOnParent': 0, 'HostResource': 'ovf:/disk/disk1'})
    item(5, 10, {'ResourceSubType': 'VmxNet3', 'Connection': 'Management', 'AutomaticAllocation': 'true'})
    node(hw, 'vmw:Config', {'ovf:required': 'false', 'vmw:key': 'firmware', 'vmw:value': 'efi'})
    ET.ElementTree(env).write(output, encoding='utf-8', xml_declaration=True)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('action', choices=['partition', 'configure', 'grub', 'inspect', 'ovf', 'manifest'])
    parser.add_argument('--size', type=int, default=96)
    parser.add_argument('--root', type=Path)
    parser.add_argument('--profile', choices=PROFILES, default='vm')
    parser.add_argument('--management-interface', default='eth0')
    parser.add_argument('--disk', type=Path)
    parser.add_argument('--output', type=Path)
    parser.add_argument('--vpp-manifest', type=Path)
    a = parser.parse_args()
    if a.action == 'partition': print(partition_script(a.size), end='')
    elif a.action == 'configure': configure(a.root, a.profile, a.management_interface)
    elif a.action == 'grub': grub_config(a.root)
    elif a.action == 'inspect': print(json.dumps(validate(a.root, a.profile), indent=2, sort_keys=True))
    elif a.action == 'ovf': ovf(a.output, a.disk, a.size * GIB)
    elif a.action == 'manifest':
        inspection = json.loads((a.output / 'inspection.json').read_text())
        artifacts = {p.name: digest(p) for p in sorted(a.output.iterdir())
                     if p.is_file() and p.name not in ['manifest.json', 'SHA256SUMS']}
        m = {'schema': 'ngfw.images/v1', 'size_bytes': a.size * GIB,
             'profile': a.profile, 'packages': inspection['packages'],
             'vpp_provenance': json.loads(a.vpp_manifest.read_text()),
             'shared_layout_sha256': digest(COMMON / 'layout.tsv'), 'artifacts': artifacts}
        put(a.output, 'manifest.json', json.dumps(m, indent=2, sort_keys=True) + '\n')
        artifacts['manifest.json'] = digest(a.output / 'manifest.json')
        put(a.output, 'SHA256SUMS', ''.join(f'{value}  {name}\n' for name, value in sorted(artifacts.items())))


if __name__ == '__main__':
    main()
