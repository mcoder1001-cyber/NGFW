#!/usr/bin/env python3
"""Build native IKEv2 patches against an existing pinned VPP build, without changing it.

This disposable-test helper compiles only the copied API and protocol sources and links the
existing IKEv2 objects. Product packages use deploy/vpp/build.sh instead.
"""
import argparse
import os
from pathlib import Path
import shlex
import shutil
import subprocess

ROOT = Path(__file__).resolve().parents[3]


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
    commands = run(['ninja', '-C', str(build), '-t', 'commands', 'ikev2_plugin.so']).splitlines()
    replacements = {}
    for filename, patch_name in [('ikev2_api.c', '0002-ikev2-safe-native-state.patch'),
                                 ('ikev2.c', '0004-ikev2-auth-capability.patch')]:
        compile_line = next(line for line in commands if ' -c ' in line and line.endswith('/' + filename))
        copied = out / 'source/src/plugins/ikev2' / filename
        copied.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(source / 'src/plugins/ikev2' / filename, copied)
        with (ROOT / 'deploy/vpp/patches' / patch_name).open('rb') as patch:
            subprocess.run(['patch', '-p1', '--forward'], cwd=out / 'source', stdin=patch, check=True)
        obj = out / (filename + '.o')
        compiler = shlex.split(compile_line)
        old_obj = compiler[compiler.index('-o') + 1]
        replacements[old_obj] = str(obj)
        compiler[compiler.index('-o') + 1] = str(obj)
        compiler[compiler.index('-c') + 1] = str(copied)
        # Never write dependency outputs into the reference build.
        for flag in ['-MF', '-MT', '-MQ']:
            while flag in compiler:
                index = compiler.index(flag)
                del compiler[index:index + 2]
        compiler = [word for word in compiler if word not in ['-MD', '-MMD']]
        subprocess.run(compiler, cwd=build, check=True)
    link_line = next(line for line in reversed(commands) if ' -shared ' in line and 'ikev2_plugin.so' in line)
    plugin = out / 'plugin/ikev2_plugin.so'
    plugin.parent.mkdir(exist_ok=True)
    link = shlex.split(link_line)
    if link[:2] == [':', '&&']:
        link = link[2:]
    if link[-2:] == ['&&', ':']:
        link = link[:-2]
    if any(word in ['&&', ';', '|'] for word in link):
        raise SystemExit('unsupported linker command structure')
    link[link.index('-o') + 1] = str(plugin) + '.new'
    link = [replacements.get(word, word) for word in link]
    subprocess.run(link, cwd=build, check=True)
    os.replace(str(plugin) + '.new', plugin)
    print(plugin)


if __name__ == '__main__':
    main()
