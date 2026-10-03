#!/usr/bin/env python3
"""Dedicated VPP forwarding smoke over the discovered ens193/ens256 VMware link.

Requires root. Leaves the shared VPP and management NIC untouched. Uses af_packet
by default; --dpdk temporarily binds only ens193 to VFIO in no-IOMMU mode. Run only with these data NICs down and without addresses.
"""
import fcntl
import os
from pathlib import Path
import subprocess
import sys
import time

ROOT = Path(__file__).resolve().parents[3]
EVIDENCE = ROOT / 'docs/status/tasks/hardware-2026-10-03-evidence'
RUNTIME = ROOT / '.scratch/hardware-smoke'
PEER_NS = 'hw32-peer'
LAN_NS = 'hw32-lan'
PEER = 'ens256'
DATA = 'ens193'


def main():
    dpdk = '--dpdk' in sys.argv[1:]
    global RUNTIME
    if dpdk:
        RUNTIME = ROOT / '.scratch/hardware-smoke-dpdk'
    EVIDENCE.mkdir(parents=True, exist_ok=True)
    RUNTIME.mkdir(parents=True, exist_ok=True)
    previous = EVIDENCE / ('dpdk-forwarding.txt' if dpdk else 'nic-vpp-forwarding.txt')
    if previous.exists():
        previous.rename(EVIDENCE / f'nic-vpp-forwarding-{time.time_ns()}.txt')
    log = previous.open('w')
    def run(args, expected=0):
        log.write('$ ' + ' '.join(map(str, args)) + '\n')
        log.flush()
        result = subprocess.run(args, capture_output=True, text=True, timeout=25)
        log.write(result.stdout + result.stderr + f'\nexit={result.returncode}\n')
        log.flush()
        if expected is not None and result.returncode != expected:
            raise RuntimeError(f'{args}: exit {result.returncode}')
        return result.stdout

    def cli(*args):
        text = run(['timeout', '10', 'vppctl', '-s', str(RUNTIME / 'cli.sock'), *args])
        if any(word in text.lower() for word in ['unknown input', 'failed', 'error:']):
            raise RuntimeError(text)
        return text

    lock = open('/run/lock/ngfw-hardware-smoke.lock', 'a')
    fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
    for name in [DATA, PEER]:
        assert Path('/sys/class/net/' + name + '/operstate').read_text().strip() == 'down'
        assert not run(['ip', '-o', 'addr', 'show', 'dev', name]).strip()
    assert PEER_NS not in run(['ip', 'netns', 'list'])
    assert LAN_NS not in run(['ip', 'netns', 'list'])
    before = run(['systemctl', 'show', 'vpp', '-p', 'MainPID', '-p', 'NRestarts'])
    ipv6 = Path('/proc/sys/net/ipv6/conf/' + DATA + '/disable_ipv6').read_text().strip()
    data_mac = Path('/sys/class/net/' + DATA + '/address').read_text().strip()
    pci = '0000:0c:00.0'
    device = Path('/sys/bus/pci/devices') / pci
    vfio_parameter = Path('/sys/module/vfio/parameters/enable_unsafe_noiommu_mode')
    vfio_previous = None
    features = run(['ethtool', '-k', PEER])
    tx_on = 'tx-checksumming: on' in features
    vpp = None
    server = None
    peer_created = lan_created = False
    try:
        if dpdk:
            run(['modprobe', 'vfio-pci'])
            vfio_previous = vfio_parameter.read_text().strip()
            if not (device / 'iommu_group').exists():
                log.write('VFIO no-IOMMU mode required in this VMware guest; restoring parameter after the run\n')
                vfio_parameter.write_text('Y')
        run(['ip', 'netns', 'add', PEER_NS]); peer_created = True
        run(['ip', 'netns', 'add', LAN_NS]); lan_created = True
        run(['ip', 'link', 'set', PEER, 'netns', PEER_NS])
        run(['ip', '-n', PEER_NS, 'link', 'set', 'lo', 'up'])
        run(['ip', 'netns', 'exec', PEER_NS, 'sysctl', '-qw', f'net.ipv6.conf.{PEER}.disable_ipv6=1'])
        run(['ip', 'netns', 'exec', PEER_NS, 'ethtool', '-K', PEER, 'tx', 'off'])
        run(['ip', '-n', PEER_NS, 'addr', 'add', '198.18.32.2/30', 'dev', PEER])
        run(['ip', '-n', PEER_NS, 'link', 'set', PEER, 'up'])
        run(['ip', '-n', PEER_NS, 'route', 'add', '198.18.33.0/30', 'via', '198.18.32.1'])
        run(['ip', 'link', 'add', 'hw32l0', 'type', 'veth', 'peer', 'name', 'hw32l1'])
        run(['ip', 'link', 'set', 'hw32l1', 'netns', LAN_NS])
        run(['ip', '-n', LAN_NS, 'link', 'set', 'lo', 'up'])
        run(['ip', '-n', LAN_NS, 'addr', 'add', '198.18.33.2/30', 'dev', 'hw32l1'])
        run(['ip', '-n', LAN_NS, 'link', 'set', 'hw32l1', 'up'])
        run(['ip', 'netns', 'exec', LAN_NS, 'ethtool', '-K', 'hw32l1', 'tx', 'off'])
        run(['ip', '-n', LAN_NS, 'route', 'add', '198.18.32.0/30', 'via', '198.18.33.1'])
        for name in [DATA, 'hw32l0']:
            run(['sysctl', '-qw', f'net.ipv6.conf.{name}.disable_ipv6=1'])
            run(['ip', 'link', 'set', name, 'down' if dpdk and name == DATA else 'up'])
        conf = RUNTIME / 'startup.conf'
        dpdk_config = f'''cpu {{ main-core 7 corelist-workers 8-9 }}
dpdk {{ uio-driver vfio-pci dev {pci} {{ name hw32-dpdk num-rx-queues 2 num-tx-queues 2 }} }}
''' if dpdk else ''
        dpdk_plugin = 'plugin dpdk_plugin.so { enable }' if dpdk else ''
        conf.write_text(f'''unix {{ nodaemon runtime-dir {RUNTIME} cli-listen {RUNTIME}/cli.sock log {RUNTIME}/vpp.log }}
api-segment {{ prefix hw32nic }}
socksvr {{ socket-name {RUNTIME}/api.sock }}
statseg {{ socket-name {RUNTIME}/stats.sock }}
memory {{ main-heap-size 256M }}
{dpdk_config}
plugins {{ plugin default {{ disable }} plugin af_packet_plugin.so {{ enable }} {dpdk_plugin} }}
''')
        vpp_output = (RUNTIME / 'process.log').open('w')
        for socket_name in ['cli.sock', 'api.sock', 'stats.sock']:
            (RUNTIME / socket_name).unlink(missing_ok=True)
        vpp = subprocess.Popen(['vpp', '-c', str(conf)], stdout=vpp_output, stderr=subprocess.STDOUT)
        for _ in range(100):
            if vpp.poll() is not None:
                raise RuntimeError((RUNTIME / 'process.log').read_text())
            if (RUNTIME / 'cli.sock').exists(): break
            time.sleep(.1)
        else: raise RuntimeError('dedicated VPP CLI did not start')
        for name, address in [(DATA, '198.18.32.1/30'), ('hw32l0', '198.18.33.1/30')]:
            # 256 TX slots accommodate a 64 KiB TCP burst; the old 16-slot lab
            # ring overflowed during the NIC transfer despite successful echo.
            vif = 'hw32-dpdk' if dpdk and name == DATA else 'host-' + name
            if not (dpdk and name == DATA):
                cli('create', 'host-interface', 'name', name, 'tx-size', '67584', 'tx-per-block', '256', 'rx-size', '2048', 'rx-per-block', '8')
            # VMware rejects forged source MACs unless explicitly allowed. Use
            # the NIC's assigned MAC rather than af_packet's generated address.
            mac = data_mac if name == DATA else Path('/sys/class/net/' + name + '/address').read_text().strip()
            cli('set', 'interface', 'mac', 'address', vif, mac)
            for family in ['ip', 'ip6']:
                cli('set', family, 'classify', 'intfc', vif, 'table-index', '-1')
            cli('set', 'interface', 'state', vif, 'up')
            cli('set', 'interface', 'mtu', 'packet', '1500', vif)
            cli('set', 'interface', 'ip', 'address', vif, address)
            cli('set', 'ip', 'classify', 'intfc', vif, 'table-index', '-1')
        for ns, target in [(LAN_NS, '198.18.32.2'), (PEER_NS, '198.18.33.2')]:
            run(['ip', 'netns', 'exec', ns, 'ping', '-n', '-c', '3', '-W', '1', target])
            result = run(['ip', 'netns', 'exec', ns, 'ping', '-n', '-c', '100', '-i', '.02', '-W', '1', target])
            assert '100 received' in result and '0% packet loss' in result
            result = run(['ip', 'netns', 'exec', ns, 'ping', '-n', '-c', '10', '-i', '.05', '-W', '1', '-M', 'do', '-s', '1472', target])
            assert '10 received' in result
        servercode = '''import socket
s=socket.socket(); s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1)
s.bind(('198.18.32.2',8032)); s.listen(1); c,a=s.accept(); n=0
while True:
 b=c.recv(65536)
 if not b: break
 c.sendall(b); n+=len(b)
print('echoed',n,flush=True)
c.close(); s.close()
'''
        server = subprocess.Popen(['ip', 'netns', 'exec', PEER_NS, 'python3', '-u', '-c', servercode], stdout=log, stderr=log)
        time.sleep(.3)
        clientcode = '''import socket
s=socket.create_connection(('198.18.32.2',8032),5); s.settimeout(10)
b=bytes(range(256))*256; n=0
for i in range(160):
 s.sendall(b); got=b''
 while len(got)<len(b):
  part=s.recv(len(b)-len(got)); assert part; got+=part
 assert got==b; n+=len(got)
s.close(); print('PASS TCP exact echo',n,'bytes')
'''
        run(['ip', 'netns', 'exec', LAN_NS, 'python3', '-c', clientcode])
        assert server.wait(timeout=10) == 0
        cli('show', 'interface')
        cli('show', 'hardware-interfaces', 'hw32-dpdk' if dpdk else 'host-' + DATA)
        run(['ip', '-n', PEER_NS, '-s', 'link', 'show', PEER])
        errors = cli('show', 'errors')
        assert 'tx ring overrun' not in errors, 'TX ring overflow during NIC test'
        assert before == run(['systemctl', 'show', 'vpp', '-p', 'MainPID', '-p', 'NRestarts'])
        log.write(f'PASS: actual ens193/ens256 path through dedicated VPP ({"DPDK" if dpdk else "af_packet"}), bidirectional ICMP, DF1500, TCP 10MiB\n')
    finally:
        if server and server.poll() is None:
            server.terminate(); server.wait(timeout=5)
        if vpp and vpp.poll() is None:
            for args in [('show', 'interface'), ('show', 'interface', 'address'), ('show', 'errors'), ('show', 'ip', 'neighbors')]:
                try: cli(*args)
                except Exception as error: log.write(str(error) + '\n')
            for name in [DATA, 'hw32l0']: run(['ip', 'link', 'set', name, 'down'], expected=None)
            vpp.terminate(); vpp.wait(timeout=10)
        if dpdk:
            driver = device / 'driver'
            if driver.exists() and driver.resolve().name != 'vmxnet3':
                (driver / 'unbind').write_text(pci)
                (device / 'driver_override').write_text('\n')
                Path('/sys/bus/pci/drivers/vmxnet3/bind').write_text(pci)
            for _ in range(50):
                if Path('/sys/class/net/' + DATA).exists(): break
                time.sleep(.1)
            if vfio_previous is not None:
                vfio_parameter.write_text(vfio_previous)
        if peer_created:
            run(['ip', '-n', PEER_NS, 'addr', 'flush', 'dev', PEER], expected=None)
            run(['ip', '-n', PEER_NS, 'link', 'set', PEER, 'down'], expected=None)
            run(['ip', 'netns', 'exec', PEER_NS, 'ethtool', '-K', PEER, 'tx', 'on' if tx_on else 'off'], expected=None)
            run(['ip', '-n', PEER_NS, 'link', 'set', PEER, 'netns', str(os.getpid())], expected=None)
            run(['ip', 'netns', 'del', PEER_NS], expected=None)
        if lan_created: run(['ip', 'netns', 'del', LAN_NS], expected=None)
        run(['ip', 'link', 'set', DATA, 'down'], expected=None)
        run(['sysctl', '-qw', f'net.ipv6.conf.{DATA}.disable_ipv6={ipv6}'], expected=None)
        run(['ip', '-br', 'addr'])
        run(['systemctl', 'show', 'vpp', '-p', 'MainPID', '-p', 'NRestarts'])
        log.close()


if __name__ == '__main__':
    main()
