#!/usr/bin/env python3
"""ROOT-only UTC/RTC repair; default inspect, separately guarded reboot."""
import argparse,ast,datetime,hashlib,json,os,pathlib,re,select,shlex,stat,subprocess,time
OUTPUT=pathlib.Path('/root/Documents/Codex/2026-10-10/hardware')
BOOTSTRAP=r'''import sys
PROGRAM_INPUT=sys.stdin.buffer
def frame(limit):
 def exact(n):
  out=bytearray()
  while len(out)<n:
   b=PROGRAM_INPUT.read(n-len(out))
   if not b:raise EOFError('short frame')
   out.extend(b)
  return bytes(out)
 h=exact(16)
 if any(c not in b'0123456789abcdef' for c in h):raise ValueError('invalid frame length')
 n=int(h,16)
 if not 0<n<=limit:raise ValueError('frame bound')
 return exact(n)
exec(frame(8388608).decode('utf-8'))
'''
REMOTE=r'''
import calendar,datetime,fcntl,hashlib,json,os,pathlib,platform,re,socket,stat,struct,subprocess,time
result={'host':HOST,'mode':MODE,'boot_proof_SHA':BOOT_SHA,'inspect_proof_SHA':INSPECT_SHA,'repair_proof_SHA':REPAIR_SHA,'commands':[],'no_timezone_NTP_service_network_certificate_PCI_or_package_change':True,'stage':'begin'}
RECORD=pathlib.Path('/var/lib/ngfw-install-recovery')/('hardware-manager-20261010-clock-'+HOST+'-'+STAMP)
RD=0x80247009;SET=0x4024700a;FMT='@9i'
def sha(b):return hashlib.sha256(b).hexdigest()
def run(a,timeout=10):
 q=subprocess.run(a,capture_output=True,text=True,timeout=timeout);result['commands'].append({'argv':a,'exit':q.returncode,'stdout_bytes':len(q.stdout.encode()),'stdout_SHA':sha(q.stdout.encode()),'stderr':q.stderr[:4096]});assert q.returncode==0,'command failed '+repr(a);return q.stdout.strip()
def trusted(p):
 fd=os.open('/',os.O_DIRECTORY|os.O_NOFOLLOW)
 try:
  for c in pathlib.Path(p).parts[1:]:
   n=os.open(c,os.O_DIRECTORY|os.O_NOFOLLOW|os.O_CLOEXEC,dir_fd=fd);os.close(fd);fd=n;s=os.fstat(fd);assert s.st_uid==s.st_gid==0 and not s.st_mode&0o022
 finally:os.close(fd)
def read(p,limit=4194304):
 p=pathlib.Path(p);trusted(p.parent);fd=os.open(p,os.O_RDONLY|os.O_NOFOLLOW|os.O_CLOEXEC)
 with os.fdopen(fd,'rb') as f:
  s=os.fstat(f.fileno());assert stat.S_ISREG(s.st_mode) and s.st_uid==0 and not s.st_mode&0o022 and s.st_nlink==1;b=f.read(limit+1);assert len(b)<=limit;return b
def synced(p,b):
 fd=os.open(p,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW|os.O_CLOEXEC,0o600)
 with os.fdopen(fd,'wb') as f:f.write(b);f.flush();os.fsync(f.fileno())
 fd=os.open(p.parent,os.O_DIRECTORY|os.O_NOFOLLOW);os.fsync(fd);os.close(fd)
def metadata(p):
 p=pathlib.Path(p);s=p.lstat();return {'dev':s.st_dev,'inode':s.st_ino,'uid':s.st_uid,'gid':s.st_gid,'mode':stat.S_IMODE(s.st_mode),'type':'symlink' if stat.S_ISLNK(s.st_mode) else 'regular','link':os.readlink(p) if stat.S_ISLNK(s.st_mode) else None,'content_SHA':sha(p.read_bytes())}
def units():
 out={}
 for u in BOOT['after']['units']:
  q=dict(x.split('=',1) for x in run(['systemctl','show',u,'-p','ActiveState','-p','MainPID','-p','NRestarts','-p','UnitFileState','-p','FragmentPath','-p','Requires','-p','After']).splitlines())
  for k in ['Requires','After']:q[k]=sorted(q[k].split())
  out[u]=q
 assert out==BOOT['after']['units'],'boot unit identity changed'
 return out
def l3():return {'addresses':{x['ifname']:x.get('addr_info',[]) for x in json.loads(run(['ip','-j','addr']))},'routes4':json.loads(run(['ip','-j','-4','route','show','table','all'])),'routes6':json.loads(run(['ip','-j','-6','route','show','table','all'])),'rules4':json.loads(run(['ip','-j','-4','rule'])),'rules6':json.loads(run(['ip','-j','-6','rule']))}
def nft(d):
 if isinstance(d,dict):return {k:nft({a:b for a,b in v.items() if a not in ['packets','bytes']}) if k=='counter' and isinstance(v,dict) else nft(v) for k,v in d.items() if k not in ['metainfo','handle']}
 if isinstance(d,list):return [nft(x) for x in d if not(isinstance(x,dict) and 'metainfo' in x)]
 return d
BAD=re.compile(r'EXT4-fs error|I/O error|Buffer I/O|blk_update_request|\bUNC\b|hard resetting link|failed command|ata\d.*(?:error|reset)|sd\s+\S+.*(?:error|fail)',re.I)
def kernel():
 q=subprocess.run(['dmesg','--color=never'],capture_output=True,text=True,timeout=10);assert q.returncode==0 and not q.stderr;bad=[x for x in q.stdout.splitlines() if BAD.search(x)];assert bad==[],'storage error in fresh kernel';return q.stdout,{'SHA':sha(q.stdout.encode()),'lines':len(q.stdout.splitlines()),'new_storage_errors':bad}
def protect():
 boot=pathlib.Path('/proc/sys/kernel/random/boot_id').read_text().strip();assert boot==BOOT['after']['boot_id']
 assert (os.major(os.stat('/').st_dev),os.minor(os.stat('/').st_dev))==(8,2) and not os.path.lexists('/run/nextroot') and not os.path.lexists('/run/ngfwrescue')
 network=l3();assert network==BOOT['after']['network'];p=pathlib.Path('/sys/class/net')/MGMT_IF/'device';g=(p/'iommu_group').resolve(strict=True);assert p.resolve().name==MGMT_PCI and (p/'driver').resolve().name=='igc' and g.name==MGMT_GROUP and sorted(x.name for x in (g/'devices').iterdir())==[MGMT_PCI]
 with socket.create_connection(('172.30.126.195',22),timeout=5):pass
 drivers={}
 for n,q in BOOT['after']['VFIO_devices'].items():
  p=pathlib.Path('/sys/bus/pci/devices')/q['PCI'];g=(p/'iommu_group').resolve(strict=True);assert q['PCI']!=MGMT_PCI and (p/'driver').resolve().name=='vfio-pci' and (p/'driver_override').read_text().strip()=='vfio-pci' and g.name==q['group'] and sorted(x.name for x in (g/'devices').iterdir())==[q['PCI']];drivers[n]={'PCI':q['PCI'],'group':g.name,'driver':'vfio-pci'}
 assert len(drivers)==(17 if HOST=='211' else 7) and pathlib.Path('/sys/module/vfio/parameters/enable_unsafe_noiommu_mode').read_text().strip()=='N';ids=pathlib.Path('/sys/module/vfio_pci/parameters/ids');assert not ids.exists() or ids.read_text().strip()==''
 sysctls={k:(pathlib.Path('/proc/sys')/k.replace('.','/')).read_text().strip() for k in BOOT['after']['sysctls']};assert sysctls==BOOT['after']['sysctls']
 dns={p:{'SHA':sha(pathlib.Path(p).read_bytes()),'realpath':str(pathlib.Path(p).resolve()),'link':os.readlink(p) if pathlib.Path(p).is_symlink() else None} for p in BOOT['after']['DNS']};assert dns==BOOT['after']['DNS']
 rules=json.loads(run(['nft','-j','list','ruleset']));assert nft(rules)==nft(BOOT['after']['nft'])
 assert sha(read('/etc/vpp/startup.conf'))==BOOT['after']['startup_SHA'] and sha(read('/usr/sbin/ngfw-agent',67108864))=='b3c7cc10671abcab6d11c7dce12323f284c64822a15cfda1c46ba78803283744' and sha(read('/usr/lib/ngfw/bin/ngfw-startupgen',67108864))=='55e490bad1c302505231df105a1d516cb074678b38b85e28740c399020aac619'
 packages=run(['dpkg-query','-W','-f','${binary:Package}\t${Version}\t${db:Status-Status}\n','ngfw-agent','ngfw-api','ngfw-web','ngfw-meta']);assert len(packages.splitlines())==4 and all(x.split('\t')[1:]==['0.1.0~dev+97ae88ee5b6a','installed'] for x in packages.splitlines())
 current=units();ssh=dict(x.split('=',1) for x in run(['systemctl','show','ssh.service','-p','ActiveState','-p','MainPID','-p','NRestarts']).splitlines());assert ssh['ActiveState']=='active' and int(ssh['MainPID'])>1
 io=pathlib.Path('/sys/block/sda/device/ioerr_cnt').read_text().strip();assert int(io,16)==int(BOOT['after']['storage_ioerr'],16)
 return {'boot_id':boot,'network':network,'management':{'interface':MGMT_IF,'PCI':MGMT_PCI,'driver':'igc','group':MGMT_GROUP},'core_units':{u:current[u] for u in ['vpp.service','ngfw-agent.service','ngfw-api.service','nginx.service']},'all21_units_SHA':sha(json.dumps(current,sort_keys=True).encode()),'all21_units_match_boot':True,'SSH':ssh,'VFIO_devices':drivers,'sysctls':sysctls,'DNS':dns,'nft_semantic_SHA':sha(json.dumps(nft(rules),sort_keys=True).encode()),'startup_SHA':BOOT['after']['startup_SHA'],'storage_ioerr':io,'timezone_file':metadata('/etc/localtime'),'certificate':metadata('/etc/ngfw/tls/server.crt')}
def rtc_open(write=False):
 assert platform.machine()=='x86_64' and struct.calcsize('i')==4 and struct.calcsize(FMT)==36
 p=pathlib.Path('/dev/rtc0');s=p.lstat();assert stat.S_ISCHR(s.st_mode) and s.st_uid==0 and (os.major(s.st_rdev),os.minor(s.st_rdev))==(247,0)
 name=pathlib.Path('/sys/class/rtc/rtc0/name').read_text().strip();assert name==RTC_NAME and pathlib.Path('/sys/class/rtc/rtc0/dev').read_text().strip()=='247:0' and pathlib.Path('/sys/class/rtc/rtc0/hctosys').read_text().strip()=='1'
 assert run(['timedatectl','show','-p','LocalRTC','--value'])=='no'
 adj=pathlib.Path('/etc/adjtime');adjmeta=None
 if os.path.lexists(adj):
  b=read(adj,4096);lines=b.decode('ascii').splitlines();assert len(lines)==3 and lines[2]=='UTC';adjmeta=metadata(adj)
 fd=os.open(p,(os.O_RDWR if write else os.O_RDONLY)|os.O_NOFOLLOW|os.O_CLOEXEC);f=os.fstat(fd);assert (f.st_dev,f.st_ino,f.st_rdev)==(s.st_dev,s.st_ino,s.st_rdev)
 return fd,{'dev':f.st_dev,'inode':f.st_ino,'uid':f.st_uid,'gid':f.st_gid,'rdev':[247,0],'rtc_name':name,'LocalRTC':'no','adjtime':adjmeta}
def rtc_read(fd):
 b=bytearray(36);fcntl.ioctl(fd,RD,b,True);v=struct.unpack(FMT,b);dt=datetime.datetime(v[5]+1900,v[4]+1,v[3],v[2],v[1],v[0],tzinfo=datetime.timezone.utc);return {'native_ints':list(v),'UTC':dt.isoformat(),'epoch':dt.timestamp()}
def rtc_pack(epoch):
 g=time.gmtime(epoch);return struct.pack(FMT,g.tm_sec,g.tm_min,g.tm_hour,g.tm_mday,g.tm_mon-1,g.tm_year-1900,(g.tm_wday+1)%7,g.tm_yday-1,0)
def beacon():
 print(json.dumps({'clock_ready':True,'host':HOST,'nonce':NONCE}),flush=True);q=json.loads(frame(4096));now=time.monotonic()
 assert q['nonce']==NONCE and isinstance(q['epoch'],(float,int)) and 0<=q['controller_elapsed']<=30 and abs(q['controller_realtime']-q['epoch'])<=1
 assert datetime.datetime.fromtimestamp(q['epoch'],datetime.timezone.utc).date()==datetime.date(2026,10,10)
 def authoritative():
  elapsed=time.monotonic()-now;assert q['controller_elapsed']+elapsed<=30,'UTC beacon exceeded30s';return q['epoch']+elapsed
 return authoritative,q
def finish_health(before_kernel,before_snapshot):
 after=protect();assert after==before_snapshot,'protected state changed';text,summary=kernel();assert text.startswith(before_kernel),'kernel prefix lost';result.update(after=after,kernel_after=summary,new_storage_errors=[],protected_unchanged=True)
def main():
 before=protect();text,k=kernel();assert text.startswith(BOOT['kernel_after']),'boot kernel baseline lost';result.update(before=before,kernel_before=k,stage='protected')
 fd,identity=rtc_open();original=rtc_read(fd);os.close(fd);result.update(RTC_identity=identity,RTC_before=original,system_epoch_before=time.time())
 if MODE=='inspect':finish_health(text,before);result.update(read_only=True,PASS=True,stage='complete');return
 if MODE=='repair':
  assert INSPECT['mode']=='inspect' and INSPECT['PASS'] and INSPECT['protected_unchanged'] and INSPECT['boot_proof_SHA']==BOOT_SHA and INSPECT['before']==before and INSPECT['RTC_identity']==identity
  trusted(RECORD.parent);s=RECORD.parent.lstat();assert stat.S_IMODE(s.st_mode)==0o700 and (os.major(s.st_dev),os.minor(s.st_dev))==(8,2);assert not os.path.lexists(RECORD);RECORD.mkdir(mode=0o700);d=os.open(RECORD.parent,os.O_DIRECTORY|os.O_NOFOLLOW);os.fsync(d);os.close(d)
  original_record={'task':'hardware-manager-20261010-clock-'+HOST,'boot_proof_SHA':BOOT_SHA,'inspect_proof_SHA':INSPECT_SHA,'source_SHA':SOURCE_SHA,'original_RTC':original,'RTC_identity':identity,'original_system_epoch':result['system_epoch_before'],'protected_before':before};raw=json.dumps(original_record,sort_keys=True).encode();synced(RECORD/'before.json',raw);result.update(record=str(RECORD),original_record_SHA=sha(raw))
  authoritative,q=beacon();result['UTC_beacon']=q;fd,new_identity=rtc_open(True)
  try:
   assert new_identity==identity;rtc_read(fd);epoch=authoritative();time.clock_settime(time.CLOCK_REALTIME,epoch);result['system_clock_set']=True
   epoch=authoritative();fcntl.ioctl(fd,SET,rtc_pack(epoch));result['RTC_set']=True;readback=rtc_read(fd);assert abs(readback['epoch']-authoritative())<=5 and abs(time.time()-authoritative())<=5;result['RTC_after_write']=readback
  finally:os.close(fd)
  finish_health(text,before);fd,check=rtc_open();result['RTC_final']=rtc_read(fd);os.close(fd);assert check==identity;result['system_epoch_final']=time.time();assert abs(result['RTC_final']['epoch']-authoritative())<=5
  completed=json.dumps({'source_SHA':SOURCE_SHA,'boot_proof_SHA':BOOT_SHA,'original_record_SHA':result['original_record_SHA'],'UTC_beacon':q,'RTC_final':result['RTC_final'],'system_epoch_final':result['system_epoch_final']},sort_keys=True).encode();synced(RECORD/'repair-complete.json',completed);result.update(repair_record_SHA=sha(completed),RTC_UTC_repaired=True,PASS=True,stage='complete')
 else:
  assert MODE=='verification-reboot' and REPAIR['mode']=='repair' and REPAIR['PASS'] and REPAIR['controller_UTC_verified'] and REPAIR['RTC_UTC_repaired'] and REPAIR['boot_proof_SHA']==BOOT_SHA and REPAIR['after']==before and REPAIR['RTC_identity']==identity
  record=pathlib.Path(REPAIR['record']);assert re.fullmatch(r'/var/lib/ngfw-install-recovery/hardware-manager-20261010-clock-'+HOST+r'-[0-9]{8}T[0-9]{6}Z-[0-9a-f]{8}',str(record));trusted(record);assert sha(read(record/'before.json'))==REPAIR['original_record_SHA'] and sha(read(record/'repair-complete.json'))==REPAIR['repair_record_SHA']
  authoritative,q=beacon();assert abs(original['epoch']-authoritative())<=5 and abs(time.time()-authoritative())<=5;finish_health(text,before)
  marker=json.dumps({'source_SHA':SOURCE_SHA,'repair_proof_SHA':REPAIR_SHA,'boot_id':before['boot_id'],'UTC_beacon':q},sort_keys=True).encode();synced(record/'verification-reboot-request.json',marker);result.update(reboot_request_recorded=True,reboot_marker_SHA=sha(marker),verification_reboot_requested=True);run(['systemctl','reboot'],20);result.update(reboot_request_exit=0,PASS=True,stage='reboot-requested')
try:main()
except Exception as e:result.update(PASS=False,failure={'type':type(e).__name__,'message':str(e)})
finally:print(json.dumps(result),flush=True)
raise SystemExit(0 if result.get('PASS') else 2)
'''
def private(p,sha):
 p=pathlib.Path(p);assert p.parent==OUTPUT and not p.is_symlink();s=p.stat();assert stat.S_ISREG(s.st_mode) and s.st_uid==0 and stat.S_IMODE(s.st_mode)==0o600;b=p.read_bytes();assert re.fullmatch('[0-9a-f]{64}',sha) and hashlib.sha256(b).hexdigest()==sha;return json.loads(b)
def save(p,b):
 assert p.parent==OUTPUT;fd=os.open(p,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
 with os.fdopen(fd,'wb') as f:f.write(b);f.flush();os.fsync(f.fileno())
 fd=os.open(OUTPUT,os.O_DIRECTORY|os.O_NOFOLLOW);os.fsync(fd);os.close(fd);return {'file':str(p),'bytes':len(b),'SHA':hashlib.sha256(b).hexdigest()}
def next_line(q,buf,deadline):
 while b'\n' not in buf:
  remaining=deadline-time.monotonic();assert remaining>0,'clock transport deadline'
  ready,_,_=select.select([q.stdout.fileno()],[],[],remaining);assert ready,'clock transport deadline';b=os.read(q.stdout.fileno(),65536)
  if not b:return bytes(buf),bytearray()
  buf.extend(b);assert len(buf)<=2097152,'clock compact output bound'
 line,_,tail=buf.partition(b'\n');return bytes(line),bytearray(tail)
def main():
 os.umask(0o077);p=argparse.ArgumentParser();p.add_argument('--host',choices=['211','37'],required=True);p.add_argument('--boot-proof',required=True);p.add_argument('--boot-sha',required=True);p.add_argument('--inspect-proof');p.add_argument('--inspect-sha');p.add_argument('--repair-proof');p.add_argument('--repair-sha');p.add_argument('--validate-source-only',action='store_true');g=p.add_mutually_exclusive_group();g.add_argument('--repair',action='store_true');g.add_argument('--verification-reboot',action='store_true');a=p.parse_args();mode='repair' if a.repair else 'verification-reboot' if a.verification_reboot else 'inspect'
 boot=private(a.boot_proof,a.boot_sha);assert boot['PASS'] and boot['mode']=='observe-boot' and boot['host']==a.host and boot['stage']=='complete' and boot['new_boot_observed'] and boot['storage_counter_epoch']=='fresh-boot' and boot['new_storage_errors']==[] and len(boot['after']['units'])==21 and boot['before']['boot_id']==boot['after']['boot_id']
 inspection=private(a.inspect_proof,a.inspect_sha) if a.repair else None;repair=private(a.repair_proof,a.repair_sha) if a.verification_reboot else None
 source_SHA=hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest();stamp=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')+'-'+os.urandom(4).hex();nonce=os.urandom(16).hex()
 fields={'HOST':a.host,'MODE':mode,'BOOT':boot,'BOOT_SHA':a.boot_sha,'INSPECT':inspection,'INSPECT_SHA':a.inspect_sha,'REPAIR':repair,'REPAIR_SHA':a.repair_sha,'SOURCE_SHA':source_SHA,'STAMP':stamp,'NONCE':nonce,'MGMT_IF':'enp4s0' if a.host=='211' else 'enp12s0','MGMT_PCI':'0000:04:00.0' if a.host=='211' else '0000:0c:00.0','MGMT_GROUP':'28' if a.host=='211' else '58','RTC_NAME':'rtc_cmos rtc_cmos' if a.host=='211' else 'rtc_cmos 00:00'}
 code=('\n'.join(k+'='+repr(v) for k,v in fields.items())+'\n'+REMOTE).encode();ast.parse(code);ast.parse(BOOTSTRAP);assert len(code)<=8388608
 if a.validate_source_only:print(json.dumps({'source_SHA':source_SHA,'AST':3,'generated_bytes':len(code),'target_contacted':False}));return
 errpath=OUTPUT/('manager-clock'+a.host+'-'+mode+'-'+stamp+'.stderr');errfd=os.open(errpath,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600);raw=bytearray();response=None;failure=None;elapsed=0
 with os.fdopen(errfd,'wb') as stderr:
  base=time.time();mono=time.monotonic();assert datetime.datetime.fromtimestamp(base,datetime.timezone.utc).date()==datetime.date(2026,10,10)
  q=subprocess.Popen(['ssh','-o','BatchMode=yes','-o','StrictHostKeyChecking=yes','-o','ConnectTimeout=15','root@'+('172.30.110.211' if a.host=='211' else '172.30.126.37'),'python3 -u -c '+shlex.quote(BOOTSTRAP)],stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=stderr)
  try:
   q.stdin.write(format(len(code),'016x').encode()+code);q.stdin.flush();buf=bytearray();deadline=mono+30;beacon_sent=False
   while True:
    line,buf=next_line(q,buf,deadline);raw.extend(line+b'\n');assert line,'empty clock response';item=json.loads(line)
    if item.get('clock_ready'):
     assert mode!='inspect' and not beacon_sent and item['host']==a.host and item['nonce']==nonce;now=time.monotonic();elapsed=now-mono;assert elapsed<=30;epoch=base+elapsed;realtime=time.time();assert abs(realtime-epoch)<=1
     beacon=json.dumps({'nonce':nonce,'epoch':epoch,'controller_elapsed':elapsed,'controller_realtime':realtime}).encode();q.stdin.write(format(len(beacon),'016x').encode()+beacon);q.stdin.flush();beacon_sent=True;continue
    response=item;break
   q.stdin.close();exitcode=q.wait(timeout=max(1,deadline-time.monotonic()))
   if mode=='repair' and response.get('PASS'):
    expected=base+time.monotonic()-mono;assert time.monotonic()-mono<=30 and abs(time.time()-expected)<=1 and abs(response['system_epoch_final']-expected)<=5 and abs(response['RTC_final']['epoch']-expected)<=5;response.update(controller_UTC_verified=True,controller_system_delta_seconds=response['system_epoch_final']-expected,controller_RTC_delta_seconds=response['RTC_final']['epoch']-expected)
  except Exception as e:
   failure={'type':type(e).__name__,'message':str(e)};exitcode=q.poll();response=response or {'host':a.host,'mode':mode,'PASS':False};response.update(PASS=False,controller_failure=failure,SSH_exit_observed=exitcode,in_flight_if_SSH_not_exited=exitcode is None)
   if q.stdin and not q.stdin.closed:q.stdin.close()
   # Do not signal/kill a possibly in-flight device write or shutdown.
  stderr.flush();os.fsync(stderr.fileno())
 response['controller_elapsed_seconds']=time.monotonic()-mono;response['SSH_exit']=exitcode;response['source_SHA']=source_SHA
 receipt=save(OUTPUT/('manager-clock'+a.host+'-'+mode+'-'+stamp+'.json'),json.dumps(response,indent=2).encode());transport=save(OUTPUT/('manager-clock'+a.host+'-'+mode+'-'+stamp+'.transport'),bytes(raw));fd=os.open(OUTPUT,os.O_DIRECTORY);os.fsync(fd);os.close(fd)
 print(json.dumps({'receipt':receipt,'transport':transport,'stderr':{'file':str(errpath),'bytes':errpath.stat().st_size},'SSH_exit':exitcode,'PASS':response.get('PASS',False)}));raise SystemExit(0 if response.get('PASS') and exitcode==0 else 2)
if __name__=='__main__':main()
