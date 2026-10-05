#!/usr/bin/env python3
"""Manager-leased Wave-C product/agent/packets transaction; no fixture PASS."""
import argparse
from datetime import datetime, timezone
import fcntl
import importlib.util
import json
import math
import os
from pathlib import Path
import re
import secrets
import signal
import stat
import subprocess
import sys
import time
import urllib.error
import urllib.request

sys.dont_write_bytecode = True
HERE = Path(__file__).resolve().parent
A = HERE.parent / 'traffic-a'
sys.path.insert(0, str(A))
spec = importlib.util.spec_from_file_location('wave_a_executor', A / 'execute.py')
wave_a = importlib.util.module_from_spec(spec)
spec.loader.exec_module(wave_a)
sys.path.insert(0, str(HERE))
from driver import applied, counter_deltas, mpls_packets, ping_outage, plan, require, srv6_packets, private_opener


class Api(wave_a.Api):
    def __init__(self, slot, token):
        super().__init__(slot, token)
        require('\n' not in token and '\r' not in token, 'invalid private API key')
        self.opener = private_opener()

    def response(self, request, timeout=30):
        require(request.full_url.startswith(self.base + '/'), 'foreign API request URL')
        return self.opener.open(request, timeout=timeout)

    def call(self, method, path, body=None, want=200):
        require(path.startswith('/') and not path.startswith('//'), 'invalid API path')
        data = None if body is None else json.dumps(body).encode()
        request = urllib.request.Request(self.base + path, data=data, method=method,
            headers={'Authorization': 'ApiKey ' + self.token,
                     'Content-Type': 'application/merge-patch+json' if method == 'PATCH' else 'application/json'})
        try:
            response = self.response(request)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            status = response.status
            raw = response.read(1048577)
        require(len(raw) <= 1048576 and status == want, f'API HTTP {status}, expected {want}')
        if want == 204:
            require(not raw, 'DELETE returned an unexpected body')
            return {}
        return json.loads(raw)


def interrupt(_signal, _frame):
    raise KeyboardInterrupt('manager interrupted Wave-C')


class Runner(wave_a.Runner):
    def command(self, argv, expected=0):
        started = datetime.now(timezone.utc).isoformat()
        try:
            return super().command(argv, expected)
        finally:
            record = {'argv': argv, 'expected': expected, 'started': started,
                      'ended': datetime.now(timezone.utc).isoformat(),
                      'evidence': f'command-{self.commands:03d}.txt'}
            with (self.root / 'commands.jsonl').open('a') as output:
                output.write(json.dumps(record) + '\n')

    @staticmethod
    def stop(process):
        # Preserve tcpdump accounting with SIGINT, then remove surviving owned
        # descendants even if their group leader exited during the grace period.
        try:
            os.killpg(process.pid, signal.SIGINT)
        except ProcessLookupError:
            pass
        try:
            process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            pass
        finally:
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            process.wait(timeout=5)

    def lease(self):
        path = Path(f'/run/ngfw-test/{self.prefix}/traffic-c-lease.json')
        fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
        try:
            info = os.fstat(fd)
            require(stat.S_ISREG(info.st_mode) and info.st_uid == 0 and stat.S_IMODE(info.st_mode) == 0o600
                    and info.st_nlink == 1 and 0 < info.st_size <= 4096, 'invalid manager window file')
            def unique(pairs):
                result = {}
                for key, value in pairs:
                    require(key not in result, 'duplicate lease field')
                    result[key] = value
                return result
            value = json.loads(os.read(fd, 4097), object_pairs_hook=unique)
        finally:
            os.close(fd)
        require(set(value) == {'task', 'slot', 'prefix', 'boot_id', 'expires_unix', 'lease_id', 'daemon_owner', 'globals_owner', 'quiet'}, 'invalid window fields')
        require(value['task'] == 'TEST-traffic-C' and type(value['slot']) is int and value['slot'] == self.slot
                and value['prefix'] == self.prefix and value['boot_id'] == self.boot
                and value['daemon_owner'] == 'keepalived' and value['globals_owner'] is True and value['quiet'] is True,
                'window does not grant this slot/daemon/globals quiet ownership')
        require(isinstance(value['lease_id'], str) and re.fullmatch('[0-9a-f]{32}', value['lease_id']), 'invalid lease identity')
        end = value['expires_unix']
        require(type(end) in (int, float) and math.isfinite(end) and time.time() < end <= time.time() + 7200, 'invalid/expired window')
        require(self.lease_id is None or self.lease_id == value['lease_id'], 'manager revoked/replaced window')
        self.lease_id = value['lease_id']

    def commit(self, patch, expected_revision):
        self.lease()
        owner = self.claim(expected_revision)
        self.candidate_owner = owner
        self.api.call('PATCH', '/config', patch)
        now = self.api.call('GET', '/config/lock')
        require(now.get('ownerKeyId') == owner['ownerKeyId'] and now.get('lockedAt') == owner['lockedAt'], 'candidate ownership changed')
        receipt = self.api.call('POST', '/config/commit?comment=traffic-c')
        (self.root / f'commit-{expected_revision}.json').write_text(json.dumps(receipt, indent=2))
        revision = applied(receipt)
        self.current = revision
        self.candidate_owner = None
        drift = self.api.call('GET', '/state/drift')
        require(not drift.get('changes') and not any(x.get('pointer', '').startswith(('/interfaces', '/vrfs', '/routing', '/ha', '/services/qos', '/services/ipfix')) for x in drift.get('ignored', [])), 'selected feature Retrieve is unsupported or drifted')
        return revision

    def rollback(self, revision):
        self.lease()
        lock = self.api.call('GET', '/config/lock')
        if lock.get('locked'):
            owner = getattr(self, 'candidate_owner', None)
            require(owner and lock.get('ownerKeyId') == owner['ownerKeyId'] and lock.get('lockedAt') == owner['lockedAt'], 'foreign candidate: preserve it during recovery')
            self.api.call('POST', '/config/discard')
            self.candidate_owner = None
        self.claim(self.current)
        receipt = self.api.call('POST', f'/config/rollback/{revision}?comment=traffic-c')
        self.current = applied(receipt)
        require(self.api.call('GET', '/config/diff') == {'baseRevision': self.current, 'changes': []}, 'rollback candidate residue')

    def ready(self, process, filename, marker):
        end = time.monotonic() + 5
        while time.monotonic() < end:
            if marker in (self.root / filename).read_text():
                return
            require(process.poll() is None, 'owned fixture exited before readiness')
            time.sleep(.05)
        require(False, 'owned fixture readiness deadline exceeded')

    def capture(self, step, side, bpf):
        letter = 'l' if side == 'lan' else 'w'
        name = step + '-' + side + '-packets'
        process = self.spawn(['ip', 'netns', 'exec', f'ns-{self.prefix}-{side}', 'tcpdump', '-l', '-nn', '-e', '-vvv', '-s', '65535', '-c', '2048', '-i', self.prefix + letter + '1', bpf], name)
        self.ready(process, name + '.txt', 'listening on')
        return process, self.root / (name + '.txt')

    def finish_capture(self, capture):
        process, path = capture
        self.stop(process)
        text = path.read_text()
        require(path.stat().st_size <= 16 * 1024 * 1024, 'oversized packet text')
        require(re.findall(r'^([0-9]+) packets dropped by kernel$', text, re.M) == ['0'], 'tcpdump lost packets or lacks accounting')
        require(len(re.findall(r'^([0-9]+) packets captured$', text, re.M)) == 1, 'missing packet accounting')
        return text

    def ping(self, target, count=5, expected=0):
        return self.ns('lan', 'ping', '-n', '-c', str(count), '-W', '1', '-i', '.2', target, expected=expected)

    def snapshot(self, operation):
        self.lease()
        binary = HERE / 'globals/traffic-c-globals'
        require(binary.is_file(), 'build finite globals helper before acquiring live window')
        fd = self.global_lock.fileno()
        environment = dict(os.environ, NGFW_TRAFFIC_C_GLOBALS='1', NGFW_GLOBAL_LOCK_FD=str(fd))
        output = subprocess.check_output([str(binary), '--operation', operation, '--snapshot', str(self.root / 'globals.json')], env=environment, pass_fds=(fd,), timeout=45)
        self.global_operations = getattr(self, 'global_operations', 0) + 1
        (self.root / (f'globals-{self.global_operations:02d}-' + operation + '.txt')).write_bytes(output)

    def stage(self, name, patch, evidence):
        started = datetime.now(timezone.utc).isoformat()
        before = self.command(['systemctl', 'show', 'vpp', '-p', 'NRestarts', '-p', 'MainPID'])
        try:
            self.commit(patch, self.current)
            proof = evidence()
            require(self.command(['systemctl', 'show', 'vpp', '-p', 'NRestarts', '-p', 'MainPID']) == before, 'VPP identity changed during stage')
            return {'stage': name, 'proof': proof, 'NRestarts': before, 'started': started, 'ended': datetime.now(timezone.utc).isoformat()}
        finally:
            self.rollback(self.rig_revision)
            require(self.api.call('GET', '/config') == self.rig_config, 'stage rollback changed rig baseline')
            self.snapshot('restore')

    def mpls(self):
        n = self.slot
        label, outgoing, bsid = n * 1000 + 60, n * 1000 + 61, n * 1000 + 100
        vrf, wan = self.prefix + '-tc', 'host-' + self.prefix + 'w0'
        patch = {'routing': {'static': [{'prefix': f'10.{n}.98.0/24', 'vrf': vrf, 'nextHops': [{'address': f'10.{n}.2.2', 'interface': wan}]}],
            'mpls': {'tables': {'0': {}}, 'interfaces': [wan],
             'labelRoutes': [{'table': 0, 'label': label, 'eos': True, 'paths': [{'nextHop': f'10.{n}.2.2', 'interface': wan, 'outLabels': [outgoing]}]}],
             'sr': {'policies': {str(bsid): {'segmentLists': [{'labels': [label], 'weight': 1}]}},
                    'steering': [{'vrf': vrf, 'prefix': f'10.{n}.98.0/24', 'bsid': bsid}]}}}}
        def evidence():
            capture = self.capture('mpls', 'wan', 'ether proto 0x8847')
            try:
                self.ping(f'10.{n}.98.2', expected='failure')
            finally:
                text = self.finish_capture(capture)
            self.vpp('show', 'mpls', 'fib', 'table', '0')
            self.vpp('show', 'ip', 'fib', 'table', str(n * 1000 + 60), f'10.{n}.98.0/24')
            return {'labelledRequests': mpls_packets(text, n, outgoing), 'expectedReply': False}
        return self.stage('mpls', patch, evidence)

    def srv6(self):
        n, vrf = self.slot, self.prefix + '-tc'
        sid, bsid = f'fd00:{n:x}:ee::1', f'fd00:{n:x}:bb::1'
        patch = {'routing': {'static': [{'prefix': sid + '/128', 'vrf': vrf, 'nextHops': [{'address': f'fd00:{n:x}:2::2', 'interface': 'host-' + self.prefix + 'w0'}]}],
          'srv6': {'encapSource': f'fd00:{n:x}::1', 'policies': {bsid: {'vrf': vrf, 'encap': True, 'encapSource': f'fd00:{n:x}::1', 'sidLists': [{'sids': [sid], 'weight': 1}]}},
                   'steering': [{'type': 'l3', 'prefix': f'10.{n}.160.0/24', 'vrf': vrf, 'bsid': bsid}]}}}
        def evidence():
            capture = self.capture('srv6', 'wan', 'ip6')
            try:
                self.ping(f'10.{n}.160.2', expected='failure')
            finally:
                text = self.finish_capture(capture)
            self.vpp('show', 'sr', 'policies')
            self.vpp('show', 'sr', 'steering-policies')
            return {'encapsulatedRequests': srv6_packets(text, n, sid)}
        return self.stage('srv6', patch, evidence)

    def vrrp(self):
        n, prefix = self.slot, self.prefix
        lan, bridge, vip, vrid = prefix + 'l1', prefix + 'br', f'10.{n}.1.254', n
        config = f'''global_defs {{\n router_id {prefix}-C\n}}\nvrrp_instance {prefix}-C {{\n state BACKUP\n interface {prefix}kb1\n virtual_router_id {vrid}\n priority 100\n advert_int 0.2\n version 3\n use_vmac\n virtual_ipaddress {{\n  {vip}/24\n }}\n}}\n'''
        path = self.root / 'keepalived.conf'
        path.write_text(config)
        # Bridge exists exclusively within this newly created LAN namespace.
        self.ns('lan', 'ip', 'link', 'add', bridge, 'type', 'bridge')
        self.ns('lan', 'ip', 'address', 'del', f'10.{n}.1.2/24', 'dev', lan)
        self.ns('lan', 'ip', 'link', 'set', lan, 'master', bridge)
        self.ns('lan', 'ip', 'link', 'set', bridge, 'up')
        self.ns('lan', 'ip', 'address', 'add', f'10.{n}.1.2/24', 'dev', bridge)
        self.command(['ip', 'netns', 'add', f'ns-{prefix}-kb'])
        self.kb_owned = True
        self.ns('lan', 'ip', 'link', 'add', prefix + 'kb0', 'type', 'veth', 'peer', 'name', prefix + 'kb1')
        self.ns('lan', 'ip', 'link', 'set', prefix + 'kb1', 'netns', f'ns-{prefix}-kb')
        self.ns('lan', 'ip', 'link', 'set', prefix + 'kb0', 'master', bridge)
        self.ns('lan', 'ip', 'link', 'set', prefix + 'kb0', 'up')
        self.ns('kb', 'ip', 'link', 'set', 'lo', 'up')
        self.ns('kb', 'ip', 'link', 'set', prefix + 'kb1', 'up')
        self.ns('kb', 'ip', 'address', 'add', f'10.{n}.1.3/24', 'dev', prefix + 'kb1')
        peer = self.spawn(['ip', 'netns', 'exec', f'ns-{prefix}-kb', 'keepalived', '--dont-fork', '--log-console', '--no-syslog', '--vrrp', '--no_bfd', '--dont-respawn', '--use-file', str(path), '--pid', str(self.root / 'keepalived.pid'), '--vrrp_pid', str(self.root / 'keepalived-vrrp.pid')], 'keepalived')
        name = prefix + '-vr'
        patch = {'ha': {'vrrp': {name: {'enabled': True, 'interface': 'host-' + prefix + 'l0', 'vrId': vrid, 'priority': 200, 'advertisementIntervalMs': 200, 'acceptMode': True, 'addresses': [vip], 'vrf': prefix + '-tc', 'engine': 'vpp'}}}}
        def evidence():
            self.ping(vip)
            capture = self.capture('vrrp', 'lan', 'arp or icmp or ip proto 112')
            start = time.time()
            pinger = self.spawn(['ip', 'netns', 'exec', f'ns-{prefix}-lan', 'ping', '-D', '-n', '-i', '.2', vip], 'vrrp-ping')
            try:
                time.sleep(1)
                self.vpp('show', 'vrrp', 'vr')
                self.commit({'ha': {'vrrp': {name: {'enabled': False}}}}, self.current)
                time.sleep(4)
                require('MASTER' in (self.root / 'keepalived.txt').read_text(), 'keepalived did not become MASTER')
                self.ns('lan', 'arping', '-c', '2', '-I', bridge, vip)
                self.commit({'ha': {'vrrp': {name: {'enabled': True}}}}, self.current)
                time.sleep(4)
                self.vpp('show', 'vrrp', 'vr')
                self.ns('lan', 'arping', '-c', '2', '-I', bridge, vip)
            finally:
                self.stop(pinger)
                end = time.time()
                text = self.finish_capture(capture)
            log = (self.root / 'keepalived.txt').read_text()
            require('MASTER' in log and 'BACKUP' in log and log.rfind('BACKUP') > log.find('MASTER'), 'peer did not transition MASTER then BACKUP')
            require(f'00:00:5e:00:01:{vrid:02x}' in text.lower(), 'VIP virtual MAC missing from packet evidence')
            return {'longestOutage': ping_outage((self.root / 'vrrp-ping.txt').read_text(), start, end), 'simulation': False, 'peer': 'keepalived', 'VPPs': 1}
        try:
            return self.stage('vrrp', patch, evidence)
        finally:
            self.stop(peer)
            self.command(['ip', 'netns', 'delete', f'ns-{prefix}-kb'])
            self.kb_owned = False
            self.ns('lan', 'ip', 'link', 'set', lan, 'nomaster')
            self.ns('lan', 'ip', 'link', 'delete', bridge)
            self.ns('lan', 'ip', 'address', 'add', f'10.{n}.1.2/24', 'dev', lan)
            self.ns('lan', 'ip', 'route', 'replace', 'default', 'via', f'10.{n}.1.1')

    def qos(self):
        name, lan = self.prefix + '-gold', 'host-' + self.prefix + 'l0'
        patch = {'services': {'qos': {'policers': {name: {'type': '1r3c-rfc2697', 'rateUnit': 'pps', 'cir': 10, 'cb': '1', 'eb': '2', 'conformAction': {'action': 'transmit'}, 'exceedAction': {'action': 'drop'}, 'violateAction': {'action': 'drop'}}}, 'interfaces': {lan: {'policer': {'input': name}}}}}}
        def counters():
            state = self.api.call('GET', '/state/services/qos/policers')
            require(state.get('countersError') is None, 'policer counters unavailable')
            matches = [x for x in state['items'] if x['name'] == name and x['present']]
            require(len(matches) == 1, 'owned policer missing or ambiguous')
            return {key: int(matches[0][key]['packets']) for key in ('conform', 'exceed', 'violate')}
        def evidence():
            receiver = self.spawn(['ip', 'netns', 'exec', f'ns-{self.prefix}-wan', 'python3', str(HERE / 'peer.py'), 'receive', '--slot', str(self.slot)], 'qos-receiver')
            self.ready(receiver, 'qos-receiver.txt', 'READY:UDP')
            before = counters()
            capture = self.capture('qos', 'wan', 'udp dst port ' + str(self.peer_port(70)))
            try:
                sender = json.loads(self.ns('lan', 'python3', str(HERE / 'peer.py'), 'send', '--slot', str(self.slot)))
            finally:
                self.stop(receiver)
                text = self.finish_capture(capture)
            after = counters()
            received = len(re.findall(r' > 10\.' + str(self.slot) + r'\.2\.2\.' + str(self.peer_port(70)) + ': UDP', text))
            actual = json.loads((self.root / 'qos-receiver.txt').read_text().splitlines()[-1])
            require(actual['received'] == received, 'WAN packet count disagrees with real UDP receiver')
            self.vpp('show', 'policer')
            return {'sent': sender['sent'], 'WAN': received, 'deltas': counter_deltas(before, after, sender['sent'], received)}
        return self.stage('qos', patch, evidence)

    def metrics(self, name):
        port = 9100 + self.slot * 10 + 1
        with urllib.request.urlopen(f'http://127.0.0.1:{port}/metrics', timeout=10) as response:
            data = response.read(1024 * 1024 + 1)
        require(len(data) <= 1024 * 1024, 'oversized metrics response')
        text = data.decode()
        (self.root / (name + '-metrics.txt')).write_text(text)
        result = {}
        for line in text.splitlines():
            if not line.startswith('ngfw_interface_') or 'packets_total{' not in line:
                continue
            if re.search(r'interface="(?:' + re.escape(self.prefix) + r')?host-' + re.escape(self.prefix) + r'[lw]0"', line):
                key, value = line.rsplit(' ', 1)
                require(key not in result, 'duplicate interface metric')
                result[key] = float(value)
        require(len(result) == 4 and all(math.isfinite(x) and x >= 0 for x in result.values()), 'owned interface packet metrics missing')
        return result

    def riders(self):
        n, vrf = self.slot, self.prefix + '-tc'
        lan, wan = 'host-' + self.prefix + 'l0', 'host-' + self.prefix + 'w0'
        patch = {'routing': {'multicast': {'igmp': {'interfaces': {lan: {'mode': 'router'}}}}},
            'services': {'ipfix': {'exporters': {self.prefix + '-ipfix': {'enabled': True, 'collector': {'address': f'10.{n}.2.2', 'port': self.peer_port(72)}, 'sourceAddress': f'10.{n}.2.1', 'vrf': vrf, 'templateIntervalSec': 1}},
                'flowprobe': {'activeTimerSec': 1, 'passiveTimerSec': 2, 'interfaces': [{'interface': lan, 'direction': 'rx', 'ip4': True}]}}}}
        def evidence():
            require(not any(x['state'] == 'running' for x in self.api.call('GET', '/state/captures')['captures']), 'another product capture is running')
            collector = self.spawn(['ip', 'netns', 'exec', f'ns-{self.prefix}-wan', 'python3', str(HERE / 'peer.py'), 'collector', '--slot', str(n)], 'ipfix-collector')
            join = self.spawn(['ip', 'netns', 'exec', f'ns-{self.prefix}-lan', 'python3', str(HERE / 'peer.py'), 'join', '--slot', str(n)], 'igmp-peer')
            self.ready(collector, 'ipfix-collector.txt', 'READY:IPFIX')
            self.ready(join, 'igmp-peer.txt', 'READY:IGMP_INCLUDE')
            capture = self.capture('riders', 'wan', 'icmp or udp')
            before = self.metrics('before')
            capture_id = None
            try:
                started = self.api.call('POST', '/actions/capture', {'interface': lan, 'direction': 'rx', 'maxPackets': 100, 'seconds': 15, 'snaplen': 9000}, want=202)
                capture_id = started.get('id')
                require(isinstance(capture_id, str) and re.fullmatch(r'[A-Za-z0-9_-][A-Za-z0-9_.-]{0,127}', capture_id), 'invalid owned capture id')
                self.ping(f'10.{n}.2.2', count=10)
                end = time.monotonic() + 25
                group = f'232.{n}.1.1'
                while time.monotonic() < end:
                    groups = self.vpp('show', 'igmp', 'groups')
                    mfib = self.vpp('show', 'ip', 'mfib', 'table', str(n * 1000 + 60))
                    if group in groups and group in mfib:
                        break
                    time.sleep(.2)
                require(group in groups and group in mfib, 'owned live IGMP group and mFIB entry absent')
                self.vpp('show', 'ipfix', 'exporter')
                end = time.monotonic() + 20
                while time.monotonic() < end:
                    found = [x for x in self.api.call('GET', '/state/captures')['captures'] if x['id'] == capture_id]
                    if len(found) == 1 and found[0]['state'] == 'done':
                        break
                    time.sleep(.1)
                require(len(found) == 1 and found[0]['state'] == 'done' and int(found[0]['packets']) > 0, 'product capture not complete/nonempty')
                request = urllib.request.Request(self.api.base + f'/state/captures/{capture_id}/file', headers={'Authorization': 'ApiKey ' + self.api.token})
                with self.api.response(request, timeout=30) as response:
                    require(response.status == 200, 'product capture download failed')
                    pcap = response.read(16 * 1024 * 1024 + 1)
                require(len(pcap) <= 16 * 1024 * 1024, 'product capture exceeded bound')
                path = self.root / 'product-capture.pcap'
                fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
                with os.fdopen(fd, 'wb') as output:
                    output.write(pcap)
                decoded = self.command(['tcpdump', '-nn', '-r', str(path)])
                require(f'10.{n}.1.2 > 10.{n}.2.2: ICMP echo request' in decoded, 'product capture lacks owned real traffic')
                after = self.metrics('after')
                require(set(before) == set(after) and all(after[key] >= before[key] for key in before)
                        and any(after[key] > before[key] for key in before), 'owned interface metrics did not increase')
                time.sleep(3)
            finally:
                self.stop(join)
                self.stop(collector)
                text = self.finish_capture(capture)
                if capture_id:
                    entries = [x for x in self.api.call('GET', '/state/captures')['captures'] if x['id'] == capture_id]
                    if entries and entries[0]['state'] == 'running':
                        self.api.call('POST', f'/actions/capture/{capture_id}/stop', want=202)
                        deadline = time.monotonic() + 10
                        while time.monotonic() < deadline:
                            entries = [x for x in self.api.call('GET', '/state/captures')['captures'] if x['id'] == capture_id]
                            if not entries or entries[0]['state'] != 'running':
                                break
                            time.sleep(.1)
                        require(not entries or entries[0]['state'] != 'running', 'owned product capture did not stop')
                    if entries:
                        self.api.call('DELETE', f'/state/captures/{capture_id}', want=204)
            stats = json.loads((self.root / 'ipfix-collector.txt').read_text().splitlines()[-1])
            require(stats['messages'] > 0 and stats['templates'] > 0 and stats['dataSets'] > 0 and stats['dataBytes'] > 0 and stats['malformed'] == 0, 'collector lacks real IPFIX template/data records')
            require(f'10.{n}.1.2 > 10.{n}.2.2: ICMP echo request' in text, 'riders WAN forwarding packet missing')
            return {'IPFIX': stats, 'productCapture': capture_id, 'IGMP': group, 'metricsIncreased': True}
        return self.stage('riders', patch, evidence)

    def peer_port(self, offset):
        return (3000 + self.slot * 100 if self.slot < 12 else 10000 + self.slot * 100) + offset

    def close(self):
        errors = []
        for process, log in reversed(self.children):
            try:
                self.stop(process)
            except (OSError, subprocess.SubprocessError) as error:
                errors.append(str(error))
            finally:
                log.close()
        if getattr(self, 'kb_owned', False):
            try:
                self.command(['ip', 'netns', 'delete', f'ns-{self.prefix}-kb'])
            except Exception as error:
                errors.append(str(error))
        if getattr(self, 'rig_owned', False):
            try:
                self.peers_down()
                self.rollback(self.original_revision)
                require(self.api.call('GET', '/config') == self.original_config, 'original product configuration not restored')
                self.command([str(self.repo / 'tools/lab'), 'rig', 'down', self.prefix])
            except Exception as error:
                errors.append(str(error))
        if getattr(self, 'globals_saved', False):
            try:
                self.snapshot('restore')
            except Exception as error:
                errors.append(str(error))
        if not errors and getattr(self, 'rig_owned', False):
            try:
                self.vpp('show', 'interface')
                for argv in [('show', 'vrrp', 'vr'), ('show', 'sr', 'policies'), ('show', 'policer')]:
                    require(self.prefix not in self.vpp(*argv), 'owned VPP feature residue remains')
                require(self.prefix not in self.vpp('show', 'interface'), 'owned VPP interface residue remains')
                namespaces = json.loads(self.command(['ip', '-j', 'netns', 'list']))
                require(not any(x['name'].startswith('ns-' + self.prefix + '-') for x in namespaces), 'owned namespace residue')
            except Exception as error:
                errors.append(str(error))
        require(not errors, 'cleanup failed; no PASS; retained private evidence: ' + '; '.join(errors))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--slot', type=int, required=True)
    parser.add_argument('--dry-run', action='store_true')
    args = parser.parse_args()
    if args.dry_run:
        output = plan(args.slot)
        output['executor'] = 'execute.py'
        output['remaining'] = ['real manager-leased host acceptance and independent review']
        print(json.dumps(output, indent=2))
        return
    wave_a.validate_environment(os.environ, args.slot)
    require(os.environ.get('NGFW_INTEGRATION') == '1' and os.environ.get('NGFW_TRAFFIC_C_HOST') == '1', 'live acceptance requires explicit integration/traffic-C opt-in')
    require(os.geteuid() == 0, 'real host namespace execution requires root')
    parent = Path(f'/run/ngfw-test/w{args.slot}/traffic-c')
    info = parent.lstat()
    require(stat.S_ISDIR(info.st_mode) and info.st_uid == 0 and stat.S_IMODE(info.st_mode) == 0o700, 'manager must provision owned0700 evidence parent')
    root = parent / secrets.token_hex(16)
    root.mkdir(mode=0o700)
    runner = Runner(args.slot, root, Api(args.slot, os.environ.get('NGFW_HOST_ACCESS_TOKEN', '')))
    runner.lease()
    with open('/run/lock/ngfw-lab.lock', 'a') as lab, open('/run/lock/ngfw-globals.lock', 'a') as globals_lock, open(f'/run/lock/ngfw-traffic-c-w{args.slot}.lock', 'a') as slot_lock:
        fcntl.flock(lab, fcntl.LOCK_SH | fcntl.LOCK_NB)
        fcntl.flock(slot_lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        fcntl.flock(globals_lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        window_started = datetime.now(timezone.utc).isoformat()
        runner.global_lock = globals_lock
        runner.original_config = runner.api.call('GET', '/config')
        require(not runner.original_config.get('interfaces') and not runner.original_config.get('vrfs'), 'manager slot datastore is not pristine')
        runner.original_revision = runner.api.call('GET', '/config/diff')['baseRevision']
        require(type(runner.original_revision) is int and runner.original_revision > 0, 'manager must bootstrap committed empty slot baseline')
        runner.current = runner.original_revision
        require(not runner.api.call('GET', '/config/lock').get('locked'), 'another API candidate writer exists')
        namespaces = json.loads(runner.command(['ip', '-j', 'netns', 'list']))
        require(not any(x['name'].startswith('ns-' + runner.prefix + '-') for x in namespaces), 'refuse existing slot namespaces')
        require(runner.prefix not in runner.vpp('show', 'interface'), 'refuse existing slot VPP interface')
        before = runner.command(['systemctl', 'show', 'vpp', '-p', 'NRestarts', '-p', 'MainPID'])
        proofs = []
        try:
            runner.snapshot('snapshot')
            saved = json.loads((root / 'globals.json').read_text())
            require(not saved['tableZero'], 'preexisting MPLS table 0 cannot be adopted by this global-owner slot agent; manager must provide an otherwise idle VPP with no table0 for product-only creation/rollback')
            runner.globals_saved = True
            runner.rig_owned = True  # Cleanup partial rig-up failures, never adopt an existing rig.
            runner.command([str(runner.repo / 'tools/lab'), 'rig', 'up', runner.prefix])
            n, p = args.slot, runner.prefix
            for side, letter in (('lan', 'l'), ('wan', 'w')):
                runner.ns(side, 'ip', '-6', 'address', 'add', f'fd00:{n:x}:{1 if side == "lan" else 2}::2/64', 'dev', p + letter + '1')
            runner.ns('lan', 'ip', 'route', 'replace', 'default', 'via', f'10.{n}.1.1')
            runner.ns('wan', 'ip', 'route', 'replace', f'10.{n}.1.0/24', 'via', f'10.{n}.2.1')
            runner.rig_revision = runner.commit({'vrfs': {p + '-tc': {'id': n * 1000 + 60}}, 'interfaces': {
                'host-' + p + 'l0': {'enabled': True, 'vrf': p + '-tc', 'ipv4': [f'10.{n}.1.1/24'], 'ipv6': [f'fd00:{n:x}:1::1/64']},
                'host-' + p + 'w0': {'enabled': True, 'vrf': p + '-tc', 'ipv4': [f'10.{n}.2.1/24'], 'ipv6': [f'fd00:{n:x}:2::1/64']}}}, runner.current)
            runner.rig_config = runner.api.call('GET', '/config')
            for step in (runner.mpls, runner.srv6, runner.vrrp, runner.qos, runner.riders):
                proofs.append(step())
            # This independent host test owns its own EX globals lock. The same
            # manager quiet lease remains in force while we release ours briefly.
            fcntl.flock(globals_lock, fcntl.LOCK_UN)
            try:
                previous = os.environ.get('NGFW_FSRV6_GLOBALS')
                os.environ['NGFW_FSRV6_GLOBALS'] = '1'
                try:
                    host_output = runner.command([str(runner.repo / 'tools/heavy.sh'), 'go', '-C', str(runner.repo / 'apps/agent'), 'test', '-count=1', '-timeout=75s', './internal/agent', '-run', '^TestSrv6GlobalsOnHost$', '-v'])
                    require('--- PASS: TestSrv6GlobalsOnHost' in host_output and '--- SKIP' not in host_output and 'restored: encap source' in host_output, 'TD-H18 actual host proof skipped or lacked restore evidence')
                finally:
                    if previous is None:
                        os.environ.pop('NGFW_FSRV6_GLOBALS', None)
                    else:
                        os.environ['NGFW_FSRV6_GLOBALS'] = previous
            finally:
                fcntl.flock(globals_lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        finally:
            runner.close()
        require(runner.command(['systemctl', 'show', 'vpp', '-p', 'NRestarts', '-p', 'MainPID']) == before, 'VPP restarted during scenario')
        result = {'task': 'TEST-traffic-C', 'status': 'LIVE_PACKETS_READBACK_CLEANUP_PASSED', 'slot': args.slot, 'lease_id': runner.lease_id,
                  'windowStarted': window_started, 'windowEnded': datetime.now(timezone.utc).isoformat(), 'TD-H18': 'actual-host-test-passed',
                  'VPPs': 1, 'peer': 'keepalived', 'proofs': proofs, 'not_exercised': plan(args.slot)['not_exercised']}
        (root / 'result.json').write_text(json.dumps(result, indent=2))
        print(json.dumps(result, indent=2))


if __name__ == '__main__':
    signal.signal(signal.SIGTERM, interrupt)
    try:
        main()
    except KeyboardInterrupt:
        raise SystemExit('Wave-C interrupted; cleanup attempted, inspect retained private evidence') from None
    except (OSError, RuntimeError, ValueError, subprocess.SubprocessError) as error:
        raise SystemExit('Wave-C refused/failed: ' + str(error)) from None
