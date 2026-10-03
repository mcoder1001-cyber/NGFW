#!/usr/bin/env python3
"""Build patch 0003 against an existing pinned VPP build, without changing it.

This disposable-test helper compiles only the copied router source and links the
existing Linux netlink objects. Product patch 0003 is registered in revision 3;
this helper still builds only a disposable test plugin and never installs it.
"""
import argparse
import os
from pathlib import Path
import shlex
import shutil
import subprocess

ROOT = Path(__file__).resolve().parents[2]


def run(argv, cwd=None):
    return subprocess.check_output(argv, cwd=cwd, text=True)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--source', type=Path, default=Path('/root/vpp'))
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    source, out = args.source.resolve(), args.output.resolve()
    if out == source or source in out.parents:
        raise SystemExit('output must be outside the reference source tree')
    version = dict(line.split('=', 1) for line in (ROOT / 'deploy/vpp/VERSION').read_text().splitlines()
                   if line and not line.startswith('#'))
    if run(['git', '-C', str(source), 'rev-parse', 'HEAD']).strip() != version['VPP_COMMIT']:
        raise SystemExit('reference source does not match pinned VPP commit')
    build = source / 'build-root/build-vpp-native/vpp'
    commands = run(['ninja', '-C', str(build), '-t', 'commands', 'linux_nl_plugin.so']).splitlines()
    compile_line = next(line for line in commands if ' -c ' in line and line.endswith('/lcp_router.c'))
    link_line = next(line for line in reversed(commands) if ' -shared ' in line and 'linux_nl_plugin.so' in line)
    copied = out / 'source/src/plugins/linux-cp/lcp_router.c'
    copied.parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(source / 'src/plugins/linux-cp/lcp_router.c', copied)
    with (ROOT / 'deploy/vpp/patches/0003-lcp-multicast-reconcile.patch').open('rb') as patch:
        subprocess.run(['patch', '-p1', '--forward'], cwd=out / 'source', stdin=patch, check=True)
    obj = out / 'lcp_router.c.o'
    compiler = shlex.split(compile_line)
    old_obj = compiler[compiler.index('-o') + 1]
    compiler[compiler.index('-o') + 1] = str(obj)
    compiler[compiler.index('-c') + 1] = str(copied)
    # Dependency outputs from the reference command must never be written there.
    for flag in ['-MF', '-MT', '-MQ']:
        while flag in compiler:
            index = compiler.index(flag)
            del compiler[index:index + 2]
    compiler = [word for word in compiler if word not in ['-MD', '-MMD']]
    subprocess.run(compiler, cwd=build, check=True)
    plugin = out / 'plugin/linux_nl_plugin.so'
    plugin.parent.mkdir(exist_ok=True)
    link = shlex.split(link_line)
    if link[:2] == [':', '&&']:
        link = link[2:]
    if link[-2:] == ['&&', ':']:
        link = link[:-2]
    if any(word in ['&&', ';', '|'] for word in link):
        raise SystemExit('unsupported linker command structure')
    # Linker dependency files must also remain outside the reference build.
    for index in range(len(link) - 1, -1, -1):
        if link[index].startswith('--dependency-file='):
            del link[index]
            if index > 0 and link[index - 1] == '-Xlinker':
                del link[index - 1]
    link[link.index('-o') + 1] = str(plugin) + '.new'
    link = [str(obj) if word == old_obj else word for word in link]
    subprocess.run(link, cwd=build, check=True)
    os.replace(str(plugin) + '.new', plugin)
    print(plugin)


if __name__ == '__main__':
    main()
