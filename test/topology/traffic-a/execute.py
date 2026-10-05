#!/usr/bin/env python3
"""Lease-gated composed Wave-A transaction, rig peers and private packet evidence."""
import argparse
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import selectors
import secrets
import signal
import stat
import subprocess
import time
import urllib.error
import sys
import urllib.request

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from private_http import private_opener
sys.dont_write_bytecode = True
from composed import topology
from evidence import read_pcap
from scenario import Refused, validate_environment, slot_values

class Api:
    def __init__(self, slot, token):
        if slot not in (*range(1, 12), *range(14, 33)) or not token:
            raise Refused('allocated developer slot and access token required')
        port = 3000 + slot * 100 if slot < 12 else 10000 + slot * 100
        self.base = f'http://127.0.0.1:{port}/api/v1'
        self.token = token

    def call(self, method, path, body=None, want=200):
        data = None if body is None else json.dumps(body).encode()
        request = urllib.request.Request(self.base + path, data=data, method=method,
            headers={'Authorization': 'ApiKey ' + self.token, 'Content-Type': 'application/json'})
        try:
            response = private_opener().open(request, timeout=30)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            status = response.status
            raw = response.read(1048577)
            content_type = response.headers.get('Content-Type', '')
        if len(raw) > 1048576 or status != want:
            raise Refused(f'{method} {path}: HTTP {status}, expected {want}')
        if want == 204:
            if raw:
                raise Refused('DELETE response unexpectedly contains a body')
            return {}
        result = json.loads(raw)
        if want == 400 and not content_type.startswith('application/problem+json'):
            raise Refused('validation response is not problem+json')
        return result


def packet_proof(slot, mode, lan_bytes, wan_bytes, probes):
    lan, _ = read_pcap(lan_bytes)
    wan, _ = read_pcap(wan_bytes)
    address = lambda third, last: f'10.{slot}.{third}.{last}'
    paths = set()
    mappings = []
    for flow in probes:
        inputs = [p for p in lan if p.protocol == 6 and p.source == address(10, 2) and
                  p.destination == address(99, 1) and p.source_port == flow['source'] and
                  p.destination_port == flow['port'] and p.tcp_flags == 2]
        if not inputs or any(p.vlans != (100,) for p in inputs):
            raise Refused('missing tagged input SYN for declared probe')
        initial = inputs[0]
        outputs = [p for p in wan if p.protocol == 6 and p.destination == initial.destination and
                   p.destination_port == initial.destination_port and p.ip_id == initial.ip_id and
                   p.tcp_sequence == initial.tcp_sequence and p.tcp_flags == initial.tcp_flags]
        if not flow['expected']:
            if outputs or flow['success']:
                raise Refused('ACL deny leaked a correlated output or successful connection')
            continue
        if not flow['success'] or not outputs:
            raise Refused('declared forwarded connection has no WAN packet')
        if any(p.source != address(21, 100) or p.ttl != initial.ttl - 1 or
               p.payload_sha256 != initial.payload_sha256 or p.vlans not in ((201,), (202,)) for p in outputs):
            raise Refused('NAT/TTL/payload/VLAN forwarding evidence differs')
        if flow['port'] == 8001 and any(p.vlans != (202,) for p in outputs):
            raise Refused('PBR flow used a path other than202')
        if flow['port'] == 8000 and flow['source'] != 48000:
            paths.update(p.vlans[0] for p in outputs)
        if flow['source'] == 48000:
            mappings.append((outputs[0].source, outputs[0].source_port))
        if not any(p.protocol == 6 and p.destination == initial.source and p.destination_port == initial.source_port
                   and p.source == initial.destination and p.source_port == initial.destination_port
                   and p.tcp_flags is not None and p.tcp_flags & 0x12 == 0x12 for p in lan):
            raise Refused('translated TCP response did not return to original inside endpoint')
    if paths != {201, 202}:
        raise Refused('ECMP did not exercise both WAN VLANs')
    echoes = [p for p in lan if p.protocol == 1 and p.icmp_type == 8 and p.source == address(10, 2)]
    spoof = [p for p in lan if p.protocol == 1 and p.icmp_type == 8 and p.source == address(9, 9)]
    if not echoes or not spoof or not any(p.protocol == 1 and p.icmp_type == 0 and p.destination == address(10, 2) for p in lan):
        raise Refused('valid ping/reply and spoof ingress must actually be captured')
    if not any(p.protocol == 1 and p.icmp_type == 8 and p.source == address(21, 100) for p in wan):
        raise Refused('translated ICMP forward missing')
    for packet in spoof:
        if any(p.protocol == 1 and p.icmp_type == 8 and p.ip_id == packet.ip_id and
               p.icmp_sequence == packet.icmp_sequence and p.payload_sha256 == packet.payload_sha256 for p in wan):
            raise Refused('uRPF spoof leaked onto WAN')
    if mode == 'ei' and (len(mappings) != 2 or len(set(mappings)) != 1):
        raise Refused('EI source mapping differs across destination ports')
    return {'mode': mode, 'ECMP': sorted(paths), 'PBR': 202, 'ACL_deny': 'observed',
            'uRPF_spoof': 'observed', 'translated_source': address(21, 100),
            'TCP_return': 'observed', 'EI_mapping': mappings if mode == 'ei' else None}


def urpf_drops(text):
    total = 0
    for line in text.splitlines():
        match = re.match(r'^\s*([0-9]+)\s+ip4-rx-urpf-strict\s+(.+)$', line)
        if match and re.search(r'fail|drop', match[2], re.I):
            total += int(match[1])
    return total


def readback_proof(slot, snapshots, errors_before, errors_after, before, after):
    required = {'bridge': (f'loop{slot}10', f'host-w{slot}l0.100'),
                'fib': (f'10.{slot}.21.2', f'10.{slot}.22.2'),
                'abf': (f'loop{slot}10',), 'acl': (f'w{slot}-in', f'w{slot}-pbr'),
                'nat': (f'10.{slot}.10.2', f'10.{slot}.21.100')}
    for name, needles in required.items():
        if any(value not in snapshots.get(name, '') for value in needles):
            raise Refused('live CLI readback missing composed ' + name + ' binding')
    if urpf_drops(errors_after) <= urpf_drops(errors_before):
        raise Refused('spoof drop has no strict-uRPF counter increase')
    expected = [f'host-w{slot}l0.100', f'host-w{slot}w0.201', f'host-w{slot}w0.202']
    for interface in expected:
        if interface not in before or interface not in after:
            raise Refused('missing allocated-interface counters')
        old, new = before[interface], after[interface]
        if new['name'] != interface or new['vppName'] != interface or old['name'] != interface or old['vppName'] != interface:
            raise Refused('counter identity outside allocated path')
        old_sum = int(old['counters']['rxPackets']) + int(old['counters']['txPackets'])
        new_sum = int(new['counters']['rxPackets']) + int(new['counters']['txPackets'])
        if new_sum <= old_sum:
            raise Refused('composed-interface packets did not increase')
    return {'strict_uRPF_drop_delta': urpf_drops(errors_after) - urpf_drops(errors_before),
            'counter_interfaces': expected, 'CLI_bindings': sorted(required)}


class Runner:
    def __init__(self, slot, root, api):
        self.slot, self.root, self.api = slot, root, api
        self.repo = Path(__file__).resolve().parents[3]
        self.commands = 0
        self.children = []
        self.prefix = f'w{slot}'
        self.boot = Path('/proc/sys/kernel/random/boot_id').read_text().strip()
        self.lease_id = None

    def lease(self):
        path = Path(f'/run/ngfw-test/{self.prefix}/traffic-a-lease.json')
        fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
        try:
            info = os.fstat(fd)
            if not stat.S_ISREG(info.st_mode) or info.st_uid != 0 or stat.S_IMODE(info.st_mode) != 0o600 or not 0 < info.st_size <= 4096:
                raise Refused('manager lease must be root-owned bounded regular0600')
            def unique(pairs):
                value = {}
                for key, item in pairs:
                    if key in value:
                        raise Refused('duplicate manager lease field')
                    value[key] = item
                return value
            value = json.loads(os.read(fd, 4097), object_pairs_hook=unique)
        finally:
            os.close(fd)
        if (set(value) != {'task', 'slot', 'prefix', 'boot_id', 'expires_unix', 'lease_id'} or
            value['task'] != 'TEST-traffic-A' or type(value['slot']) is not int or value['slot'] != self.slot or value['prefix'] != self.prefix or
            value['boot_id'] != self.boot or not re.fullmatch('[0-9a-f]{32}', value['lease_id']) or
            type(value['expires_unix']) not in (int, float) or not time.time() < value['expires_unix'] <= time.time() + 7200):
            raise Refused('manager lease identity/window invalid')
        if self.lease_id is not None and self.lease_id != value['lease_id']:
            raise Refused('manager lease replaced during transaction')
        self.lease_id = value['lease_id']

    def command(self, argv, expected=0):
        self.lease()
        self.commands += 1
        from commands import run_command, CommandFailed
        log = self.root / f'command-{self.commands:03d}.txt'
        try:
            result = run_command(argv, self.repo, dict(os.environ, LC_ALL='C'), log, 90)
        except CommandFailed:
            if expected != 'failure':
                raise
            return log.read_text()
        if expected == 'failure':
            raise Refused('negative traffic probe unexpectedly succeeded')
        return log.read_text()

    def ns(self, side, *argv, expected=0):
        return self.command(['ip', 'netns', 'exec', f'ns-{self.prefix}-{side}', *argv], expected)

    def vpp(self, *arguments):
        return self.command(['timeout', '10', 'vppctl', *arguments])

    def spawn(self, argv, name):
        self.lease()
        log = open(self.root / (name + '.txt'), 'xb', buffering=0)
        process = subprocess.Popen(argv, stdin=subprocess.DEVNULL, stdout=log, stderr=log,
            start_new_session=True, env=dict(os.environ, LC_ALL='C'))
        self.children.append((process, log))
        return process

    @staticmethod
    def stop(process):
        if process.poll() is None:
            os.killpg(process.pid, signal.SIGINT)
        try:
            process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            os.killpg(process.pid, signal.SIGKILL)
            process.wait(timeout=5)

    def peers_down(self):
        errors = []
        for side, letter in (('lan', 'l'), ('wan', 'w')):
            for operation in (lambda side=side, letter=letter: self.ns(side, 'ip', 'link', 'set', self.prefix + letter + '1', 'down'),
                              lambda letter=letter: self.command(['ip', 'link', 'set', self.prefix + letter + '0', 'down'])):
                try:
                    operation()
                except (OSError, RuntimeError, ValueError) as error:
                    errors.append(str(error))
        if errors:
            raise Refused('owned links could not all be lowered: ' + '; '.join(errors))

    def restore_original(self, original_revision, current_revision):
        # D-101: never delete an af_packet interface while its veth can transmit.
        self.peers_down()
        if original_revision in (None, 0):
            return self.commit({'interfaces': {f'host-{self.prefix}l0': None, f'host-{self.prefix}w0': None}}, current_revision)
        self.claim(current_revision)
        result = self.api.call('POST', f'/config/rollback/{original_revision}?comment=traffic-a-original')
        if result.get('status') != 'applied':
            raise Refused('original slot rollback failed')
        return result['revision']['id']

    def cleanup(self, rig_owned):
        errors = []
        # Every owned PID is stopped before any fixture/host cleanup error can escape.
        for process, log in reversed(self.children):
            try:
                self.stop(process)
            except (OSError, subprocess.SubprocessError) as error:
                errors.append(str(error))
            finally:
                log.close()
        if rig_owned:
            try:
                self.peers_down()
            except (OSError, RuntimeError, ValueError) as error:
                errors.append(str(error))
        if hasattr(self, 'fixture'):
            try:
                fcntl.flock(self.global_lock, fcntl.LOCK_UN)
                self.fixture.stdin.close()
                try:
                    self.fixture.wait(timeout=15)
                except subprocess.TimeoutExpired:
                    self.stop(self.fixture)
                if self.fixture.returncode != 0:
                    errors.append('owned NAT fixture cleanup failed')
                fcntl.flock(self.global_lock, fcntl.LOCK_SH | fcntl.LOCK_NB)
            except (OSError, subprocess.SubprocessError) as error:
                errors.append(str(error))
                self.stop(self.fixture)
            finally:
                self.fixture.stdout.close()
                self.fixture_log.close()
        if errors:
            raise Refused('cleanup retained down rig/evidence; no PASS: ' + '; '.join(errors))
        if rig_owned:
            running = self.api.call('GET', '/config')
            if running.get('interfaces') or self.api.call('GET', '/config/diff').get('changes'):
                raise Refused('unfinished applied transaction: rig retained down for manager recovery')
            self.command([str(self.repo / 'tools/lab'), 'rig', 'down', self.prefix])
            residue = self.vpp('show', 'interface')
            namespaces = json.loads(self.command(['ip', '-j', 'netns', 'list']))
            if self.prefix in residue or any(item['name'].startswith('ns-' + self.prefix + '-') for item in namespaces):
                raise Refused('slot rig residue after cleanup')

    def claim(self, expected_revision):
        if self.api.call('GET', '/config/lock').get('locked'):
            raise Refused('candidate already locked')
        self.api.call('PATCH', '/config', {})
        owner = self.api.call('GET', '/config/lock')
        difference = self.api.call('GET', '/config/diff')
        if not owner.get('ownerKeyId') or difference != {'baseRevision': expected_revision, 'changes': []}:
            raise Refused('candidate/revision changed while claiming; preserve for manager')
        return owner

    def commit(self, patch, expected_revision):
        self.lease()
        owner = self.claim(expected_revision)
        self.api.call('PATCH', '/config', patch)
        current = self.api.call('GET', '/config/lock')
        if current.get('ownerKeyId') != owner['ownerKeyId'] or current.get('lockedAt') != owner['lockedAt']:
            raise Refused('candidate ownership changed')
        result = self.api.call('POST', '/config/commit?comment=traffic-a-composed')
        (self.root / f'commit-{expected_revision}.json').write_text(json.dumps(result, indent=2))
        if result.get('status') != 'applied' or result.get('notApplied'):
            raise Refused('whole-chain commit not fully applied; preserve state for manager')
        drift = self.api.call('GET', '/state/drift')
        selected = ('/interfaces', '/vrfs', '/routing', '/acl', '/nat', '/objects')
        if drift.get('changes') or any(issue.get('pointer', '').startswith(selected) for issue in drift.get('ignored', [])):
            raise Refused('applied chain Retrieve differs from desired')
        return result['revision']['id']

    def fixture_start(self):
        self.lease()
        binary = self.repo / 'test/topology/traffic-a/globals/traffic-a-globals'
        self.fixture_log = open(self.root / 'nat-fixture.txt', 'xb')
        self.fixture = subprocess.Popen([str(binary), '--slot', str(self.slot)], stdin=subprocess.PIPE,
            stdout=subprocess.PIPE, stderr=self.fixture_log, start_new_session=True, env=dict(os.environ, LC_ALL='C'))

    def fixture_mode(self, mode):
        self.lease()
        fcntl.flock(self.global_lock, fcntl.LOCK_UN)
        self.fixture.stdin.write((mode + '\n').encode())
        self.fixture.stdin.flush()
        selector = selectors.DefaultSelector()
        try:
            selector.register(self.fixture.stdout, selectors.EVENT_READ)
            if not selector.select(15):
                raise Refused('NAT fixture response timeout')
            response = self.fixture.stdout.readline(4097)
            if not response.endswith(b'\n') or len(response) > 4096:
                raise Refused('NAT fixture response invalid')
            output = json.loads(response)
            if output.get('ok') is not True:
                raise Refused('NAT fixture refused: ' + output.get('error', 'unknown'))
        finally:
            selector.close()
            fcntl.flock(self.global_lock, fcntl.LOCK_SH | fcntl.LOCK_NB)

    def configure_peers(self):
        prefix = self.prefix
        for side, letter, vlans in (('lan', 'l', (100,)), ('wan', 'w', (201, 202))):
            parent = prefix + letter + '1'
            for vlan in vlans:
                device = parent + '.' + str(vlan)
                self.ns(side, 'ip', 'link', 'add', 'link', parent, 'name', device, 'type', 'vlan', 'id', str(vlan))
                self.ns(side, 'ip', 'link', 'set', device, 'up')
                third = 10 if side == 'lan' else vlan - 180
                self.ns(side, 'ip', 'address', 'add', f'10.{self.slot}.{third}.2/24', 'dev', device)
            self.ns(side, 'ip', 'link', 'set', parent, 'up')
            self.ns(side, 'ethtool', '-K', parent, 'tx', 'off')
            self.command(['ip', 'link', 'set', prefix + letter + '0', 'up'])
        self.ns('lan', 'ip', 'address', 'add', f'10.{self.slot}.9.9/32', 'dev', 'lo')
        self.ns('lan', 'ip', 'route', 'replace', 'default', 'via', f'10.{self.slot}.10.1')
        self.ns('wan', 'ip', 'address', 'add', f'10.{self.slot}.99.1/32', 'dev', 'lo')
        self.ns('wan', 'ip', 'route', 'replace', f'10.{self.slot}.21.100/32', 'via', f'10.{self.slot}.21.1', 'dev', prefix + 'w1.201')

    def traffic(self, mode):
        captures = []
        for side, letter in (('lan', 'l'), ('wan', 'w')):
            path = self.root / (mode + '-' + side + '.pcap')
            fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
            os.close(fd)
            process = self.spawn(['ip', 'netns', 'exec', f'ns-{self.prefix}-{side}', 'tcpdump', '-n', '-U',
                '-s', '65535', '-c', '2048', '-i', self.prefix + letter + '1', '-w', str(path),
                'vlan', 'and', '(icmp or tcp)'], mode + '-' + side + '-capture')
            captures.append((side, path, process))
        deadline = time.monotonic() + 5
        for side, _, process in captures:
            capture_log = self.root / (mode + '-' + side + '-capture.txt')
            while time.monotonic() < deadline and 'listening on' not in capture_log.read_text():
                if process.poll() is not None:
                    raise Refused('capture exited before probes')
                time.sleep(0.05)
            if 'listening on' not in capture_log.read_text():
                raise Refused('capture not ready before probe window')
        self.ns('lan', 'ping', '-n', '-c', '3', '-W', '1', f'10.{self.slot}.99.1')
        self.ns('lan', 'ping', '-n', '-c', '3', '-W', '1', '-I', f'10.{self.slot}.9.9', f'10.{self.slot}.99.1', expected='failure')
        probes = json.loads(self.ns('lan', 'python3', str(Path(__file__).with_name('peer.py')), 'client', '--slot', str(self.slot), '--mode', mode))
        time.sleep(2)
        data = {}
        for side, path, process in captures:
            self.stop(process)
            accounting = (self.root / (mode + '-' + side + '-capture.txt')).read_text()
            if re.findall(r'^([0-9]+) packets dropped by kernel$', accounting, re.M) != ['0'] or path.stat().st_size > 16 * 1024 * 1024:
                raise Refused('capture lost packets or exceeded byte bound')
            data[side] = path.read_bytes()
            _, count = read_pcap(data[side])
            captured = re.findall(r'^([0-9]+) packets captured$', accounting, re.M)
            received = re.findall(r'^([0-9]+) packets received by filter$', accounting, re.M)
            if captured != [str(count)] or len(received) != 1 or int(received[0]) < count:
                raise Refused('pcap record count does not match tcpdump accounting')
        proof = packet_proof(self.slot, mode, data['lan'], data['wan'], probes)
        proof['capture_sha256'] = {side: hashlib.sha256(raw).hexdigest() for side, raw in data.items()}
        return proof

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--slot', type=int, required=True)
    args = parser.parse_args()
    runner = None
    try:
        validate_environment(os.environ, args.slot)
        # A manager creates this protected directory and lease. The driver cannot grant a window.
        parent = Path(f'/run/ngfw-test/w{args.slot}/traffic-a')
        info = parent.lstat()
        if not stat.S_ISDIR(info.st_mode) or info.st_uid != 0 or stat.S_IMODE(info.st_mode) != 0o700:
            raise Refused('manager must provision private root-owned0700 evidence parent')
        run_id = secrets.token_hex(16)
        root = parent / run_id
        root.mkdir(mode=0o700)
        api = Api(args.slot, os.environ.get('NGFW_HOST_ACCESS_TOKEN', ''))
        runner = Runner(args.slot, root, api)
        runner.lease()
        with open('/run/lock/ngfw-lab.lock', 'a') as lab, open(f'/run/lock/ngfw-traffic-a-w{args.slot}.lock', 'a') as slot_lock, open(f'/run/ngfw-test/w{args.slot}/nat44.lock', 'a') as nat_lock, open('/run/lock/ngfw-globals.lock', 'a') as globals_lock:
            fcntl.flock(lab, fcntl.LOCK_SH | fcntl.LOCK_NB)
            fcntl.flock(slot_lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            fcntl.flock(nat_lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            runner.global_lock = globals_lock
            fcntl.flock(globals_lock, fcntl.LOCK_SH | fcntl.LOCK_NB)
            original = api.call('GET', '/config')
            if any(original.get(key) for key in ('interfaces', 'vrfs')) or original.get('nat', {}).get('inside') or original.get('nat', {}).get('outside'):
                raise Refused('isolated slot datastore must have no existing packet objects')
            namespaces = json.loads(runner.command(['ip', '-j', 'netns', 'list']))
            if any(item['name'].startswith('ns-' + runner.prefix + '-') for item in namespaces):
                raise Refused('slot namespace already exists; refuse to adopt unknown rig')
            interfaces = runner.vpp('show', 'interface')
            if runner.prefix in interfaces:
                raise Refused('slot VPP interface already exists; refuse adoption')
            # The slot agent can only require globals, never alter another owner's NAT configuration.
            # Manager provisions the ED/EI plugin/global prerequisites under the exclusive window.
            baseline_revision = api.call('GET', '/config/diff')['baseRevision']
            current_revision = baseline_revision
            restart_before = runner.command(['systemctl', 'show', 'vpp', '-p', 'NRestarts', '-p', 'MainPID'])
            rig_owned = False
            try:
                runner.command([str(runner.repo / 'apps/agent/bin/ngfw-vpp-preflight')])
                runner.command([str(runner.repo / 'tools/lab'), 'rig', 'up', runner.prefix])
                rig_owned = True
                for side, letter in (('lan', 'l'), ('wan', 'w')):
                    runner.ns(side, 'ip', 'link', 'set', runner.prefix + letter + '1', 'down')
                mac = json.loads(runner.ns('wan', 'ip', '-j', 'link', 'show', 'dev', runner.prefix + 'w1'))[0]['address']
                baseline, chain = topology(args.slot, mac)
                current_revision = runner.commit(baseline, current_revision)
                rig_revision = current_revision
                runner.fixture_start()
                runner.fixture_mode('ed')
                current_revision = runner.commit(chain, current_revision)
                runner.configure_peers()
                server = runner.spawn(['ip', 'netns', 'exec', f'ns-{runner.prefix}-wan', 'python3',
                    str(Path(__file__).with_name('peer.py')), 'server', '--slot', str(args.slot)], 'peer-server')
                deadline = time.monotonic() + 5
                while time.monotonic() < deadline and (root / 'peer-server.txt').read_text().count('READY:') != 3:
                    if server.poll() is not None:
                        raise Refused('TCP peer server exited')
                    time.sleep(0.05)
                if (root / 'peer-server.txt').read_text().count('READY:') != 3:
                    raise Refused('TCP peer listeners not ready')
                proof = []
                for mode in ('ed', 'ei'):
                    if mode == 'ei':
                        # Remove owned ED objects via product before the empty-plugin transition.
                        current_revision = runner.commit({'nat': None}, current_revision)
                        runner.fixture_mode('ei')
                        current_revision = runner.commit({'nat': dict(chain['nat'], mode='ei')}, current_revision)
                    commands = {'bridge': ('show', 'bridge-domain', str(args.slot * 1000 + 10), 'detail'),
                        'fib': ('show', 'ip', 'fib', 'table', str(args.slot * 1000 + 10), f'10.{args.slot}.99.0/24'),
                        'neighbors': ('show', 'ip', 'neighbors'), 'abf': ('show', 'abf', 'attach', f'loop{args.slot}10'),
                        'acl': ('show', 'acl-plugin', 'acl')}
                    snapshots = {name: runner.vpp(*command) for name, command in commands.items()}
                    errors_before = runner.vpp('show', 'errors')
                    counter_names = [f'host-{runner.prefix}l0.100', f'host-{runner.prefix}w0.201', f'host-{runner.prefix}w0.202']
                    before = {name: api.call('GET', '/state/interfaces/' + name + '/counters') for name in counter_names}
                    packet_result = runner.traffic(mode)
                    snapshots['nat'] = runner.vpp('show', 'nat44', *(['ei'] if mode == 'ei' else []), 'sessions')
                    errors_after = runner.vpp('show', 'errors')
                    after = {name: api.call('GET', '/state/interfaces/' + name + '/counters') for name in counter_names}
                    (root / (mode + '-counters.json')).write_text(json.dumps({'before': before, 'after': after}, indent=2))
                    packet_result['readback'] = readback_proof(args.slot, snapshots, errors_before, errors_after, before, after)
                    packet_result['revision'] = current_revision
                    packet_result['candidate_sha256'] = hashlib.sha256(json.dumps(api.call('GET', '/config'), sort_keys=True).encode()).hexdigest()
                    proof.append(packet_result)
                    runner.vpp('show', 'interface')
                    if runner.command(['systemctl', 'show', 'vpp', '-p', 'NRestarts', '-p', 'MainPID']) != restart_before:
                        raise Refused('VPP restarted during composed traffic')
                # Restore the interface-only baseline, then the pristine original slot revision.
                runner.claim(current_revision)
                rollback = api.call('POST', f'/config/rollback/{rig_revision}?comment=traffic-a-baseline')
                if rollback.get('status') != 'applied':
                    raise Refused('chain rollback failed')
                current_revision = rollback['revision']['id']
                runner.fixture_mode('off')
                retrieved = api.call('GET', '/config')
                def nonempty(value):
                    if isinstance(value, dict):
                        return any(nonempty(item) for item in value.values())
                    if isinstance(value, list):
                        return bool(value)
                    return value not in (None, False, '', 0)
                if any(nonempty(retrieved.get(key)) for key in ('objects', 'acl')) or retrieved.get('routing', {}).get('static'):
                    raise Refused('chain object persisted after rollback')
                current_revision = runner.restore_original(baseline_revision, current_revision)
                api.call('POST', '/config/discard')
                result = {'task': 'TEST-traffic-A', 'status': 'PACKETS_READBACK_VALIDATED_CLEANUP_PENDING',
                    'whole_chain_proven': False, 'packet_outcomes_proven': True, 'run_id': run_id,
                    'slot': args.slot, 'proof': proof,
                    'note': 'leased in-tree packet and CLI/counter assertions passed; manager independently reviews retained evidence'}
                (root / 'result.json').write_text(json.dumps(result, indent=2))
            finally:
                runner.cleanup(rig_owned)
            tables = runner.vpp('show', 'ip', 'fib')
            if re.search(r'ipv4-VRF:\s*' + str(args.slot * 1000 + 10) + r'\b', tables):
                raise Refused('slot FIB table remains after original rollback')
            for mode in ('ed', 'ei'):
                addresses = runner.vpp('show', 'nat44', *(['ei'] if mode == 'ei' else []), 'addresses')
                if f'10.{args.slot}.21.100' in addresses:
                    raise Refused('slot NAT pool remains after original rollback')
            if runner.command(['systemctl', 'show', 'vpp', '-p', 'NRestarts', '-p', 'MainPID']) != restart_before:
                raise Refused('VPP identity changed during cleanup')
            result['whole_chain_proven'] = True
            result['status'] = 'COMPOSED_PACKET_AND_READBACK_ACCEPTANCE_PASSED'
            result['cleanup'] = 'slot interfaces/namespaces/table/pool absent; original revision restored'
            (root / 'result.json').write_text(json.dumps(result, indent=2))
            print(json.dumps(result, indent=2))
        return 0
    except (Refused, OSError, ValueError, KeyError, TypeError, subprocess.SubprocessError, RuntimeError) as error:
        print(f'composed traffic refused: {error}', file=sys.stderr)
        return 1


if __name__ == '__main__':
    raise SystemExit(main())
