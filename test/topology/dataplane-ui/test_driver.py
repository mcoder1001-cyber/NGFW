import unittest
from run import Api, Refused, acceptance, cores


class FakeApi:
    def __init__(self, broken=False):
        self.discarded = False
        self.broken = broken
        self.locked = False

    def call(self, method, path, body=None, want=200):
        if path == '/config/lock':
            return {'locked': self.locked, 'ownerKeyId': 'driver-key', 'lockedAt': 'fixture'}
        if method == 'PATCH' and path == '/config':
            self.locked = True
            return {}
        if path == '/config/diff':
            return {'changes': []}
        if path == '/config':
            return {'dataplane': {}}
        if path == '/state/dataplane':
            return {'runtimeThreads': [{'id': 0}], 'onlineCpus': '0-3', 'mainCore': 0}
        if path == '/actions/dataplane/preview':
            import hashlib
            rendered = 'cpu { corelist-workers 1 }'
            return {'rendered': rendered, 'diff': '+ corelist-workers 1', 'changed': True,
                    'sha256': hashlib.sha256(rendered.encode()).hexdigest(),
                    'restartRequired': True, 'applyAvailable': 'invalid' if self.broken else False}
        if path == '/config/validate':
            return {'errors': [{'pointer': '/dataplane/workers'}]}
        if path == '/config/discard':
            self.discarded = True
        return {}


class DriverTests(unittest.TestCase):
    def test_cpu_inventory(self):
        self.assertEqual(cores('0-2,4'), [0, 1, 2, 4])
        for value in ('3-1', '0-9999', '0-1-2'):
            with self.assertRaises(Refused):
                cores(value)

    def test_cleanup_success_and_failure(self):
        for broken in (False, True):
            api = FakeApi(broken)
            if broken:
                with self.assertRaises(Refused):
                    acceptance(api, lambda: ('digest', '0'))
            else:
                self.assertEqual(acceptance(api, lambda: ('digest', '0'))['status'], 'API_ACCEPTANCE_PASSED')
            self.assertTrue(api.discarded)

    def test_no_shared_or_ci_slot(self):
        for slot in (0, 12, 13, 33):
            with self.assertRaises(Refused):
                Api(slot, 'fixture')
