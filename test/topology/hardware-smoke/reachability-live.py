#!/usr/bin/env python3
"""Authenticate to the disposable slot API without logging its bearer token."""
import json
import os
from pathlib import Path
import subprocess
import sys
import urllib.request

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from private_http import private_opener

prefix = os.environ['NGFW_TEST_PREFIX']
base = 'http://127.0.0.1:' + os.environ['NGFW_HTTP_PORT']
password = Path('/run/ngfw-test/' + prefix + '/admin.pw').read_text().strip()
req = urllib.request.Request(base + '/api/v1/auth/login', data=json.dumps({'username':'admin','password':password}).encode(), headers={'Content-Type':'application/json'}, method='POST')
with private_opener().open(req, timeout=20) as response:
    token = json.load(response)['accessToken']
env = dict(os.environ, NGFW_API_URL=base, NGFW_API_TOKEN=token,
           NGFW_REACH_LOOPBACK=str(int(os.environ['NGFW_SLOT']) * 100 + 99))
raise SystemExit(subprocess.call(['go', '-C', 'test/integration/reachability', 'test', '-json', '-race', '-count=1', '-timeout', '5m', '-run', 'TestReachabilityLoopback', './...'], env=env))
