"""Bounded IPv4 pcap/evidence checks; validation alone is never live traffic proof."""
from dataclasses import dataclass
import hashlib
import ipaddress
import math
import os
from pathlib import Path
import re
import stat
import struct

from scenario import Refused, STAGES, slot_values


class Indeterminate(Refused):
    """Captured bytes cannot establish packet integrity, including offload ambiguity."""
    status = 'INDETERMINATE'


def checksum(data):
    """Internet one's-complement checksum over bounded packet bytes."""
    if len(data) % 2:
        data += b'\x00'
    total = sum(struct.unpack('!' + 'H' * (len(data) // 2), data))
    while total >> 16:
        total = (total & 0xffff) + (total >> 16)
    return (~total) & 0xffff


@dataclass(frozen=True)
class Packet:
    timestamp: float
    source: str
    destination: str
    protocol: int
    source_port: int | None
    destination_port: int | None
    destination_mac: str
    vlans: tuple[int, ...]
    ip_id: int = 0
    ttl: int = 0
    tcp_sequence: int | None = None
    tcp_ack: int | None = None
    tcp_flags: int | None = None
    icmp_type: int | None = None
    icmp_code: int | None = None
    icmp_identifier: int | None = None
    icmp_sequence: int | None = None
    payload_sha256: str = ''


def read_pcap(data):
    """Read classic Ethernet pcap, rejecting truncation and unknown framing."""
    formats = {b'\xd4\xc3\xb2\xa1': ('<', 1000000), b'\xa1\xb2\xc3\xd4': ('>', 1000000),
               b'\x4d\x3c\xb2\xa1': ('<', 1000000000), b'\xa1\xb2\x3c\x4d': ('>', 1000000000)}
    if not isinstance(data, (bytes, bytearray)) or len(data) < 24 or len(data) > 16 * 1024 * 1024 or bytes(data[:4]) not in formats:
        raise Refused('unsupported/empty/oversize pcap')
    endian, scale = formats[bytes(data[:4])]
    major, minor, _, _, snap, link = struct.unpack(endian + 'HHIIII', data[4:24])
    if (major, minor) != (2, 4) or not 14 <= snap <= 65535 or link != 1:
        raise Refused('pcap must be bounded classic Ethernet2.4')
    packets = []; offset = 24; count = 0; previous = -1
    while offset < len(data):
        if len(data) - offset < 16:
            raise Refused('truncated packet header')
        seconds, fraction, length, original = struct.unpack(endian + 'IIII', data[offset:offset+16])
        offset += 16
        if fraction >= scale or length != original or length > snap or length < 14 or length > len(data)-offset:
            raise Refused('invalid/truncated packet record')
        timestamp = seconds + fraction / scale
        if timestamp < previous:
            raise Refused('capture timestamps are not monotonic')
        previous = timestamp
        frame = data[offset:offset+length]; offset += length; count += 1
        if count > 2048:
            raise Refused('capture record count exceeds the bounded2048 limit')
        ethertype = struct.unpack('!H', frame[12:14])[0]; cursor = 14; vlans = []
        while ethertype in (0x8100, 0x88a8):
            if len(vlans) == 2 or len(frame) < cursor + 4:
                raise Refused('unsupported/truncated VLAN stack')
            tag, ethertype = struct.unpack('!HH', frame[cursor:cursor+4]); cursor += 4
            vlans.append(tag & 0x0fff)
        if ethertype != 0x0800:
            raise Indeterminate('unsupported non-IPv4 capture record')
        ip = frame[cursor:]
        if len(ip) < 20 or ip[0] >> 4 != 4:
            raise Refused('malformed IPv4 packet')
        header_length = (ip[0] & 15) * 4
        total, fragment = struct.unpack('!HH', ip[2:4] + ip[6:8])
        if header_length < 20 or total < header_length or total > len(ip) or fragment & 0x3fff:
            raise Refused('truncated/fragmented IPv4 evidence unsupported')
        if checksum(bytes(ip[:header_length])) != 0:
            raise Indeterminate('IPv4 checksum invalid/unknown; offload cannot prove integrity')
        protocol = ip[9]; payload = ip[header_length:total]
        if protocol not in (1, 6):
            raise Indeterminate('unsupported IPv4 transport capture record')
        source_port = destination_port = None
        tcp_sequence = tcp_ack = tcp_flags = None
        icmp_type = icmp_code = icmp_identifier = icmp_sequence = None
        application = b''
        if protocol == 6:
            if len(payload) < 20 or payload[12] >> 4 < 5 or (payload[12] >> 4)*4 > len(payload):
                raise Refused('truncated TCP evidence')
            pseudo = bytes(ip[12:20]) + struct.pack('!BBH', 0, protocol, len(payload))
            if checksum(pseudo + bytes(payload)) != 0:
                raise Indeterminate('TCP checksum invalid/unknown; offload cannot prove integrity')
            source_port, destination_port = struct.unpack('!HH', payload[:4])
            tcp_sequence, tcp_ack = struct.unpack('!II', payload[4:12])
            tcp_flags = ((payload[12] & 1) << 8) | payload[13]
            application = payload[(payload[12] >> 4)*4:]
        elif len(payload) < 8:
            raise Refused('truncated ICMP evidence')
        else:
            if checksum(bytes(payload)) != 0:
                raise Indeterminate('ICMP checksum invalid/unknown')
            icmp_type, icmp_code = payload[:2]
            icmp_identifier, icmp_sequence = struct.unpack('!HH', payload[4:8])
            application = payload[8:]
        packets.append(Packet(timestamp, str(ipaddress.IPv4Address(ip[12:16])),
                              str(ipaddress.IPv4Address(ip[16:20])), protocol,
                              source_port, destination_port, ':'.join(f'{byte:02x}' for byte in frame[:6]), tuple(vlans),
                              struct.unpack('!H', ip[4:6])[0], ip[8], tcp_sequence, tcp_ack, tcp_flags,
                              icmp_type, icmp_code, icmp_identifier, icmp_sequence, hashlib.sha256(application).hexdigest()))
    return packets, count


def capture(path, metadata, slot, side, run_id, fixture=False, expected_stage=None, with_identity=False):
    """Validate one executor-owned capture, never a claimed pass from a log string."""
    values = slot_values(slot); prefix = values['NGFW_TEST_PREFIX']
    if side not in ('lan', 'wan') or not re.fullmatch(r'[0-9a-f]{32}', run_id):
        raise Refused('invalid capture side/run identity')
    fields = {'origin', 'run_id', 'namespace', 'device', 'argv', 'sha256', 'started', 'ended', 'received', 'dropped'}
    if expected_stage is not None:
        if expected_stage not in {stage.name for stage in STAGES} or metadata.get('stage') != expected_stage:
            raise Refused('capture does not identify the expected stage')
        fields.add('stage')
    if set(metadata) != fields:
        raise Refused('capture metadata fields differ from the explicit contract')
    expected_origin = 'source_fixture' if fixture else 'tcpdump_live'
    namespace = f'ns-{prefix}-{side}'; device = f'{prefix}{"l" if side == "lan" else "w"}1'
    expected_argv = ['ip', 'netns', 'exec', namespace, 'tcpdump', '-n', '-U', '-s', '65535', '-i', device, '-w', str(path), 'icmp', 'or', 'tcp']
    pipe_argv = list(expected_argv); pipe_argv[12] = '-'
    if (metadata['origin'] != expected_origin or metadata['run_id'] != run_id
            or metadata['namespace'] != namespace or metadata['device'] != device
            or metadata['argv'] not in (expected_argv, pipe_argv)):
        raise Refused('capture provenance/argv does not identify this run and namespace')
    stage_path = f'{expected_stage}/' if expected_stage is not None else ''
    if not fixture and str(path) != f'/run/ngfw-test/{prefix}/traffic-a/{run_id}/{stage_path}{side}.pcap':
        raise Refused('live capture must be in the fixed private run/slot path')
    for name in ('started', 'ended'):
        if type(metadata[name]) not in (int, float) or not math.isfinite(metadata[name]):
            raise Refused('invalid capture interval')
    if not metadata['started'] < metadata['ended'] <= metadata['started'] + 1200:
        raise Refused('invalid/unbounded capture interval')
    if any(type(metadata[name]) is not int or metadata[name] < 0 for name in ('received', 'dropped')) or metadata['dropped'] != 0:
        raise Refused('capture loss/unknown packet accounting cannot prove an outcome')
    try:
        fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC | os.O_NONBLOCK)
    except OSError as error:
        raise Refused('capture cannot be safely opened') from error
    try:
        info = os.fstat(fd)
        if (not stat.S_ISREG(info.st_mode) or stat.S_IMODE(info.st_mode) != 0o600
                or info.st_uid != (os.geteuid() if fixture else 0)
                or not 24 <= info.st_size <= 16 * 1024 * 1024):
            raise Refused('capture must be an owned bounded private regular file')
        chunks = []; size = 0
        while True:
            chunk = os.read(fd, min(65536, 16 * 1024 * 1024 + 1 - size))
            if not chunk:
                break
            chunks.append(chunk); size += len(chunk)
            if size > 16 * 1024 * 1024:
                raise Refused('capture grew beyond byte bound')
        after = os.fstat(fd)
        if (size != info.st_size or (after.st_size, after.st_mtime_ns, after.st_ctime_ns)
                != (info.st_size, info.st_mtime_ns, info.st_ctime_ns)):
            raise Refused('capture changed while acquired')
        data = b''.join(chunks)
    finally:
        os.close(fd)
    if not re.fullmatch(r'[0-9a-f]{64}', metadata['sha256']) or hashlib.sha256(data).hexdigest() != metadata['sha256']:
        raise Refused('capture bytes differ from executor digest')
    packets, count = read_pcap(data)
    if metadata['received'] < count or any(not metadata['started'] <= packet.timestamp <= metadata['ended'] for packet in packets):
        raise Refused('packet count/timestamps differ from capture interval')
    return (packets, (info.st_dev, info.st_ino)) if with_identity else packets


def validate_stage_contract(stage_name, report):
    """Validate outcomes/config/counters without promoting them to traffic PASS."""
    stage = next((item for item in STAGES if item.name == stage_name), None)
    if stage is None or set(report) != {'slot', 'run_id', 'origin', 'outcomes', 'readback', 'counters', 'capture_sides'}:
        raise Refused('unknown stage or incomplete evidence contract')
    slot = report['slot']; slot_values(slot)
    if report['origin'] not in ('source_fixture', 'tcpdump_live') or not re.fullmatch(r'[0-9a-f]{32}', report['run_id']):
        raise Refused('unclassified or missing execution identity')
    if set(report['outcomes']) != set(stage.required_outcomes) or len(report['outcomes']) != len(stage.required_outcomes):
        raise Refused('required forwarding/drop/translation/path outcomes missing or duplicated')
    if report['capture_sides'] != ['lan', 'wan']:
        raise Refused('both ingress and egress capture contracts required')
    readback = report['readback']
    if (set(readback) != {'desired_sha256', 'retrieved_sha256', 'revision'}
            or not re.fullmatch(r'[0-9a-f]{64}', readback['desired_sha256'])
            or readback['desired_sha256'] != readback['retrieved_sha256']
            or type(readback['revision']) is not int or readback['revision'] <= 0):
        raise Refused('desired/config readback and positive committed revision required')
    if not isinstance(report['counters'], list) or len(report['counters']) < 2:
        raise Refused('scoped ingress/egress counters required')
    for counter in report['counters']:
        if (set(counter) != {'interface', 'before', 'after'} or not isinstance(counter['interface'], str)
                or not re.fullmatch(rf'(?:host-w{slot}[lw]0(?:\.[0-9]+)?|loop{slot}[0-9]{{2}})', counter['interface'])
                or type(counter['before']) is not int or type(counter['after']) is not int
                or not 0 <= counter['before'] <= counter['after']):
            raise Refused('counter reset/invalid or foreign interface evidence')
    return {'status': 'CONTRACT_VALIDATED', 'origin': report['origin'], 'whole_chain_proven': False,
            'packet_outcomes_proven': False}


def validate_go_events(raw, selected_test, package):
    """Require actual selected-test execution; suite exit/skip is insufficient."""
    import json
    if (not isinstance(raw, str) or len(raw.encode()) > 16 * 1024 * 1024
            or not re.fullmatch(r'Test[A-Za-z0-9_]+', selected_test) or not package):
        raise Refused('invalid selected test or bounded Go JSON input')
    started = finished = package_passed = False
    lines = raw.splitlines()
    if not lines or len(lines) > 100000:
        raise Refused('empty/oversize Go event stream')
    for line in lines:
        try:
            event = json.loads(line)
        except ValueError as error:
            raise Refused('non-JSON Go event') from error
        if not isinstance(event, dict) or event.get('Package') != package:
            raise Refused('foreign or missing Go package identity')
        action = event.get('Action'); test = event.get('Test')
        if action not in ('start', 'run', 'pause', 'cont', 'output', 'pass', 'fail', 'skip'):
            raise Refused('unsupported Go event status')
        if test is not None and (not isinstance(test, str) or (test != selected_test and not test.startswith(selected_test + '/'))):
            raise Refused('unselected Go test executed')
        if action in ('fail', 'skip'):
            raise Refused('selected suite/test failed or skipped')
        if test == selected_test and action == 'run':
            if started or finished:
                raise Refused('duplicate/out-of-order selected test run')
            started = True
        if test == selected_test and action == 'pass':
            if not started or finished:
                raise Refused('selected pass without exactly one run')
            finished = True
        if test is None and action == 'pass':
            if not finished or package_passed:
                raise Refused('package pass without completed selected test')
            package_passed = True
    if not (started and finished and package_passed):
        raise Refused('selected test/package completion missing')
    return {'status': 'SUPPORT_TEST_VALIDATED', 'whole_chain_proven': False, 'packet_outcomes_proven': False}
