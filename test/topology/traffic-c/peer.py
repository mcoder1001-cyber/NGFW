#!/usr/bin/env python3
"""Owned namespace peers: finite packet sender, IGMPv3 join, IPFIX collector."""
import argparse
import json
import signal
import socket
import struct
import time


def parse_ipfix(data):
    if len(data) < 16 or struct.unpack_from('!HH', data) != (10, len(data)):
        raise ValueError('invalid IPFIX header')
    sets = []
    offset = 16
    while offset < len(data):
        if offset + 4 > len(data):
            raise ValueError('truncated IPFIX set')
        identifier, length = struct.unpack_from('!HH', data, offset)
        if length < 4 or offset + length > len(data) or identifier not in (2, 3) and identifier < 256:
            raise ValueError('invalid IPFIX set')
        sets.append((identifier, length - 4))
        offset += length
    return sets


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('mode', choices=['send', 'receive', 'join', 'collector'])
    parser.add_argument('--slot', type=int, required=True)
    args = parser.parse_args()
    n = args.slot
    if n not in (*range(1, 12), *range(14, 33)):
        raise SystemExit('invalid slot')
    base = 3000 + n * 100 if n < 12 else 10000 + n * 100
    if args.mode == 'send':
        with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as sock:
            sock.bind((f'10.{n}.1.2', 0))
            sent = 0
            for _ in range(200):
                if sock.sendto(b'wave-c-' + bytes(1193), (f'10.{n}.2.2', base + 70)) != 1200:
                    raise RuntimeError('short packet send')
                sent += 1
                time.sleep(.01)
            print(json.dumps({'sent': sent, 'payloadBytes': 1200}), flush=True)
        return
    if args.mode == 'receive':
        running = True
        def stop_udp(_signal, _frame):
            nonlocal running
            running = False
        signal.signal(signal.SIGINT, stop_udp)
        signal.signal(signal.SIGTERM, stop_udp)
        received = 0
        with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as sock:
            sock.bind((f'10.{n}.2.2', base + 70))
            sock.settimeout(.1)
            print('READY:UDP', flush=True)
            end = time.monotonic() + 60
            while running and time.monotonic() < end:
                try:
                    data, source = sock.recvfrom(65535)
                except socket.timeout:
                    continue
                if source[0] != f'10.{n}.1.2' or data != b'wave-c-' + bytes(1193):
                    raise RuntimeError('foreign or malformed UDP packet')
                received += 1
        print(json.dumps({'received': received}), flush=True)
        return
    if args.mode == 'join':
        with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as sock:
            sock.bind(('', base + 71))
            # Linux IP_ADD_SOURCE_MEMBERSHIP (ip_mreq_source, IGMPv3 INCLUDE).
            request = socket.inet_aton(f'232.{n}.1.1') + socket.inet_aton(f'10.{n}.1.2') + socket.inet_aton(f'10.{n}.2.2')
            sock.setsockopt(socket.IPPROTO_IP, 39, request)
            print('READY:IGMP_INCLUDE', flush=True)
            time.sleep(60)
        return
    running = True
    def stop(_signal, _frame):
        nonlocal running
        running = False
    signal.signal(signal.SIGINT, stop)
    signal.signal(signal.SIGTERM, stop)
    stats = {'messages': 0, 'templates': 0, 'dataSets': 0, 'dataBytes': 0, 'malformed': 0}
    with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as sock:
        sock.bind((f'10.{n}.2.2', base + 72))
        sock.settimeout(.2)
        print('READY:IPFIX', flush=True)
        end = time.monotonic() + 90
        while running and time.monotonic() < end:
            try:
                data, source = sock.recvfrom(65535)
            except socket.timeout:
                continue
            if source[0] != f'10.{n}.2.1':
                stats['malformed'] += 1
                continue
            try:
                sets = parse_ipfix(data)
            except ValueError:
                stats['malformed'] += 1
                continue
            stats['messages'] += 1
            stats['templates'] += sum(identifier == 2 for identifier, _ in sets)
            stats['dataSets'] += sum(identifier >= 256 and size > 0 for identifier, size in sets)
            stats['dataBytes'] += sum(size for identifier, size in sets if identifier >= 256)
    print(json.dumps(stats), flush=True)


if __name__ == '__main__':
    main()
