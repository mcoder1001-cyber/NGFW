#!/usr/bin/env python3
"""Private mount + network wrapper for all owned protocol fixture daemons.

The child sees private /run/netns, /run/frr, /run/ngfw-test and /run/vpp.
The parent's shared socket identity and system VPP PID/restarts must not change.
"""
import argparse
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import time
from scenario import Refused,slot_values
from pgrelay import Relay
ROOT=Path(__file__).resolve().parents[3]


def identity():
    return subprocess.check_output(['systemctl','show','vpp.service','-p','MainPID','-p','NRestarts'])


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--slot',type=int,required=True);parser.add_argument('--child',action='store_true')
    parser.add_argument('--host-network',action='store_true',help='slot API database stack only; no root FRR')
    parser.add_argument('command',nargs=argparse.REMAINDER);args=parser.parse_args()
    values=slot_values(args.slot)
    if not args.command or os.environ.get('NGFW_INTEGRATION')!='1':raise Refused('integration command required')
    if not args.child:
        before=identity()
        cmd=['unshare','--mount','--propagation','private']
        if not args.host_network:cmd+=['--net']
        cmd += [sys.executable,str(Path(__file__).resolve()),'--child','--slot',str(args.slot)]
        if args.host_network:cmd+=['--host-network']
        cmd+=args.command
        relay=None
        env=dict(os.environ,**values)
        if env.get('NGFW_TRAFFIC_B_REST')=='1':
            relay=Relay(ROOT/'.scratch'/('traffic-pg-relay-'+str(os.getpid())))
            env['NGFW_TRAFFIC_PG_PROXY_DIR']=str(relay.directory)
        def terminate(signum,frame):raise SystemExit(128+signum)
        signal.signal(signal.SIGTERM,terminate)
        child=None
        try:
            child=subprocess.Popen(cmd,env=env,start_new_session=True)
            result=child.wait(timeout=2100)
        finally:
            if child and child.poll() is None:
                os.killpg(child.pid,signal.SIGTERM)
                try:child.wait(timeout=30)
                except subprocess.TimeoutExpired:os.killpg(child.pid,signal.SIGKILL);child.wait()
            if relay:relay.close()

        if identity()!=before:raise Refused('shared VPP changed during private campaign')
        print('SHARED_VPP_UNCHANGED',flush=True)
        return result
    current_mnt=os.readlink('/proc/self/ns/mnt')
    parent_mnt=os.readlink('/proc/'+str(os.getppid())+'/ns/mnt')
    if current_mnt==parent_mnt or current_mnt==os.readlink('/proc/1/ns/mnt'):
        raise Refused('child requires independently observed new mount namespace')
    if not args.host_network and os.readlink('/proc/self/ns/net')==os.readlink('/proc/'+str(os.getppid())+'/ns/net'):
        raise Refused('child requires independently observed new network namespace')
    os.umask(0o022)  # daemon traversal dirs; diagnostic logs set0600 explicitly
    runtime=ROOT/'.scratch'/('traffic-b-private-'+str(os.getpid()))
    runtime.mkdir(mode=0o700,parents=True)
    for target,label in [('/run/netns','netns'),('/run/ngfw-test','ngfw-test'),('/run/vpp','vpp'),('/run/frr','frr')]:
        directory=runtime/label;directory.mkdir(mode=0o755)
        subprocess.run(['mount','--bind',str(directory),target],check=True)
    stock=os.environ.get('NGFW_TRAFFIC_STOCK_ROOT')
    if stock:
        source=Path(stock).resolve(strict=True)
        if source==Path('/') or not (source/'usr/sbin/charon-systemd').is_file():raise Refused('extracted stock root required')
        target=Path('/run/ngfw-test/w10/swan-stock/root');target.mkdir(parents=True,mode=0o755)
        subprocess.run(['mount','--bind',str(source),str(target)],check=True)
        subprocess.run(['mount','-o','remount,bind,ro',str(target)],check=True)
    if not args.host_network:subprocess.run(['ip','link','set','lo','up'],check=True)
    conf=runtime/'startup.conf'
    plugins=os.environ.get('NGFW_ISOLATED_PLUGIN_PATH','')
    directive='path '+plugins if plugins else ''
    conf.write_text('unix { nodaemon cli-listen /run/vpp/cli.sock log /run/vpp/vpp.log }\n'
                    +f'api-segment {{ prefix trafficb{os.getpid()} }}\n'
                    +'socksvr { default }\nstatseg { socket-name /run/vpp/stats.sock }\n'
                    +'cpu { main-core 4 }\ndpdk { no-pci }\n'
                    +f'plugins {{ {directive} plugin linux_cp_plugin.so {{ enable }} plugin linux_nl_plugin.so {{ enable }} }}\n')
    vpp=None
    def terminate(signum,frame):raise SystemExit(128+signum)
    signal.signal(signal.SIGTERM,terminate)
    try:
        fd=os.open(runtime/'vpp-private.log',os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
        with os.fdopen(fd,'w') as stream:
            vpp=subprocess.Popen(['vpp','-c',str(conf)],stdout=stream,stderr=subprocess.STDOUT)
            for _ in range(200):
                if vpp.poll() is not None:raise Refused('private VPP startup failed')
                if (runtime/'vpp/api.sock').exists() and (runtime/'vpp/cli.sock').exists():break
                time.sleep(.1)
            else:raise Refused('private VPP startup deadline')
            env=dict(os.environ,**values,NGFW_DISPOSABLE_VPP='1',NGFW_TRAFFIC_B='1',
                     NGFW_TRAFFIC_PRIVATE_VPP_PID=str(vpp.pid),
                     NGFW_VPP_API_SOCKET=str(runtime/'vpp/api.sock'),
                     NGFW_TRAFFIC_B_EVIDENCE=str(runtime/'evidence'))
            child=subprocess.Popen(args.command,env=env,cwd=ROOT,start_new_session=True)
            try:
                result=child.wait(timeout=1800)
            finally:
                if child.poll() is None:
                    os.killpg(child.pid,signal.SIGTERM)
                    try:child.wait(timeout=15)
                    except subprocess.TimeoutExpired:
                        os.killpg(child.pid,signal.SIGKILL);child.wait()
            if vpp.poll() is not None:raise Refused('private VPP died during campaign')
            print(json.dumps({'private_runtime':str(runtime),'exit_code':result,'slot':args.slot}),flush=True)
            return result
    finally:
        if vpp and vpp.poll() is None:
            vpp.terminate()
            try:vpp.wait(timeout=15)
            except subprocess.TimeoutExpired:vpp.kill();vpp.wait()

if __name__=='__main__':
    try:sys.exit(main())
    except (OSError,ValueError,subprocess.SubprocessError) as error:
        print(type(error).__name__+': private campaign failed; inspect owned logs',file=sys.stderr);sys.exit(1)
