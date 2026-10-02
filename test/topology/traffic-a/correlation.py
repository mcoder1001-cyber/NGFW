"""Pure typed packet expectation matching; never attest live provenance/whole-chain PASS."""
from dataclasses import dataclass
import ipaddress
import re

from evidence import capture
from scenario import Refused, STAGES, slot_values


@dataclass(frozen=True)
class Flow:
    source: str
    destination: str
    protocol: int
    source_port: int | None = None
    destination_port: int | None = None
    icmp_identifier: int | None = None
    icmp_type: int | None = None

    def validate(self, slot):
        network = ipaddress.IPv4Network(f'10.{slot}.0.0/16')
        for value in (self.source, self.destination):
            try:
                address = ipaddress.IPv4Address(value)
            except (ValueError, TypeError) as error:
                raise Refused('invalid flow IPv4 identity') from error
            if address not in network or address in (network.network_address, network.broadcast_address):
                raise Refused('flow leaves the allocated rig address range')
        if type(self.protocol) is not int or self.protocol not in (1, 6):
            raise Refused('only typed IPv4 ICMP/TCP expectations are supported')
        if self.protocol == 6:
            if (any(type(port) is not int or not 1 <= port <= 65535 for port in (self.source_port, self.destination_port))
                    or self.icmp_identifier is not None or self.icmp_type is not None):
                raise Refused('malformed TCP flow contract')
        elif (self.source_port is not None or self.destination_port is not None
              or type(self.icmp_identifier) is not int or not 0 <= self.icmp_identifier <= 65535
              or type(self.icmp_type) is not int or self.icmp_type not in (0, 8)):
            raise Refused('only explicit echo request/reply ICMP contracts supported')

    def matches(self, packet):
        return (packet.source == self.source and packet.destination == self.destination and packet.protocol == self.protocol
                and packet.source_port == self.source_port and packet.destination_port == self.destination_port
                and packet.icmp_identifier == self.icmp_identifier and packet.icmp_type == self.icmp_type)


@dataclass(frozen=True)
class Probe:
    input_flow: Flow
    output_flow: Flow | None
    ip_id: int
    sequence: int
    payload_sha256: str
    input_ttl: int = 64
    tcp_ack: int = 0
    tcp_flags: int = 2

    def identity(self):
        return (self.input_flow.protocol, self.ip_id, self.sequence)


@dataclass(frozen=True)
class ExpectedCase:
    stage: str
    outcome: str
    mode: str
    input_side: str
    probes: tuple[Probe, ...]
    probe_started: float
    probe_ended: float
    input_vlans: tuple[int, ...] = ()
    output_vlans: tuple[int, ...] = ()
    ttl_decrement: int = 1
    egress_macs: tuple[str, ...] = ()
    max_latency_seconds: float = 1.0
    settle_seconds: float = 1.0


def identity(packet):
    return (packet.protocol, packet.ip_id, packet.tcp_sequence if packet.protocol == 6 else packet.icmp_sequence)


def correlate(case, files, metadata, slot, run_id, fixture=False):
    """Require both captured streams to satisfy every explicitly typed probe."""
    import math
    slot_values(slot)
    stage = next((item for item in STAGES if item.name == case.stage), None)
    outcome_modes = {
        ('vlan', 'tagged-ingress'): 'forward', ('vlan', 'forward'): 'forward', ('vlan', 'drop-wrong-tag'): 'drop',
        ('bridge-bvi', 'bridge-forward'): 'forward', ('bridge-bvi', 'bvi-route'): 'forward',
        ('vrf-ecmp', 'vrf-isolation'): 'drop', ('vrf-ecmp', 'both-ecmp-paths'): 'ecmp',
        ('urpf-pbr', 'urpf-valid-forward'): 'forward', ('urpf-pbr', 'urpf-spoof-drop'): 'drop',
        ('urpf-pbr', 'pbr-selected-path'): 'forward', ('acl', 'permit'): 'forward', ('acl', 'deny'): 'drop',
        ('nat44-ed', 'translated-source'): 'translate', ('nat44-ed', 'tcp-response'): 'translate',
        ('nat44-ei', 'translated-source'): 'translate', ('nat44-ei', 'tcp-response'): 'translate',
        ('nat44-ei', 'endpoint-independent-mapping'): 'endpoint-independent',
    }
    if (stage is None or case.outcome not in stage.required_outcomes
            or outcome_modes.get((case.stage, case.outcome)) != case.mode
            or case.mode not in ('forward', 'drop', 'translate', 'ecmp', 'endpoint-independent')
            or case.input_side not in ('lan', 'wan') or type(case.ttl_decrement) is not int or case.ttl_decrement not in (0, 1)):
        raise Refused('unsupported stage/outcome/mode/direction contract')
    if (not isinstance(case.probes, tuple) or not 1 <= len(case.probes) <= 64
            or any(type(value) not in (float, int) or not math.isfinite(value) for value in (case.probe_started, case.probe_ended))
            or not case.probe_started < case.probe_ended <= case.probe_started + 60):
        raise Refused('bounded explicit probe set/window required')
    if (any(type(value) not in (int, float) or not math.isfinite(value) for value in (case.max_latency_seconds, case.settle_seconds))
            or not 0 < case.max_latency_seconds <= case.settle_seconds <= 10):
        raise Refused('positive bounded latency and sufficient quiet settle window required')
    for tags in (case.input_vlans, case.output_vlans):
        if not isinstance(tags, tuple) or len(tags) > 2 or any(type(tag) is not int or not 1 <= tag <= 4094 for tag in tags):
            raise Refused('invalid explicit VLAN stack')
    if (not isinstance(case.egress_macs, tuple) or len(set(case.egress_macs)) != len(case.egress_macs)
            or any(not isinstance(mac, str) or not re.fullmatch(r'(?:[0-9a-f]{2}:){5}[0-9a-f]{2}', mac)
                   or int(mac[:2], 16) & 1 for mac in case.egress_macs)):
        raise Refused('invalid/duplicate explicit next-hop identities')
    if case.outcome == 'pbr-selected-path' and len(case.egress_macs) != 1:
        raise Refused('PBR requires one explicit selected next-hop identity')
    if case.outcome == 'tagged-ingress' and not case.input_vlans:
        raise Refused('tagged-ingress requires actual explicit VLAN tags')
    if case.mode == 'ecmp' and len(case.egress_macs) < 2:
        raise Refused('ECMP requires at least two explicit next-hop MACs')
    if set(files) != {'lan', 'wan'} or set(metadata) != {'lan', 'wan'}:
        raise Refused('both input/output capture streams required')
    left = files['lan'].lstat(); right = files['wan'].lstat()
    if (left.st_dev, left.st_ino) == (right.st_dev, right.st_ino):
        raise Refused('input and output must be distinct executor capture files')
    packets = {side: capture(files[side], metadata[side], slot, side, run_id, fixture=fixture, expected_stage=case.stage) for side in ('lan', 'wan')}
    for side in ('lan', 'wan'):
        if not metadata[side]['started'] <= case.probe_started < case.probe_ended + case.settle_seconds <= metadata[side]['ended']:
            raise Refused('both capture intervals must bracket the entire probe window')
    seen = set(); observed_macs = set()
    output_side = 'wan' if case.input_side == 'lan' else 'lan'
    for probe in case.probes:
        if not isinstance(probe, Probe):
            raise Refused('typed probe contracts required')
        if not isinstance(probe.input_flow, Flow):
            raise Refused('typed input flow required')
        probe.input_flow.validate(slot)
        if (type(probe.ip_id) is not int or not 1 <= probe.ip_id <= 65535
                or type(probe.sequence) is not int or not 0 <= probe.sequence <= (0xffffffff if probe.input_flow.protocol == 6 else 65535)
                or not isinstance(probe.payload_sha256, str) or not re.fullmatch(r'[0-9a-f]{64}', probe.payload_sha256)
                or type(probe.input_ttl) is not int or not 1 <= probe.input_ttl <= 255
                or probe.input_ttl <= case.ttl_decrement
                or type(probe.tcp_ack) is not int or not 0 <= probe.tcp_ack <= 0xffffffff
                or type(probe.tcp_flags) is not int or not 1 <= probe.tcp_flags <= 511):
            raise Refused('invalid probe packet identity/payload/TTL')
        key = probe.identity()
        if key in seen:
            raise Refused('duplicate ambiguous probe identity')
        seen.add(key)
        ingress = [packet for packet in packets[case.input_side] if identity(packet) == key]
        egress = [packet for packet in packets[output_side] if identity(packet) == key]
        if len(ingress) != 1:
            raise Refused('missing/duplicate input packet: a drop cannot be inferred from no input')
        source = ingress[0]
        if (not probe.input_flow.matches(source) or source.vlans != case.input_vlans or source.ttl != probe.input_ttl
                or source.payload_sha256 != probe.payload_sha256 or not case.probe_started <= source.timestamp <= case.probe_ended):
            raise Refused('input flow/VLAN/TTL/payload/timing differs from expectation')
        if source.protocol == 6 and (source.tcp_ack != probe.tcp_ack or source.tcp_flags != probe.tcp_flags):
            raise Refused('input TCP identity differs from expectation')
        if source.protocol == 1 and source.icmp_code != 0:
            raise Refused('unsupported ICMP code')
        if case.mode == 'drop':
            if probe.output_flow is not None or egress:
                raise Refused('drop violated: correlated packet crossed to output')
            continue
        if not isinstance(probe.output_flow, Flow):
            raise Refused('explicit output flow required')
        probe.output_flow.validate(slot)
        if probe.output_flow.protocol != probe.input_flow.protocol:
            raise Refused('cross-protocol transformation unsupported')
        if case.mode in ('forward', 'ecmp') and probe.input_flow != probe.output_flow:
            raise Refused('forwarding cannot silently translate a flow')
        if case.mode in ('translate', 'endpoint-independent') and probe.input_flow == probe.output_flow:
            raise Refused('translation must specify actual IP/port/identifier change')
        if len(egress) != 1:
            raise Refused('missing/duplicate matched output packet')
        target = egress[0]
        if (not probe.output_flow.matches(target) or target.vlans != case.output_vlans
                or target.ttl != probe.input_ttl - case.ttl_decrement or target.payload_sha256 != source.payload_sha256
                or not source.timestamp <= target.timestamp <= metadata[output_side]['ended']
                or target.timestamp - source.timestamp > case.max_latency_seconds):
            raise Refused('unexpected output flow/VLAN/TTL/payload/timing')
        if target.protocol == 6 and (target.tcp_ack != source.tcp_ack or target.tcp_flags != source.tcp_flags):
            raise Refused('unexpected TCP header mutation')
        if target.protocol == 1 and target.icmp_code != source.icmp_code:
            raise Refused('unexpected ICMP code mutation')
        if case.egress_macs and target.destination_mac not in case.egress_macs:
            raise Refused('output does not identify an expected next hop')
        observed_macs.add(target.destination_mac)
    if case.mode == 'ecmp' and observed_macs != set(case.egress_macs):
        raise Refused('both configured ECMP next-hop identities were not observed')
    if case.mode == 'endpoint-independent':
        inputs = [probe.input_flow for probe in case.probes]; outputs = [probe.output_flow for probe in case.probes]
        if (len(inputs) < 2 or len({(flow.destination, flow.destination_port) for flow in inputs}) < 2
                or len({(flow.source, flow.source_port, flow.protocol) for flow in inputs}) != 1
                or len({(flow.source, flow.source_port, flow.protocol) for flow in outputs}) != 1):
            raise Refused('endpoint-independent mapping requires one stable mapping across distinct destinations')
    return {'status': 'FIXTURE_CORRELATED' if fixture else 'PACKET_EXPECTATIONS_MATCHED',
            'stage': case.stage, 'outcome': case.outcome, 'matched_probes': len(case.probes),
            'whole_chain_proven': False, 'live_provenance_verified': False}
