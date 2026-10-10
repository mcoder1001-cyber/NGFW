#!/usr/bin/env python3
"""Read-only completion of the reviewed .37 staging startup-race check."""
import ast, hashlib, json, os, pathlib, subprocess
PRIVATE=pathlib.Path('/root/Documents/Codex/2026-10-10/hardware/recovery-private/host-37')
STAGE=pathlib.Path(__file__).with_name('hardware-37-20261010-stage.py')
SOURCE=r'''
import hashlib,json,os,pathlib,stat,subprocess,time
R=pathlib.Path('/run/ngfwrescue');ramdev=R.stat().st_dev;rootdev=os.stat('/').st_dev
assert (os.major(rootdev),os.minor(rootdev))==(8,2) and ramdev!=rootdev
unit='ngfw-rescue.service'
def run(args):
 p=subprocess.run(args,capture_output=True,text=True);assert p.returncode==0, args[0]+' failed'
 return {'command':args,'exit':p.returncode,'stdout':p.stdout,'stderr':p.stderr}
props=run(['systemctl','show',unit,'-p','MainPID','-p','ActiveState','-p','ControlGroup','-p','SurviveFinalKillSignal','-p','IgnoreOnIsolate','-p','DefaultDependencies','-p','PrivateMounts','-p','RootDirectory','-p','RequiresMountsFor','-p','Conflicts','-p','After','-p','Before','-p','FragmentPath','-p','DropInPaths','-p','WorkingDirectory'])
d=dict(line.split('=',1) for line in props['stdout'].splitlines() if '=' in line)
assert d['ActiveState']=='active' and d['SurviveFinalKillSignal']=='yes' and d['IgnoreOnIsolate']=='yes' and d['DefaultDependencies']=='no' and d['PrivateMounts']=='no'
assert all(not d[x] for x in ['RootDirectory','RequiresMountsFor','DropInPaths','WorkingDirectory'])
assert d['FragmentPath']=='/run/systemd/system/'+unit
pid=int(d['MainPID']);assert pid>1
p=pathlib.Path('/proc')/str(pid);deadline=time.monotonic()+15
while not all(os.stat(p/n).st_dev==ramdev for n in ['root','cwd','exe']):
 assert time.monotonic()<deadline,'RAM daemon readiness timeout'
 time.sleep(.1)
assert int(subprocess.check_output(['systemctl','show',unit,'-p','MainPID','--value'],text=True))==pid
assert os.readlink(p/'ns/mnt')==os.readlink('/proc/1/ns/mnt')
oldmaps=[];oldfds=[]
for line in (p/'maps').read_text().splitlines():
 f=line.split()
 if len(f)>4:
  a,b=f[3].split(':')
  if (int(a,16),int(b,16))==(8,2):oldmaps.append(f[:5])
for fd in (p/'fd').iterdir():
 try:
  st=fd.stat()
  if (stat.S_ISREG(st.st_mode) or stat.S_ISDIR(st.st_mode)) and st.st_dev==rootdev:oldfds.append(fd.name)
 except FileNotFoundError:pass
assert not oldmaps and not oldfds
units={}
for path,expected in EXPECTED_UNITS.items():
 data=pathlib.Path(path).read_bytes();assert data.decode()==expected,'staged unit/config differs'
 units[path]={'sha256':hashlib.sha256(data).hexdigest(),'bytes':len(data)}
checks=[]
for binary in ['/usr/lib/systemd/systemd','/usr/lib/systemd/systemd-executor','/usr/lib/systemd/systemd-shutdown','/usr/sbin/sshd','/usr/lib/openssh/sshd-session','/usr/lib/openssh/sshd-auth','/usr/sbin/e2fsck','/usr/sbin/e2image','/usr/sbin/e2undo','/usr/bin/python3','/usr/bin/bash']:
 x=run(['chroot',str(R),'/usr/bin/ldd',binary]);assert 'not found' not in x['stdout'];checks.append(x)
checks.append(run(['chroot',str(R),'/usr/sbin/sshd','-t','-f','/etc/ssh/sshd_config']))
x=run(['systemd-analyze','verify','--root='+str(R),'ngfw-rescue.service','ngfw-rescue-runtime.service','default.target','basic.target']);assert not x['stderr'].strip();checks.append(x)
probe=subprocess.run([str(R/'usr/local/sbin/block-check'),'/dev/sda2','8','2'],capture_output=True,text=True);assert probe.returncode==3
assert subprocess.check_output(['ss','-H','-ltn','sport = :22'],text=True).strip()
assert subprocess.check_output(['ss','-H','-ltn','sport = :2222'],text=True).strip()
assert not os.path.lexists('/run/nextroot')
network={name:json.loads(subprocess.check_output(args,text=True)) for name,args in NETWORK_COMMANDS.items()}
print(json.dumps({'stage_validated_after_startup_race':True,'main_pid':pid,'ram_major_minor':f'{os.major(ramdev)}:{os.minor(ramdev)}','root_cwd_exe_ram':True,'same_pid1_mount_namespace':True,'oldroot_maps':oldmaps,'oldroot_file_directory_fds':oldfds,'service_properties':d,'units':units,'checks':checks,'mounted_negative_guard_exit':probe.returncode,'mounted_negative_guard_stdout':probe.stdout,'original22_and_rescue2222_listening':True,'nextroot_absent':True,'network':network,'transition_executed':False,'repair_executed':False}))
'''

def main():
 parsed=ast.parse(STAGE.read_text())
 remote=next(ast.literal_eval(x.value) for x in parsed.body if isinstance(x,ast.Assign) and any(isinstance(y,ast.Name) and y.id=='REMOTE' for y in x.targets))
 values={x.targets[0].id:ast.literal_eval(x.value) for x in ast.parse(remote).body if isinstance(x,ast.Assign) and isinstance(x.targets[0],ast.Name) and x.targets[0].id in ['MOUNT','HOST_UNIT','RAM_UNIT','PREP_UNIT','SSH_CONFIG']}
 expected={'/run/systemd/system/run-ngfwrescue.mount':values['MOUNT'],'/run/systemd/system/ngfw-rescue.service':values['HOST_UNIT'],'/run/ngfwrescue/etc/systemd/system/ngfw-rescue.service':values['RAM_UNIT'],'/run/ngfwrescue/etc/systemd/system/ngfw-rescue-runtime.service':values['PREP_UNIT'],'/run/ngfwrescue/etc/ssh/sshd_config':values['SSH_CONFIG']}
 commands={'addresses':['ip','-j','-details','address','show'],'routes-ipv4-all':['ip','-j','-4','route','show','table','all'],'routes-ipv6-all':['ip','-j','-6','route','show','table','all'],'rules-ipv4':['ip','-j','-4','rule','show'],'rules-ipv6':['ip','-j','-6','rule','show']}
 payload='EXPECTED_UNITS='+repr(expected)+'\nNETWORK_COMMANDS='+repr(commands)+'\n'+SOURCE
 out=PRIVATE/'ram-stage-validation-20261010.json';err=PRIVATE/'ram-stage-validation-20261010.stderr'
 with out.open('wb') as a,err.open('wb') as b:
  os.chmod(out,0o600);os.chmod(err,0o600)
  p=subprocess.run(['ssh','-o','BatchMode=yes','-o','StrictHostKeyChecking=yes','-o','ConnectTimeout=15','root@172.30.126.37','python3 -'],input=payload.encode(),stdout=a,stderr=b,timeout=90)
 assert p.returncode==0,'Validation SSH exit '+str(p.returncode)+'; private stderr bytes '+str(err.stat().st_size)
 j=json.loads(out.read_text()); comparisons={}
 def addresses(data):
  return sorted((iface['ifname'],tuple(sorted(tuple(sorted((k,json.dumps(v,sort_keys=True)) for k,v in a.items() if k not in ['valid_life_time','preferred_life_time'])) for a in iface.get('addr_info',[])))) for iface in data)
 for name,now in j['network'].items():
  baseline=json.loads((PRIVATE/(name+'.json')).read_text())
  comparisons[name]=addresses(baseline)==addresses(now) if name=='addresses' else baseline==now
 assert all(comparisons.values()),'Saved original network snapshot differs; no continuation'
 j['original_snapshot_network_comparisons']=comparisons
 j['address_lifetime_counters_excluded']=True
 out.write_text(json.dumps(j,indent=2)+'\n');os.chmod(out,0o600)
 print(json.dumps({'stage_validated_after_startup_race':True,'main_pid':j['main_pid'],'original_network_address_configuration_routes_rules_unchanged':comparisons,'original22_and_rescue2222_listening':True,'nextroot_absent':True,'transition_executed':False,'repair_executed':False,'receipt_sha256':hashlib.sha256(out.read_bytes()).hexdigest()}))
if __name__=='__main__':main()
