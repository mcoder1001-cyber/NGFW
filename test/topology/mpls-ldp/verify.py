#!/usr/bin/env python3
"""Read-only live acceptance probe. Setup/withdrawal stays with the lab owner."""
import argparse
import json
import re
import subprocess


def run(argv):
    return subprocess.run(argv, check=True, text=True, capture_output=True, timeout=20).stdout


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--pathspace', required=True)
    parser.add_argument('--table', type=int, required=True)
    parser.add_argument('--read-table-zero', action='store_true', help='read-only probe of a label in production table 0')
    parser.add_argument('--peer', required=True)
    parser.add_argument('--label', type=int, required=True)
    parser.add_argument('--withdrawn', action='store_true')
    args = parser.parse_args()
    if not re.fullmatch(r'w(?:[1-9]|1[014-9]|2[0-9]|3[0-2])', args.pathspace):
        parser.error('use your assigned lab slot pathspace')
    slot = int(args.pathspace[1:])
    if not (args.table == 0 and args.read_table_zero) and not slot * 1000 <= args.table < (slot + 1) * 1000:
        parser.error('table must be in the assigned slot range')
    if not 16 <= args.label <= 1048575:
        parser.error('label outside unreserved range')
    neighbors = json.loads(run(['vtysh', '-N', args.pathspace, '-c', 'show mpls ldp neighbor json']))
    bindings = json.loads(run(['vtysh', '-N', args.pathspace, '-c', 'show mpls ldp binding json']))
    fib = run(['vppctl', 'show', 'mpls', 'fib', str(args.table)])
    operational = any(n.get('neighborId') == args.peer and n.get('state') == 'OPERATIONAL' for n in neighbors.get('neighbors', []))
    bound = any(str(b.get('localLabel')) == str(args.label) and b.get('inUse') for b in bindings.get('bindings', []))
    installed = bool(re.search(r'(?<!\d)' + str(args.label) + r'(?!\d)', fib))
    print(json.dumps({'neighbors': neighbors, 'bindings': bindings, 'fib': fib}, indent=2))
    if args.withdrawn:
        assert not bound and not installed, 'withdrawn binding or label remains'
    else:
        assert operational and bound and installed, 'session, in-use LIB and VPP label must all exist'


if __name__ == '__main__':
    main()
