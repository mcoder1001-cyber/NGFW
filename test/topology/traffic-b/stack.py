#!/usr/bin/env python3
"""Launch and clean up the slot product API/agent for private tunnel probes."""
import argparse
from contextlib import contextmanager
import json
import os
from pathlib import Path
import secrets
import signal
import socket
import subprocess
import stat
import struct
import sys
import time
import urllib.request
from urllib.parse import urlsplit, urlunsplit, urlencode, parse_qsl
from scenario import Refused,slot_values,private_identity
from probe import stop
from tunnels import Api,run,check_commit
ROOT=Path(__file__).resolve().parents[3]


def attached_identity(path, owner):
    path=Path(path)
    entry=path.lstat(); parent=path.parent.stat()
    if not stat.S_ISSOCK(entry.st_mode) or entry.st_uid!=os.getuid() or parent.st_uid!=os.getuid() or parent.st_mode & 0o022:
        raise Refused('attached agent socket/path is not protected and owned')
    with socket.socket(socket.AF_UNIX) as peer:
        peer.connect(str(path));pid,uid,gid=struct.unpack('3i',peer.getsockopt(socket.SOL_SOCKET,socket.SO_PEERCRED,12))
    expected=int(os.environ.get('NGFW_TRAFFIC_EXPECTED_AGENT_PID','0'))
    if pid!=expected or uid!=os.getuid():raise Refused('attached agent must be explicitly recorded owned fixture process')
    fixture=os.getppid()
    if pid!=fixture:
        fields=dict(line.split(':',1) for line in Path(f'/proc/{pid}/status').read_text().splitlines() if ':' in line)
        if int(fields['PPid'])!=fixture or Path(os.readlink(f'/proc/{pid}/exe')).resolve()!=(ROOT/'apps/agent/bin/ngfw-agent').resolve():
            raise Refused('attached process is not the fixture owned production agent child')
    for namespace in ('mnt','net'):
        if os.readlink(f'/proc/{pid}/ns/{namespace}')!=os.readlink(f'/proc/self/ns/{namespace}'):
            raise Refused('attached agent namespace differs from private fixture')
    executable=Path(os.readlink(f'/proc/{pid}/exe')).name
    fixture_executable=Path(os.readlink(f'/proc/{fixture}/exe')).name
    if not fixture_executable.endswith('.test') or (pid==fixture and not executable.endswith('.test')) or not owner or os.environ.get('NGFW_TRAFFIC_VERIFIED_OWNER')!=owner:
        raise Refused('attached owner must be verified by parent fixture Retrieve RPC')
    return {'pid':pid,'uid':uid,'owner':owner,'socket_inode':entry.st_ino}


def stack_owner(slot,variant=''):
    if variant not in ('','r','i'):raise Refused('unknown owned native stack variant')
    return slot_values(slot)['NGFW_TEST_PREFIX']+'tb'+variant


@contextmanager
def product_stack(slot, *, target_socket=None, target_owner=None, agent_binary=None, wg_secrets=None):
    private_identity()
    values=slot_values(slot);owner=stack_owner(slot,os.environ.get('NGFW_TRAFFIC_STACK_VARIANT',''))
    agent_owner=target_owner or owner
    if os.environ.get('NGFW_DISPOSABLE_VPP')!='1' or not os.environ.get('NGFW_TRAFFIC_PRIVATE_VPP_PID'):
        raise Refused('private wrapper process identity required')
    api_socket=Path(os.environ['NGFW_VPP_API_SOCKET']);mounted=Path('/run/vpp/api.sock')
    if api_socket.resolve()==mounted.resolve() or not os.path.samefile(api_socket,mounted):
        raise Refused('private API socket does not match mounted private VPP')
    if target_socket is not None:attached_identity(target_socket,target_owner)
    agent=Path(agent_binary) if agent_binary else ROOT/'apps/agent/bin/ngfw-agent';api_bin=ROOT/'apps/api/dist/main.js'
    if not agent.is_file() or not api_bin.is_file():raise Refused('run complete quick gate to build product binaries first')
    port=int(values['NGFW_HTTP_PORT'])
    with socket.socket() as check:
        check.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1);check.bind(('127.0.0.1',port));check.listen(1)
    runtime=Path('/run/ngfw-test')/owner
    if runtime.exists():raise Refused('slot stack runtime already exists')
    # Never reuse/drop a foreign existing database. pg-test list contains no passwords.
    existing=subprocess.check_output([str(ROOT/'deploy/dev/pg-test.sh'),'list'],text=True)
    if any(line.split()[0]=='ngfw_'+owner for line in existing.splitlines() if line.split()):raise Refused('slot stack database exists')
    role=subprocess.check_output(['runuser','-u','postgres','--','psql','-X','-qAt','-c',
        "select rolname from pg_roles where rolname='ngfw_"+owner+"'"],cwd='/',text=True,timeout=10)
    if role.strip():raise Refused('slot stack role exists')
    processes=[];streams=[];database=False
    try:
        database=True  # reserved absent owner; partial create failures must also clean up
        subprocess.run([str(ROOT/'deploy/dev/pg-test.sh'),'create',owner],check=True,stdout=subprocess.DEVNULL,stdin=subprocess.DEVNULL,
                       env=dict(os.environ,**({'NGFW_PG_HOST':os.environ['NGFW_TRAFFIC_PG_PROXY_DIR']} if os.environ.get('NGFW_TRAFFIC_PG_PROXY_DIR') else {})))
        runtime.chmod(0o700)
        pg=dict(line.split('=',1) for line in (runtime/'pg.env').read_text().splitlines())
        dsn=pg['NGFW_PG_DSN']
        proxy=os.environ.get('NGFW_TRAFFIC_PG_PROXY_DIR')
        if proxy:
            dsn=urlunsplit(('postgres',pg['NGFW_PG_USER']+':'+pg['NGFW_PG_PASSWORD']+'@127.0.0.1:5432','/'+pg['NGFW_PG_DATABASE'],urlencode({'sslmode':'disable','host':proxy}),''))
        keys=runtime/'license-keys';licence=runtime/'test.ngfwlic';serial=owner+'-traffic-test'
        issuer=str(ROOT/'tools/license/ngfw-license')
        for arguments in ([issuer,'keygen','--out-dir',str(keys)],
                          [issuer,'issue','--key',str(keys/'ngfw-license-signing.pem'),'--customer','Traffic B isolated fixture','--id',serial,'--days','1','--serial',serial,'--features','ipsec,wireguard,bgp,ospf','--limit','ipsecTunnels=8','--limit','wireguardInterfaces=8','--out',str(licence)],
                          [issuer,'verify','--pub',str(keys/'ngfw-license-public.pem'),str(licence)]):
            subprocess.run(arguments,check=True,stdout=subprocess.DEVNULL,stdin=subprocess.DEVNULL,timeout=15)
        common={'PATH':os.environ['PATH'],'HOME':os.environ['HOME'],**values}
        agent_env=dict(common,NGFW_OWNER=agent_owner,NGFW_GLOBALS_OWNER='0',NGFW_AGENT_SOCKET=str(runtime/'agent.sock'),
                       NGFW_AGENT_STATE_DIR=str(runtime/'state'),NGFW_AGENT_VPP_API_SOCKET=str(api_socket),
                       NGFW_METRICS_ADDR='off',NGFW_SOCKET_GROUP='root',NGFW_KEA_MODE='off',NGFW_LOG_LEVEL='info')
        if wg_secrets is not None:
            if agent_binary is None or target_socket is not None:raise Refused('WireGuard fixture requires an owned tagged agent')
            build=subprocess.check_output(['go','version','-m',str(agent)],text=True,timeout=10)
            if '-tags=ngfwtestsecrets' not in build:raise Refused('WireGuard fixture binary lacks approved test tag')
            agent_env['NGFW_TEST_WG_SECRETS']=str(wg_secrets)
        def start(argv,env,label):
            path=runtime/(label+'.private.log');fd=os.open(path,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
            stream=os.fdopen(fd,'wb');streams.append(stream)
            process=subprocess.Popen(argv,env=env,cwd=ROOT,stdin=subprocess.DEVNULL,stdout=stream,stderr=subprocess.STDOUT)
            processes.append(process);return process
        ag=start([str(agent)],agent_env,'agent') if target_socket is None else None
        agent_socket=target_socket or str(runtime/'agent.sock')
        kvport=port+80
        with socket.socket() as check:
            check.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1);check.bind(('127.0.0.1',kvport));check.listen(1)
        kv=start(['valkey-server','--bind','127.0.0.1','--port',str(kvport),'--save','','--appendonly','no','--dir',str(runtime)],common,'valkey')
        admin=secrets.token_urlsafe(24)
        api_env=dict(common,NODE_ENV='production',NGFW_HTTP_PORT=str(port),NGFW_HTTP_HOST='127.0.0.1',
                     NGFW_LICENSE_FILE=str(licence),NGFW_LICENSE_SERIAL=serial,NGFW_LICENSE_PUBKEY_FILE=str(keys/'ngfw-license-public.pem'),
                     NGFW_PG_DSN=dsn,NGFW_VALKEY_URL='redis://127.0.0.1:'+str(kvport),NGFW_VALKEY_DB='0',NGFW_VALKEY_PREFIX='ngfw:'+owner+':tb:',
                     NGFW_AGENT_SOCKET=agent_socket,NGFW_AGENT_OWNER=agent_owner,NGFW_AGENT_TIMEOUT_MS='60000',
                     NGFW_JWT_SECRET=secrets.token_hex(32),NGFW_SECRET_KEY_FILE=str(runtime/'secret.key'),
                     NGFW_BOOTSTRAP_ADMIN_PASSWORD=admin,NGFW_COOKIE_SECURE='0',NGFW_LOG_LEVEL='warn')
        ap=start(['node',str(api_bin)],api_env,'api')
        endpoint='http://127.0.0.1:'+str(port)+'/api/v1'
        deadline=time.monotonic()+90
        while True:
            if (ag is not None and ag.poll() is not None) or ap.poll() is not None or kv.poll() is not None:raise Refused('owned stack exited; inspect private logs')
            try:
                with urllib.request.urlopen(endpoint+'/health',timeout=2) as response:
                    if response.status==200:break
            except OSError:pass
            if time.monotonic()>deadline:raise Refused('stack readiness deadline')
            time.sleep(.2)
        request=urllib.request.Request(endpoint+'/auth/login',data=json.dumps({'username':'admin','password':admin}).encode(),headers={'Content-Type':'application/json'})
        with urllib.request.urlopen(request,timeout=10) as response:token=json.load(response)['accessToken']
        api=Api(slot,token)
        def restart():
            nonlocal ag
            if target_socket is not None:raise Refused('attached agent restart belongs to fixture')
            stop(ag);ag=start([str(agent)],agent_env,'agent-restart')
            deadline=time.monotonic()+30
            while time.monotonic()<deadline:
                try:
                    api.call('GET','/state/interfaces');return
                except (OSError,Refused):time.sleep(.2)
            raise Refused('agent restart readiness deadline')
        yield api, runtime, restart

    finally:
        for process in reversed(processes):stop(process)
        for stream in streams:stream.close()
        if database:subprocess.run([str(ROOT/'deploy/dev/pg-test.sh'),'drop',owner],check=True,stdout=subprocess.DEVNULL,stdin=subprocess.DEVNULL)

def main():
    parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('--slot',type=int,required=True)
    parser.add_argument('--output',type=Path,required=True);args=parser.parse_args()
    def terminate(signum,frame):raise SystemExit(128+signum)
    signal.signal(signal.SIGTERM,terminate)
    with product_stack(args.slot) as (api,runtime,restart):
        base_name=f'w{args.slot}tb-pristine'
        api.call('PATCH','/config/vrfs',{base_name:{'id':args.slot*1000+40}})
        baseline_warnings=check_commit(api.call('POST','/config/commit?comment=traffic-b-pristine'), changed_paths=('/vrfs',))
        try:
            events=run(args.slot,api,args.output,baseline_warnings)
        finally:
            api.call('POST','/config/discard')
            api.call('PATCH','/config/vrfs',{base_name:None})
            check_commit(api.call('POST','/config/commit?comment=traffic-b-pristine-cleanup'), baseline_warnings=baseline_warnings, changed_paths=('/vrfs',))
        print(json.dumps({'phase':'tunnels','passed':len(events)==2,'source_sha':subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip()}))
        return 0

if __name__=='__main__':
    try:sys.exit(main())
    except (OSError,ValueError,subprocess.SubprocessError) as error:
        print((str(error) if isinstance(error,(Refused,OSError)) else type(error).__name__)+': slot stack failed; inspect private owned logs',file=sys.stderr);sys.exit(1)
