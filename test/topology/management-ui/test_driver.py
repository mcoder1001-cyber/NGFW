import json
from pathlib import Path
import socket
import ssl
import tempfile
import threading
import unittest
from run import Refused, handshake, own_lock, private_material, safe_public, claim_revision, commit_observation


class ApiFixture:
    def __init__(self, locked=False):
        self.locked = locked

    def call(self, method, path, body=None):
        if path == '/config/diff':
            return {'baseRevision': 7, 'changes': []}
        if method == 'PATCH':
            self.locked = True
        return {'locked': self.locked, 'ownerKeyId': 'fixture-key', 'lockedAt': 'fixture'}


class DriverTests(unittest.TestCase):
    def test_private_data_refused(self):
        for value in ({'error': 'PRIVATE KEY'}, {'after': 'secret-fixture'}):
            with self.assertRaises(Refused):
                safe_public(value, ['secret-fixture'])
        safe_public({'fingerprint': 'safe'}, ['secret-fixture'])

    def test_candidate_owner(self):
        api = ApiFixture()
        owner = own_lock(api)
        own_lock(api, owner)
        with self.assertRaises(Refused):
            own_lock(ApiFixture(True))
        with self.assertRaises(Refused):
            own_lock(api, dict(owner, ownerKeyId='another-worker'))

    def test_revision_rechecked_after_claim(self):
        with self.assertRaises(Refused):
            claim_revision(ApiFixture(), 6)
        self.assertTrue(claim_revision(ApiFixture(), 7)['locked'])

    def test_partial_commit_retains_observed_revision(self):
        api = ApiFixture(True)
        owner = own_lock(ApiFixture())
        self.assertEqual(commit_observation(api, {'status': 'partial', 'revision': {'id': 7}}, 6, owner), (7, True))
        with self.assertRaises(Refused):
            commit_observation(api, {'status': 'partial'}, 6, owner)

    def test_real_transient_tls_helper(self):
        # A local transient helper test, explicitly not a product/API acceptance run.
        with tempfile.TemporaryDirectory() as directory:
            cert, key = private_material(Path(directory), 'host-driver-fixture')
            context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
            context.minimum_version = ssl.TLSVersion.TLSv1_3
            context.load_cert_chain(cert, key)
            server = socket.socket()
            server.bind(('127.0.0.1', 0))
            server.listen(2)
            port = server.getsockname()[1]
            errors = []

            def accept():
                try:
                    for _ in range(2):
                        raw, _ = server.accept()
                        try:
                            with context.wrap_socket(raw, server_side=True):
                                pass
                        except ssl.SSLError:
                            raw.close()
                except Exception as error:
                    errors.append(error)
                finally:
                    server.close()

            thread = threading.Thread(target=accept, daemon=True)
            thread.start()
            result = handshake(port, cert)
            self.assertEqual(result['version'], 'TLSv1.3')
            with self.assertRaises(ssl.SSLError):
                handshake(port, cert, '1.2', '1.2')
            thread.join(timeout=10)
            self.assertFalse(thread.is_alive())
            self.assertFalse(errors)
            self.assertEqual(key.stat().st_mode & 0o777, 0o600)
