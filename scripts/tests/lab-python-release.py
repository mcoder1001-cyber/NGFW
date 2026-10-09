#!/usr/bin/env python3
"""Offline integrity/refusal tests; no upstream download or package execution."""
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest import mock

SCRIPT = Path(__file__).resolve().parents[1] / 'lab-python-release.py'
spec = importlib.util.spec_from_file_location('lab_release', SCRIPT)
release = importlib.util.module_from_spec(spec)
spec.loader.exec_module(release)


class ReleaseTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.artifact = {'filename': 'example.whl', 'size': 4,
                         'sha256': hashlib.sha256(b'data').hexdigest(),
                         'url': 'https://files.pythonhosted.org/example.whl'}

    def test_cache_bytes_require_upstream_digest_and_size(self):
        path = self.root / 'example.whl'
        path.write_bytes(b'data')
        self.assertEqual(release.obtain(self.artifact, self.root), b'data')
        for invalid in (b'evil', b'dataextra'):
            path.write_bytes(invalid)
            with self.assertRaisesRegex(ValueError, 'mismatch'):
                release.obtain(self.artifact, self.root)

    def test_cache_symlinks_and_traversal_refused(self):
        (self.root / 'real').write_bytes(b'data')
        (self.root / 'example.whl').symlink_to(self.root / 'real')
        with self.assertRaises(OSError):
            release.obtain(self.artifact, self.root)
        with self.assertRaisesRegex(ValueError, 'unsafe'):
            release.obtain(dict(self.artifact, filename='../real'), self.root)

    def test_untrusted_download_origin_refused_before_network(self):
        with mock.patch.object(release.urllib.request, 'urlopen') as network:
            for url in ('http://files.pythonhosted.org/x', 'https://evil.invalid/x',
                        'https://files.pythonhosted.org@evil.invalid/x'):
                with self.assertRaisesRegex(ValueError, 'official PyPI'):
                    release.obtain(dict(self.artifact, url=url), None)
            network.assert_not_called()

    def test_output_cannot_overwrite_or_follow_symlink(self):
        target = self.root / 'existing'
        target.mkdir()
        (target / 'keep').write_text('original')
        output = self.root / 'linked'
        output.symlink_to(target, target_is_directory=True)
        with self.assertRaises(FileExistsError):
            release.publish(output, {'keep': b'bad'})
        self.assertEqual((target / 'keep').read_text(), 'original')
        release.publish(self.root / 'new', {'example.whl': b'data'})
        self.assertEqual((self.root / 'new' / 'example.whl').read_bytes(), b'data')

    def test_committed_lock_and_artifacts_agree(self):
        folder = release.RELEASE
        resolution = json.loads((folder / 'resolution.json').read_text())
        upstream = json.loads((folder / 'upstream.json').read_text())
        lock = (folder / 'requirements.lock').read_bytes()
        self.assertEqual(hashlib.sha256(lock).hexdigest(), resolution['lock_sha256'])
        expected = ''.join(f"{a['name']}=={a['version']} --hash=sha256:{a['sha256']}\n"
                           for a in resolution['artifacts']).encode()
        self.assertEqual(lock, expected)
        published = {a['filename']: a['sha256'] for a in upstream['artifacts']}
        built = upstream['build']['artifact']
        published[built['wheel']] = built['sha256']
        self.assertEqual(len(resolution['artifacts']), 22)
        for artifact in resolution['artifacts']:
            self.assertEqual(published[artifact['wheel']], artifact['sha256'])


if __name__ == '__main__':
    unittest.main()
