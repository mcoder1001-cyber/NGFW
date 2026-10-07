"""Recording-only fixture: no package/network command delegates to the host."""
import json
import os
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[2]
PACKAGES = ('pip', 'robotframework', 'robotframework-sshlibrary', 'scapy', 'pytest', 'requests')


class Fixture:
    def __init__(self):
        self.directory = tempfile.TemporaryDirectory()
        self.base = Path(self.directory.name)
        self.root = self.base / 'root'
        self.root.mkdir()
        self.stubs = self.base / 'stubs'
        self.stubs.mkdir()
        self.log = self.base / 'calls'
        self.lock = self.base / 'lab.lock'
        self.lock.write_text(''.join(f'{name}==1.0 --hash=sha256:{"a" * 64}\n' for name in PACKAGES))
        # Synthetic fixture versions/hashes are not usable release pins.
        recorder = '''#!/usr/bin/python3
import json, os, pathlib, subprocess, sys
name = pathlib.Path(sys.argv[0]).name
if name == 'python3' and sys.argv[1:3] != ['-m', 'venv']:
    os.execv('/usr/bin/python3', ['/usr/bin/python3', *sys.argv[1:]])
with open(os.environ['CALL_LOG'], 'a') as stream:
    stream.write(json.dumps([name, *sys.argv[1:]]) + '\\n')
if name == 'apt-get':
    raise SystemExit(42 if sys.argv[1] == os.environ.get('FAIL_AT') else 0)
if name == 'go':
    if sys.argv[1] == 'version': print('go version go1.26.0 linux/amd64')
    raise SystemExit(0)
if name == 'corepack':
    raise SystemExit(43 if os.environ.get('COREPACK_FAIL') else 0)
if name in {'npm', 'pip'}: raise SystemExit(0)
if name == 'curl':
    pathlib.Path(sys.argv[sys.argv.index('-o')+1]).write_bytes(b'fixture-invalid-digest')
    raise SystemExit(0)
if name == 'python3':
    target = pathlib.Path(sys.argv[3]) / 'bin'
    target.mkdir(parents=True)
    pip = target / 'pip'; pip.write_bytes(pathlib.Path(sys.argv[0]).read_bytes()); pip.chmod(0o755)
    raise SystemExit(0)
raise SystemExit(91)
'''
        for name in ('apt-get', 'curl', 'gpg', 'go', 'npm', 'corepack', 'python3', 'tar'):
            stub = self.stubs / name
            stub.write_text(recorder)
            stub.chmod(0o755)
        self.env = dict(os.environ, PATH=f'{self.stubs}:/usr/bin:/bin', CALL_LOG=str(self.log),
                        NGFW_INSTALL_ROOT=str(self.root), NGFW_INSTALL_STUBS='1',
                        NGFW_INSTALL_STUB_DIR=str(self.stubs), NGFW_LAB_REQUIREMENTS=str(self.lock))
        self.env.pop('NGFW_GO_SHA256', None)
        self.env.pop('FAIL_AT', None)

    def run(self, script, args=()):
        return subprocess.run(['bash', str(ROOT / 'scripts' / script), *args], env=self.env,
                              text=True, capture_output=True, timeout=20)

    def calls(self):
        return [json.loads(line) for line in self.log.read_text().splitlines()] if self.log.exists() else []

    def existing_go(self):
        target = self.root / 'usr/local/go/bin/go'
        target.parent.mkdir(parents=True)
        target.write_bytes((self.stubs / 'go').read_bytes())
        target.chmod(0o755)

    def close(self):
        self.directory.cleanup()
