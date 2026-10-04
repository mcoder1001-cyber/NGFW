"""One candidate document for the complete tagged Wave-A path."""
from scenario import Refused, slot_values


def topology(slot, wan_mac):
    slot_values(slot)
    import re
    if not re.fullmatch(r'(?:[0-9a-f]{2}:){5}[0-9a-f]{2}', wan_mac):
        raise Refused('WAN peer MAC must be read from the owned namespace')
    prefix = f'w{slot}'
    lan, wan = f'host-{prefix}l0', f'host-{prefix}w0'
    bvi, vrf = f'loop{slot}10', prefix + '-ta'
    bd, address, group, allow, pbr_acl, policy = [prefix + '-' + item for item in ('bd', 'srv-ip', 'srv', 'in', 'pbr', 'to202')]
    ip = lambda third, fourth: f'10.{slot}.{third}.{fourth}'
    path = lambda vlan: {'address': ip(vlan - 180, 2), 'interface': wan + '.' + str(vlan), 'weight': 1}
    match = {'kind': 'object', 'name': group}
    rule = lambda sequence, protocol, ports=None: {'sequence': sequence, 'action': 'permit',
        'destination': match, 'service': {'kind': 'inline', 'spec': dict({'protocol': protocol},
            **({'destinationPorts': ports} if ports else {}))}}
    baseline = {'interfaces': {lan: {'enabled': True}, wan: {'enabled': True}}}
    chain = {
        'vrfs': {vrf: {'id': slot * 1000 + 10}},
        'interfaces': {
            lan: {'enabled': True, 'subinterfaces': {'100': {'vlanId': 100, 'enabled': True,
                'l2': {'bridgeDomain': bd, 'tagRewrite': {'op': 'pop-1'}}}}},
            bvi: {'enabled': True, 'vrf': vrf, 'ipv4': [ip(10, 1) + '/24'],
                'l2': {'bridgeDomain': bd, 'bvi': True}, 'urpf': {'ipv4': 'strict', 'direction': 'rx'}},
            wan: {'enabled': True, 'subinterfaces': {str(vlan): {'enabled': True, 'vlanId': vlan,
                'vrf': vrf, 'ipv4': [ip(vlan - 180, 1) + '/24']} for vlan in (201, 202)}},
        },
        'objects': {'addresses': {address: {'type': 'host', 'address': ip(99, 1)}},
                    'addressGroups': {group: {'members': [address]}}},
        'acl': {'lists': {
            allow: {'rules': [rule(10, 'icmp'), rule(20, 'tcp', ['8000', '8001']),
                             {'sequence': 30, 'action': 'deny', 'ipVersion': 'ipv4'}]},
            pbr_acl: {'rules': [rule(10, 'tcp', ['8001'])]},
        }, 'attachments': [{'list': allow, 'target': {'kind': 'interface', 'interface': bvi},
                            'direction': 'in', 'sequence': 10, 'vrf': vrf}]},
        'routing': {
            'l2': {'bridgeDomains': {bd: {'id': slot * 1000 + 10}}},
            'static': [{'prefix': ip(99, 0) + '/24', 'vrf': vrf,
                        'nextHops': [path(201), path(202)]}],
            'neighbors': {'static': [{'ip': ip(21, 2), 'interface': wan + '.201', 'mac': wan_mac}]},
            'pbr': {'policies': {policy: {'acl': pbr_acl, 'priority': 10, 'paths': [path(202)]}},
                    'attachments': [{'policy': policy, 'interface': bvi, 'family': 'ipv4'}]},
        },
        'nat': {'mode': 'ed', 'inside': [bvi], 'outside': [wan + '.201', wan + '.202'],
                'insideVrf': vrf, 'outsideVrf': vrf,
                'pools': [{'name': prefix + '-pool', 'range': ip(21, 100) + '-' + ip(21, 100)}]},
    }
    return baseline, chain
