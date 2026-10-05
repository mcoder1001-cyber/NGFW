#!/usr/bin/env python3
"""Compose real Wave-B protocol fixtures sequentially, retaining honest evidence."""
import argparse
import fcntl
import hashlib
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import time
sys.dont_write_bytecode=True
from owned_process import stop_session
from scenario import PHASES,Refused,slot_values
ROOT=Path(__file__).resolve().parents[3]
HERE=Path(__file__).resolve().parent
PRIMARY=('ipsec','ipsec-cert','wireguard','tunnels','bgp','ospf','dhcp-relay')


def jobs(slot, output, agent, plugin):
    heavy=str(ROOT/'tools/heavy.sh')
    prefix=[sys.executable,str(HERE/'private.py'),'--slot',str(slot)]
    gotest=lambda name,package:[heavy,'go','-C','apps/agent','test','-count=1','-v','-timeout','8m','-run','^'+name+'$',package]
    native={'NGFW_NATIVE_AGENT_BIN':str(agent),'NGFW_ISOLATED_PLUGIN_PATH':str(plugin)+':/usr/lib/x86_64-linux-gnu/vpp_plugins'}
    return {
        'ipsec':(prefix+[str(ROOT/'test/topology/ipsec/run.sh'),'--skip-build','--agent',str(agent),'--plugin-directory',str(plugin)],native,None),
        'ipsec-cert':(prefix+[sys.executable,str(HERE/'native-cert.py')],native,'PRODUCTION_CERTIFICATE_PEER_PACKETS=PASS'),
        'wireguard':(prefix+['--host-network',sys.executable,str(HERE/'wireguard_rest.py'),'--slot',str(slot),'--output',str(output/'wireguard-packets')],{},'REST_WIREGUARD_PACKETS=PASS'),
        'tunnels':(prefix+['--host-network',sys.executable,str(HERE/'stack.py'),'--slot',str(slot),'--output',str(output/'tunnel-packets')],{},'"passed": true'),
        'bgp':(prefix+gotest('TestP12TopologyOnHost','./internal/agent'),{'NGFW_P12_TOPOLOGY':'1','NGFW_P12_FIB':'private'},'--- PASS: TestP12TopologyOnHost'),
        'ospf':(prefix+gotest('TestOSPFTopologyOnHost','./internal/agent'),{'NGFW_OSPF_TOPOLOGY':'1','NGFW_OSPF_FIB':'root'},'--- PASS: TestOSPFTopologyOnHost'),
        'dhcp-relay':(prefix+['--host-network',heavy,str(ROOT/'test/topology/kea-dhcp-relay/run.sh'),'-run','^TestKeaDhcpRelay$'],{},'--- PASS: TestKeaDhcpRelay'),
    }


def execute(argv, env, log, *, timeout=2100):
    fd=os.open(log,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
    with os.fdopen(fd,'wb') as stream:
        child=subprocess.Popen(argv,cwd=ROOT,env=env,stdout=stream,stderr=subprocess.STDOUT,start_new_session=True)
        try:return child.wait(timeout=timeout)
        finally:
            stop_session(child)



def campaign(slot,output,selected,*,agent,plugin):
    values=slot_values(slot)
    output.mkdir(mode=0o700,parents=True,exist_ok=False)
    summary={'task':'TEST-traffic-B','source_sha':subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip(),
             'slot':slot,'started_utc':time.strftime('%Y-%m-%dT%H:%M:%SZ',time.gmtime()),'phases':[],
             'primary_passed':False,'whole_wave_passed':False}
    try:
        for name in selected:
            argv,extra,marker=jobs(slot,output,agent,plugin)[name]
            env=dict(os.environ,**values,**extra,NGFW_INTEGRATION='1',NGFW_TRAFFIC_B_REST='1',NGFW_TRAFFIC_B_SLOT=str(slot),NGFW_TRAFFIC_B_PHASE=name)
            if 'ipsec' in name:
                source=Path(os.environ.get('NGFW_TRAFFIC_STOCK_ROOT','/run/ngfw-test/w10/swan-stock/root')).resolve(strict=True)
                env['NGFW_TRAFFIC_STOCK_ROOT']=str(source)
            entry={'phase':name,'status':'failed'};summary['phases'].append(entry)
            log=output/(name+'.private.log')
            print('START '+name,flush=True)
            try:
                rc=execute(argv,env,log)
                info=log.stat()
                if info.st_size>16777216:raise Refused('phase diagnostics exceed bounded readback')
                text=log.read_text(errors='replace')
                entry['exit_code']=rc
                if rc or '--- SKIP:' in text or 'SHARED_VPP_UNCHANGED' not in text:
                    raise Refused('phase failed, skipped, or shared VPP identity unverified')
                if marker and marker not in text:raise Refused('required executed acceptance marker absent')
                if name=='ipsec' and '"passed": true' not in text:raise Refused('native production acceptance not passed')
                if name in ('ipsec','ipsec-cert','bgp','ospf'):
                    runtimes=[]
                    for line in text.splitlines():
                        if line.startswith('{"private_runtime":'):
                            runtimes.append(Path(json.loads(line)['private_runtime']))
                    if len(runtimes)!=1:raise Refused('one owned runtime identity required')
                    runtime=runtimes[0].resolve(strict=True)
                    if runtime.parent!=ROOT/'.scratch' or not runtime.name.startswith('traffic-b-private-'):
                        raise Refused('foreign REST evidence runtime')
                    labels=('ipsec-responder','ipsec-initiator') if name=='ipsec' else (name,)
                    proofs=[]
                    for label in labels:
                        proof_path=runtime/'evidence'/(label+'-rest.json')
                        if proof_path.stat().st_size>1048576:raise Refused('REST evidence exceeds bound')
                        proof=json.loads(proof_path.read_text())
                        events=proof.get('events',[])
                        if not events or events[0].get('status')!='applied' or not events[0].get('candidate_owner') or len(events[0].get('candidate_sha256',''))!=64:
                            raise Refused('actual initial REST applied candidate proof absent')
                        if not any(event.get('txn')=='rest-baseline-rollback' and event.get('status')=='applied' for event in events):
                            raise Refused('actual REST rollback proof absent')
                        proofs.append({'subphase':label,'proof_sha256':hashlib.sha256(proof_path.read_bytes()).hexdigest(),
                                       'initial_revision':events[0]['revision'],'candidate_sha256':events[0]['candidate_sha256']})
                    entry['rest_proofs']=proofs
                entry.update(status='passed',diagnostic_sha256=hashlib.sha256(log.read_bytes()).hexdigest())
            except (OSError,ValueError,subprocess.SubprocessError) as error:
                entry['reason']=type(error).__name__+': acceptance failed; private diagnostic retained'
            print('END '+name+' '+entry['status'],flush=True)
            (output/'summary.json').write_text(json.dumps(summary,indent=2)+'\n')
        summary['primary_passed']=set(selected)==set(PRIMARY) and all(p['status']=='passed' for p in summary['phases'])
        # Phase6 smoke steps are explicitly optional/not-run under the task prompt.
        # They can never silently turn a partial campaign into wave acceptance.
        summary['smoke_not_run']=[{'phase':p.name,'reason':p.limitation} for p in PHASES if p.name not in ('ipsec','wireguard','gre','vxlan','bgp','ospf','dhcp-relay')]
        return 0 if all(p['status']=='passed' for p in summary['phases']) else 1
    finally:
        summary['ended_utc']=time.strftime('%Y-%m-%dT%H:%M:%SZ',time.gmtime())
        (output/'summary.json').write_text(json.dumps(summary,indent=2)+'\n')


def main():
    parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('--slot',type=int,default=27)
    parser.add_argument('--dry-run',action='store_true');parser.add_argument('--phase',action='append',choices=PRIMARY)
    parser.add_argument('--output',type=Path);parser.add_argument('--agent',type=Path,default=ROOT/'apps/agent/bin/ngfw-agent')
    parser.add_argument('--plugin-directory',type=Path,default=ROOT/'.scratch/traffic-b-native-plugin/plugin')
    parser.add_argument('--build-native-plugin',action='store_true')
    parser.add_argument('--build-wg-test-agent',action='store_true',help='build envelope-approved ngfwtestsecrets agent in owned scratch directory')
    args=parser.parse_args()
    def terminate(signum,frame):raise SystemExit(128+signum)
    signal.signal(signal.SIGTERM,terminate)
    try:
        slot_values(args.slot);selected=args.phase or list(PRIMARY)
        if len(set(selected))!=len(selected):raise Refused('duplicate phase')
        if args.dry_run:
            print(json.dumps({'mode':'plan-only','passed':False,'primary':selected,'phases':[p.__dict__ for p in PHASES]},indent=2));return 0
        if os.environ.get('NGFW_INTEGRATION')!='1' or os.geteuid()!=0:raise Refused('root and explicit integration opt-in required')
        if any(name.startswith('ipsec') for name in selected):
            if args.build_native_plugin:
                subprocess.run([str(ROOT/'tools/heavy.sh'),'python3',str(ROOT/'test/topology/ipsec/build-native-plugin.py'),'--output',str(args.plugin_directory.parent)],check=True)
            if not (args.plugin_directory/'ikev2_plugin.so').is_file() or not args.agent.is_file():raise Refused('built native plugin and product agent required')
        if 'wireguard' in selected:
            tagged=ROOT/'.scratch/traffic-b-wg-agent/ngfw-agent'
            if args.build_wg_test_agent:
                tagged.parent.mkdir(mode=0o700,parents=True,exist_ok=True)
                subprocess.run([str(ROOT/'tools/heavy.sh'),'go','-C','apps/agent','build','-tags','ngfwtestsecrets','-o',str(tagged),'./cmd/ngfw-agent'],cwd=ROOT,check=True)
            if not tagged.is_file():raise Refused('approved tagged WireGuard test agent required; use --build-wg-test-agent')
        output=(args.output or ROOT/'.scratch'/('traffic-b-campaign-'+str(os.getpid()))).resolve()
        with open('/run/lock/ngfw-traffic-b-w'+str(args.slot)+'.lock','a') as ownership:
            fcntl.flock(ownership,fcntl.LOCK_EX|fcntl.LOCK_NB)
            with open('/run/lock/ngfw-lab.lock','a') as lab:
                fcntl.flock(lab,fcntl.LOCK_SH)
                return campaign(args.slot,output,selected,agent=args.agent.resolve(),plugin=args.plugin_directory.resolve())
    except (OSError,ValueError,subprocess.SubprocessError) as error:
        print(type(error).__name__+': campaign refused; inspect private evidence',file=sys.stderr);return 2

if __name__=='__main__':sys.exit(main())
