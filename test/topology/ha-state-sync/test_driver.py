import importlib.util
import os
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch
spec=importlib.util.spec_from_file_location('ha_driver',Path(__file__).with_name('driver.py'))
driver=importlib.util.module_from_spec(spec)
spec.loader.exec_module(driver)
class Safety(unittest.TestCase):
 def test_fetch_rejects_foreign_and_downgrade_redirects(self):
  import urllib.request
  from acceptance import NoRedirect, Refused
  for target in ('https://foreign.invalid/capture', 'http://a.invalid/capture'):
   class RedirectingOpener:
    def open(self, request, timeout):
     self_request = request
     self_handler = NoRedirect()
     return self_handler.redirect_request(self_request, None, 302, 'redirect', {}, target)
   with patch('acceptance.urllib.request.build_opener', return_value=RedirectingOpener()) as make:
    with self.assertRaisesRegex(Refused, 'redirect refused'):
     driver.fetch('https://a.invalid', 'test-token', 'state/ha/sync')
    self.assertIsInstance(make.call_args.args[0], NoRedirect)
 def test_fetch_refuses_non_origin_before_sending_credentials(self):
  from acceptance import Refused
  for base in ('http://a.invalid', 'https://user:password@a.invalid', 'https://a.invalid/path', 'https://a.invalid?query=x'):
   with patch('acceptance.urllib.request.build_opener') as make:
    with self.assertRaises(Refused): driver.fetch(base, 'test-token', 'state/ha/sync')
    make.assert_not_called()

 def test_readonly_never_executes_a_probe(self):
  with tempfile.TemporaryDirectory() as d:
   args=['driver','--node-a','https://a','--node-b','https://b','--output',d+'/out.json']
   with patch.object(sys,'argv',args),patch.dict(os.environ,{'NGFW_HA_TOKEN_A':'secretA','NGFW_HA_TOKEN_B':'secretB'}),patch.object(driver,'fetch',return_value={'kinds':[]}) as fetch,patch.object(driver,'command') as command:
    driver.main();self.assertEqual(fetch.call_count,2);command.assert_not_called();self.assertNotIn('secret',Path(d+'/out.json').read_text())
 def test_same_node_and_unguarded_kill_refused(self):
  for extra in [[],['--node-b','https://b','--kill-vpp']]:
   args=['driver','--node-a','https://a','--node-b','https://a','--output','unused']+extra
   with patch.object(sys,'argv',args),patch.object(driver,'fetch') as fetch:
    with self.assertRaises(SystemExit):driver.main()
    fetch.assert_not_called()
 def test_exercise_requires_all_explicit_probes(self):
  args=['driver','--node-a','https://a','--node-b','https://b','--output','unused','--isolated-lab','--exercise']
  with patch.object(sys,'argv',args),patch.object(driver,'command') as command:
   with self.assertRaises(SystemExit):driver.main()
   command.assert_not_called()
if __name__=='__main__':unittest.main()
