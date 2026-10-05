"""Real loopback transport regressions; fixtures never grant live lab authority."""
from contextlib import contextmanager
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import importlib.util
import os
from pathlib import Path
import sys
import threading
import unittest
from unittest.mock import patch
import urllib.error
import urllib.request

from private_http import private_opener

TOKEN = 'NGFW_TEST_PSK_SECURITY_REDIRECT'
ROOT = Path(__file__).resolve().parent


@contextmanager
def endpoint(status=200, location=None):
    seen = []
    class Handler(BaseHTTPRequestHandler):
        def do_GET(self):
            seen.append(self.headers.get('Authorization'))
            self.send_response(status)
            if location is not None:
                self.send_header('Location', location)
            self.send_header('Content-Type', 'application/json')
            self.end_headers()
            self.wfile.write(b'{}')
        def do_POST(self):
            self.do_GET()
        def log_message(self, *_args):
            pass
    server = ThreadingHTTPServer(('127.0.0.1', 0), Handler)
    worker = threading.Thread(target=lambda: server.serve_forever(poll_interval=.01), daemon=True)
    worker.start()
    try:
        yield f'http://127.0.0.1:{server.server_port}', seen
    finally:
        server.shutdown()
        server.server_close()
        worker.join(timeout=2)


def module(name, path):
    spec = importlib.util.spec_from_file_location(name, ROOT / path)
    loaded = importlib.util.module_from_spec(spec)
    original = list(sys.path)
    try:
        sys.path.insert(0, str((ROOT / path).parent))
        spec.loader.exec_module(loaded)
    finally:
        sys.path[:] = original
    return loaded


class PrivateTransportTests(unittest.TestCase):
    def test_every_redirect_is_refused_before_credentials_leave_origin(self):
        with endpoint() as (sink, received):
            for code in (301, 302, 303, 307, 308):
                for method in ('GET', 'POST'):
                    with self.subTest(code=code, method=method), endpoint(code, sink + '/foreign') as (origin, sent):
                        request = urllib.request.Request(origin, data=b'{}' if method == 'POST' else None,
                            method=method, headers={'Authorization': 'Bearer ' + TOKEN})
                        with self.assertRaises(urllib.error.HTTPError) as error:
                            private_opener().open(request, timeout=2)
                        self.assertEqual(error.exception.code, code)
                        error.exception.close()
                        self.assertEqual(sent, ['Bearer ' + TOKEN])
                        self.assertEqual(received, [])

    def test_ambient_proxy_never_receives_private_origin_credentials(self):
        with endpoint() as (proxy, leaked), endpoint() as (origin, sent):
            with patch.dict(os.environ, {'http_proxy': proxy, 'HTTP_PROXY': proxy, 'ALL_PROXY': proxy,
                                         'all_proxy': proxy, 'no_proxy': '', 'NO_PROXY': ''}):
                request = urllib.request.Request(origin, headers={'Authorization': 'ApiKey ' + TOKEN})
                with private_opener().open(request, timeout=2) as response:
                    self.assertEqual(response.status, 200)
            self.assertEqual(sent, ['ApiKey ' + TOKEN])
            self.assertEqual(leaked, [])

    def test_existing_slot_clients_keep_normal_responses_and_refuse_redirects(self):
        for name, path in (('wave_a_transport', 'traffic-a/execute.py'),
                           ('management_transport', 'management-ui/run.py'),
                           ('dataplane_transport', 'dataplane-ui/run.py')):
            loaded = module(name, path)
            client = loaded.Api(14, TOKEN)
            with self.subTest(client=name), endpoint() as (origin, sent):
                client.base = origin
                self.assertEqual(client.call('GET', '/config'), {})
                self.assertEqual(sent, ['ApiKey ' + TOKEN])
            with endpoint() as (sink, received), endpoint(302, sink) as (origin, _sent):
                client.base = origin
                with self.assertRaises(loaded.Refused):
                    client.call('GET', '/config')
                self.assertEqual(received, [])

    def test_capture_transport_keeps_status_and_never_redirects_private_download(self):
        capture = module('capture_transport', 'capture-trace/live.py')
        with endpoint(409) as (origin, _sent):
            self.assertEqual(capture.request(origin, TOKEN, 'GET', '/api/v1/state/captures'), (409, b'{}'))
        with endpoint() as (sink, received), endpoint(302, sink) as (origin, sent):
            self.assertEqual(capture.request(origin, TOKEN, 'GET', '/api/v1/state/captures/id/file'), (302, b'{}'))
            self.assertEqual(sent, ['Bearer ' + TOKEN])
            self.assertEqual(received, [])


if __name__ == '__main__':
    unittest.main()
