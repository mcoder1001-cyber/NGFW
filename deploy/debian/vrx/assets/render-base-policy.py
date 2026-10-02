#!/usr/bin/env python3
"""Render the packaging-owned static base table; never load the host ruleset."""
import argparse
import json
import re
import sys


def interface(value):
    if not re.fullmatch(r'[A-Za-z0-9_][A-Za-z0-9_.-]{0,14}', value) or value == 'lo':
        raise ValueError('invalid explicit interface')
    return value


def render(management, punts):
    management = interface(management)
    entries = [] if punts == '' else punts.split(',')
    if len(entries) > 64 or len(set(entries)) != len(entries):
        raise ValueError('punt interfaces must be unique, maximum 64')
    entries = sorted(interface(value) for value in entries)
    if management in entries:
        raise ValueError('management interface cannot be a punt interface')
    elements = '' if not entries else ' elements = { ' + ', '.join(map(json.dumps, entries)) + ' };'
    return f'''# Packaging owns only inet vrx_base; never flush other tables.
add table inet vrx_base
delete table inet vrx_base
table inet vrx_base {{
    set punt_interfaces {{ type ifname;{elements} }}
    set dynamic_punt_interfaces {{ type ifname; }}
    chain input {{
        type filter hook input priority -10; policy drop;
        iifname "lo" accept
        ct state invalid drop
        ct state established,related accept
        iifname {json.dumps(management)} tcp dport {{ 22, 443 }} accept
        iifname {json.dumps(management)} meta l4proto ipv6-icmp icmpv6 type {{ nd-router-advert, nd-neighbor-solicit, nd-neighbor-advert }} accept
        iifname @punt_interfaces accept
        iifname @dynamic_punt_interfaces accept
    }}
}}
'''


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--management-interface', required=True)
    parser.add_argument('--punt-interfaces', default='')
    args = parser.parse_args()
    try:
        sys.stdout.write(render(args.management_interface, args.punt_interfaces))
    except ValueError as error:
        parser.error(str(error))
