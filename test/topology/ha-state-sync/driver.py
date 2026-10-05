#!/usr/bin/env python3
"""Observe two isolated appliances and run explicit lab probes; no implicit host writes."""
import argparse
import json
import os
import subprocess
import time
import urllib.request
from urllib.parse import urlparse

from acceptance import Api, Refused


def fetch(base, token, path, action=False):
    api = Api(base, token)  # Validates the origin and installs redirect refusal.
    req = urllib.request.Request(api.base + '/api/v1/' + path,
                                 data=b'' if action else None,
                                 headers={'Authorization': 'Bearer ' + token})
    try:
        with api.opener.open(req, timeout=25) as response:
            raw = response.read(1048577)
            if response.status != 200 or len(raw) > 1048576:
                raise Refused('unexpected API status or oversized response')
        return json.loads(raw)
    except Refused:
        raise
    except Exception:
        raise Refused('appliance API request failed; response withheld') from None


def command(args):
    result = subprocess.run(args, timeout=60, check=False, capture_output=True, text=True)
    if result.returncode:
        raise RuntimeError('lab command failed: ' + result.stderr[:1000])
    return result.stdout[:8000]


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--node-a', required=True)
    p.add_argument('--node-b', required=True)
    p.add_argument('--output', required=True)
    p.add_argument('--isolated-lab', action='store_true')
    p.add_argument('--exercise', action='store_true')
    p.add_argument('--kill-vpp', action='store_true')
    p.add_argument('--after-handover', action='store_true')
    # JSON argv keeps shell expansion out of probe/failover commands.
    p.add_argument('--create-session', type=json.loads)
    p.add_argument('--verify-session-on-b', type=json.loads)
    p.add_argument('--failover-command', type=json.loads)
    p.add_argument('--continuity-probe', type=json.loads)
    a = p.parse_args()
    for base in (a.node_a, a.node_b):
        u = urlparse(base)
        if u.scheme != 'https' or not u.hostname or u.username or u.password:
            p.error('use credential-free HTTPS URLs with trusted certificates')
    if a.node_a.rstrip('/') == a.node_b.rstrip('/'):
        p.error('two distinct appliances are required')
    if a.kill_vpp and not (a.exercise and a.after_handover):
        p.error('--kill-vpp requires --exercise --after-handover and explicit failover-command')
    if a.exercise and not (a.isolated_lab and all((a.create_session, a.verify_session_on_b,
                                                a.failover_command, a.continuity_probe))):
        p.error('exercise requires isolated lab and all four explicit argv probes')
    for argv in (a.create_session, a.verify_session_on_b, a.failover_command, a.continuity_probe):
        if argv is not None and (not isinstance(argv, list) or not argv or
                                 not all(isinstance(v, str) for v in argv)):
            p.error('probe commands must be nonempty JSON arrays of strings')
    tokens = (os.environ['NGFW_HA_TOKEN_A'], os.environ['NGFW_HA_TOKEN_B'])
    report = {'two_node_continuity': 'not exercised', 'native_observations': [
        fetch(base, token, 'state/ha/sync') for base, token in zip((a.node_a, a.node_b), tokens)]}
    if a.exercise:
        if not all(any(k['kind'] == 'nat44-ei' and k['active'] for k in s['kinds'])
                   for s in report['native_observations']):
            p.error('both appliances must report observed EI endpoints active')
        report['create_session'] = command(a.create_session)
        report['resync'] = fetch(a.node_a, tokens[0], 'actions/ha/sync/resync', True)
        report['session_on_b'] = command(a.verify_session_on_b)
        start = time.monotonic()
        report['failover'] = command(a.failover_command)
        report['continuity'] = command(a.continuity_probe)
        report['convergence_upper_bound_ms'] = round((time.monotonic() - start) * 1000)
        report['two_node_continuity'] = 'explicit probes passed; inspect probe evidence'
    with open(a.output, 'x', encoding='utf-8') as f:
        json.dump(report, f, indent=2)


if __name__ == '__main__':
    main()
