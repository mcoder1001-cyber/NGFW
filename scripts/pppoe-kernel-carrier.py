#!/usr/bin/python3
"""Root-only, opt-in namespace helper for the PPP kernel carrier.

Not installed or activated until the matching VPP owner and private-PPP unit are
reviewed together. No arbitrary command, path, interface or shell is accepted.
The ledger is root-owned, boot-local and is NOT a forwarding-ready receipt.
"""
import argparse
import contextlib
import fcntl
import hashlib
import ipaddress
import json
import os
import re
import secrets
import stat
import subprocess
import time

IP = '/usr/sbin/ip'
NFT = '/usr/sbin/nft'
NSENTER = '/usr/bin/nsenter'
SYSCTL = '/usr/sbin/sysctl'
SETPRIV = '/usr/bin/setpriv'
HELPER = '/usr/lib/ngfw/pppoe-carrier.py'
WAN_PROBE = '/usr/lib/ngfw/ngfw-wan-probe'
PPP_CAPS = (1 << 12) | (1 << 13)
ROOT = '/run/ngfw-pppoe-carrier'
NETNS = '/run/netns'
BROKER = '/run/ngfw/pppoe-broker'
TOKEN = re.compile(r'ngp-[0-9a-f]{12}\Z')
NAME = re.compile(r'[a-zA-Z0-9][a-zA-Z0-9_.-]{0,62}\Z')
GEN = re.compile(r'[0-9a-f]{32}\Z')
MAC = re.compile(r'(?:[0-9a-f]{2}:){5}[0-9a-f]{2}\Z')


def token_for(owner, logical):
    if not NAME.fullmatch(owner) or not NAME.fullmatch(logical):
        raise ValueError('invalid owner or logical interface')
    return 'ngp-' + hashlib.sha256((owner + '\0' + logical).encode()).hexdigest()[:12]


def validate_spec(owner, logical, spec):
    required = {'owner', 'logical', 'parent', 'mtu', 'host4', 'peer4', 'host6', 'peer6'}
    if (set(spec) != required or spec['owner'] != owner or spec['logical'] != logical
            or not NAME.fullmatch(spec['parent']) or spec['parent'] == logical
            or type(spec['mtu']) is not int or not 128 <= spec['mtu'] <= 1492):
        raise ValueError('invalid immutable carrier specification')
    normalized = dict(spec)
    for family in (4, 6):
        local = ipaddress.ip_interface(spec['host' + str(family)])
        peer = ipaddress.ip_address(spec['peer' + str(family)])
        if (local.version != family or peer.version != family or peer not in local.network
                or peer == local.ip or local.ip.is_multicast or peer.is_multicast
                or local.ip.is_unspecified or peer.is_unspecified
                or local.network.prefixlen != (30 if family == 4 else 126)
                or local.ip in (local.network.network_address, local.network.broadcast_address)
                or peer in (local.network.network_address, local.network.broadcast_address)):
            raise ValueError('invalid point-to-point transit allocation')
        normalized['host' + str(family)] = str(local)
        normalized['peer' + str(family)] = str(peer)
    return normalized


def run(argv, *, data=None, pass_fds=(), timeout=20):
    return subprocess.run(argv, input=data, text=True, check=True,
                          stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                          timeout=timeout, pass_fds=pass_fds,
                          env={'PATH': '/usr/sbin:/usr/bin:/sbin:/bin', 'LC_ALL': 'C'}).stdout


def protected_directory(path):
    """Every component must be root-controlled; reject symlinks and writable parents."""
    current = '/'
    for component in path.split('/')[1:]:
        current = os.path.join(current, component)
        info = os.lstat(current)
        if not stat.S_ISDIR(info.st_mode) or info.st_uid != 0 or info.st_mode & 0o022:
            raise ValueError('directory is not root-controlled')


def boot_id():
    with open('/proc/sys/kernel/random/boot_id', encoding='ascii') as stream:
        return stream.read().strip()


def link_names(token):
    if not TOKEN.fullmatch(token):
        raise ValueError('invalid carrier token')
    return 'pw' + token[4:], 'pt' + token[4:]


def nft_policy(enabled, token):
    # Atomic table replacement: the initial add makes this also work on first use.
    # No NAT table, flow offload, raw-WAN IP or bypass-LAN forwarding is permitted.
    forward = ''
    if enabled:
        forward = ('iifname "ppptransit" oifname "ppp0" accept\n'
                   'iifname "ppp0" oifname "ppptransit" accept\n')
    policy = '''add table inet ngfw_ppp
delete table inet ngfw_ppp
add table inet ngfw_ppp
add chain inet ngfw_ppp input { type filter hook input priority 0; policy drop; }
add chain inet ngfw_ppp output { type filter hook output priority 0; policy drop; }
add chain inet ngfw_ppp forward { type filter hook forward priority 0; policy drop; }
add rule inet ngfw_ppp input iifname "lo" accept
add rule inet ngfw_ppp output oifname "lo" accept
add rule inet ngfw_ppp input iifname "ppp0" ip6 saddr fe80::/10 meta l4proto ipv6-icmp icmpv6 type { nd-router-advert, nd-neighbor-solicit, nd-neighbor-advert } ip6 hoplimit 255 accept
add rule inet ngfw_ppp input iifname "ppptransit" meta l4proto ipv6-icmp icmpv6 type { nd-neighbor-solicit, nd-neighbor-advert } ip6 hoplimit 255 accept
add rule inet ngfw_ppp input iifname "ppp0" meta nfproto ipv6 udp sport 547 udp dport 546 accept
add rule inet ngfw_ppp output oifname { "ppp0", "ppptransit" } meta l4proto ipv6-icmp icmpv6 type { nd-router-solicit, nd-neighbor-solicit, nd-neighbor-advert } ip6 hoplimit 255 accept
add rule inet ngfw_ppp output oifname { "ppp0", "ppptransit" } meta l4proto ipv6-icmp icmpv6 type { destination-unreachable, packet-too-big, time-exceeded, parameter-problem } accept
add rule inet ngfw_ppp output oifname { "ppp0", "ppptransit" } ip protocol icmp icmp type { destination-unreachable, time-exceeded, parameter-problem } accept
add rule inet ngfw_ppp output oifname "ppp0" udp sport 546 udp dport 547 accept
''' + ''.join('add rule inet ngfw_ppp forward ' + line + '\n'
              for line in forward.splitlines())
    raw, transit_name = link_names(token)
    return policy.replace('pppwan', raw).replace('ppptransit', transit_name)


def request_digest(request):
    return hashlib.sha256(json.dumps(request, sort_keys=True, separators=(',', ':')).encode()).hexdigest()


def request_token(request):
    if not isinstance(request, dict) or len(json.dumps(request)) > 8192:
        raise ValueError('invalid bounded broker request')
    op = request.get('op')
    fields = {'provision': {'op', 'owner', 'logical', 'spec'}, 'list': {'op', 'owner'},
              'prepare': {'op', 'token', 'generation', 'physical_mac', 'transit'},
              'configure': {'op', 'token', 'generation', 'accept_default_route'},
              'probe': {'op', 'token', 'generation', 'kind', 'target'}}
    for name in ('verify', 'inspect', 'delete', 'withdraw'):
        fields[name] = {'op', 'token', 'generation'}
    if op not in fields or set(request) != fields[op]:
        raise ValueError('unsupported broker operation or fields')
    if op == 'provision':
        validate_spec(request['owner'], request['logical'], request['spec'])
        return token_for(request['owner'], request['logical'])
    if op == 'list':
        if not NAME.fullmatch(request['owner']):
            raise ValueError('invalid inventory owner')
        return 'ngp-' + hashlib.sha256(('inventory\0' + request['owner']).encode()).hexdigest()[:12]
    if not TOKEN.fullmatch(request['token']) or not GEN.fullmatch(request['generation']):
        raise ValueError('invalid broker generation identity')
    if op == 'configure' and type(request['accept_default_route']) is not bool:
        raise ValueError('invalid IPv6 default-route policy')
    if op == 'prepare':
        if (not isinstance(request['transit'], dict)
                or set(request['transit']) != {'local4', 'peer4', 'local6', 'peer6'}
                or not MAC.fullmatch(request['physical_mac'])):
            raise ValueError('invalid preparation contract')
    if op == 'probe':
        validate_probe(request['kind'], request['target'])
    return request['token']


def open_verified_probe():
    protected_directory(os.path.dirname(WAN_PROBE))
    digest_fd = os.open(WAN_PROBE + '.sha256', os.O_RDONLY | os.O_NOFOLLOW)
    try:
        info = os.fstat(digest_fd)
        if not stat.S_ISREG(info.st_mode) or info.st_uid != 0 or info.st_mode & 0o022:
            raise ValueError('untrusted probe digest file')
        digest = os.read(digest_fd, 66)
        if re.fullmatch(rb'[0-9a-f]{64}\n', digest) is None:
            raise ValueError('invalid probe digest format')
    finally:
        os.close(digest_fd)
    fd = os.open(WAN_PROBE, os.O_RDONLY | os.O_NOFOLLOW)
    try:
        info = os.fstat(fd)
        if (not stat.S_ISREG(info.st_mode) or info.st_uid != 0 or info.st_mode & 0o6022
                or not info.st_mode & 0o111 or info.st_size > 64 * 1024 * 1024):
            raise ValueError('untrusted probe executable')
        checksum = hashlib.sha256()
        while True:
            chunk = os.read(fd, 65536)
            if not chunk:
                break
            checksum.update(chunk)
        if checksum.hexdigest().encode() != digest.strip():
            raise ValueError('probe executable digest mismatch')
        os.lseek(fd, 0, os.SEEK_SET)
        return fd
    except Exception:
        os.close(fd)
        raise


def validate_probe(kind, target):
    if kind not in ('icmp', 'dns', 'http'):
        raise ValueError('unsupported fixed probe kind')
    address = ipaddress.ip_address(target)
    if (address.is_multicast or address.is_unspecified or address.is_loopback or address.is_link_local
            or getattr(address, 'ipv4_mapped', None) is not None or str(address) == '255.255.255.255'
            or address.version != 4):
        raise ValueError('unsupported literal probe target')
    return address


def probe_policy(token, kind, target, local):
    address = validate_probe(kind, target)
    family = 'ip' if address.version == 4 else 'ip6'
    set_type = 'ipv4_addr' if address.version == 4 else 'ipv6_addr'
    own = ', '.join(str(ipaddress.ip_address(value)) + ' timeout 5s' for value in local)
    if not own or any(ipaddress.ip_address(value).version != address.version for value in local):
        raise ValueError('invalid probe local address set')
    outgoing = 'icmp type echo-request' if kind == 'icmp' else ('udp dport 53' if kind == 'dns' else 'tcp dport 80')
    incoming = 'icmp type echo-reply' if kind == 'icmp' else ('udp sport 53' if kind == 'dns' else 'tcp sport 80')
    # No forward-chain exemption. Only the fixed, device-bound local probe may
    # originate this traffic; expires after five seconds even if supervisor dies.
    return nft_policy(True, token) + f"""add set inet ngfw_ppp probe_target {{ type {set_type}; flags timeout; timeout 5s; elements = {{ {address} timeout 5s }}; }}
add set inet ngfw_ppp probe_local {{ type {set_type}; flags timeout; timeout 5s; elements = {{ {own} }}; }}
add chain inet ngfw_ppp probe_return {{ type filter hook prerouting priority -150; policy accept; }}
add rule inet ngfw_ppp probe_return iifname "ppp0" {family} saddr @probe_target {family} daddr @probe_local {incoming} ct mark 0x4e47 ct state established meta mark set 0x4e47
add rule inet ngfw_ppp input iifname "ppp0" {family} saddr @probe_target {family} daddr @probe_local {incoming} ct mark 0x4e47 ct state established accept
add rule inet ngfw_ppp output oifname "ppp0" {family} daddr @probe_target {family} saddr @probe_local {outgoing} ct mark set 0x4e47 accept
"""


def pristine_kernel_fallback(link):
    """Accept only strictly unconfigured immutable kernel fallback devices."""
    flags = link.get('flags')
    if not isinstance(flags, list) or any(not isinstance(flag, str) for flag in flags):
        return False
    defaults = {
        'gre0': ('gre', 'gre', {'remote': 'any', 'local': 'any', 'ttl': 0, 'pmtudisc': False}),
        'gretap0': ('gretap', 'ether', {'remote': 'any', 'local': 'any', 'ttl': 0, 'pmtudisc': False}),
        'erspan0': ('erspan', 'ether', {'remote': 'any', 'local': 'any', 'ttl': 0,
                                      'pmtudisc': False, 'okey': '0.0.0.0',
                                      'erspan_index': 0, 'erspan_ver': 1}),
        'ip6tnl0': ('ip6tnl', 'tunnel6', {'proto': 'ip6ip6', 'remote': 'any', 'local': 'any',
                                        'ttl': 0, 'encap_limit': 0, 'tclass': '0x00',
                                        'flowlabel': '0x00000'}),
    }
    expected = defaults.get(link.get('ifname'))
    if expected is None:
        return False
    kind, link_type, data = expected
    mtu = {'gre0': 1476, 'gretap0': 1462, 'erspan0': 1450, 'ip6tnl0': 1452}[link['ifname']]
    info = link.get('linkinfo', {})
    actual_data = info.get('info_data')
    if not isinstance(actual_data, dict) or set(actual_data) != set(data):
        return False
    if any(type(actual_data[key]) is not type(value) for key, value in data.items()):
        return False
    expected_flags = {'NOARP'} if link_type in ('gre', 'tunnel6') else {'BROADCAST', 'MULTICAST'}
    return (set(flags) == expected_flags
            and link.get('netns-immutable') is True and link.get('operstate') == 'DOWN'
            and not {'UP', 'LOWER_UP', 'MASTER'}.intersection(flags)
            and link.get('link') is None and 'master' not in link
            and link.get('mtu') == mtu and not link.get('ifalias', '')
            and link.get('group') == 'default' and link.get('promiscuity') == 0
            and type(link.get('promiscuity')) is int and type(link.get('allmulti')) is int
            and link.get('allmulti') == 0 and link.get('addr_info') == []
            and link.get('link_type') == link_type and info.get('info_kind') == kind
            and info.get('info_data') == data)


class Carrier:
    def __init__(self, runner=run, root=ROOT, netns=NETNS):
        self.run = runner
        self.root = root
        self.netns = netns

    def path(self, token):
        if not TOKEN.fullmatch(token):
            raise ValueError('invalid carrier token')
        return os.path.join(self.root, token + '.json')

    @contextlib.contextmanager
    def locked(self):
        if os.geteuid() != 0:
            raise PermissionError('carrier provisioner requires root')
        for path in (self.root, self.netns):
            os.makedirs(path, mode=0o700, exist_ok=True)
            protected_directory(path)
        fd = os.open(self.root + '/lock', os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
        try:
            info = os.fstat(fd)
            if not stat.S_ISREG(info.st_mode) or info.st_uid != 0 or info.st_mode & 0o077:
                raise ValueError('unsafe carrier lock')
            fcntl.flock(fd, fcntl.LOCK_EX)
            yield
        finally:
            os.close(fd)

    @contextlib.contextmanager
    def readonly_locked(self):
        protected_directory(self.root)
        fd = os.open(self.root + '/lock', os.O_RDONLY | os.O_NOFOLLOW)
        try:
            info = os.fstat(fd)
            if not stat.S_ISREG(info.st_mode) or info.st_uid != 0 or info.st_mode & 0o077:
                raise ValueError('unsafe carrier lock')
            fcntl.flock(fd, fcntl.LOCK_EX)
            yield
        finally:
            os.close(fd)

    def save(self, token, record):
        temporary = self.path(token) + '.' + secrets.token_hex(8)
        fd = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
        try:
            with os.fdopen(fd, 'w', encoding='ascii') as stream:
                json.dump(record, stream, sort_keys=True)
                stream.write('\n')
                stream.flush()
                os.fsync(stream.fileno())
            os.replace(temporary, self.path(token))
        finally:
            if os.path.exists(temporary):
                os.unlink(temporary)

    def load(self, token, generation=None):
        fd = os.open(self.path(token), os.O_RDONLY | os.O_NOFOLLOW)
        with os.fdopen(fd, 'r', encoding='ascii') as stream:
            info = os.fstat(stream.fileno())
            if not stat.S_ISREG(info.st_mode) or info.st_uid != 0 or info.st_mode & 0o077:
                raise ValueError('unsafe carrier ledger')
            record = json.load(stream)
        if (record['boot'] != boot_id() or token_for(record['owner'], record['logical']) != token
                or not GEN.fullmatch(record['generation'])
                or generation is not None and record['generation'] != generation):
            raise ValueError('stale or foreign carrier ledger')
        return record

    @contextlib.contextmanager
    def pinned(self, token, record):
        fd = os.open(os.path.join(self.netns, token), os.O_RDONLY | os.O_NOFOLLOW)
        try:
            info = os.fstat(fd)
            if [info.st_dev, info.st_ino] != record['namespace']:
                raise ValueError('namespace identity changed')
            # NS_GET_NSTYPE verifies a real network namespace, not a regular file.
            if fcntl.ioctl(fd, 0xb703) != 0x40000000:
                raise ValueError('not a network namespace')
            yield fd
        finally:
            os.close(fd)

    def inside(self, fd, argv, data=None, timeout=20):
        return self.run([NSENTER, '--net=/proc/self/fd/' + str(fd), '--'] + argv,
                        data=data, pass_fds=(fd,), timeout=timeout)

    def provision(self, owner, logical, spec):
        token = token_for(owner, logical)
        spec = validate_spec(owner, logical, spec)
        if os.path.lexists(self.path(token)):
            record = self.load(token)
            # Check full owner strings, even in the improbable 48-bit token collision.
            if (record['owner'], record['logical']) != (owner, logical):
                raise ValueError('carrier token collision')
            if record.get('spec') != spec:
                raise ValueError('carrier specification changed; consumers must be removed before recreation')
            with self.pinned(token, record) as fd:
                if not record.get('bound'):
                    self.inside(fd, [NFT, '-f', '-'], nft_policy(False, token))
                return record
        if os.path.lexists(os.path.join(self.netns, token)):
            raise ValueError('unowned namespace; explicit recovery required')
        for filename in os.listdir(self.root):
            if filename.endswith('.json'):
                existing = self.load(filename[:-5])
                previous = existing.get('spec')
                if previous:
                    for family in (4, 6):
                        if ipaddress.ip_interface(previous['host' + str(family)]).network.overlaps(
                                ipaddress.ip_interface(spec['host' + str(family)]).network):
                            raise ValueError('transit allocation overlaps existing carrier')
        self.run([IP, 'netns', 'add', token])
        info = os.stat(os.path.join(self.netns, token), follow_symlinks=False)
        record = dict(token=token, owner=owner, logical=logical, generation=secrets.token_hex(16),
                      boot=boot_id(), namespace=[info.st_dev, info.st_ino], configured=False, spec=spec)
        # A crash before this save leaves an orphan that is never silently adopted.
        self.save(token, record)
        with self.pinned(token, record) as fd:
            self.inside(fd, [NFT, '-f', '-'], nft_policy(False, token))
        return record

    def tap_identity(self, token, record, name, link):
        raw, _ = link_names(token)
        if (link.get('ifalias') != token + ':' + record['generation'] + ':' + name
                or link.get('linkinfo', {}).get('info_kind') != 'tun'
                or link.get('linkinfo', {}).get('info_data', {}).get('type') != 'tap'):
            raise ValueError('unverified carrier TAP')
        if record.get('bound', {}).get(name) != link.get('ifindex'):
            raise ValueError('carrier TAP binding changed')
        if name == raw and link.get('address') != record.get('physical_mac'):
            raise ValueError('carrier raw MAC changed')
        return link['ifindex']

    def owned_links(self, fd):
        links = json.loads(self.inside(fd, [IP, '-j', '-d', 'link', 'show']))
        addresses = None
        result = []
        for link in links:
            if link.get('ifname') in ('gre0', 'gretap0', 'erspan0', 'ip6tnl0'):
                if addresses is None:
                    addresses = json.loads(self.inside(fd, [IP, '-j', 'address', 'show']))
                rows = [row for row in addresses if row.get('ifname') == link.get('ifname')
                        and row.get('ifindex') == link.get('ifindex')]
                observed = dict(link)
                if len(rows) == 1:
                    observed['addr_info'] = rows[0].get('addr_info')
                if pristine_kernel_fallback(observed):
                    continue
            result.append(link)
        return result

    def links(self, fd, token, record, require_ppp):
        raw, transit_name = link_names(token)
        links = self.owned_links(fd)
        by_name = {link['ifname']: link for link in links}
        allowed = {'lo', raw, transit_name, 'ppp0'}
        if set(by_name) - allowed:
            raise ValueError('foreign interface in carrier namespace')
        expected = {}
        for name in (raw, transit_name):
            expected[name] = self.tap_identity(token, record, name, by_name.get(name, {}))
        if require_ppp:
            ppp = by_name.get('ppp0', {})
            if ppp.get('link_type') != 'ppp' or 'UP' not in ppp.get('flags', []):
                raise ValueError('PPP link is not up')
            expected['ppp0'] = ppp['ifindex']
        return expected

    def prepare(self, token, generation, raw_index, transit_index, physical_mac, transit):
        raw, transit_name = link_names(token)
        if (not MAC.fullmatch(physical_mac) or int(physical_mac[:2], 16) & 1
                or physical_mac == '00:00:00:00:00:00'
                or (raw_index is None) != (transit_index is None)
                or raw_index is not None and (raw_index <= 0 or transit_index <= 0 or raw_index == transit_index)):
            raise ValueError('invalid TAP identity or unicast WAN MAC')
        normalized = {}
        for family in (4, 6):
            local = ipaddress.ip_interface(transit['local' + str(family)])
            peer = ipaddress.ip_address(transit['peer' + str(family)])
            if (local.version != family or peer.version != family or peer not in local.network
                    or peer == local.ip or local.ip.is_multicast or peer.is_multicast
                    or local.ip.is_unspecified or peer.is_unspecified
                    or local.network.prefixlen != (30 if family == 4 else 126)
                    or local.ip in (local.network.network_address, local.network.broadcast_address)
                    or peer in (local.network.network_address, local.network.broadcast_address)):
                raise ValueError('invalid point-to-point transit allocation')
            normalized['local' + str(family)] = str(local)
            normalized['peer' + str(family)] = str(peer)
        record = self.withdraw(token, generation)
        expected = {key: record['spec'][key.replace('local', 'host')] for key in normalized}
        if expected != normalized:
            raise ValueError('transit endpoints do not match immutable specification')
        if record.get('transit') and record['transit'] != normalized:
            raise ValueError('transit allocation is immutable within a generation')
        with self.pinned(token, record) as fd:
            links = self.owned_links(fd)
            by_name = {link['ifname']: link for link in links}
            if set(by_name) - {'lo', raw, transit_name}:
                raise ValueError('stop PPP and remove foreign links before preparation')
            if raw_index is None:
                raw_index = by_name.get(raw, {}).get('ifindex')
                transit_index = by_name.get(transit_name, {}).get('ifindex')
            if not raw_index or not transit_index or raw_index == transit_index:
                raise ValueError('missing TAP indices')
            bound = {raw: raw_index, transit_name: transit_index}
            if record.get('bound') and (record['bound'] != bound or record['physical_mac'] != physical_mac):
                raise ValueError('carrier is already bound; recreate generation before rebinding')
            # Validate the entire proposal before changing any TAP identity.
            for name, index in bound.items():
                link = by_name.get(name, {})
                alias = token + ':' + generation + ':' + name
                if (link.get('ifindex') != index or link.get('ifalias', '') not in ('', alias)
                        or link.get('linkinfo', {}).get('info_kind') != 'tun'
                        or link.get('linkinfo', {}).get('info_data', {}).get('type') != 'tap'):
                    raise ValueError('unverified newly created TAP')
            for name in bound:
                self.inside(fd, [IP, 'link', 'set', 'dev', name, 'alias', token + ':' + generation + ':' + name])
            self.inside(fd, [IP, 'link', 'set', 'dev', raw, 'address', physical_mac])
            # IPv6 autoconfiguration on raw Ethernet must never create a plain-IP bypass.
            self.inside(fd, [SYSCTL, '-q', '-w', f'net.ipv6.conf.{raw}.disable_ipv6=1'])
            self.inside(fd, [IP, '-4', 'address', 'flush', 'dev', raw])
            self.inside(fd, [IP, 'link', 'set', 'dev', raw, 'up'])
            if record['spec']['mtu'] < 1280:
                self.inside(fd, [SYSCTL, '-q', '-w', f'net.ipv6.conf.{transit_name}.disable_ipv6=1'])
            self.inside(fd, [IP, 'link', 'set', 'dev', transit_name, 'up'])
            self.inside(fd, [IP, 'link', 'set', 'dev', 'lo', 'up'])
            record['bound'] = bound
            record['physical_mac'] = physical_mac
            record['transit'] = normalized
            self.links(fd, token, record, False)
            self.save(token, record)
        return record

    def nft_digest(self, state, token):
        objects = state.get('nftables', [])
        chains = [item['chain'] for item in objects if 'chain' in item]
        if (len(chains) != 3 or {chain.get('name') for chain in chains} != {'input', 'output', 'forward'}
                or any(chain.get('policy') != 'drop' or chain.get('hook') != chain['name']
                       or chain.get('type') != 'filter' or chain.get('prio') != 0 for chain in chains)):
            raise ValueError('carrier firewall base chains differ')
        rules = [item['rule'] for item in objects if 'rule' in item]
        if len(rules) != nft_policy(True, token).count('add rule '):
            raise ValueError('carrier firewall rule count differs')
        stable = []
        for item in objects:
            if 'metainfo' in item:
                continue
            kind, value = next(iter(item.items()))
            if kind not in ('table', 'chain', 'rule'):
                raise ValueError('unexpected carrier firewall object')
            if value.get('family') != 'inet' or (value.get('table', value.get('name')) != 'ngfw_ppp'):
                raise ValueError('foreign carrier firewall object')
            stable.append({kind: {key: val for key, val in value.items() if key not in ('handle', 'index')}})
        return hashlib.sha256(json.dumps(stable, sort_keys=True, separators=(',', ':')).encode()).hexdigest()

    def policy_rules(self, fd, version, token, record):
        raw, transit_name = link_names(token)
        rules = json.loads(self.inside(fd, [IP, '-' + str(version), '-j', 'rule', 'show']))
        normalized = []
        for rule in rules:
            table = rule.get('table')
            table = {'local': 255, 'main': 254}.get(table, table)
            try:
                table = int(table)
            except (TypeError, ValueError) as error:
                raise ValueError('unsupported policy rule table') from error
            extra = set(rule) - {'priority', 'src', 'dst', 'table', 'iif', 'protocol', 'ipproto', 'sport', 'dport', 'sport_mask', 'dport_mask', 'dstlen'}
            if extra or rule.get('src', 'all') not in ('all', '0.0.0.0/0', '::/0'):
                raise ValueError('unexpected policy rule selector')
            for field in ('sport', 'dport'):
                mask = rule.get(field + '_mask')
                if field + '_mask' in rule and (field not in rule or not
                        (type(mask) is str and mask == '0xffff' or type(mask) is int and mask == 65535)):
                    raise ValueError('unsupported policy port mask')
            destination = rule.get('dst', 'all')
            if type(destination) is not str:
                raise ValueError('unsupported policy destination type')
            prefixlen = rule.get('dstlen')
            if 'dstlen' in rule:
                if (type(prefixlen) is not int or not 0 <= prefixlen <= (32 if version == 4 else 128)
                        or destination == 'all'):
                    raise ValueError('unsupported policy destination length')
                if '/' in destination:
                    if ipaddress.ip_network(destination, strict=False).prefixlen != prefixlen:
                        raise ValueError('policy destination lengths disagree')
                else:
                    destination += '/' + str(prefixlen)
            if destination != 'all':
                network = ipaddress.ip_network(destination, strict=False)
                if network.version != version:
                    raise ValueError('policy destination family differs')
                destination = str(network)
            normalized.append((rule.get('priority'), table, rule.get('iif'), destination,
                               rule.get('ipproto'), str(rule.get('sport', '')), str(rule.get('dport', ''))))
        expected = [(10, 100, 'ppp0', 'all', None, '', ''),
                    (20, 101, transit_name, 'all', None, '', ''),
                    (100, 255, None, 'all', None, '', ''), (32766, 254, None, 'all', None, '', '')]
        if version == 4:
            expected += [(6, 255, transit_name, str(ipaddress.ip_interface(record['transit']['local4']).ip) + '/32', None, '', '')]
        if version == 6:
            expected += [(4, 255, 'ppp0', 'all', 'udp', '547', '546'),
                         (6, 255, transit_name, str(ipaddress.ip_interface(record['transit']['local6']).ip) + '/128', None, '', ''),
                         (5, 255, None, 'fe80::/10', None, '', ''),
                         (5, 255, None, 'ff00::/8', None, '', '')]
        if sorted(normalized, key=repr) != sorted(expected, key=repr):
            raise ValueError('carrier ingress policy rules differ')
        return rules

    def verify_state(self, fd, token, record):
        raw, transit_name = link_names(token)
        live = self.links(fd, token, record, True)
        tables = json.loads(self.inside(fd, [NFT, '-j', 'list', 'tables']))
        for item in tables.get('nftables', []):
            table = item.get('table')
            if table and (table['family'], table['name']) != ('inet', 'ngfw_ppp'):
                raise ValueError('foreign firewall table in carrier namespace')
        if live != record.get('links'):
            raise ValueError('PPP session link identity changed')
        addresses = json.loads(self.inside(fd, [IP, '-j', 'address', 'show']))
        by_name = {item['ifname']: item for item in addresses}
        if by_name.get(raw, {}).get('addr_info'):
            raise ValueError('raw WAN acquired an IP address')
        ppp_mtu = by_name.get('ppp0', {}).get('mtu', 0)
        if (ppp_mtu != record['spec']['mtu']
                or by_name.get(transit_name, {}).get('mtu') != ppp_mtu):
            raise ValueError('carrier MTU does not match negotiated PPP')
        transit = by_name.get(transit_name, {}).get('addr_info', [])
        versions = (4,) if record['spec']['mtu'] < 1280 else (4, 6)
        if versions == (4,) and any(item.get('family') == 'inet6' for item in transit):
            raise ValueError('IPv4-only transit acquired IPv6 address')
        for version in versions:
            configured = ipaddress.ip_interface(record['transit']['local' + str(version)])
            if not any(item.get('local') == str(configured.ip) and item.get('prefixlen') == configured.network.prefixlen
                       and not item.get('tentative') and not item.get('dadfailed') for item in transit):
                raise ValueError('transit address not ready')
            self.policy_rules(fd, version, token, record)
            for table, interface in ((100, transit_name), (101, 'ppp0')):
                routes = json.loads(self.inside(fd, [IP, '-' + str(version), '-j', 'route', 'show', 'table', str(table)]))
                if (len(routes) != 1 or routes[0].get('dst') != 'default' or routes[0].get('dev') != interface
                        or routes[0].get('type', 'unicast') != 'unicast'
                        or routes[0].get('gateway') != (record['transit']['peer' + str(version)] if table == 100 else None)
                        or 'nexthops' in routes[0]):
                    raise ValueError('carrier default route differs')
        values = {'net.ipv4.ip_forward': '1', 'net.ipv4.conf.all.rp_filter': '0',
                  'net.ipv4.conf.ppp0.rp_filter': '0', f'net.ipv4.conf.{transit_name}.rp_filter': '0',
                  f'net.ipv6.conf.{raw}.disable_ipv6': '1'}
        if record['spec']['mtu'] < 1280:
            values.update({'net.ipv6.conf.all.forwarding': '0', 'net.ipv6.conf.ppp0.disable_ipv6': '1',
                           f'net.ipv6.conf.{transit_name}.disable_ipv6': '1'})
        else:
            values.update({'net.ipv6.conf.all.forwarding': '1', 'net.ipv6.conf.ppp0.accept_ra': '2',
                           'net.ipv6.conf.ppp0.autoconf': '1',
                           'net.ipv6.conf.ppp0.accept_ra_defrtr': '1' if record.get('accept_default_route') else '0'})
        for key, expected in values.items():
            if self.inside(fd, [SYSCTL, '-n', key]).strip() != expected:
                raise ValueError('carrier forwarding sysctl differs')
        firewall = json.loads(self.inside(fd, [NFT, '-j', 'list', 'table', 'inet', 'ngfw_ppp']))
        if (record.get('policy_source') != hashlib.sha256(nft_policy(True, token).encode()).hexdigest()
                or record.get('firewall_digest') != self.nft_digest(firewall, token)):
            raise ValueError('carrier firewall changed')
        ppp_addresses = sorted({str(ipaddress.ip_interface(str(item['local']) + '/' + str(item['prefixlen'])))
                                for item in by_name.get('ppp0', {}).get('addr_info', [])
                                if not item.get('tentative') and not item.get('dadfailed')})
        return {'verified': True, 'token': token, 'generation': record['generation'], 'boot': record['boot'],
                'namespace': record['namespace'], 'links': live, 'mtu': ppp_mtu, 'transit': record['transit'],
                'ppp_addresses': ppp_addresses}

    def verify(self, token, generation):
        record = self.load(token, generation)
        if not record.get('configured'):
            raise ValueError('carrier has not been configured')
        with self.pinned(token, record) as fd:
            return self.verify_state(fd, token, record)

    def inspect(self, token, generation):
        record = self.load(token, generation)
        with self.pinned(token, record) as fd:
            result = dict(record)
            if record.get('bound'):
                raw, transit_name = link_names(token)
                actual = self.owned_links(fd)
                by_name = {item['ifname']: item for item in actual}
                if raw not in by_name or transit_name not in by_name:
                    # VPP losing TAP FDs is a recoverable namespace object,
                    # never an invitation to adopt replacement links or generation.
                    if set(by_name) - {'lo', 'ppp0', raw, transit_name}:
                        raise ValueError('foreign link prevents namespace repair')
                    remaining = {name: self.tap_identity(token, record, name, by_name[name])
                                 for name in (raw, transit_name) if name in by_name}
                    ppp = by_name.get('ppp0')
                    previous = record.get('links', {}).get('ppp0')
                    if ppp and (ppp.get('link_type') != 'ppp'
                                or previous is not None and ppp.get('ifindex') != previous):
                        raise ValueError('PPP identity changed before namespace repair')
                    result['repair_required'] = True
                    result['configured'] = False
                    result['live_links'] = {**remaining, **({'ppp0': ppp['ifindex']} if ppp else {})}
                else:
                    result['live_links'] = self.links(fd, token, record, record['configured'])
            result['addresses'] = json.loads(self.inside(fd, [IP, '-j', 'address', 'show']))
            result['rules4'] = json.loads(self.inside(fd, [IP, '-4', '-j', 'rule', 'show']))
            result['rules6'] = json.loads(self.inside(fd, [IP, '-6', '-j', 'rule', 'show']))
            result['firewall'] = json.loads(self.inside(fd, [NFT, '-j', 'list', 'table', 'inet', 'ngfw_ppp']))
            return result

    def inventory(self, owner):
        if not NAME.fullmatch(owner):
            raise ValueError('invalid owner')
        rows = []
        for filename in sorted(os.listdir(self.root)):
            if not filename.endswith('.json'):
                continue
            token = filename[:-5]
            record = self.load(token)
            if record['owner'] == owner:
                rows.append(self.inspect(token, record['generation']))
        return rows

    def withdraw(self, token, generation):
        record = self.load(token, generation)
        record['configured'] = False
        self.save(token, record)  # Withdraw before any fallible network operation.
        with self.pinned(token, record) as fd:
            self.inside(fd, [NFT, '-f', '-'], nft_policy(False, token))
        return record

    def configure(self, token, generation, accept_default_route=False):
        raw, transit_name = link_names(token)
        if type(accept_default_route) is not bool:
            raise ValueError('accept_default_route must be boolean')
        record = self.withdraw(token, generation)
        if record['spec']['mtu'] < 1280 and accept_default_route:
            raise ValueError('IPv6 policy is unsupported below MTU1280')
        record['accept_default_route'] = accept_default_route
        with self.pinned(token, record) as fd:
            before = self.links(fd, token, record, True)
            link_state = json.loads(self.inside(fd, [IP, '-j', 'address', 'show']))
            ppp_mtu = next((item.get('mtu', 0) for item in link_state if item['ifname'] == 'ppp0'), 0)
            if ppp_mtu != record['spec']['mtu']:
                raise ValueError('negotiated PPP MTU differs from immutable carrier MTU; explicit reconfiguration required')
            self.inside(fd, [IP, 'link', 'set', 'dev', transit_name, 'mtu', str(ppp_mtu)])
            tables = json.loads(self.inside(fd, [NFT, '-j', 'list', 'tables']))
            for item in tables.get('nftables', []):
                table = item.get('table')
                if table and (table['family'], table['name']) != ('inet', 'ngfw_ppp'):
                    raise ValueError('foreign firewall table in carrier namespace')
            self.inside(fd, [IP, 'link', 'set', 'lo', 'up'])
            self.inside(fd, [IP, 'link', 'set', transit_name, 'up'])
            for version in ((4,) if record['spec']['mtu'] < 1280 else (4, 6)):
                family = '-' + str(version)
                address = record['transit']['local' + str(version)]
                peer = record['transit']['peer' + str(version)]
                self.inside(fd, [IP, family, 'address', 'replace', address, 'dev', transit_name])
                self.inside(fd, [IP, family, 'route', 'replace', 'table', '100', 'default',
                                 'via', peer, 'dev', transit_name, 'onlink'])
                self.inside(fd, [IP, family, 'route', 'replace', 'table', '101', 'default', 'dev', 'ppp0'])
                # Only an exclusively owned netns is touched. Local priority 0 would
                # consume NAT replies addressed to the negotiated local PPP address.
                rules = json.loads(self.inside(fd, [IP, family, '-j', 'rule', 'show']))
                for rule in rules:
                    priority = rule.get('priority')
                    if priority not in (0, 4, 5, 6, 10, 20, 100, 32766, 32767):
                        raise ValueError('unexpected policy rule in owned namespace')
                for rule in rules:
                    self.inside(fd, [IP, family, 'rule', 'delete', 'pref', str(rule['priority'])])
                if family == '-4':
                    self.inside(fd, [IP, family, 'rule', 'add', 'pref', '6', 'iif', transit_name,
                                     'to', str(ipaddress.ip_interface(record['transit']['local4']).ip) + '/32', 'lookup', 'local'])
                if family == '-6':
                    self.inside(fd, [IP, family, 'rule', 'add', 'pref', '4', 'iif', 'ppp0',
                                     'ipproto', 'udp', 'sport', '547', 'dport', '546', 'lookup', 'local'])
                    for destination in ('fe80::/10', 'ff00::/8'):
                        self.inside(fd, [IP, family, 'rule', 'add', 'pref', '5', 'to', destination,
                                         'lookup', 'local'])
                    self.inside(fd, [IP, family, 'rule', 'add', 'pref', '6', 'iif', transit_name,
                                     'to', str(ipaddress.ip_interface(record['transit']['local6']).ip) + '/128', 'lookup', 'local'])
                self.inside(fd, [IP, family, 'rule', 'add', 'pref', '10', 'iif', 'ppp0', 'lookup', '100'])
                self.inside(fd, [IP, family, 'rule', 'add', 'pref', '20', 'iif', transit_name, 'lookup', '101'])
                self.inside(fd, [IP, family, 'rule', 'add', 'pref', '100', 'lookup', 'local'])
                self.inside(fd, [IP, family, 'rule', 'add', 'pref', '32766', 'lookup', 'main'])
            self.inside(fd, [SYSCTL, '-q', '-w', 'net.ipv4.ip_forward=1',
                             'net.ipv4.conf.all.rp_filter=0', 'net.ipv4.conf.default.rp_filter=0',
                             'net.ipv4.conf.ppp0.rp_filter=0', f'net.ipv4.conf.{transit_name}.rp_filter=0'])
            if record['spec']['mtu'] < 1280:
                self.inside(fd, [SYSCTL, '-q', '-w', 'net.ipv6.conf.all.forwarding=0',
                                 'net.ipv6.conf.ppp0.disable_ipv6=1', f'net.ipv6.conf.{transit_name}.disable_ipv6=1'])
            else:
                self.inside(fd, [SYSCTL, '-q', '-w', 'net.ipv6.conf.all.forwarding=1',
                                 'net.ipv6.conf.ppp0.accept_ra=2', 'net.ipv6.conf.ppp0.autoconf=1',
                                 'net.ipv6.conf.ppp0.accept_ra_defrtr=' + ('1' if accept_default_route else '0')])
            if self.links(fd, token, record, True) != before:
                raise ValueError('carrier links changed during configuration')
            self.inside(fd, [NFT, '-f', '-'], nft_policy(True, token))
            record['links'] = before
            # This records only completion of helper operations. Consumer must read
            # back kernel + VPP state and current PPP session before publishing ready.
            record['configured'] = True
            try:
                record['policy_source'] = hashlib.sha256(nft_policy(True, token).encode()).hexdigest()
                record['firewall_digest'] = self.nft_digest(json.loads(self.inside(fd, [NFT, '-j', 'list', 'table', 'inet', 'ngfw_ppp'])), token)
                self.verify_state(fd, token, record)
                self.save(token, record)
            except Exception:
                self.inside(fd, [NFT, '-f', '-'], nft_policy(False, token))
                raise
        return record

    @contextlib.contextmanager
    def broker_locked(self):
        if os.geteuid() != 0:
            raise PermissionError('root-owned broker requests required')
        for directory in (BROKER, BROKER + '/requests', BROKER + '/results'):
            os.makedirs(directory, mode=0o700, exist_ok=True)
            protected_directory(directory)
        fd = os.open(BROKER + '/lock', os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
        try:
            info = os.fstat(fd)
            if not stat.S_ISREG(info.st_mode) or info.st_uid != 0 or info.st_mode & 0o077:
                raise ValueError('unsafe broker lock')
            fcntl.flock(fd, fcntl.LOCK_EX)
            yield
        finally:
            os.close(fd)

    def broker_file(self, token, area):
        if not TOKEN.fullmatch(token) or area not in ('requests', 'results'):
            raise ValueError('invalid broker identity')
        return BROKER + '/' + area + '/' + token + '.json'

    def broker_read(self, token, area):
        fd = os.open(self.broker_file(token, area), os.O_RDONLY | os.O_NOFOLLOW)
        with os.fdopen(fd, 'r', encoding='ascii') as stream:
            info = os.fstat(stream.fileno())
            if (not stat.S_ISREG(info.st_mode) or info.st_uid != 0
                    or info.st_mode & 0o077 or info.st_size > (8192 if area == 'requests' else 1048576)):
                raise ValueError('unsafe broker document')
            return json.load(stream)

    def broker_write(self, token, area, document):
        path = self.broker_file(token, area)
        temporary = path + '.' + secrets.token_hex(8)
        fd = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
        try:
            with os.fdopen(fd, 'w', encoding='ascii') as stream:
                json.dump(document, stream, sort_keys=True)
                stream.write('\n')
                stream.flush()
                os.fsync(stream.fileno())
            os.replace(temporary, path)
        finally:
            if os.path.exists(temporary):
                os.unlink(temporary)

    def broker_queue(self, token, nonce, request):
        if not GEN.fullmatch(nonce) or request_token(request) != token:
            raise ValueError('broker request identity differs')
        try:
            pending = self.broker_read(token, 'requests')
        except FileNotFoundError:
            pending = None
        if pending and pending.get('boot') == boot_id() and pending.get('expires', 0) > time.monotonic():
            raise ValueError('broker request already pending')
        document = {'token': token, 'nonce': nonce, 'boot': boot_id(),
                    'expires': time.monotonic() + 10, 'request': request,
                    'request_sha256': request_digest(request)}
        self.broker_write(token, 'requests', document)
        try:
            os.unlink(self.broker_file(token, 'results'))
        except FileNotFoundError:
            pass
        return {key: value for key, value in document.items() if key != 'request'}

    def broker_result(self, token, nonce):
        if not GEN.fullmatch(nonce):
            raise ValueError('invalid broker nonce')
        result = self.broker_read(token, 'results')
        if (result.get('nonce') != nonce or result.get('token') != token
                or result.get('boot') != boot_id() or result.get('expires', 0) <= time.monotonic()):
            raise ValueError('stale or mismatched broker result')
        return result

    def broker_execute(self, token):
        document = self.broker_read(token, 'requests')
        request = document.get('request')
        if (document.get('token') != token or not GEN.fullmatch(document.get('nonce', ''))
                or document.get('boot') != boot_id() or document.get('expires', 0) <= time.monotonic()
                or request_token(request) != token or request_digest(request) != document.get('request_sha256')):
            raise ValueError('stale or mismatched broker request')
        result = {key: value for key, value in document.items() if key != 'request'}
        try:
            with self.locked():
                if time.monotonic() >= document['expires']:
                    raise TimeoutError('broker request expired while awaiting carrier lock')
                op = request['op']
                if op == 'provision':
                    value = self.provision(request['owner'], request['logical'], request['spec'])
                elif op == 'list':
                    value = self.inventory(request['owner'])
                elif op == 'prepare':
                    value = self.prepare(token, request['generation'], None, None,
                                         request['physical_mac'], request['transit'])
                elif op == 'configure':
                    value = self.configure(token, request['generation'], request['accept_default_route'])
                elif op == 'probe':
                    value = self.probe(token, request['generation'], request['kind'], request['target'])
                else:
                    # request_token already restricts this exact finite method set.
                    value = getattr(self, op)(token, request['generation'])
                if time.monotonic() >= document['expires']:
                    raise TimeoutError('broker request expired during operation')
                result.update(ok=True, result=value)
        except Exception as error:
            result.update(ok=False, error='carrier operation failed: ' + type(error).__name__)
        self.broker_write(token, 'results', result)
        os.unlink(self.broker_file(token, 'requests'))
        return result

    def leaf_argv(self, token, generation, action, extra=()):
        # UID0 cannot regain removed bounding capabilities at the subsequent exec.
        return [SETPRIV, '--bounding-set=-all,+net_admin,+net_raw', '--inh-caps=-all',
                '--ambient-caps=-all', '--no-new-privs', '--', '/usr/bin/python3', '-I',
                HELPER, action, token, generation, *extra]

    def check_leaf(self, token, generation, status=None, namespace=None):
        record = self.load(token, generation)
        if os.geteuid() != 0:
            raise ValueError('leaf requires restricted root identity')
        if status is None:
            with open('/proc/self/status', encoding='ascii') as stream:
                status = dict(line.split(':', 1) for line in stream if ':' in line)
        for name in ('CapEff', 'CapPrm', 'CapBnd'):
            if int(status.get(name, '-1').strip(), 16) != PPP_CAPS:
                raise ValueError('leaf capabilities are not restricted')
        if (int(status.get('CapInh', '-1').strip(), 16) != 0
                or int(status.get('CapAmb', '-1').strip(), 16) != 0
                or status.get('NoNewPrivs', '').strip() != '1'):
            raise ValueError('leaf can regain privileges')
        if namespace is None:
            info = os.stat('/proc/self/ns/net')
            namespace = [info.st_dev, info.st_ino]
        if namespace != record['namespace']:
            raise ValueError('leaf entered a different namespace')
        return record

    def exec_leaf(self, token, generation, action, kind=None, target=None):
        self.check_leaf(token, generation)
        if action == 'ppp-exec':
            executable = '/usr/sbin/pppd'
            argv = [executable, 'call', 'carrier', 'nodetach', 'unit', '0']
        elif action == 'probe-exec':
            validate_probe(kind, target)
            executable = open_verified_probe()
            argv = [WAN_PROBE, '--kind', kind, '--target', target, '--timeout-ms', '3000']
        else:
            raise ValueError('unknown fixed leaf')
        os.execve(executable, argv, {'PATH': '/usr/sbin:/usr/bin:/sbin:/bin', 'LC_ALL': 'C'})

    def launch(self, token):
        record = self.load(token)
        if record.get('configured'):
            raise ValueError('broker must withdraw carrier before PPP start')
        if not record.get('bound'):
            raise ValueError('prepare carrier links before starting PPP')
        with self.pinned(token, record) as fd:
            self.links(fd, token, record, False)
            os.set_inheritable(fd, True)
            os.execve(NSENTER, [NSENTER, '--net=/proc/self/fd/' + str(fd), '--',
                               *self.leaf_argv(token, record['generation'], 'ppp-exec')],
                      {'PATH': '/usr/sbin:/usr/bin:/sbin:/bin', 'LC_ALL': 'C'})

    def probe(self, token, generation, kind, target):
        address = validate_probe(kind, target)
        record = self.load(token, generation)
        if not record.get('configured'):
            raise ValueError('carrier not configured')
        family = '-' + str(address.version)
        with self.pinned(token, record) as fd:
            self.verify_state(fd, token, record)
            addresses = json.loads(self.inside(fd, [IP, '-j', 'address', 'show']))
            local = [entry['local'] for link in addresses if link['ifname'] == 'ppp0'
                     for entry in link.get('addr_info', [])
                     if ipaddress.ip_address(entry['local']).version == address.version
                     and not ipaddress.ip_address(entry['local']).is_link_local]
            if not local:
                raise ValueError('PPP has no local address for requested probe family')
            policy = probe_policy(token, kind, str(address), local)
            dirty = False
            try:
                dirty = True  # Every possibly partial mutation has a cleanup path.
                self.inside(fd, [NFT, '-f', '-'], policy)
                self.inside(fd, [IP, family, 'rule', 'add', 'pref', '2', 'oif', 'ppp0', 'lookup', '101'])
                self.inside(fd, [IP, family, 'rule', 'add', 'pref', '3', 'fwmark', '0x4e47', 'lookup', 'local'])
                output = self.inside(fd, self.leaf_argv(token, generation, 'probe-exec',
                                                     ('--kind', kind, '--target', str(address))), timeout=4)
                result = json.loads(output)
                if (not isinstance(result, dict) or set(result) != {'sent', 'received', 'latencyMs', 'unavailable'}
                        or type(result['sent']) is not int or result['sent'] != 1
                        or type(result['received']) is not int or result['received'] not in (0, 1)
                        or type(result['latencyMs']) is not int or not 0 <= result['latencyMs'] <= 3000
                        or type(result['unavailable']) is not bool):
                    raise ValueError('invalid fixed probe result')
                return result
            finally:
                if dirty:
                    try:
                        # Restore normal policy first: all probe exceptions disappear
                        # atomically even if subsequent RPDB cleanup fails.
                        self.inside(fd, [NFT, '-f', '-'], nft_policy(True, token))
                        rules = json.loads(self.inside(fd, [IP, family, '-j', 'rule', 'show']))
                        for rule in rules:
                            if rule.get('priority') in (2, 3):
                                self.inside(fd, [IP, family, 'rule', 'delete', 'pref', str(rule['priority'])])
                        self.verify_state(fd, token, record)
                    except Exception:
                        record['configured'] = False
                        try:
                            self.save(token, record)
                        finally:
                            self.inside(fd, [NFT, '-f', '-'], nft_policy(False, token))
                        raise

    def delete(self, token, generation):
        record = self.withdraw(token, generation)
        with self.pinned(token, record) as fd:
            links = self.owned_links(fd)
            if any(link['ifname'] != 'lo' for link in links):
                raise ValueError('remove owned VPP TAPs and stop PPP before namespace deletion')
        self.run([IP, 'netns', 'delete', token])
        os.unlink(self.path(token))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=('provision', 'prepare', 'inspect', 'inventory', 'list', 'verify', 'configure', 'withdraw', 'delete', 'run', 'ppp-exec', 'probe', 'probe-exec', 'broker-queue', 'broker-result', 'broker-execute'))
    parser.add_argument('identity', help='owner for provision; namespace token otherwise')
    parser.add_argument('generation', nargs='?', help='logical interface for provision; generation otherwise')
    parser.add_argument('--raw-index', type=int)
    parser.add_argument('--transit-index', type=int)
    parser.add_argument('--physical-mac')
    parser.add_argument('--spec-json')
    parser.add_argument('--request-json')
    parser.add_argument('--accept-default-route', choices=('yes', 'no'), default='no')
    parser.add_argument('--kind')
    parser.add_argument('--target')
    for field in ('local4', 'peer4', 'local6', 'peer6'):
        parser.add_argument('--' + field)
    args = parser.parse_args()
    if args.action not in ('inventory', 'list', 'run', 'broker-execute') and args.generation is None:
        parser.error('generation or logical interface required')
    if args.action == 'provision' and args.spec_json is None:
        parser.error('provision requires immutable --spec-json')
    if args.action == 'prepare' and None in (args.physical_mac, args.local4, args.peer4, args.local6, args.peer6):
        parser.error('prepare requires physical MAC and allocated IPv4/IPv6 transit endpoints')
    if args.action in ('probe', 'probe-exec') and None in (args.kind, args.target):
        parser.error('probe requires --kind and literal --target')
    carrier = Carrier()
    if args.action.startswith('broker-'):
        with carrier.broker_locked():
            if args.action == 'broker-queue':
                if args.request_json is None:
                    parser.error('broker-queue requires --request-json')
                result = carrier.broker_queue(args.identity, args.generation, json.loads(args.request_json))
            elif args.action == 'broker-result':
                result = carrier.broker_result(args.identity, args.generation)
            else:
                result = carrier.broker_execute(args.identity)
        print(json.dumps(result, sort_keys=True))
        return
    if args.action == 'run':
        with carrier.readonly_locked():
            carrier.launch(args.identity)
        return
    if args.action in ('ppp-exec', 'probe-exec'):
        carrier.exec_leaf(args.identity, args.generation, args.action, args.kind, args.target)
        return
    with carrier.locked():
        if args.action == 'configure':
            record = carrier.configure(args.identity, args.generation, args.accept_default_route == 'yes')
        elif args.action == 'probe':
            record = carrier.probe(args.identity, args.generation, args.kind, args.target)
        elif args.action == 'provision':
            record = carrier.provision(args.identity, args.generation, json.loads(args.spec_json))
        elif args.action == 'prepare':
            record = carrier.prepare(args.identity, args.generation, args.raw_index, args.transit_index, args.physical_mac,
                                     {field: getattr(args, field) for field in ('local4', 'peer4', 'local6', 'peer6')})
        elif args.action in ('inventory', 'list'):
            record = carrier.inventory(args.identity)
        else:
            record = getattr(carrier, args.action)(args.identity, args.generation)
        print(json.dumps(record, sort_keys=True))


if __name__ == '__main__':
    main()
