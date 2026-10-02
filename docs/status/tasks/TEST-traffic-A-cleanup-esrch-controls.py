"""Separate helper controls: these never change original47 discovery/counts."""
import errno
import json
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import Mock, patch

sys.dont_write_bytecode=True
ROOT=Path(__file__).resolve().parents[3]
sys.path.insert(0,str(ROOT/'test/topology/traffic-a'))
from test_foundation import Foundation


class Controls(unittest.TestCase):
    def test_postcleanup_accepts_only_missing_process_errors(self):
        subject=Foundation();identity={'proc_pid':123,'birth':'99'}
        for error in (FileNotFoundError(errno.ENOENT,'gone'),ProcessLookupError(errno.ESRCH,'gone')):
            with self.subTest(error=type(error).__name__),patch.object(Path,'read_text',side_effect=error):
                subject.assert_verified_child_stopped(identity)
        for code in (errno.EACCES,errno.EPERM,errno.EIO,errno.EINVAL):
            error=OSError(code,'not evidence of process exit')
            with self.subTest(code=code),patch.object(Path,'read_text',side_effect=error):
                with self.assertRaises(OSError) as caught:subject.assert_verified_child_stopped(identity)
                self.assertIs(caught.exception,error)

    def test_readiness_esrch_permissions_and_wrong_identity_still_fail(self):
        subject=Foundation()
        with tempfile.TemporaryDirectory() as directory:
            ready=Path(directory)/'ready';ready.write_text(json.dumps({'proc_pid':123,'birth':'99'}))
            original_read=Path.read_text
            def exercise(raw=None,error=None):
                observed=[];created=[];process=Mock();process.poll.return_value=None
                def read(path,*args,**kwargs):
                    if path==ready:return original_read(path,*args,**kwargs)
                    if error:raise error
                    return raw
                with patch('test_foundation.subprocess.Popen',return_value=process):
                    start=subject.verified_ready_process(ready,observed,created)
                with patch.object(Path,'read_text',read):
                    start(['explicit-fake'])
                return observed
            for code in (errno.ESRCH,errno.EACCES,errno.EPERM,errno.EIO):
                with self.subTest(code=code),self.assertRaises(OSError):exercise(error=OSError(code,'readiness unavailable'))
            def stat(pid,birth,state='S'):
                fields=[state]+['0']*18+[birth]
                return str(pid)+' (comm with spaces) '+' '.join(fields)
            for raw in (stat(124,'99'),stat(123,'100'),stat(123,'99','Z')):
                with self.subTest(raw=raw),self.assertRaises(AssertionError):exercise(raw=raw)
            self.assertEqual(exercise(raw=stat(123,'99')),[{'proc_pid':123,'birth':'99'}])


if __name__=='__main__':
    result=unittest.TextTestRunner(verbosity=2).run(unittest.defaultTestLoader.loadTestsFromTestCase(Controls))
    raise SystemExit(not result.wasSuccessful() or result.testsRun!=2 or bool(result.skipped or result.expectedFailures or result.unexpectedSuccesses))
