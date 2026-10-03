#!/usr/bin/env python3
"""deploy/image/iso/lib/render.py — pure text renderers of the NGFW ISO build (P14), shared by build-iso.sh and tests.

  render.py user-data <user-data.in> <storage.yaml> <out> <version> <source-id>
  render.py grub <base grub.cfg> <out> <version> <extra kernel args>

Standard library only (no PyYAML needed to render; the build validates the result separately).
"""
import re
import sys

PLACEHOLDERS = ('@STORAGE@', '@NGFW_VERSION@', '@SOURCE_ID@')


def user_data(tpl, storage, ver, src):
    if not re.fullmatch(r'[0-9A-Za-z.+~-]+', ver):
        raise SystemExit('odd version %r' % ver)
    if src not in ('ubuntu-server-minimal', 'ubuntu-server'):
        raise SystemExit('odd source %r' % src)
    body = ''.join('  ' + l for l in storage.splitlines(True) if l.strip() and not l.lstrip().startswith('#'))
    if body and not body.endswith('\n'):
        body += '\n'
    s = tpl.replace('@STORAGE@\n', body).replace('@NGFW_VERSION@', ver).replace('@SOURCE_ID@', src)
    if any(k in s for k in PLACEHOLDERS):
        raise SystemExit('unreplaced placeholder')
    return s


def grub(cfg, ver, extra):
    if not re.fullmatch(r'[0-9A-Za-z.+~-]+', ver) or not re.fullmatch(r'[ A-Za-z0-9_.,=/-]*', extra):
        raise SystemExit('odd version or kernel args')
    kernel_match = re.search(r'^\s*linux\s+(\S*vmlinuz)\s', cfg, re.M)
    initrd_match = re.search(r'^\s*initrd\s+(\S+)', cfg, re.M)
    menu_match = re.search(r'^\s*menuentry\s', cfg, re.M)
    if not all((kernel_match, initrd_match, menu_match)):
        raise SystemExit('base GRUB config needs a menuentry, kernel and initrd')
    kernel = kernel_match.group(1)
    initrd = initrd_match.group(1)
    entries = ''.join(
        'menuentry "%s" {\n\tset gfxpayload=keep\n\tlinux\t%s autoinstall%s ds=nocloud\\;s=/cdrom/nocloud/ ---%s\n'
        '\tinitrd\t%s\n}\n' % (title, kernel, arg, extra, initrd)
        for title, arg in (('Install NGFW %s (unattended: ERASES the largest disk)' % ver, ''),
                           ('Reinstall NGFW %s (ERASES the largest disk)' % ver, ' ngfw.reinstall=1')))
    s = re.sub(r'^set default=.*\n?', '', cfg, flags=re.M)
    s = re.sub(r'^set timeout=.*$', 'set timeout=5', s, flags=re.M)
    if 'set timeout=' not in s:
        s = 'set timeout=5\n' + s
    i = re.search(r'^\s*menuentry\s', s, re.M).start()
    return (s[:i] + 'set default=0\n' + entries +
            'submenu "Ubuntu Server installer (interactive, no NGFW)" {\n' + s[i:].rstrip('\n') + '\n}\n')


def main(argv):
    if argv[:1] == ['user-data'] and len(argv) == 6:
        _, tpl, storage, out, ver, src = argv
        open(out, 'w').write(user_data(open(tpl).read(), open(storage).read(), ver, src))
    elif argv[:1] == ['grub'] and len(argv) == 5:
        _, src, out, ver, extra = argv
        open(out, 'w').write(grub(open(src).read(), ver, extra))
    else:
        raise SystemExit(__doc__)


if __name__ == '__main__':
    main(sys.argv[1:])
