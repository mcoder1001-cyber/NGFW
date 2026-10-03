#!/usr/bin/env python3
"""Private-VPP-only lifecycle regression for patch 0003 (never shared VPP)."""
import os
import subprocess
import time


def cli(command):
    result = subprocess.run(['vppctl', '-s', '/run/vpp/cli.sock', *command.split()],
                            check=True, text=True, capture_output=True)
    if result.stderr or any(word in result.stdout.lower() for word in ('unknown input', 'failed', 'error:')):
        raise AssertionError((command, result.stdout, result.stderr))
    return result.stdout


def accept(proto, interface, expected):
    command = f'show {proto} mfib'
    deadline = time.monotonic() + 3
    while True:
        text = cli(command)
        # Restrict assertions to the linux-cp special entry, not core /32 entries.
        prefix = '(*, 224.0.0.0/24)' if proto == 'ip' else '(*, ff00::/8)'
        start = text.find(prefix)
        section = text[start:] if start >= 0 else ''
        end = section.find('\n(*,', 1)
        if end >= 0:
            section = section[:end]
        lines = section.splitlines()
        found = any(interface in line and 'Accept' in line for line in lines)
        if found == expected:
            print(f'{proto} {interface} Accept={expected}', flush=True)
            return
        if time.monotonic() >= deadline:
            raise AssertionError((proto, interface, expected, text))
        time.sleep(.05)


def main():
    if os.environ.get('VRX_DISPOSABLE_VPP') != '1':
        raise SystemExit('requires isolated-vpp.py; never run against shared VPP')
    if os.readlink('/proc/self/ns/net') == os.readlink('/proc/1/ns/net'):
        raise SystemExit('requires a private network namespace')
    # Keep Linux from acquiring extra LL references: this fixture exercises
    # direct VPP address and pair callbacks, independently of netlink ordering.
    subprocess.run(['sysctl', '-q', '-w', 'net.ipv6.conf.default.disable_ipv6=1'], check=True)
    cli('lcp lcp-sync off')
    for idx in range(2):
        cli('create loopback interface')
        cli(f'set interface state loop{idx} up')
        cli(f'set interface ip address loop{idx} 198.18.{idx}.1/24')
        cli(f'lcp create loop{idx} host-if lcp-mc{idx}')
    accept('ip', 'loop0', True)
    accept('ip', 'loop1', True)
    cli('lcp delete loop0')
    accept('ip', 'loop0', False)
    accept('ip', 'loop1', True)
    cli('lcp create loop0 host-if lcp-mc0')
    accept('ip', 'loop0', True)
    cli('set interface ip address loop0 198.19.0.1/24')
    cli('set interface ip address del loop0 198.18.0.1/24')
    accept('ip', 'loop0', True)
    cli('set interface ip address del loop0 198.19.0.1/24')
    accept('ip', 'loop0', False)
    accept('ip', 'loop1', True)
    cli('set interface ip address loop0 198.18.0.1/24')
    accept('ip', 'loop0', True)
    cli('enable ip6 interface loop0')
    accept('ip6', 'loop0', True)
    assert 'linux-cp multicast input' in cli('show ip6 interface loop0')
    cli('lcp delete loop0')
    accept('ip6', 'loop0', False)
    cli('lcp create loop0 host-if lcp-mc0')
    accept('ip6', 'loop0', True)
    assert 'linux-cp multicast input' in cli('show ip6 interface loop0')
    cli('set interface ip address loop0 2001:db8:1::1/64')
    cli('set interface ip address del loop0 2001:db8:1::1/64')
    accept('ip6', 'loop0', True)
    assert 'linux-cp multicast input' in cli('show ip6 interface loop0')
    # Exercise the legacy Linux netlink deletion branch with a retained VPP
    # link-local reference, not only the direct VPP address callback.
    subprocess.run(['ip', 'link', 'set', 'dev', 'lcp-mc0', 'addrgenmode', 'none'], check=True)
    subprocess.run(['sysctl', '-q', '-w', 'net.ipv6.conf.lcp-mc0.disable_ipv6=0'], check=True)
    subprocess.run(['ip', '-6', 'addr', 'add', '2001:db8:2::1/64', 'dev', 'lcp-mc0'], check=True)
    deadline = time.monotonic() + 5
    while '2001:db8:2::1/64' not in cli('show interface address loop0'):
        if time.monotonic() >= deadline:
            raise AssertionError('Linux global address did not reach VPP')
        time.sleep(.05)
    subprocess.run(['ip', '-6', 'addr', 'del', '2001:db8:2::1/64', 'dev', 'lcp-mc0'], check=True)
    deadline = time.monotonic() + 5
    while '2001:db8:2::1/64' in cli('show interface address loop0'):
        if time.monotonic() >= deadline:
            raise AssertionError('Linux global address deletion did not reach VPP')
        time.sleep(.05)
    accept('ip6', 'loop0', True)
    cli('disable ip6 interface loop0')
    accept('ip6', 'loop0', False)
    cli('lcp delete loop0')
    accept('ip', 'loop0', False)
    accept('ip', 'loop1', True)
    cli('lcp delete loop1')
    accept('ip', 'loop1', False)
    print('PASS: pair recreation, IPv4 multiple/last addresses, IPv6 LL and final disable', flush=True)


if __name__ == '__main__':
    main()
