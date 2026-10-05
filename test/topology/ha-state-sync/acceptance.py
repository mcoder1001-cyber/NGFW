#!/usr/bin/env python3
"""Concrete isolated two-appliance EI/VRRP acceptance; never reconnect the TCP flow."""
import argparse
import hashlib
import ipaddress
import json
import os
from pathlib import Path
import re
import selectors
import shlex
import subprocess
import sys
import time
import urllib.request
from urllib.parse import quote, urlencode, urlparse


class Refused(RuntimeError):
    pass


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        raise Refused('API redirect refused')


class Api:
    def __init__(self, base, token):
        parsed = urlparse(base)
        if (parsed.scheme != 'https' or not parsed.hostname or parsed.username or
                parsed.password or parsed.path not in ('', '/') or parsed.query or parsed.fragment):
            raise Refused('credential-free HTTPS appliance origin required')
        if not token:
            raise Refused('dedicated API key required')
        self.base, self.token = base.rstrip('/'), token
        self.opener = urllib.request.build_opener(NoRedirect())

    def call(self, method, path, body=None):
        request = urllib.request.Request(self.base + '/api/v1' + path, method=method,
            data=None if body is None else json.dumps(body).encode(),
            headers={'Authorization': 'ApiKey ' + self.token, 'Content-Type': 'application/json'})
        try:
            with self.opener.open(request, timeout=20 if path == '/actions/ha/sync/resync' else 3) as response:
                raw = response.read(1048577)
                if response.status != 200 or len(raw) > 1048576:
                    raise Refused('unexpected API status or oversized response')
        except Refused:
            raise
        except Exception:
            raise Refused('appliance API request failed; response withheld') from None
        return json.loads(raw)


def digest(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True, separators=(',', ':')).encode()).hexdigest()


def router(api, name):
    matches = [r for r in api.call('GET', '/state/ha/vrrp')['routers'] if r['name'] == name]
    if len(matches) != 1 or matches[0].get('error'):
        raise Refused('VRRP router unavailable/ambiguous')
    return matches[0]


def identity(session):
    return tuple(session[k] for k in ('insideAddress', 'insidePort', 'outsideAddress',
                                      'outsidePort', 'protocol', 'tableId'))


def session(api, flow, vrf):
    query = urlencode({'inside': flow['inside'][0], 'port': flow['inside'][1],
                       'protocol': 'tcp', 'vrf': vrf, 'pageSize': 256})
    page = api.call('GET', '/state/nat/ei/sessions?' + query)
    if page.get('truncated') or page['total'] > len(page['items']):
        raise Refused('session evidence truncated')
    matches = [s for s in page['items'] if s['insideAddress'] == flow['inside'][0]
        and s['insidePort'] == flow['inside'][1] and s['protocol'].lower() in ('tcp', '6')
        and s['vrf'] == vrf and not s['timedOut']]
    if len(matches) != 1:
        raise Refused('exact active TCP session absent or ambiguous')
    result = matches[0]
    if [result['outsideAddress'], result['outsidePort']] != flow['outside']:
        raise Refused('NAT dump disagrees with actual server-observed tuple')
    return result


class Worker:
    def __init__(self, namespace, inside, server, port):
        if not re.fullmatch(r'w(?:[1-9]|[12][0-9]|3[0-2])-ha-client', namespace):
            raise Refused('allocated lab client namespace required')
        ipaddress.IPv4Address(inside)
        ipaddress.IPv4Address(server)
        if not 1024 <= port <= 65535:
            raise Refused('unprivileged echo port required')
        script = str(Path(__file__).with_name('flow.py').resolve())
        # No appliance credentials are inherited by the traffic generator.
        self.process = subprocess.Popen(['ip', 'netns', 'exec', namespace, sys.executable,
            script, '--inside', inside, '--address', server, '--port', str(port)],
            stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
            env={'PATH': '/usr/sbin:/usr/bin:/sbin:/bin'}, bufsize=0)
        self.selector = selectors.DefaultSelector()
        self.selector.register(self.process.stdout, selectors.EVENT_READ)
        self.sequence = 0
        self.tuple = None

    def exchange(self):
        self.process.stdin.write(b'exchange\n')
        if not self.selector.select(11):
            raise Refused('established flow stopped responding')
        raw = self.process.stdout.readline(4097)
        if not raw or len(raw) > 4096:
            raise Refused('flow worker failed')
        response = json.loads(raw)
        self.sequence += 1
        if response['sequence'] != self.sequence:
            raise Refused('flow worker sequence changed')
        current = (response['inside'], response['outside'])
        if self.tuple is not None and current != self.tuple:
            raise Refused('flow reconnected or changed translation')
        self.tuple = current
        return response

    def close(self):
        self.process.stdin.close()
        try:
            self.process.wait(timeout=4)
        except subprocess.TimeoutExpired:
            self.process.terminate()
            self.process.wait(timeout=4)
        self.selector.close()


class PriorityTransition:
    """Use an unconfirmed commit: the appliance auto-reverts even if this probe dies."""
    def __init__(self, api, name, priority):
        self.api, self.name, self.priority = api, name, priority
        self.owner = None
        self.expected_candidate = None
        self.txn = None

    def lock(self):
        state = self.api.call('GET', '/config/lock')
        if not self.owner or (state.get('ownerKeyId'), state.get('lockedAt')) != self.owner:
            raise Refused('candidate lease changed; preserve all work')

    def begin(self):
        api = self.api
        if api.call('GET', '/config/commit/pending')['pending']:
            raise Refused('pending commit already exists')
        if api.call('GET', '/config/lock')['locked']:
            raise Refused('candidate already locked')
        difference = api.call('GET', '/config/diff')
        if difference['changes']:
            raise Refused('candidate must be clean')
        self.baseline = difference['baseRevision']
        self.original = api.call('GET', '/config')
        if self.original['ha'].get('cluster', {}).get('configSync', False):
            raise Refused('lab priority transition requires configuration sync disabled')
        api.call('PATCH', '/config', {})
        state = api.call('GET', '/config/lock')
        if not state.get('ownerKeyId'):
            raise Refused('dedicated API key lock required')
        self.owner = (state['ownerKeyId'], state['lockedAt'])
        difference = api.call('GET', '/config/diff')
        candidate = api.call('GET', '/config/candidate')
        if difference['baseRevision'] != self.baseline or difference['changes'] or digest(candidate) != digest(self.original):
            raise Refused('configuration changed while acquiring lease')
        self.expected_candidate = digest(candidate)
        self.lock()
        path = '/config/ha/vrrp/' + quote(self.name, safe='')
        api.call('PATCH', path, {'priority': self.priority})
        candidate['ha']['vrrp'][self.name]['priority'] = self.priority
        self.expected_candidate = digest(candidate)
        if digest(api.call('GET', '/config/candidate')) != self.expected_candidate:
            raise Refused('candidate contains unexpected concurrent edits')
        self.lock()
        api.call('POST', '/config/validate', {})
        response = api.call('POST', '/config/commit?confirm=60', {})
        if response.get('status') != 'pending' or response.get('notApplied') or not response.get('txnId'):
            raise Refused('bounded priority commit was not fully applied')
        self.txn = response['txnId']
        pending = api.call('GET', '/config/commit/pending')['pending']
        if not pending or pending['txnId'] != self.txn:
            raise Refused('priority commit outcome ambiguous')

    def cleanup(self):
        # Never confirm or override an ambiguous/foreign transaction.
        if self.txn:
            deadline = time.monotonic() + 75
            while time.monotonic() < deadline:
                pending = self.api.call('GET', '/config/commit/pending')['pending']
                if not pending:
                    if digest(self.api.call('GET', '/config')) != digest(self.original):
                        raise Refused('automatic revert did not restore original configuration')
                    self.discard_owned()
                    return
                if pending['txnId'] != self.txn:
                    raise Refused('pending transaction changed; preserve all work')
                time.sleep(1)
            raise Refused('automatic revert not observed')
        self.discard_owned()

    def discard_owned(self):
        if self.owner and self.expected_candidate:
            if self.api.call('GET', '/config/commit/pending')['pending']:
                raise Refused('ambiguous pending commit; cleanup refused')
            self.lock()
            if self.api.call('GET', '/config/diff')['baseRevision'] != self.baseline:
                raise Refused('running revision changed; cleanup refused')
            if digest(self.api.call('GET', '/config/candidate')) != self.expected_candidate:
                raise Refused('candidate changed; cleanup refused')
            self.api.call('POST', '/config/discard', {})


def exercise(a, b, worker, name, vrf, failover, killed=False):
    if router(a, name)['state'].lower() != 'master' or router(b, name)['state'].lower() != 'backup':
        raise Refused('initial MASTER/BACKUP roles required')
    for api in (a, b):
        kinds = api.call('GET', '/state/ha/sync')['kinds']
        if not any(k['kind'] == 'nat44-ei' and k['active'] for k in kinds):
            raise Refused('both observed EI endpoints must be active')
    flow = worker.exchange()
    source = session(a, flow, vrf)
    a.call('POST', '/actions/ha/sync/resync', {})
    deadline = time.monotonic() + 15
    while True:
        flow = worker.exchange()
        try:
            replica = session(b, flow, vrf)
            if identity(replica) != identity(source):
                raise Refused('peer session differs')
            break
        except Refused:
            if time.monotonic() >= deadline:
                raise
            time.sleep(.1)
    before = replica['packets']
    start = time.monotonic()
    failover()
    deadline = start + 20
    while True:
        flow = worker.exchange()  # failure/drop aborts; never opens a replacement socket
        replica = session(b, flow, vrf)
        if identity(replica) != identity(source):
            raise Refused('replicated tuple changed across failover')
        backup = True if killed else router(a, name)['state'].lower() == 'backup'
        if backup and router(b, name)['state'].lower() == 'master' and replica['packets'] > before:
            break
        if time.monotonic() >= deadline:
            raise Refused('VRRP roles or packet forwarding did not converge')
        time.sleep(.1)
    return {'two_node_continuity': 'passed', 'same_connection_exchanges': flow['sequence'],
            'exact_session': dict(zip(('insideAddress', 'insidePort', 'outsideAddress',
                'outsidePort', 'protocol', 'tableId'), identity(replica))),
            'convergence_upper_bound_ms': round((time.monotonic() - start) * 1000)}


class KillTransition:
    def __init__(self, target, boot, pid, nonce, handover):
        if (not handover or not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9.-]{0,252}', target or '')
                or not re.fullmatch(r'[0-9a-f-]{36}', boot or '') or not pid or pid <= 1
                or not re.fullmatch(r'[0-9a-f]{32,64}', nonce or '')):
            raise Refused('kill mode requires explicit appliance SSH target, boot/PID and post-handover nonce')
        self.target, self.boot, self.pid, self.nonce = target, boot, pid, nonce

    def begin(self):
        remote = shlex.join(['/usr/local/libexec/ngfw-ha-lab-kill-vpp', '--boot-id',
            self.boot, '--pid', str(self.pid), '--handover-nonce', self.nonce, '--after-handover'])
        result = subprocess.run(['ssh', '-oBatchMode=yes', '-oStrictHostKeyChecking=yes',
            '-oConnectTimeout=3', 'root@' + self.target, remote], timeout=8,
            stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, check=False)
        if result.returncode or len(result.stdout) > 4096:
            raise Refused('isolated appliance fault injection refused/failed')
        proof = json.loads(result.stdout)
        if proof != {'bootId': self.boot, 'pid': self.pid, 'fault': 'VPP SIGKILL'}:
            raise Refused('unexpected appliance fault evidence')

    def cleanup(self):
        # Service supervision belongs to the isolated appliance, never this shared host.
        pass


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('node-a', 'node-b', 'namespace', 'inside', 'echo-address', 'router', 'output'):
        parser.add_argument('--' + name, required=True)
    parser.add_argument('--echo-port', type=int, default=18750)
    parser.add_argument('--vrf', default='default')
    parser.add_argument('--isolated-lab', action='store_true', required=True)
    parser.add_argument('--kill-vpp', action='store_true')
    parser.add_argument('--after-handover', action='store_true')
    parser.add_argument('--ssh-node-a')
    parser.add_argument('--boot-id-a')
    parser.add_argument('--vpp-pid-a', type=int)
    parser.add_argument('--handover-nonce')
    args = parser.parse_args()
    a = Api(args.node_a, os.environ['NGFW_HA_TOKEN_A'])
    b = Api(args.node_b, os.environ['NGFW_HA_TOKEN_B'])
    if a.base == b.base:
        raise Refused('distinct appliances required')
    if args.kill_vpp:
        if urlparse(a.base).hostname != args.ssh_node_a:
            raise Refused('SSH fault target must match appliance A origin')
        transition = KillTransition(args.ssh_node_a, args.boot_id_a, args.vpp_pid_a,
            args.handover_nonce, args.after_handover)
    else:
        priority = router(b, args.router)['currentPriority'] - 1
        if priority < 1:
            raise Refused('backup priority must permit a bounded transition')
        transition = PriorityTransition(a, args.router, priority)
    # Reserve private evidence before any mutation; refuse overwrite/symlink.
    fd = os.open(args.output, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
    with os.fdopen(fd, 'w') as evidence:
        worker = Worker(args.namespace, args.inside, args.echo_address, args.echo_port)
        try:
            report = exercise(a, b, worker, args.router, args.vrf, transition.begin, args.kill_vpp)
        finally:
            worker.close()
            transition.cleanup()
        if not args.kill_vpp and (router(a, args.router)['state'].lower() != 'master' or
                                 router(b, args.router)['state'].lower() != 'backup'):
            raise Refused('original VRRP roles did not return after automatic revert')
        json.dump(report, evidence, indent=2)


if __name__ == '__main__':
    main()
