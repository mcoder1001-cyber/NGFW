#!/usr/bin/env python3
"""Real-agent REST WireGuard campaign with an owned kernel peer."""
import argparse
import base64
import json
import os
from pathlib import Path
import signal
import subprocess
import time
from stack import product_stack
from rest_bridge import Controller
from probe import Capture
from scenario import Refused,private_identity
ROOT=Path(__file__).resolve().parents[3]

def cleanup(control,api,created,command,prefix):
    try:
        control.close()
        if api.call('GET','/state/vpn/wireguard').get('interfaces'):
            raise Refused('WireGuard residue after REST rollback')
    finally:
        if created:command([str(ROOT/'tools/lab'),'rig','down',prefix])


def main():
    parser=argparse.ArgumentParser();parser.add_argument('--slot',type=int,required=True);parser.add_argument('--output',type=Path,required=True);args=parser.parse_args()
    private_identity();args.output.mkdir(mode=0o700,parents=True,exist_ok=False)
    def terminate(signum,frame):raise SystemExit(128+signum)
    signal.signal(signal.SIGTERM,terminate)
    prefix=f'w{args.slot}';namespace=f'ns-{prefix}-wan';device='tbwg'
    def command(argv,data=None):return subprocess.check_output(argv,input=data,stderr=subprocess.STDOUT,timeout=40)
    def peer(*argv):return command(['ip','netns','exec',namespace,*argv]).decode()
    created=False
    vpp_key=command(['wg','genkey']).strip();kernel_key=command(['wg','genkey']).strip()
    fixture=args.output/'wg-secret.private.json';fd=os.open(fixture,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
    with os.fdopen(fd,'w') as stream:json.dump({'key/'+prefix+'tb-a':vpp_key.decode()},stream)
    tagged=ROOT/'.scratch/traffic-b-wg-agent/ngfw-agent'
    with product_stack(args.slot,agent_binary=tagged,wg_secrets=fixture) as (api,runtime,restart):
        control=Controller(api,prefix+'tb',args.output/'wireguard-rest.json')
        try:
            created=True;command([str(ROOT/'tools/lab'),'rig','up',prefix])
            command(['vppctl','delete','host-interface','name',prefix+'w0'])
            vpp_pub=command(['wg','pubkey'],vpp_key+b'\n').decode().strip();kernel_pub=command(['wg','pubkey'],kernel_key+b'\n').decode().strip()
            key_path=runtime/'kernel.key';fd=os.open(key_path,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
            with os.fdopen(fd,'wb') as stream:stream.write(kernel_key+b'\n')
            peer('ip','link','add',device,'type','wireguard')
            vpp_port=20000+args.slot*100+10;kernel_port=vpp_port+1
            peer('wg','set',device,'private-key',str(key_path),'listen-port',str(kernel_port),'peer',vpp_pub,'allowed-ips',f'10.{args.slot}.61.1/32','endpoint',f'10.{args.slot}.2.1:{vpp_port}','persistent-keepalive','1')
            peer('ip','addr','add',f'10.{args.slot}.61.2/24','dev',device);peer('ip','link','set',device,'up')
            desired={'interfaces':{'host-'+prefix+'w0':{'enabled':True,'ipv4':[f'10.{args.slot}.2.1/24']}},
              'vpn':{'wireguard':{'interfaces':{'site':{'instance':args.slot*100+60,'listenAddress':f'10.{args.slot}.2.1','listenPort':vpp_port,
                  'privateKeyRef':'key/'+prefix+'tb-a','address':[f'10.{args.slot}.61.1/24'],'routeAllowedIps':True,
                  'peers':{'kernel':{'publicKey':kernel_pub,'endpoint':{'address':f'10.{args.slot}.2.2','port':kernel_port},'allowedIps':[f'10.{args.slot}.61.2/32'],'persistentKeepaliveSec':1}}}}}}}
            capture=Capture(namespace,prefix+'w1',f'udp port {vpp_port} or udp port {kernel_port}',args.output/'wireguard-tcpdump.txt')
            try:
                first=control.apply({'txn':'wireguard-rest-initial','desired':desired,'secrets':{'key/'+prefix+'tb-a':base64.b64encode(vpp_key).decode()}})
                if first['status']!='applied':raise Refused('primary WireGuard REST commit must apply')
                def established():
                    deadline=time.monotonic()+30
                    while time.monotonic()<deadline:
                        state=api.call('GET','/state/vpn/wireguard')
                        (args.output/'wireguard-last-state.json').write_text(json.dumps(state,indent=2))
                        if any(p.get('established') and p.get('lastHandshake') for i in state.get('interfaces',[]) for p in i.get('peers',[])):
                            return state
                        time.sleep(.2)
                    raise Refused('WireGuard handshake not established')
                state=established();ping=peer('ping','-n','-c','4','-W','3','-I',device,f'10.{args.slot}.61.1')
                (args.output/'wireguard-ping.txt').write_text(ping)
                capture.wait_text(lambda text:f'.{vpp_port} >' in text and f'> 10.{args.slot}.2.2.{kernel_port}' in text)
                # Tagged owned process restart reloads the approved secret fixture; state/ping must recover.
                restart();after=established();peer('ping','-n','-c','3','-W','3','-I',device,f'10.{args.slot}.61.1')
                (args.output/'wireguard-state.json').write_text(json.dumps({'before_restart':state,'after_restart':after,'secret_source':'approved ngfwtestsecrets slot fixture; production secret channel pending'},indent=2))
            finally:capture.close()
        finally:
            cleanup(control,api,created,command,prefix)
        print('REST_WIREGUARD_PACKETS=PASS',flush=True)
        return 0

if __name__=='__main__':raise SystemExit(main())
