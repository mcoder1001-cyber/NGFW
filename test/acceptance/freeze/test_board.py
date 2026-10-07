"""A corrupted publication must fail validation without rewriting task state."""
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[3]


class BoardTests(unittest.TestCase):
    def check(self, body):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'tools').mkdir()
            (root / 'plan').mkdir()
            script = root / 'tools/board.py'
            shutil.copyfile(ROOT / 'tools/board.py', script)
            board = root / 'plan/tasks.yaml'
            board.write_text(body)
            result = subprocess.run(['python3', str(script), '--check'], capture_output=True, text=True)
            self.assertEqual(board.read_text(), body)
            self.assertFalse((root / 'docs').exists())
            return result

    def test_read_only_does_not_promote_ready_task(self):
        result = self.check('version: 2\ntasks:\n- id: one\n  state: todo\n  deps: []\n')
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_dangling_dependency_from_truncated_board_refused(self):
        result = self.check('version: 2\ntasks:\n- id: one\n  state: todo\n  deps: [missing]\n')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('unknown_deps', result.stderr)

    def test_tool_output_header_refused(self):
        result = self.check('Warning: truncated output (original token count: 76531)\nversion: 2\ntasks: []\n')
        self.assertNotEqual(result.returncode, 0)

    def test_dependency_cycle_refused(self):
        result = self.check('version: 2\ntasks:\n- id: one\n  state: todo\n  deps: [one]\n')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('cycles', result.stderr)


if __name__ == '__main__':
    unittest.main()
