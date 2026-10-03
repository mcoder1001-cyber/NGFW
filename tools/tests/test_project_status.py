import importlib.util
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('project_status', Path(__file__).resolve().parents[1] / 'project-status.py')
status = importlib.util.module_from_spec(spec)
spec.loader.exec_module(status)


class ProjectStatusTests(unittest.TestCase):
    def test_counts_hours_unknown_and_live_inventory(self):
        result = status.board_summary({'tasks': [
            {'id': 'a', 'state': 'merged', 'est_hours': 3},
            {'id': 'b', 'state': 'running', 'est_hours': 5},
            {'id': 'c', 'state': 'paused', 'est_hours': 2}]})
        self.assertEqual(result['total_tasks'], 3)
        self.assertEqual(result['state_counts']['running'], 1)
        self.assertEqual(result['estimated_hours']['progress_percent'], 30)
        self.assertEqual(result['unknown_states'], ['paused'])
        self.assertIsNone(result['live_developers']['count'])

    def test_email_alias_dedup(self):
        result = status.contributors_summary('Alice\tA@Example.com\nAlias\ta@example.com\nBob\tb@example.com\n')
        self.assertEqual(result['count'], 2)
        self.assertEqual(result['authors'][0]['names'], ['Alias', 'Alice'])

    def test_empty_and_invalid_boards(self):
        self.assertIsNone(status.board_summary({'tasks': []})['task_progress_percent'])
        for tasks in ([{'id': 'x', 'state': 'todo', 'est_hours': float('nan')}],
                      [{'id': 'x', 'state': 'todo', 'est_hours': True}],
                      [{'id': 'x', 'state': 'todo', 'est_hours': 1}] * 2):
            with self.assertRaises(ValueError):
                status.board_summary({'tasks': tasks})

    def test_missing_remote_or_git_is_unknown(self):
        with patch.object(status, 'git_read', return_value=None):
            result = status.git_summary(Path('/unused'))
        self.assertEqual(result['origin_main']['status'], 'unverifiable')
        self.assertIsNone(result['origin_main']['ahead'])
        self.assertIsNone(result['contributors']['count'])

    def test_divergence_direction(self):
        with patch.object(status, 'git_read', side_effect=['head', '2\t3', '', 'task/status']):
            result = status.git_summary(Path('/unused'))
        self.assertEqual(result['origin_main']['ahead'], 2)
        self.assertEqual(result['origin_main']['behind'], 3)

    def test_not_a_repository(self):
        with tempfile.TemporaryDirectory() as root:
            self.assertIsNone(status.git_summary(Path(root))['head'])


if __name__ == '__main__':
    unittest.main()
