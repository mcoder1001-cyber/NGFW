import json
import pathlib
import runpy
import subprocess
import unittest
from unittest.mock import patch

ROOT = pathlib.Path(__file__).resolve().parent

class CollectorTest(unittest.TestCase):
    def test_fixed_readonly_arguments_and_no_raw_journal(self):
        calls = []
        def fake(argv, **kwargs):
            calls.append(argv)
            return subprocess.CompletedProcess(argv, 0, b'ActiveState=active\nSubState=running\n', b'')
        with patch('subprocess.run', side_effect=fake), patch('builtins.print') as output:
            runpy.run_path(str(ROOT / 'ngfw-support-collect'), run_name='__main__')
        payload = json.loads(output.call_args.args[0])
        self.assertEqual(len(payload['services']), 5)
        self.assertEqual(len(calls), 7)
        for argv in calls[:5]:
            self.assertEqual(argv[0], '/usr/bin/systemctl')
            self.assertEqual(argv[1], 'show')
            self.assertEqual(argv[3:], ['--property=ActiveState,SubState', '--no-pager'])
        self.assertFalse(any('journalctl' in item for argv in calls for item in argv))
    def test_dispatch_rejects_unknown_instance_without_execution(self):
        with patch('sys.argv', ['ngfw-upgrade-dispatch', 'stage;reboot']), patch('subprocess.run') as execute:
            with self.assertRaises(SystemExit) as error:
                runpy.run_path(str(ROOT / 'ngfw-upgrade-dispatch'), run_name='__main__')
            self.assertEqual(error.exception.code, 2)
            execute.assert_not_called()

if __name__ == '__main__':
    unittest.main()
