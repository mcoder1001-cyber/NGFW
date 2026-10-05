#!/usr/bin/env python3
"""Launch and clean up the slot product API/agent for private tunnel probes."""
import argparse
import json
import os
from pathlib import Path
import secrets
import socket
import subprocess
import sys
import time
import urllib.request
from scenario import Refused,slot_values
from probe import stop
from tunnels import Api,run,check_commit
ROOT=Path(__file__).resolve().parents[3]


def main():
    parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('--slot',type=int,required=True)
    parser.add_argument('--output',type=Path,required=True);args=parser.parse_args()
    values=slot_values(args.slot);owner=values['NGFW_TEST_PREFIX']+'tb'
    if os.environ.get('NGFW_DISPOSABLE_VPP')!='1' or not os.environ.get('NGFW_TRAFFIC_PRIVATE_VPP_PID'):
        raise Refused('private wrapper process identity required')
    api_socket=Path(os.environ['NGFW_VPP_API_SOCKET']);mounted=Path('/run/vpp/api.sock')
    if api_socket.resolve()==mounted.resolve() or not os.path.samefile(api_socket,mounted):
        raise Refused('private API socket does not match mounted private VPP')
    agent=ROOT/'apps/agent/bin/ngfw-agent';api_bin=ROOT/'apps/api/dist/main.js'
    if not agent.is_file() or not api_bin.is_file():raise Refused('run complete quick gate to build product binaries first')
    port=int(values['NGFW_HTTP_PORT'])
    with socket.socket() as check:check.bind(('127.0.0.1',port))
    runtime=Path('/run/ngfw-test')/owner
    if runtime.exists():raise Refused('slot stack runtime already exists')
    # Never reuse/drop a foreign existing database. pg-test list contains no passwords.
    existing=subprocess.check_output([str(ROOT/'deploy/dev/pg-test.sh'),'list'],text=True)
    if any(line.split()[0]=='ngfw_'+owner for line in existing.splitlines() if line.split()):raise Refused('slot stack database exists')
    processes=[];streams=[];database=False
    try:
        subprocess.run([str(ROOT/'deploy/dev/pg-test.sh'),'create',owner],check=True,stdout=subprocess.DEVNULL)
        database=True
        runtime.chmod(0o700)
        dsn=dict(line.split('=',1) for line in (runtime/'pg.env').read_text().splitlines())['NGFW_PG_DSN']
        common={'PATH':os.environ['PATH'],'HOME':os.environ['HOME'],**values}
        agent_env=dict(common,NGFW_OWNER=owner,NGFW_GLOBALS_OWNER='0',NGFW_AGENT_SOCKET=str(runtime/'agent.sock'),
                       NGFW_AGENT_STATE_DIR=str(runtime/'state'),NGFW_AGENT_VPP_API_SOCKET=str(api_socket),
                       NGFW_METRICS_ADDR='off',NGFW_SOCKET_GROUP='root',NGFW_KEA_MODE='off',NGFW_LOG_LEVEL='info')
        def start(argv,env,label):
            path=runtime/(label+'.private.log');fd=os.open(path,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
            stream=os.fdopen(fd,'wb');streams.append(stream)
            process=subprocess.Popen(argv,env=env,cwd=ROOT,stdout=stream,stderr=subprocess.STDOUT)
            processes.append(process);return process
        ag=start([str(agent)],agent_env,'agent')
        kvport=port+80
        with socket.socket() as check:check.bind(('127.0.0.1',kvport))
        kv=start(['valkey-server','--bind','127.0.0.1','--port',str(kvport),'--save','','--appendonly','no','--dir',str(runtime)],common,'valkey')
        admin=secrets.token_urlsafe(24)
        api_env=dict(common,NODE_ENV='production',NGFW_HTTP_PORT=str(port),NGFW_HTTP_HOST='127.0.0.1',
                     NGFW_PG_DSN=dsn,NGFW_VALKEY_URL='redis://127.0.0.1:'+str(kvport),NGFW_VALKEY_DB='0',NGFW_VALKEY_PREFIX='ngfw:'+owner+':tb:',
                     NGFW_AGENT_SOCKET=str(runtime/'agent.sock'),NGFW_AGENT_OWNER=owner,NGFW_AGENT_TIMEOUT_MS='60000',
                     NGFW_JWT_SECRET=secrets.token_hex(32),NGFW_SECRET_KEY_FILE=str(runtime/'secret.key'),
                     NGFW_BOOTSTRAP_ADMIN_PASSWORD=admin,NGFW_COOKIE_SECURE='0',NGFW_LOG_LEVEL='warn')
        ap=start(['node',str(api_bin)],api_env,'api')
        endpoint='http://127.0.0.1:'+str(port)+'/api/v1'
        deadline=time.monotonic()+90
        while True:
            if ag.poll() is not None or ap.poll() is not None or kv.poll() is not None:raise Refused('owned stack exited; inspect private logs')
            try:
                with urllib.request.urlopen(endpoint+'/health',timeout=2) as response:
                    if response.status==200:break
            except OSError:pass
            if time.monotonic()>deadline:raise Refused('stack readiness deadline')
            time.sleep(.2)
        request=urllib.request.Request(endpoint+'/auth/login',data=json.dumps({'username':'admin','password':admin}).encode(),headers={'Content-Type':'application/json'})
        with urllib.request.urlopen(request,timeout=10) as response:token=json.load(response)['accessToken']
        api=Api(args.slot,token)
        base_name=owner+'-pristine'
        # A fresh datastore has no historical revision. Establish an owned
        # harmless VRF baseline; never fabricate revision0 or rollback/null.
        api.call('PATCH','/config/vrfs',{base_name:{'id':args.slot*1000+40}})
        check_commit(api.call('POST','/config/commit?comment=traffic-b-pristine'))
        try:
            events=run(args.slot,api,args.output)
        finally:
            api.call('POST','/config/discard')
            api.call('PATCH','/config/vrfs',{base_name:None})
            check_commit(api.call('POST','/config/commit?comment=traffic-b-pristine-cleanup'))
        print(json.dumps({'phase':'tunnels','passed':len(events)==2,'source_sha':subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip()}))
        return 0
    finally:
        for process in reversed(processes):stop(process)
        for stream in streams:stream.close()
        if database:subprocess.run([str(ROOT/'deploy/dev/pg-test.sh'),'drop',owner],check=True,stdout=subprocess.DEVNULL)

if __name__=='__main__':
    try:sys.exit(main())
    except (OSError,ValueError,subprocess.SubprocessError) as error:
        print((str(error) if isinstance(error,Refused) else type(error).__name__)+': slot stack failed; inspect private owned logs',file=sys.stderr);sys.exit(1)
