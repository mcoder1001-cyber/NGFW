import importlib.util
import unittest
from pathlib import Path

SPEC = importlib.util.spec_from_file_location('executor', Path(__file__).with_name('apply-executor.py'))
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class ApprovalTests(unittest.TestCase):
    def setUp(self):
        self.now = 0
        self.live = 'b' * 64
        self.rendered = 'a' * 64
        self.calls = []
        self.executor = MODULE.Executor(render=lambda doc: self.rendered,
                                        installed=lambda: self.live,
                                        apply=lambda *args: self.calls.append(args) or {'accepted': True},
                                        clock=lambda: self.now)
        self.body = {'actor': '1', 'sha256': self.rendered, 'dataplane': {'workers': 4}}

    def approval(self):
        return self.executor.execute('/approve', self.body)['token']

    def test_single_use(self):
        token = self.approval()
        self.executor.execute('/apply', {**self.body, 'token': token})
        self.assertEqual(len(self.calls), 1)
        with self.assertRaises(MODULE.Refused):
            self.executor.execute('/apply', {**self.body, 'token': token})

    def test_expiry_and_restart(self):
        token = self.approval()
        self.now = 120
        with self.assertRaises(MODULE.Refused):
            self.executor.execute('/apply', {**self.body, 'token': token})
        self.assertFalse(self.calls)

    def test_actor_document_and_sha_are_bound(self):
        for change in ({'actor': '2'}, {'dataplane': {'workers': 8}}, {'sha256': 'c' * 64}):
            token = self.approval()
            with self.assertRaises(MODULE.Refused):
                self.executor.execute('/apply', {**self.body, 'token': token, **change})
        self.assertFalse(self.calls)

    def test_installed_and_host_render_changes_burn_token(self):
        for field in ('live', 'rendered'):
            token = self.approval()
            setattr(self, field, 'c' * 64)
            with self.assertRaises(MODULE.Refused):
                self.executor.execute('/apply', {**self.body, 'token': token})
            self.assertNotIn(token, self.executor.tokens)
            setattr(self, field, 'b' * 64 if field == 'live' else 'a' * 64)
        self.assertFalse(self.calls)

    def test_request_fields_restrict_privileged_surface(self):
        for change in ({'command': 'evil'}, {'actor': '../1'}, {'sha256': 'invalid'}, {'dataplane': []}):
            with self.assertRaises(MODULE.Refused):
                self.executor.execute('/approve', {**self.body, **change})

    def test_approval_cap(self):
        for _ in range(128):
            self.approval()
        with self.assertRaises(MODULE.Refused):
            self.approval()
        self.now = 121
        self.approval()
        self.assertEqual(len(self.executor.tokens), 1)


if __name__ == '__main__':
    unittest.main()
