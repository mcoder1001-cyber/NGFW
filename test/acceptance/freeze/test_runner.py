"""Acceptance report regressions: failures and dry runs cannot certify a release."""
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import Mock, patch

spec = importlib.util.spec_from_file_location('freeze_runner', Path(__file__).with_name('run.py'))
runner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(runner)


class ReportTests(unittest.TestCase):
    def execute(self, codes=None):
        with tempfile.TemporaryDirectory() as directory:
            argv = ['run.py', '--output', directory]
            children = [Mock(wait=Mock(return_value=code)) for code in (codes or [])]
            if codes is not None:
                argv.append('--run-offline')
            with patch('sys.argv', argv), patch.object(runner.subprocess, 'check_output', side_effect=['a' * 40, '']), \
                    patch.object(runner.subprocess, 'Popen', side_effect=children) as spawn:
                code = runner.main()
                report = json.loads((Path(directory) / 'summary.json').read_text())
                return code, report, spawn.call_count

    def test_dry_run_cannot_pass(self):
        code, report, count = self.execute()
        self.assertEqual((code, count), (0, 0))
        self.assertFalse(report['offline_passed'])
        self.assertFalse(report['release_accepted'])
        self.assertTrue(all(case['status'] == 'NOT RUN' for case in report['cases']))

    def test_failure_retained_and_later_cases_execute(self):
        code, report, count = self.execute([0, 1, 0, 0])
        self.assertEqual((code, count), (1, 4))
        self.assertEqual([case['status'] for case in report['cases']], ['PASS', 'FAIL', 'PASS', 'PASS'])
        self.assertFalse(report['offline_passed'])
        self.assertFalse(report['release_accepted'])

    def test_offline_success_does_not_certify_live_acceptance(self):
        code, report, count = self.execute([0, 0, 0, 0])
        self.assertEqual((code, count), (0, 4))
        self.assertTrue(report['offline_passed'])
        self.assertFalse(report['release_accepted'])
        self.assertTrue(all(case['status'] == 'NOT RUN' for case in report['live'].values()))

    def test_preflight_failure_replaces_previous_success(self):
        with tempfile.TemporaryDirectory() as directory:
            summary = Path(directory) / 'summary.json'
            summary.write_text(json.dumps({'offline_passed': True, 'release_accepted': True}))
            with patch('sys.argv', ['run.py', '--output', directory]), \
                    patch.object(runner.subprocess, 'check_output', side_effect=OSError):
                self.assertEqual(runner.main(), 1)
            report = json.loads(summary.read_text())
            self.assertEqual(report['status'], 'FAIL')
            self.assertFalse(report['offline_passed'])
            self.assertFalse(report['release_accepted'])

    def test_interrupt_stops_owned_child_and_records_failure(self):
        with tempfile.TemporaryDirectory() as directory:
            child = Mock(pid=12345, wait=Mock(side_effect=[KeyboardInterrupt, 0, 0]))
            with patch('sys.argv', ['run.py', '--output', directory, '--run-offline']), \
                    patch.object(runner.subprocess, 'check_output', side_effect=['a' * 40, '']), \
                    patch.object(runner.subprocess, 'Popen', return_value=child), \
                    patch.object(runner.os, 'killpg') as stop:
                self.assertEqual(runner.main(), 130)
            self.assertEqual(stop.call_args_list, [unittest.mock.call(12345, runner.signal.SIGTERM),
                                                  unittest.mock.call(12345, runner.signal.SIGKILL)])
            report = json.loads((Path(directory) / 'summary.json').read_text())
            self.assertEqual(report['status'], 'INTERRUPTED')
            self.assertFalse(report['offline_passed'])
            self.assertEqual(report['cases'][0]['status'], 'FAIL')


if __name__ == '__main__':
    unittest.main()
