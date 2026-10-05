#!/usr/bin/env python3
"""Read-only baseline inspection; configuration PASS does not prove runtime enforcement."""
import argparse
import json
from pathlib import Path
import sys

SOURCE = Path(__file__).resolve().parent


def inspect(root):
    root = Path(root)
    checks = []
    def exact(control, path, expected):
        target = root / path
        passed = target.is_file() and not target.is_symlink() and target.read_text() == expected
        checks.append((control, 'PASS' if passed else 'FAIL', path))
    exact('CIS-inspired 1.5/5.1 kernel disclosure and filesystem protection',
          'etc/sysctl.d/60-ngfw.conf', (SOURCE / 'baseline/60-ngfw.conf').read_text())
    exact('CIS-inspired 1.1 unused filesystems', 'etc/modprobe.d/60-ngfw-modules.conf',
          (SOURCE / 'baseline/60-ngfw-modules.conf').read_text())
    for source in sorted((SOURCE / 'systemd').glob('*.service.d/*.conf')):
        exact('systemd ' + source.parent.name, 'usr/lib/systemd/system/' + str(source.relative_to(SOURCE / 'systemd')), source.read_text())
    manifest = root / 'etc/ngfw/hardening.json'
    try:
        config = json.loads(manifest.read_text())
    except (OSError, ValueError):
        config = {}
        checks.append(('baseline manifest', 'FAIL', str(manifest)))
    if config.get('ssh_key_only'):
        exact('CIS-inspired 5.1 SSH key-only', 'etc/ssh/sshd_config.d/60-ngfw.conf',
              (SOURCE / 'baseline/60-ngfw-ssh.conf').read_text())
    else:
        if (root / 'etc/ssh/sshd_config.d/60-ngfw.conf').exists():
            checks.append(('undeclared SSH control', 'FAIL', 'optional profile exists without manifest selection'))
        checks.append(('SSH key-only', 'EXCEPTION', 'bootstrap requires keys before password SSH is disabled'))
    interface = config.get('management_interface')
    if interface:
        import re
        if not isinstance(interface, str) or interface in ['all', 'default', 'lo'] or not re.fullmatch(r'[A-Za-z0-9_-]{1,15}', interface):
            checks.append(('management interface selection', 'FAIL', 'invalid manifest interface'))
        else:
            exact('management rp_filter', 'etc/sysctl.d/61-ngfw-management.conf',
                  'net.ipv4.conf.' + interface + '.rp_filter = 2\n')
    elif (root / 'etc/sysctl.d/61-ngfw-management.conf').exists():
        checks.append(('undeclared management control', 'FAIL', 'optional profile exists without manifest selection'))
    else:
        checks.append(('management rp_filter', 'EXCEPTION', 'not selected; no all/default or linux-cp changes'))
    for control, reason in [
        ('password quality/PAM', 'libpam-pwquality absent from pinned package set; no unauthorised package additions'),
        ('auditd', 'auditd absent from pinned package set; product mutation audit remains in API'),
        ('AppArmor', 'retain distribution profiles; runtime enforcement requires appliance acceptance'),
        ('cron/at', 'service/package removal belongs to pinned image package policy'),
        ('daemon runtime compatibility', 'offline drop-ins require per-daemon appliance smoke before release'),
    ]:
        checks.append((control, 'EXCEPTION', reason))
    return checks


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--root', default='/')
    p.add_argument('--json', action='store_true')
    a = p.parse_args()
    checks = inspect(a.root)
    if a.json:
        print(json.dumps([dict(control=c, status=s, evidence=e) for c, s, e in checks]))
    else:
        for control, status, evidence in checks:
            print(status + ' ' + control + ': ' + evidence)
    return int(any(status == 'FAIL' for _, status, _ in checks))


if __name__ == '__main__':
    sys.exit(main())
