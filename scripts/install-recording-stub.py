#!/usr/bin/python3 -I
import json, os, pathlib, subprocess, sys
name = pathlib.Path(sys.argv[0]).name
if name == 'python3' and sys.argv[1:3] != ['-m', 'venv']:
    os.execv('/usr/bin/python3', ['/usr/bin/python3', '-I', *sys.argv[1:]])
if name == 'gpg':
    args = sys.argv[1:]
    if '--homedir' not in args: raise SystemExit(91)
    home = pathlib.Path(args[args.index('--homedir') + 1]).resolve()
    if not home.is_relative_to(pathlib.Path(os.environ['NGFW_INSTALL_ROOT']).resolve()): raise SystemExit(91)
    os.execv('/usr/bin/gpg', ['/usr/bin/gpg', *args])
with open(pathlib.Path(os.environ['NGFW_INSTALL_ROOT']) / '.ngfw-fixture-calls', 'a') as stream:
    stream.write(json.dumps([name, *sys.argv[1:]]) + '\n')
if name == 'apt-get':
    raise SystemExit(42 if sys.argv[1] == os.environ.get('FAIL_AT') else 0)
if name == 'go':
    if sys.argv[1] == 'version': print('go version go1.26.0 linux/amd64')
    raise SystemExit(0)
if name == 'corepack':
    raise SystemExit(43 if os.environ.get('COREPACK_FAIL') else 0)
if name in {'npm', 'pip'}: raise SystemExit(0)
if name == 'curl':
    target = pathlib.Path(sys.argv[sys.argv.index('-o')+1])
    if not target.resolve().is_relative_to(pathlib.Path(os.environ['NGFW_INSTALL_ROOT']).resolve()): raise SystemExit(91)
    data = b'fixture-invalid-digest'
    if os.environ.get('NGFW_INSTALL_FIXTURE_KEYS'):
        urls = {'https://deb.frrouting.org/frr/keys.gpg': 'frr.bundle', 'https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key': 'node.bundle'}
        data = (pathlib.Path(os.environ['WORK']) / urls[next(a for a in sys.argv if a.startswith('https://'))]).read_bytes()
    target.write_bytes(data)
    raise SystemExit(0)
if name == 'python3':
    target = pathlib.Path(sys.argv[3]) / 'bin'
    if not target.resolve().is_relative_to(pathlib.Path(os.environ['NGFW_INSTALL_ROOT']).resolve()): raise SystemExit(91)
    target.mkdir(parents=True)
    pip = target / 'pip'; pip.write_bytes(pathlib.Path(sys.argv[0]).read_bytes()); pip.chmod(0o755)
    raise SystemExit(0)
raise SystemExit(91)
