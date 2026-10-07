#!/usr/bin/env python3
"""Replay the merged slot14 MPLS/SRv6 real API/browser campaign against this worktree's builds.

Run from the worktree root: python3 <this> <attempt>. Same runner, same deadlines and assertions as
the original attempt11 launcher; only the product artifacts point at this worktree (agent with the
AutoBlock confirmed-inactive no-op). The browser binary is the previously verified Chrome bundle.
"""
import os
import signal
import subprocess
import sys
from pathlib import Path

root = Path.cwd()
attempt = sys.argv[1]
out = root / '.scratch' / ('mpls-srv6-replay' + attempt)
out.mkdir(parents=True, exist_ok=True)
browser = Path('/root/.codex/worktrees/6189/NGFW/artifacts/test-closeout/browser/root')
env = dict(os.environ, NGFW_SLOT='14', NGFW_TEST_PREFIX='w14', NGFW_VPP_TABLE_BASE='14000', NGFW_HTTP_PORT='11400',
           NGFW_AGENT_SOCKET='/run/ngfw-test/w14/agent.sock', NGFW_METRICS_ADDR='off', NGFW_VALKEY_DB='14',
           NGFW_ISOLATED_TEST_RUN='1', NGFW_BROWSER_OUTPUT=str(out),
           NGFW_BROWSER_WEB_DIST=str(root / 'apps/web/dist'), NGFW_BROWSER_VERIFIED_WORKTREE=str(root),
           NGFW_BROWSER_VITE=str(root / 'apps/web/node_modules/.bin/vite'),
           NGFW_BROWSER_BINARY=str(browser / 'opt/google/chrome/chrome'),
           NGFW_ACCEPTANCE_AGENT=str(root / 'apps/agent/bin/ngfw-agent'),
           NGFW_ACCEPTANCE_API_MAIN=str(root / 'apps/api/dist/main.js'),
           NGFW_SHARED_CLI_INODE=str(os.stat('/run/vpp/cli.sock').st_ino), TMPDIR='/root/.cache/ngfw-ci-host',
           LD_LIBRARY_PATH=str(browser / 'usr/lib/x86_64-linux-gnu'))
env.pop('NGFW_VPP_ID_RANGE', None)
# Launcher-only warm-up (diagnosis of replays 1/2/7): the owned API spends 3.5-19 s wall (4-8 s CPU,
# rest IO/contention) merely importing dist/app.js from a cold page cache, while the unchanged fixture
# waits ~30 s for /health. Import the same module graph once beforehand so the measured run starts
# warm. No product, assertion or deadline change; importing app.js has no side effects (no listen).
WARM_JS = ("const t=Date.now();import(process.argv[1]).then(()=>{"
           "console.log('API_IMPORT_WARM_MS='+(Date.now()-t));process.exit(0)})")
warm = subprocess.run(['node', '-e', WARM_JS, str(root / 'apps/api/dist/app.js')], cwd=root / 'apps/api',
                      stdout=subprocess.PIPE, text=True, timeout=120, check=True)
print(warm.stdout.strip(), flush=True)
with (out / 'actual.log').open('wb') as log:
    child = subprocess.Popen(['flock', '-n', '/run/lock/ngfw-acceptance-slot14.lock', 'python3',
                              'test/topology/hardware-smoke/isolated-vpp.py', 'python3',
                              'test/topology/mpls-srv6-browser-live/run.py'],
                             env=env, stdout=log, stderr=subprocess.STDOUT, start_new_session=True)
    try:
        code = child.wait(timeout=900)
    finally:
        try:
            os.killpg(child.pid, signal.SIGTERM)
        except ProcessLookupError:
            pass
        if child.poll() is None:
            try:
                child.wait(timeout=20)
            except subprocess.TimeoutExpired:
                os.killpg(child.pid, signal.SIGKILL)
                child.wait()
print('MPLS_SRV6_REPLAY' + attempt + '_EXIT=' + str(code))
raise SystemExit(code)
