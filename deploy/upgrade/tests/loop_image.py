#!/usr/bin/python3
"""Opt-in P14-layout sparse loop image acceptance. Own scratch only; no reboot."""
import hashlib
import importlib.machinery
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

if os.environ.get('NGFW_INTEGRATION') != '1' or os.geteuid() != 0:
    sys.exit('requires NGFW_INTEGRATION=1 and root')
BASE = Path(__file__).resolve().parents[1]
REPO = BASE.parents[1]
loader = importlib.machinery.SourceFileLoader('upgrade', str(BASE / 'ngfw-upgrade'))
spec = importlib.util.spec_from_loader(loader.name, loader)
u = importlib.util.module_from_spec(spec); loader.exec_module(u)
# Reuse the same signed-bundle factory as adversarial unit tests; no debootstrap,
# packages, firmware or host services are modified.
sys.path.insert(0, str(BASE / 'tests'))
from test_upgrade import Verification, root_tar


def evidence():
    boot = Path('/boot/grub/grubenv')
    return {'lsblk': u.run('lsblk', '--json', '-o', 'NAME,TYPE,SIZE,MOUNTPOINTS').decode(),
            'host_grubenv_sha256': hashlib.sha256(boot.read_bytes()).hexdigest() if boot.exists() else 'absent'}


scratch = REPO / '.scratch'
scratch.mkdir(exist_ok=True)
work = Path(tempfile.mkdtemp(prefix='w15-ab-', dir=scratch))
before = evidence()
print('host-before:', json.dumps(before, sort_keys=True))
loop = None
mounts = []
try:
    image = work / 'disk.img'
    with open(image, 'wb') as f:
        f.truncate(96 * 1024**3)
    layout = '''label: gpt
unit: sectors
start=2048,size=2048,type=21686148-6449-6e6f-744e-656564454649,name="bios-grub"
size=2097152,type=U,name="NGFW-EFI"
size=41943040,type=L,name="ngfw-rootA"
size=41943040,type=L,name="ngfw-rootB"
size=41943040,type=L,name="ngfw-log"
size=41943040,type=L,name="ngfw-pg"
type=L,name="ngfw-data"
'''
    subprocess.run(['sfdisk', str(image)], input=layout.encode(), check=True, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE)
    loop = u.run('losetup', '--find', '--show', '--partscan', str(image)).decode().strip()
    devices = {'A': loop + 'p3', 'B': loop + 'p4', 'efi': loop + 'p2', 'log': loop + 'p5', 'pg': loop + 'p6', 'data': loop + 'p7'}
    for name in ('A', 'efi', 'log', 'pg', 'data'):
        label = {'A': 'ngfw-rootA', 'efi': 'NGFW-EFI', 'log': 'ngfw-log', 'pg': 'ngfw-pg', 'data': 'ngfw-data'}[name]
        # EFI ext4 in this no-firmware harness only, as documented in the task;
        # production provisioning explicitly requires vfat.
        u.run('mkfs.ext4', '-q', '-F', '-E', 'lazy_itable_init=1,lazy_journal_init=1', '-L', label, devices[name])
    root = work / 'root'; root.mkdir()
    u.run('mount', devices['A'], str(root)); mounts.append(root)
    for name, mountpoint in [('efi', 'boot/efi'), ('data', 'data'), ('pg', 'var/lib/postgresql'), ('log', 'var/log')]:
        target = root / mountpoint; target.mkdir(parents=True)
        u.run('mount', devices[name], str(target)); mounts.append(target)
    (root / 'var/lib/ngfw').mkdir(parents=True)
    (root / 'data/ngfw').mkdir(mode=0o750)
    u.run('mount', '--bind', str(root / 'data/ngfw'), str(root / 'var/lib/ngfw')); mounts.append(root / 'var/lib/ngfw')
    (root / 'etc/ngfw').mkdir(parents=True)
    (root / 'etc/ngfw/appliance').write_text('ngfw\n')
    config = {'format': 1, 'root_devices': {'A': devices['A'], 'B': devices['B']},
              'efi_device': devices['efi'], 'data_device': devices['data'], 'pg_device': devices['pg'], 'log_device': devices['log']}
    (root / 'etc/ngfw/upgrade.json').write_bytes(u.json_bytes(config))
    (root / 'etc/machine-id').write_text('0123456789abcdef0123456789abcdef\n')
    (root / 'etc/hostname').write_text('w15-ab-image\n')
    updates = root / 'data/updates'; updates.mkdir()
    factory = Verification(); factory.setUp()
    try:
        (root / 'etc/ngfw/upgrade-signing.pub').write_bytes(factory.pub.read_bytes())
        app = u.Appliance(root)
        app.env.parent.mkdir(parents=True)
        app.grub('create'); app.grub('set', 'saved_entry=ngfw-A')
        app.save({'format': 1, 'versions': {'A': '1.0.0', 'B': None}, 'default_slot': 'A', 'pending_slot': None, 'staged_slot': None})
        # Prove untrusted input does not format or change the inactive device.
        with open(devices['B'], 'rb') as f:
            prior = f.read(4096)
        unsigned = updates / 'unsigned.tar'; shutil.copy2(factory.bundle(signed=False), unsigned)
        try:
            app.stage(unsigned)
            raise AssertionError('unsigned stage succeeded')
        except u.Refused:
            pass
        with open(devices['B'], 'rb') as f:
            assert f.read(4096) == prior
        bundle = updates / 'ngfw-update-1.1.0.tar'; shutil.copy2(factory.bundle(), bundle)
        # A changed compressed byte must be refused before inactive-slot writes.
        import tarfile, io
        with tarfile.open(bundle) as tf:
            payloads = {m.name: tf.extractfile(m).read() for m in tf}
        changed = bytearray(payloads['rootfs.tar.zst']); changed[len(changed) // 2] ^= 1
        payloads['rootfs.tar.zst'] = changed
        tampered = updates / 'tampered.tar'
        with tarfile.open(tampered, 'w') as tf:
            for name, raw in payloads.items():
                m = tarfile.TarInfo(name); m.size = len(raw); tf.addfile(m, io.BytesIO(raw))
        try:
            app.stage(tampered)
            raise AssertionError('tampered stage succeeded')
        except u.Refused:
            pass
        with open(devices['B'], 'rb') as f:
            assert f.read(4096) == prior
        print('PASS: unsigned and one-byte-tampered bundles refused; inactive device unchanged')
        app.stage(bundle)
        assert app.state()['staged_slot'] == 'B'
        target = work / 'inspect'; target.mkdir()
        u.run('mount', devices['B'], str(target)); mounts.append(target)
        fstab = (target / 'etc/fstab').read_text()
        assert ' / ext4 ' in fstab and '/data/ngfw /var/lib/ngfw none bind' in fstab
        assert (target / 'etc/machine-id').read_bytes() == (root / 'etc/machine-id').read_bytes()
        assert json.loads((target / 'etc/ngfw/staged-manifest.json').read_text())['version'] == '1.1.0'
        assert (target / 'etc/systemd/system/ngfw-api.service.requires/ngfw-upgrade-prepare.service').is_symlink()
        u.run('umount', str(target)); mounts.remove(target)
        app.activate()
        assert 'next_entry=ngfw-B' in app.grub('list').decode()
        assert 'saved_entry=ngfw-A' in app.grub('list').decode()
        app.grub('unset', 'next_entry')
        app.active = 'B'  # simulation only; no host root/boot mutation
        state = app.state(); state['migration_backup'] = '/data/test.dump'; app.save(state)
        app.confirm()
        assert 'saved_entry=ngfw-B' in app.grub('list').decode()
        # Second trial fails: old default A remains through explicit rollback.
        app.grub('set', 'saved_entry=ngfw-A')
        app.active = 'A'
        state = app.state(); state.update(default_slot='A', staged_slot='B', pending_slot=None); app.save(state)
        app.activate(); app.active = 'B'; app.rollback()
        assert 'saved_entry=ngfw-A' in app.grub('list').decode()
        assert 'next_entry=' not in app.grub('list').decode()
        print('PASS: stage/fstab/identity/manifest -> one-shot B -> confirm B; failed health -> default A')
    finally:
        factory.doCleanups()
finally:
    for target in reversed(mounts):
        u.run('umount', str(target))
    if loop:
        u.run('losetup', '--detach', loop)
    shutil.rmtree(work)
    after = evidence()
    print('host-after:', json.dumps(after, sort_keys=True))
    if before != after:
        raise AssertionError('host block layout/mounts or grubenv changed')
    owned = [line for line in u.run('losetup', '-a').decode().splitlines() if str(work) in line]
    if owned:
        raise AssertionError('owned loop device leaked')
    print('PASS: host lsblk/grubenv unchanged; no owned loop devices remain')
