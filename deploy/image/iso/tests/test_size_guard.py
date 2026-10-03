"""Run production size verifier with stub lsblk; never inspect real disks."""
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

HELPER = Path(__file__).resolve().parents[1] / 'installer/ngfw-size-guard.py'
MIN = 96 * 1024 ** 3


class SizeGuardTests(unittest.TestCase):
    def probe(self, output, status=0):
        with tempfile.TemporaryDirectory(prefix='ngfw-size-guard-') as directory:
            root = Path(directory)
            (root / 'response').write_text(output)
            stub = root / 'lsblk'
            stub.write_text('#!/bin/sh\n[ "$1" = -bdnro ] && [ "$2" = SIZE,TYPE,RM ] || exit 90\n'
                            'cat "$(dirname "$0")/response"\nexit %d\n' % status)
            stub.chmod(0o700)
            return subprocess.run([sys.executable, str(HELPER)], capture_output=True, text=True,
                                  env={**os.environ, 'PATH': directory + ':/usr/bin:/bin'})

    def test_boundary_and_largest_fixed_disk(self):
        result = self.probe(f'{MIN} disk 0\n{MIN + 1} disk 0\n{MIN * 2} disk 1\n')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.strip(), str(MIN + 1))
        self.assertNotEqual(self.probe(f'{MIN - 1} disk 0\n').returncode, 0)

    def test_failure_and_partial_output_refused(self):
        for output in ('', f'{MIN} disk 0\n'):
            with self.subTest(output=output):
                result = self.probe(output, 9)
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(result.stdout, '')

    def test_uncertain_inventory_refused_even_after_valid_disk(self):
        for row in ('1e300 disk 0', 'NaN disk 0', 'Inf disk 0', '-1 disk 0',
                    '18446744073709551616 disk 0', '9' * 39 + ' disk 0',
                    f'{MIN} disk invalid', f'{MIN} disk', f'{MIN} disk 0 extra'):
            with self.subTest(row=row):
                result = self.probe(f'{MIN} disk 0\n{row}\n')
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(result.stdout, '')

    def test_no_fixed_disk_and_unsigned_upper_boundary(self):
        for output in ('', f'{MIN} disk 1\n', f'{MIN} loop 0\n'):
            with self.subTest(output=output):
                self.assertNotEqual(self.probe(output).returncode, 0)
        # Boundary is representable; there is no floating-point roundoff/overflow.
        result = self.probe('18446744073709551615 disk 0\n')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.strip(), '18446744073709551615')


if __name__ == '__main__':
    unittest.main()
