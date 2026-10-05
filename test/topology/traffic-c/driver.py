#!/usr/bin/env python3
"""Wave-C acceptance building blocks. Offline checks never claim live acceptance."""
import argparse
import json
import math
import re
import urllib.request
from pathlib import Path


class Refused(RuntimeError):
    pass


def require(condition, message):
    if not condition:
        raise Refused(message)


def slot_values(slot):
    require(type(slot) is int and slot in (*range(1, 12), *range(14, 33)), 'worker slot must be 1–11 or 14–32')
    return {'prefix': f'w{slot}', 'table': slot * 1000 + 60,
            'lan': f'ns-w{slot}-lan', 'wan': f'ns-w{slot}-wan',
            'api_port': 3000 + slot * 100 if slot < 12 else 10000 + slot * 100,
            'metrics_port': 9100 + slot * 10 + 1}


def applied(receipt):
    require(isinstance(receipt, dict), 'commit receipt must be an object')
    require(receipt.get('status') == 'applied', 'commit was not applied')
    require(receipt.get('notApplied') == [], 'missing or nonempty notApplied')
    for field in ('warnings', 'results'):
        require(isinstance(receipt.get(field), list), f'missing {field} receipt list')
        for item in receipt[field]:
            require(isinstance(item, dict), 'malformed agent result')
            require(item.get('code') != 'agent.unsupported-field', 'agent rejected unsupported field')
    metadata = receipt.get('revision')
    require(isinstance(metadata, dict), 'missing revision metadata')
    revision = metadata.get('id')
    require(type(revision) is int and revision > 0, 'missing positive committed revision')
    return revision


def mpls_packets(text, slot, label):
    slot_values(slot)
    require(type(label) is int and 16 <= label <= 1048575, 'invalid MPLS label')
    # Match one tcpdump packet line, not unrelated lines from separate packets.
    pattern = re.compile(r'ethertype MPLS unicast \(0x8847\).*label ' + str(label) +
                         r'\b.*10\.' + str(slot) + r'\.1\.2 > 10\.' + str(slot) + r'\.98\.2: ICMP echo request')
    count = sum(bool(pattern.search(line)) for line in text.splitlines())
    require(count > 0, 'no owned labelled inner IPv4 request in WAN tcpdump')
    return count


def srv6_packets(text, slot, sid):
    slot_values(slot)
    require(sid == f'fd00:{slot:x}:ee::1', 'foreign SRv6 SID')
    # tcpdump -nn -vvv: outer destination + routing header + inner tuple must
    # occur in the same line. Unknown tcpdump formats fail closed for inspection.
    pattern = re.compile(r'IP6 .* > ' + re.escape(sid) + r':.*RT6.*type=4.*' +
                         re.escape(sid) + r'.*10\.' + str(slot) + r'\.1\.2 > 10\.' +
                         str(slot) + r'\.160\.2: ICMP echo request')
    count = sum(bool(pattern.search(line)) for line in text.splitlines())
    require(count > 0, 'no owned outer IPv6/SRH/inner IPv4 request in WAN tcpdump')
    return count


def counter_deltas(before, after, sent, received):
    require(type(sent) is int and type(received) is int and 0 < received < sent,
            'policer needs a positive WAN packet count smaller than sent')
    deltas = {}
    for key in ('conform', 'exceed', 'violate'):
        start, end = before.get(key), after.get(key)
        require(type(start) is int and type(end) is int and 0 <= start < end,
                f'{key} packet counter did not increase')
        deltas[key] = end - start
    return deltas


def ping_outage(text, started, ended):
    require(type(started) in (int, float) and type(ended) in (int, float)
            and math.isfinite(started) and math.isfinite(ended) and 0 < started < ended,
            'invalid measured ping interval')
    replies = []
    sequences = []
    for line in text.splitlines():
        match = re.search(r'^\[(\d+\.\d+)\] .*bytes from .*icmp_seq=(\d+).*time=', line)
        if match:
            replies.append(float(match[1]))
            sequences.append(int(match[2]))
    require(len(replies) >= 2, 'insufficient timestamped ping replies')
    require(all(a < b for a, b in zip(replies, replies[1:])) and
            all(a < b for a, b in zip(sequences, sequences[1:])), 'unordered/duplicate ping replies')
    require(started <= replies[0] <= replies[-1] <= ended, 'replies outside measured interval')
    points = [started, *replies, ended]
    longest = max(b - a for a, b in zip(points, points[1:]))
    require(longest <= 3, f'VRRP outage {longest:.3f}s exceeds 3s')
    return longest


class API:
    """Only the assigned local slot API; token is never included in diagnostics."""
    def __init__(self, slot, token, opener=urllib.request.urlopen):
        values = slot_values(slot)
        require(isinstance(token, str) and token and '\n' not in token and '\r' not in token,
                'invalid private bearer token')
        self.base = f'http://127.0.0.1:{values["api_port"]}/api/v1/'
        self.token = token
        self.opener = opener

    def request(self, method, path, payload=None):
        require(method in ('GET', 'PATCH', 'POST'), 'unsupported HTTP method')
        require(re.fullmatch(r'config(?:/(?:candidate|revisions|commit|discard|rollback/[1-9][0-9]*|routing/mpls|routing/srv6|services/qos))?', path)
                is not None, 'unowned API endpoint')
        data = None if payload is None else json.dumps(payload).encode()
        request = urllib.request.Request(self.base + path, data=data, method=method,
            headers={'Authorization': 'Bearer ' + self.token,
                     'Content-Type': 'application/merge-patch+json' if method == 'PATCH' else 'application/json'})
        try:
            with self.opener(request, timeout=60) as response:
                require(response.status == 200, 'API returned unexpected HTTP status')
                raw = response.read(1024 * 1024 + 1)
                require(len(raw) <= 1024 * 1024, 'oversized API response')
                return json.loads(raw)
        except (OSError, ValueError) as error:
            raise Refused('API request failed; inspect private slot service logs') from error


def transaction(api, endpoint, patch, evidence):
    """Always restore the committed baseline after even a failed apply/evidence.

    The caller must exclusively own the slot candidate, baseline rig and globals
    window. This primitive creates no fixture or authority to mutate shared VPP.
    """
    require(endpoint in ('routing/mpls', 'routing/srv6', 'services/qos'), 'unowned feature path')
    baseline = api.request('GET', 'config')
    candidate = api.request('GET', 'config/candidate')
    # Candidate endpoint returns the config envelope; require exact equality.
    require(candidate == baseline, 'dirty candidate: refusing to overwrite another edit')
    revisions = api.request('GET', 'config/revisions')
    items = revisions.get('items', [])
    require(items and type(items[0].get('id')) is int and items[0]['id'] > 0,
            'missing rollback revision')
    revision = items[0]['id']
    try:
        api.request('PATCH', 'config/' + endpoint, patch)
        receipt = api.request('POST', 'config/commit', {})
        applied(receipt)
        return evidence(receipt)
    finally:
        try:
            api.request('POST', 'config/discard', {})
        finally:
            applied(api.request('POST', f'config/rollback/{revision}', {}))
        require(api.request('GET', 'config') == baseline, 'rollback changed baseline document')
        require(api.request('GET', 'config/candidate') == baseline, 'candidate residue after rollback')


def plan(slot):
    values = slot_values(slot)
    return {'task': 'TEST-traffic-C', 'mode': 'DRY_RUN_ONLY', 'slot': slot, **values,
            'steps': ['MPLS label packet', 'SRv6 SRH packet', 'VRRP commit failover',
                      'QoS three-colour policer', 'IPFIX/capture/IGMP/metrics riders'],
            'not_exercised': {'LISP': 'no peer; separate V14 window',
                              'LB': 'loopback-only host proof', 'host stack': 'separate host proof',
                              'SNMP': 'no packet proof in this scenario',
                              'HA state sync': 'requires second VPP; INTEGRATE-E2E',
                              'LDP': 'requires merged F-mpls-ldp-host and owned FRR'},
            'live_acceptance': False,
            'remaining': ['manager-issued quiet window and owned rig/stack',
                          'global before/after restoration transaction',
                          'live tcpdump/process/NRestarts collection',
                          'keepalived fixture and VRRP API choreography',
                          'rider collection and residue proof']}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--slot', type=int, required=True)
    parser.add_argument('--dry-run', action='store_true')
    args = parser.parse_args()
    require(args.dry_run, 'live orchestration incomplete: no host mutation is authorized by this driver')
    print(json.dumps(plan(args.slot), indent=2))


if __name__ == '__main__':
    try:
        main()
    except Refused as error:
        raise SystemExit(str(error)) from None
