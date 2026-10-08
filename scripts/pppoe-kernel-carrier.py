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

IP = '/usr/sbin/ip'
NFT = '/usr/sbin/nft'
NSENTER = '/usr/bin/nsenter'
SYSCTL = '/usr/sbin/sysctl'
ROOT = '/run/ngfw-pppoe-carrier'
NETNS = '/run/netns'
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
            or type(spec['mtu']) is not int or not 1280 <= spec['mtu'] <= 1492):
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


def run(argv, *, data=None, pass_fds=()):
    return subprocess.run(argv, input=data, text=True, check=True,
                          stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                          timeout=20, pass_fds=pass_fds,
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


def nft_policy(enabled):
    # Atomic table replacement: the initial add makes this also work on first use.
    # No NAT table, flow offload, raw-WAN IP or bypass-LAN forwarding is permitted.
    forward = ''
    if enabled:
        forward = ('iifname "ppptransit" oifname "ppp0" accept\n'
                   'iifname "ppp0" oifname "ppptransit" accept\n')
    return '''add table inet ngfw_ppp
flush table inet ngfw_ppp
add chain inet ngfw_ppp input { type filter hook input priority 0; policy drop; }
add chain inet ngfw_ppp output { type filter hook output priority 0; policy drop; }
add chain inet ngfw_ppp forward { type filter hook forward priority 0; policy drop; }
add rule inet ngfw_ppp input iifname "lo" accept
add rule inet ngfw_ppp output oifname "lo" accept
add rule inet ngfw_ppp input iifname "ppp0" ip6 saddr fe80::/10 meta l4proto ipv6-icmp icmpv6 type { nd-router-advert, nd-neighbor-solicit, nd-neighbor-advert } ip6 hoplimit 255 accept
add rule inet ngfw_ppp input iifname "ppptransit" meta l4proto ipv6-icmp icmpv6 type { nd-neighbor-solicit, nd-neighbor-advert } ip6 hoplimit 255 accept
add rule inet ngfw_ppp input iifname "ppp0" ip6 saddr fe80::/10 udp sport 547 udp dport 546 accept
add rule inet ngfw_ppp output oifname { "ppp0", "ppptransit" } meta l4proto ipv6-icmp icmpv6 type { nd-router-solicit, nd-neighbor-solicit, nd-neighbor-advert } ip6 hoplimit 255 accept
add rule inet ngfw_ppp output oifname { "ppp0", "ppptransit" } meta l4proto ipv6-icmp icmpv6 type { destination-unreachable, packet-too-big, time-exceeded, parameter-problem } accept
add rule inet ngfw_ppp output oifname { "ppp0", "ppptransit" } ip protocol icmp icmp type { destination-unreachable, time-exceeded, parameter-problem } accept
add rule inet ngfw_ppp output oifname "ppp0" udp sport 546 udp dport 547 accept
''' + ''.join('add rule inet ngfw_ppp forward ' + line + '\n'
              for line in forward.splitlines())


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

    def inside(self, fd, argv, data=None):
        return self.run([NSENTER, '--net=/proc/self/fd/' + str(fd), '--'] + argv,
                        data=data, pass_fds=(fd,))

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
                    self.inside(fd, [NFT, '-f', '-'], nft_policy(False))
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
            self.inside(fd, [NFT, '-f', '-'], nft_policy(False))
        return record

    def links(self, fd, token, record, require_ppp):
        links = json.loads(self.inside(fd, [IP, '-j', '-d', 'link', 'show']))
        by_name = {link['ifname']: link for link in links}
        allowed = {'lo', 'pppwan', 'ppptransit', 'ppp0'}
        if set(by_name) - allowed:
            raise ValueError('foreign interface in carrier namespace')
        expected = {}
        for name in ('pppwan', 'ppptransit'):
            link = by_name.get(name, {})
            if (link.get('ifalias') != token + ':' + record['generation'] + ':' + name
                    or link.get('linkinfo', {}).get('info_kind') != 'tun'
                    or link.get('linkinfo', {}).get('info_data', {}).get('type') != 'tap'):
                raise ValueError('unverified carrier TAP')
            expected[name] = link['ifindex']
            if record.get('bound', {}).get(name) != link['ifindex']:
                raise ValueError('carrier TAP binding changed')
            if name == 'pppwan' and link.get('address') != record.get('physical_mac'):
                raise ValueError('carrier raw MAC changed')
        if require_ppp:
            ppp = by_name.get('ppp0', {})
            if ppp.get('link_type') != 'ppp' or 'UP' not in ppp.get('flags', []):
                raise ValueError('PPP link is not up')
            expected['ppp0'] = ppp['ifindex']
        return expected

    def prepare(self, token, generation, raw_index, transit_index, physical_mac, transit):
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
            links = json.loads(self.inside(fd, [IP, '-j', '-d', 'link', 'show']))
            by_name = {link['ifname']: link for link in links}
            if set(by_name) - {'lo', 'pppwan', 'ppptransit'}:
                raise ValueError('stop PPP and remove foreign links before preparation')
            if raw_index is None:
                raw_index = by_name.get('pppwan', {}).get('ifindex')
                transit_index = by_name.get('ppptransit', {}).get('ifindex')
            if not raw_index or not transit_index or raw_index == transit_index:
                raise ValueError('missing TAP indices')
            bound = {'pppwan': raw_index, 'ppptransit': transit_index}
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
            self.inside(fd, [IP, 'link', 'set', 'dev', 'pppwan', 'address', physical_mac])
            # IPv6 autoconfiguration on raw Ethernet must never create a plain-IP bypass.
            self.inside(fd, [SYSCTL, '-q', '-w', 'net.ipv6.conf.pppwan.disable_ipv6=1'])
            self.inside(fd, [IP, '-4', 'address', 'flush', 'dev', 'pppwan'])
            self.inside(fd, [IP, 'link', 'set', 'dev', 'pppwan', 'up'])
            self.inside(fd, [IP, 'link', 'set', 'dev', 'ppptransit', 'up'])
            self.inside(fd, [IP, 'link', 'set', 'dev', 'lo', 'up'])
            record['bound'] = bound
            record['physical_mac'] = physical_mac
            record['transit'] = normalized
            self.links(fd, token, record, False)
            self.save(token, record)
        return record

    def inspect(self, token, generation):
        record = self.load(token, generation)
        with self.pinned(token, record) as fd:
            result = dict(record)
            if record.get('bound'):
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
            self.inside(fd, [NFT, '-f', '-'], nft_policy(False))
        return record

    def configure(self, token, generation):
        record = self.withdraw(token, generation)
        with self.pinned(token, record) as fd:
            before = self.links(fd, token, record, True)
            tables = json.loads(self.inside(fd, [NFT, '-j', 'list', 'tables']))
            for item in tables.get('nftables', []):
                table = item.get('table')
                if table and (table['family'], table['name']) != ('inet', 'ngfw_ppp'):
                    raise ValueError('foreign firewall table in carrier namespace')
            self.inside(fd, [IP, 'link', 'set', 'lo', 'up'])
            self.inside(fd, [IP, 'link', 'set', 'ppptransit', 'up'])
            for version in (4, 6):
                family = '-' + str(version)
                address = record['transit']['local' + str(version)]
                peer = record['transit']['peer' + str(version)]
                self.inside(fd, [IP, family, 'address', 'replace', address, 'dev', 'ppptransit'])
                self.inside(fd, [IP, family, 'route', 'replace', 'table', '100', 'default',
                                 'via', peer, 'dev', 'ppptransit', 'onlink'])
                self.inside(fd, [IP, family, 'route', 'replace', 'table', '101', 'default', 'dev', 'ppp0'])
                # Only an exclusively owned netns is touched. Local priority 0 would
                # consume NAT replies addressed to the negotiated local PPP address.
                rules = json.loads(self.inside(fd, [IP, family, '-j', 'rule', 'show']))
                for rule in rules:
                    priority = rule.get('priority')
                    if priority not in (0, 5, 10, 20, 100, 32766, 32767):
                        raise ValueError('unexpected policy rule in owned namespace')
                for rule in rules:
                    self.inside(fd, [IP, family, 'rule', 'delete', 'pref', str(rule['priority'])])
                if family == '-6':
                    for destination in ('fe80::/10', 'ff00::/8'):
                        self.inside(fd, [IP, family, 'rule', 'add', 'pref', '5', 'to', destination,
                                         'lookup', 'local'])
                self.inside(fd, [IP, family, 'rule', 'add', 'pref', '10', 'iif', 'ppp0', 'lookup', '100'])
                self.inside(fd, [IP, family, 'rule', 'add', 'pref', '20', 'iif', 'ppptransit', 'lookup', '101'])
                self.inside(fd, [IP, family, 'rule', 'add', 'pref', '100', 'lookup', 'local'])
                self.inside(fd, [IP, family, 'rule', 'add', 'pref', '32766', 'lookup', 'main'])
            self.inside(fd, [SYSCTL, '-q', '-w', 'net.ipv4.ip_forward=1',
                             'net.ipv4.conf.all.rp_filter=0', 'net.ipv4.conf.default.rp_filter=0',
                             'net.ipv4.conf.ppp0.rp_filter=0', 'net.ipv4.conf.ppptransit.rp_filter=0',
                             'net.ipv6.conf.all.forwarding=1', 'net.ipv6.conf.ppp0.accept_ra=2'])
            if self.links(fd, token, record, True) != before:
                raise ValueError('carrier links changed during configuration')
            self.inside(fd, [NFT, '-f', '-'], nft_policy(True))
            record['links'] = before
            # This records only completion of helper operations. Consumer must read
            # back kernel + VPP state and current PPP session before publishing ready.
            record['configured'] = True
            try:
                self.save(token, record)
            except Exception:
                self.inside(fd, [NFT, '-f', '-'], nft_policy(False))
                raise
        return record

    def launch(self, token):
        record = self.load(token)
        if not record.get('bound'):
            raise ValueError('prepare carrier links before starting PPP')
        with self.pinned(token, record) as fd:
            self.links(fd, token, record, False)
            # The packaged unit supplies the private /etc/ppp bind mount. The
            # helper accepts neither an arbitrary executable nor peer pathname.
            os.set_inheritable(fd, True)
            os.execve(NSENTER, [NSENTER, '--net=/proc/self/fd/' + str(fd), '--',
                               '/usr/sbin/pppd', 'call', 'carrier', 'nodetach', 'unit', '0'],
                      {'PATH': '/usr/sbin:/usr/bin:/sbin:/bin', 'LC_ALL': 'C'})

    def delete(self, token, generation):
        record = self.withdraw(token, generation)
        with self.pinned(token, record) as fd:
            links = json.loads(self.inside(fd, [IP, '-j', 'link', 'show']))
            if any(link['ifname'] != 'lo' for link in links):
                raise ValueError('remove owned VPP TAPs and stop PPP before namespace deletion')
        self.run([IP, 'netns', 'delete', token])
        os.unlink(self.path(token))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=('provision', 'prepare', 'inspect', 'inventory', 'list', 'configure', 'withdraw', 'delete', 'run'))
    parser.add_argument('identity', help='owner for provision; namespace token otherwise')
    parser.add_argument('generation', nargs='?', help='logical interface for provision; generation otherwise')
    parser.add_argument('--raw-index', type=int)
    parser.add_argument('--transit-index', type=int)
    parser.add_argument('--physical-mac')
    parser.add_argument('--spec-json')
    for field in ('local4', 'peer4', 'local6', 'peer6'):
        parser.add_argument('--' + field)
    args = parser.parse_args()
    if args.action not in ('inventory', 'list', 'run') and args.generation is None:
        parser.error('generation or logical interface required')
    if args.action == 'provision' and args.spec_json is None:
        parser.error('provision requires immutable --spec-json')
    if args.action == 'prepare' and None in (args.physical_mac, args.local4, args.peer4, args.local6, args.peer6):
        parser.error('prepare requires physical MAC and allocated IPv4/IPv6 transit endpoints')
    carrier = Carrier()
    with carrier.locked():
        if args.action == 'run':
            record = carrier.launch(args.identity)
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
