#!/usr/bin/env python3
"""Stage baseline into an offline appliance root; never apply runtime controls."""
import argparse
import json
import os
from pathlib import Path
import re
import shutil
import subprocess

SOURCE = Path(__file__).resolve().parent


def safe_target(root, relative):
    target = root / relative
    current = root
    for part in Path(relative).parts:
        current = current / part
        if current.is_symlink():
            raise ValueError('symlink destination refused')
    target.parent.mkdir(parents=True, exist_ok=True)
    return target


def stage(root, ssh_key_only=False, management_interface=None):
    root = Path(root).absolute()
    if root == Path('/') or root.is_symlink() or not root.is_dir():
        raise ValueError('an existing offline root is required')
    # Parent aliases into / are refused too.
    if root.resolve() == Path('/') or root.resolve() != root:
        raise ValueError('root must be a canonical offline path')
    # Preserve prior opt-in controls across repeat staging; omission does not
    # silently weaken an appliance baseline. Refuse stale or forged selections.
    manifest = root / 'etc/ngfw/hardening.json'
    for path in [root / 'etc', root / 'etc/ngfw', manifest]:
        if path.is_symlink():
            raise ValueError('symlink manifest refused')
    if manifest.exists():
        try:
            previous = json.loads(manifest.read_text())
        except (OSError, ValueError):
            raise ValueError('invalid existing hardening manifest') from None
        if not isinstance(previous, dict) or type(previous.get('ssh_key_only')) is not bool:
            raise ValueError('invalid prior SSH selection')
        prior_interface = previous.get('management_interface')
        if prior_interface is not None and not isinstance(prior_interface, str):
            raise ValueError('invalid prior management interface')
        ssh_key_only = ssh_key_only or previous['ssh_key_only']
        management_interface = management_interface or prior_interface
    if management_interface and (management_interface in ['all', 'default', 'lo'] or
                                 not re.fullmatch(r'[A-Za-z0-9_-]{1,15}', management_interface)):
        raise ValueError('invalid management interface')
    if not ssh_key_only and (root / 'etc/ssh/sshd_config.d/60-ngfw.conf').exists():
        raise ValueError('existing SSH control requires explicit adoption')
    if not management_interface and (root / 'etc/sysctl.d/61-ngfw-management.conf').exists():
        raise ValueError('existing management control requires explicit adoption')
    files = {
        'baseline/60-ngfw.conf': 'etc/sysctl.d/60-ngfw.conf',
        'baseline/60-ngfw-modules.conf': 'etc/modprobe.d/60-ngfw-modules.conf',
    }
    if ssh_key_only:
        key = root / 'root/.ssh/authorized_keys'
        if (root / 'root').is_symlink() or (root / 'root/.ssh').is_symlink():
            raise ValueError('symlink authorized_keys refused')
        if key.is_symlink() or not key.is_file() or not key.read_text().strip():
            raise ValueError('provision root authorized_keys before disabling password SSH')
        result = subprocess.run(['ssh-keygen', '-l', '-f', str(key)],
                                stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        if result.returncode != 0:
            raise ValueError('authorized_keys contains no valid public key')
        files['baseline/60-ngfw-ssh.conf'] = 'etc/ssh/sshd_config.d/60-ngfw.conf'
    for source in (SOURCE / 'systemd').glob('*.service.d/*.conf'):
        files[str(source.relative_to(SOURCE))] = 'usr/lib/systemd/system/' + str(source.relative_to(SOURCE / 'systemd'))
    # Validate all existing destination components before writing any file.
    for target in [*files.values(), 'etc/ngfw/hardening.json', 'etc/sysctl.d/61-ngfw-management.conf']:
        current = root
        for part in Path(target).parts:
            current /= part
            if current.is_symlink():
                raise ValueError('symlink destination refused')
    for source, target in files.items():
        dst = safe_target(root, target)
        shutil.copyfile(SOURCE / source, dst)
        os.chmod(dst, 0o644)
    controls = {'ssh_key_only': ssh_key_only, 'management_interface': management_interface}
    if management_interface:
        if not re.fullmatch(r'[A-Za-z0-9_-]{1,15}', management_interface):
            raise ValueError('invalid management interface')
        # This is explicit input, never all/default. linux-cp taps must not be supplied.
        target = safe_target(root, 'etc/sysctl.d/61-ngfw-management.conf')
        target.write_text('net.ipv4.conf.' + management_interface + '.rp_filter = 2\n')
    safe_target(root, 'etc/ngfw/hardening.json').write_text(json.dumps(controls) + '\n')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--root', required=True)
    parser.add_argument('--ssh-key-only', action='store_true')
    parser.add_argument('--management-interface')
    args = parser.parse_args()
    try:
        stage(args.root, args.ssh_key_only, args.management_interface)
    except (ValueError, OSError) as error:
        parser.exit(1, str(error) + '\n')


if __name__ == '__main__':
    main()
