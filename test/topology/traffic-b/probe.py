#!/usr/bin/env python3
"""Packet probes called while reviewed constituent topology fixtures are alive."""
import argparse
import json
import os
from pathlib import Path
import selectors
import re
import socket
import subprocess
import sys
import time
from scenario import Refused, slot_values


def command(argv, timeout=30):
    return subprocess.check_output(argv, stderr=subprocess.STDOUT, timeout=timeout, text=True)


def stop(process):
    if process.poll() is None:
        process.terminate()
        try: process.wait(timeout=5)
        except subprocess.TimeoutExpired: process.kill(); process.wait()


class Capture:
    def __init__(self, namespace, device, expression, output):
        self.output=output
        self.stream = os.fdopen(os.open(output,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600),'w')
        self.process = subprocess.Popen(['ip','netns','exec',namespace,'tcpdump','--immediate-mode','-n','-l','-vv','-i',device,expression],
                                        stdout=self.stream,stderr=subprocess.PIPE)
        try:
            selector=selectors.DefaultSelector();selector.register(self.process.stderr,selectors.EVENT_READ)
            deadline=time.monotonic()+10;data=b''
            try:
                while b'listening on' not in data:
                    if self.process.poll() is not None or time.monotonic()>deadline:
                        raise Refused('capture readiness not observed')
                    for key,_ in selector.select(.2):
                        data += os.read(key.fileobj.fileno(),4096)
                        if len(data)>16384:raise Refused('capture readiness exceeds bound')
            finally:selector.close()
        except BaseException:
            self.close();raise
    def wait_text(self, predicate):
        deadline=time.monotonic()+10
        while time.monotonic()<deadline:
            if self.output.stat().st_size>1048576:raise Refused('capture text exceeds bound')
            data=self.output.read_text()
            if predicate(data):return data
            if self.process.poll() is not None:raise Refused('capture exited before required packets')
            time.sleep(.05)
        raise Refused('required packet capture deadline')
    def close(self):
        stop(self.process)
        self.process.stderr.close();self.stream.close()


def probe(slot, phase, output):
    values=slot_values(slot);prefix=values['NGFW_TEST_PREFIX']
    if os.environ.get('NGFW_DISPOSABLE_VPP')!='1' or os.environ.get('NGFW_TRAFFIC_B')!='1':
        raise Refused('private Wave-B campaign only')
    output.mkdir(mode=0o700,parents=True,exist_ok=True)
    def peer(ns,*argv):return command(['ip','netns','exec',ns,*argv])
    if phase in ('bgp','ospf'):
        lan=f'ns-{prefix}-lan';wan=f'ns-{prefix}-wan';device=prefix+'w1'
        target=f'10.{slot}.64.129' if phase=='bgp' else f'10.{slot}.128.129'
        port=int(values['NGFW_HTTP_PORT'])+42
        peer(wan,'ip','addr','add',target+'/32','dev','lo')
        peer(wan,'ip','route','add',f'10.{slot}.1.2/32','via',f'10.{slot}.2.1','dev',device)
        server=None;capture=None
        try:
            ready=output/(phase+'-server-ready')
            if ready.exists():raise Refused('stale TCP readiness artifact')
            server=subprocess.Popen(['ip','netns','exec',wan,sys.executable,str(Path(__file__).resolve()),'--server',target,str(port),str(ready)],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
            deadline=time.monotonic()+10
            while not ready.exists():
                if server.poll() is not None or time.monotonic()>deadline:raise Refused('TCP server did not start')
                time.sleep(.05)
            learned=f'10.{slot}.64.128/25' if phase=='bgp' else f'10.{slot}.128.0/24'
            before=command(['timeout','10','vppctl','show','ip','fib',learned])
            if learned not in before or 'lcp-rt' not in before:raise Refused('exact learned FIB prefix absent')
            (output/(phase+'-peer-route.txt')).write_text(peer(wan,'ip','route','get',f'10.{slot}.1.2'))
            cap=output/(phase+'-tcpdump.txt')
            capture=Capture(wan,device,'host '+target+' and (icmp or tcp port '+str(port)+')',cap)
            ping=peer(lan,'ping','-n','-c','4','-s','347','-W','3',target)
            peer(lan,sys.executable,str(Path(__file__).resolve()),'--client',target,str(port))
            capture.wait_text(lambda text:'ICMP echo reply' in text and 'Flags [S.]' in text)
            capture.close();capture=None
            text=cap.read_text()
            if target not in text or 'ICMP echo request' not in text or 'ICMP echo reply' not in text or 'Flags [S]' not in text or 'Flags [S.]' not in text:
                raise Refused('learned-route bidirectional ICMP/TCP absent from capture')
            (output/(phase+'-ping.txt')).write_text(ping)
            after=command(['timeout','10','vppctl','show','ip','fib',learned])
            if learned not in after or 'lcp-rt' not in after:raise Refused('learned FIB prefix vanished during packets')
            (output/(phase+'-fib.txt')).write_text('BEFORE\n'+before+'AFTER\n'+after)
        finally:
            if capture:capture.close()
            if server:stop(server)
            peer(wan,'ip','route','del',f'10.{slot}.1.2/32','via',f'10.{slot}.2.1','dev',device)
            peer(wan,'ip','addr','del',target+'/32','dev','lo')
    elif phase=='wireguard':
        ns=f'ns-{prefix}wh';device=prefix+'wh-t0';target=f'10.{slot}.61.1'
        cap=output/'wireguard-tcpdump.txt';capture=Capture(ns,device,'udp port '+str(20000+100*slot+10),cap)
        try:
            ping=peer(ns,'ping','-n','-c','4','-s','347','-W','3',target)
            (output/'wireguard-ping.txt').write_text(ping)
        finally:capture.close()
        if 'UDP' not in cap.read_text():raise Refused('WireGuard underlay UDP absent')
    elif phase=='dhcp-relay':
        wan=f'ns-{prefix}-wan';lan=f'ns-{prefix}-lan';cap=output/'dhcp-tcpdump.txt'
        lease=output/'dhcp-private.leases';pidfile=output/'dhcp-private.pid'
        capture=Capture(wan,prefix+'w1','udp port 67 or udp port 68',cap)
        try:
            # Foreground dhclient, harmless script; fixture's ordinary lease test
            # subsequently exercises the actual client address configuration.
            peer(lan,'dhclient','-4','-1','-d','-v','-sf','/bin/true','-lf',str(lease),'-pf',str(pidfile),prefix+'l1')
        finally:capture.close()
        text=cap.read_text()
        if 'Discover' not in text or ('Gateway-IP 10.'+str(slot)+'.1.1') not in text:
            raise Refused('relayed DISCOVER with exact VPP giaddr absent')
        if 'fixed-address 10.'+str(slot)+'.1.' not in lease.read_text():raise Refused('DHCP lease missing')
    else:raise Refused('unknown packet probe')
    (output/(phase+'-probe.json')).write_text(json.dumps({'phase':phase,'slot':slot,'passed':True,'scope':'packet probe only; constituent fixture verifies application and rollback'},indent=2))


def main():
    if len(sys.argv)>1 and sys.argv[1]=='--server':
        address,port,ready=sys.argv[2:];server=socket.socket();server.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1)
        server.bind((address,int(port)));server.listen(4)
        fd=os.open(ready,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600);os.write(fd,b'ready');os.close(fd)
        while True:
            connection,_=server.accept()
            with connection:
                connection.settimeout(10)
                while data:=connection.recv(65536):connection.sendall(data)
    if len(sys.argv)>1 and sys.argv[1]=='--client':
        with socket.create_connection((sys.argv[2],int(sys.argv[3])),timeout=10) as client:
            payload=b'NGFW-traffic-B-exact-echo'*512
            client.sendall(payload);received=b''
            while len(received)<len(payload):
                chunk=client.recv(65536)
                if not chunk:raise Refused('short TCP echo')
                received+=chunk
            if received!=payload:raise Refused('TCP payload changed')
        return
    parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('--slot',type=int,required=True)
    parser.add_argument('--phase',required=True);parser.add_argument('--output',type=Path,required=True)
    args=parser.parse_args();probe(args.slot,args.phase,args.output)

if __name__=='__main__':main()
