#!/usr/bin/env python3
"""Capture released RAM transition/network proof and remove only owned marker."""
import hashlib
import json
import os
import pathlib
import subprocess

PRIVATE=pathlib.Path('/root/Documents/Codex/2026-10-10/hardware/recovery-private/host-37')
COMMANDS={'addresses':['ip','-j','-details','address','show'],
          'routes-ipv4-all':['ip','-j','-4','route','show','table','all'],
          'routes-ipv6-all':['ip','-j','-6','route','show','table','all'],
          'rules-ipv4':['ip','-j','-4','rule','show'],'rules-ipv6':['ip','-j','-6','rule','show']}
REMOTE=r'''
import os,pathlib,subprocess,json,stat
assert os.stat('/').st_dev==51 and os.stat('/proc/1/root').st_dev==51
assert os.stat('/proc/1/exe').st_dev==51 and os.stat('/proc/3940/root').st_dev==51
assert os.stat('/proc/3866/root').st_dev==51
props=subprocess.check_output(['systemctl','show','ngfw-rescue.service','-p','MainPID','-p','ActiveState','-p','SubState','-p','FragmentPath','-p','DropInPaths'],text=True)
helper=subprocess.check_output(['systemctl','show','ngfw-rescue-runtime.service','-p','ActiveState','-p','SubState','-p','Result'],text=True)
assert 'MainPID=3866' in props and 'ActiveState=active' in props
assert 'ActiveState=active' in helper and 'SubState=exited' in helper and 'Result=success' in helper
st=os.stat('/run/sshd');assert stat.S_ISDIR(st.st_mode) and st.st_uid==0 and st.st_gid==0 and stat.S_IMODE(st.st_mode)==0o755
network={name:json.loads(subprocess.check_output(args,text=True)) for name,args in COMMANDS.items()}
marker={'present':os.path.lexists('/run/nextroot'),'removed':False}
if marker['present']:
    st=os.lstat('/run/nextroot');assert stat.S_ISLNK(st.st_mode) and st.st_uid==0
    assert os.readlink('/run/nextroot')=='/run/ngfwrescue'
    assert subprocess.check_output(['findmnt','-no','FSTYPE','/run'],text=True).strip()=='tmpfs'
    os.unlink('/run/nextroot');marker['removed']=True
assert not os.path.lexists('/run/nextroot')
print(json.dumps({'PID1_root_exe_device':51,'held_shell_pid':3940,'held_root_device':51,
                  'rescue_MainPID':3866,'service_properties':props,'helper_properties':helper,
                  'run_sshd_owner_mode':'0:0/0755','network':network,'owned_nextroot_marker':marker,
                  'corrective_repair_started':False},indent=2))
'''


def save(name,data):
    path=PRIVATE/name;assert not os.path.lexists(path)
    fd=os.open(path,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
    with os.fdopen(fd,'wb') as f:f.write(data);f.flush();os.fsync(f.fileno())


def addresses(data):
    return sorted((iface['ifname'],tuple(sorted(tuple(sorted((k,json.dumps(v,sort_keys=True))
                  for k,v in a.items() if k not in ['valid_life_time','preferred_life_time']))
                  for a in iface.get('addr_info',[])))) for iface in data)


def main():
    os.umask(0o077)
    payload='COMMANDS='+repr(COMMANDS)+'\n'+REMOTE
    p=subprocess.run(['ssh','-o','BatchMode=yes','-o','StrictHostKeyChecking=yes',
                      '-o','HostKeyAlias=172.30.126.37','-o','ConnectTimeout=15','-p','2222',
                      'root@172.30.126.37','LC_ALL=C /usr/bin/python3 -B -'],
                      input=payload.encode(),capture_output=True,timeout=90)
    save('post-transition-proof-ready-20261010.json',p.stdout)
    save('post-transition-proof-ready-20261010.stderr',p.stderr)
    assert p.returncode==0,'posttransition SSH/check failure retained privately'
    d=json.loads(p.stdout);comparisons={}
    for name,current in d['network'].items():
        original=json.loads((PRIVATE/(name+'.json')).read_text())
        comparisons[name]=addresses(original)==addresses(current) if name=='addresses' else original==current
    receipt={'network_comparisons':comparisons,'address_lifetime_counters_excluded':True,
             'source_sha256':hashlib.sha256(payload.encode()).hexdigest(),'ssh_exit':p.returncode,
             'stderr_bytes':len(p.stderr),'proof_bytes':len(p.stdout),
             'proof_sha256':hashlib.sha256(p.stdout).hexdigest()}
    save('post-transition-comparisons-ready-20261010.json',json.dumps(receipt,indent=2).encode())
    print(json.dumps(receipt))
    assert all(comparisons.values()),'allL3 mismatch; correction remains held'


if __name__=='__main__':
    main()
