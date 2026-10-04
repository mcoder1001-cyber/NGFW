#!/usr/bin/python3
"""Root-owned Unix HTTP executor. No TCP listener, shell or user-selected command.

systemd passes the group-restricted listening socket as fd 3. Tokens exist only
in root memory, expire after 120 seconds and are burned before apply begins.
"""
import hashlib
import http.server
import json
import os
import re
import secrets
import socket
import subprocess
import tempfile
import time
from pathlib import Path

MAX_BODY = 65536
HEX = re.compile(r'^[0-9a-f]{64}$')
TOKEN = re.compile(r'^[0-9a-f]{64}$')
GENERATOR = '/usr/lib/ngfw/bin/ngfw-startupgen'
APPLY = '/usr/lib/ngfw/apply-startup.sh'
STARTUP = '/etc/vpp/startup.conf'


class Refused(Exception):
    def __init__(self, status, reason):
        self.status, self.reason = status, reason


class Executor:
    def __init__(self, render=None, installed=None, apply=None, clock=time.monotonic):
        self.render = render or self._render
        self.installed = installed or self._installed
        self.apply = apply or self._apply
        self.clock = clock
        self.tokens = {}

    @staticmethod
    def _installed():
        return hashlib.sha256(Path(STARTUP).read_bytes()).hexdigest()

    @staticmethod
    def _render(document):
        with tempfile.TemporaryDirectory(prefix='ngfw-approval-') as directory:
            doc = Path(directory) / 'doc.json'
            doc.write_bytes(document)
            result = subprocess.run([GENERATOR, str(doc)], capture_output=True,
                                    timeout=15, check=True, env={'PATH': '/usr/sbin:/usr/bin:/sbin:/bin'})
            return hashlib.sha256(result.stdout).hexdigest()

    @staticmethod
    def _apply(document, rendered, installed):
        with tempfile.TemporaryDirectory(prefix='ngfw-apply-') as directory:
            doc = Path(directory) / 'doc.json'
            doc.write_bytes(document)
            # Fixed argv; product planner retains locks, dead-man and health rollback.
            result = subprocess.run([APPLY, '--mode', 'product', '--doc', str(doc), '--apply',
                                     '--expect-sha256', installed, '--expect-new-sha256', rendered,
                                     '--approve-rendering', rendered], capture_output=True,
                                    timeout=1200, env={'PATH': '/usr/sbin:/usr/bin:/sbin:/bin'})
            if result.returncode:
                raise Refused(409, 'product apply refused or failed; inspect root journal')
            return {'accepted': True, 'sha256': rendered}

    def execute(self, operation, body):
        if not isinstance(body, dict):
            raise Refused(400, 'invalid request')
        fields = {'actor', 'sha256', 'dataplane'} | ({'token'} if operation == '/apply' else set())
        if operation not in ('/approve', '/apply') or set(body) != fields:
            raise Refused(400, 'invalid request fields')
        actor, rendered = body['actor'], body['sha256']
        if (not isinstance(actor, str) or not re.fullmatch(r'[0-9]{1,16}', actor)
                or not isinstance(rendered, str) or not HEX.fullmatch(rendered)
                or not isinstance(body['dataplane'], dict)):
            raise Refused(400, 'invalid request fields')
        document = json.dumps({'dataplane': body['dataplane']}, sort_keys=True,
                              separators=(',', ':'), allow_nan=False).encode()
        now = self.clock()
        self.tokens = {k: v for k, v in self.tokens.items() if v[0] > now}
        if operation == '/apply':
            token = body['token']
            if not isinstance(token, str) or not TOKEN.fullmatch(token):
                raise Refused(409, 'invalid or expired approval')
            approval = self.tokens.pop(token, None)  # consume even on a later failure
            if approval is None or approval[1:4] != (actor, rendered, document):
                raise Refused(409, 'invalid or expired approval')
            installed = approval[4]
            if self.installed() != installed or self.render(document) != rendered:
                raise Refused(409, 'configuration changed; preview and approve again')
            return self.apply(document, rendered, installed)
        if self.render(document) != rendered:
            raise Refused(409, 'preview changed; preview again')
        installed = self.installed()
        if installed == rendered:
            raise Refused(409, 'configuration is unchanged')
        if len(self.tokens) >= 128:
            raise Refused(429, 'too many outstanding approvals')
        token = secrets.token_hex(32)
        self.tokens[token] = (now + 120, actor, rendered, document, installed)
        return {'token': token, 'sha256': rendered, 'expiresIn': 120}


class Handler(http.server.BaseHTTPRequestHandler):
    def log_message(self, _format, *_args):
        pass  # token/body never enter logs

    def do_POST(self):
        try:
            self.connection.settimeout(15)
            length = int(self.headers.get('Content-Length', '0'))
            if not 0 < length <= MAX_BODY or self.headers.get('Transfer-Encoding'):
                raise Refused(400, 'invalid request size')
            raw = self.rfile.read(length)
            if len(raw) != length:
                raise Refused(400, 'incomplete request')
            result = self.server.executor.execute(self.path, json.loads(raw))
            status = 200
        except Refused as error:
            result, status = {'error': error.reason}, error.status
        except (ValueError, UnicodeError, TypeError):
            result, status = {'error': 'invalid request'}, 400
        except Exception:
            result, status = {'error': 'executor unavailable; inspect root journal'}, 503
        data = json.dumps(result).encode()
        self.send_response(status)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(data)))
        self.end_headers()
        self.wfile.write(data)


def main():
    if os.geteuid() != 0 or os.environ.get('LISTEN_PID') != str(os.getpid()) or os.environ.get('LISTEN_FDS') != '1':
        raise SystemExit('requires root and one systemd Unix listening socket')
    server = http.server.HTTPServer(('localhost', 0), Handler, bind_and_activate=False)
    server.socket.close()
    server.socket = socket.socket(fileno=3)
    if server.socket.family != socket.AF_UNIX:
        raise SystemExit('Unix socket required')
    server.server_address = server.socket.getsockname()
    server.executor = Executor()
    server.serve_forever()  # serialized issuance/apply; bounded token memory


if __name__ == '__main__':
    main()
