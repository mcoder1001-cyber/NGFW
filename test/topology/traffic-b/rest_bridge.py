#!/usr/bin/env python3
"""Test-only REST controller attached to a fixture's real agent socket.

Only requests on its private stdin pipe are accepted. Secrets are never written
into diagnostics/evidence. EOF or SIGTERM rolls back and cleans the owned stack.
"""
import argparse
import base64
import copy
import hashlib
import json
import os
from pathlib import Path
import signal
import sys
from urllib.parse import quote
from stack import product_stack
from scenario import Refused, private_identity
from tunnels import check_commit


def digest(document):
    return hashlib.sha256(json.dumps(document,sort_keys=True,separators=(',',':')).encode()).hexdigest()


def merge_delta(old,new):
    if not isinstance(old,dict) or not isinstance(new,dict):return new
    patch={key:None for key in old.keys()-new.keys()}
    for key,value in new.items():
        if key not in old:patch[key]=value
        elif old[key]!=value:patch[key]=merge_delta(old[key],value)
    return patch


class Controller:
    def __init__(self,api,owner,output):
        self.api=api;self.owner=owner;self.output=output;self.events=[]
        original=api.call('GET','/config/diff')
        if original.get('changes') or original.get('baseRevision') is not None:
            raise Refused('attach stack requires pristine owned API datastore')
        self.base_name=owner+'-traffic-pristine'
        api.call('PATCH','/config/vrfs',{self.base_name:{'id':int(os.environ['NGFW_VPP_TABLE_BASE'])+40}})
        initial=api.call('POST','/config/commit?comment=traffic-b-rest-baseline')
        self.warnings=check_commit(initial,changed_paths=('/vrfs',))
        self.revision=initial['revision']['id'];self.baseline=api.call('GET','/config')
        self.identity=private_identity()

    def save(self):
        self.output.parent.mkdir(mode=0o700,parents=True,exist_ok=True)
        fd=os.open(self.output,os.O_WRONLY|os.O_CREAT|os.O_TRUNC,0o600)
        with os.fdopen(fd,'w') as stream:
            json.dump({'owner':self.owner,'private_vpp':self.identity,'baseline_revision':self.revision,
                       'baseline_warnings':self.warnings,'events':self.events},stream,indent=2)

    def apply(self,request):
        desired=request['desired'];target=copy.deepcopy(self.baseline);target.update(desired)
        for ref,encoded in request.get('secrets',{}).items():
            kind,name=ref.split('/',1)
            material=base64.b64decode(encoded,validate=True).decode('utf8')
            response=self.api.call('POST','/secrets?replace=true',{'kind':kind,'name':name,'value':material})
            if response.get('ref')!=ref:raise Refused('secret reference mismatch')
        lock=self.api.call('GET','/config/lock')
        if lock.get('locked') or self.api.call('GET','/config/diff').get('changes'):
            raise Refused('foreign candidate present')
        self.api.call('PATCH','/config',merge_delta(self.api.call('GET','/config'),target))
        claimed=self.api.call('GET','/config/lock')
        if not claimed.get('locked') or not claimed.get('ownerId'):raise Refused('candidate ownership absent')
        candidate=self.api.call('GET','/config/candidate');sha=digest(candidate)
        observed=self.api.call('GET','/config/lock')
        if not observed.get('locked') or observed.get('ownerId')!=claimed.get('ownerId'):
            raise Refused('candidate owner changed')
        result=self.api.call('POST','/config/commit?comment='+quote(request['txn'],safe=''))
        if result.get('status')=='applied':
            check_commit(result,baseline_warnings=self.warnings,changed_paths=tuple('/'+k for k in desired))
            revision=result['revision']['id']
        elif result.get('status')=='unchanged':
            current=self.api.call('GET','/config/diff')
            if current.get('changes') or type(current.get('baseRevision')) is not int:
                raise Refused('unchanged commit has no applied revision')
            revision=current['baseRevision']
        else:raise Refused('REST commit did not apply')
        if digest(self.api.call('GET','/config'))!=sha:raise Refused('committed candidate hash mismatch')
        event={'txn':request['txn'],'status':result['status'],'revision':revision,
               'candidate_sha256':sha,'candidate_owner':claimed['ownerId'],'results':result.get('results',[]),
               'warnings':result.get('warnings',[]),'notApplied':result.get('notApplied',[])}
        self.events.append(event);self.save()
        # Adapt actual API object result enums to their protobuf JSON names.
        results=[]
        for item in result.get('results',[]):
            item=dict(item);item['op']='APPLY_OPERATION_'+item['op'].upper().replace('-','_')
            item['code']='OBJECT_RESULT_CODE_'+item['code'].upper().replace('-','_');results.append(item)
        return {'status':result['status'],'response':{'status':'APPLY_STATUS_APPLIED' if result['status']=='applied' else 'APPLY_STATUS_UNSPECIFIED',
                'results':results,'summary':result.get('summary',{})}}

    def close(self):
        self.api.call('POST','/config/discard')
        result=self.api.call('POST',f'/config/rollback/{self.revision}?comment=traffic-b-rest-rollback')
        check_commit(result,baseline_warnings=self.warnings,changed_paths=('/interfaces','/routing','/tunnels','/vpn'))
        if digest(self.api.call('GET','/config'))!=digest(self.baseline):raise Refused('baseline rollback mismatch')
        self.events.append({'txn':'rest-baseline-rollback','status':result['status'],'revision':result['revision']['id'],'baseline_sha256':digest(self.baseline)})
        self.api.call('PATCH','/config/vrfs',{self.base_name:None})
        check_commit(self.api.call('POST','/config/commit?comment=traffic-b-rest-cleanup'),baseline_warnings=self.warnings,changed_paths=('/vrfs',))
        self.save()


def main():
    parser=argparse.ArgumentParser();parser.add_argument('--slot',type=int,required=True)
    parser.add_argument('--agent-socket',required=True);parser.add_argument('--owner',required=True)
    parser.add_argument('--evidence',type=Path,required=True);args=parser.parse_args()
    private_identity()
    def terminate(signum,frame):raise SystemExit(128+signum)
    signal.signal(signal.SIGTERM,terminate)
    with product_stack(args.slot,target_socket=args.agent_socket,target_owner=args.owner) as (api,runtime,restart):
        controller=Controller(api,args.owner,args.evidence)
        try:
            print(json.dumps({'ready':True}),flush=True)
            for line in sys.stdin:
                if len(line)>1048576:raise Refused('bounded private request required')
                request=json.loads(line)
                if request.get('close'):break
                try:response=controller.apply(request)
                except (OSError,ValueError) as error:
                    print(json.dumps({'error':str(error) if isinstance(error,Refused) else type(error).__name__}),flush=True)
                    raise
                print(json.dumps(response),flush=True)
        finally:controller.close()

if __name__=='__main__':main()
