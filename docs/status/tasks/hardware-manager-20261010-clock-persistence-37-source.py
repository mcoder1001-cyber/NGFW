import json,pathlib,subprocess,time,datetime,os,hashlib
p=pathlib.Path('/root/Documents/Codex/2026-10-10/hardware');host='172.30.126.37';expected='66e81101-eb23-4183-a845-098d0f69a11d';code="""import os,stat,struct,fcntl,json,time,pathlib,datetime,subprocess
p='/dev/rtc0';s=os.lstat(p);assert stat.S_ISCHR(s.st_mode) and s.st_uid==0 and (os.major(s.st_rdev),os.minor(s.st_rdev))==(247,0);assert struct.calcsize('@9i')==36
fd=os.open(p,os.O_RDONLY|os.O_NOFOLLOW);b=bytearray(36);fcntl.ioctl(fd,0x80247009,b,True);os.close(fd);v=struct.unpack('@9i',b);rtc=datetime.datetime(v[5]+1900,v[4]+1,v[3],v[2],v[1],v[0],tzinfo=datetime.timezone.utc)
q=subprocess.run(['timedatectl','show','-p','LocalRTC','--value'],capture_output=True,text=True);assert q.returncode==0 and q.stdout.strip()=='no';assert not os.path.lexists('/etc/adjtime');assert pathlib.Path('/sys/class/rtc/rtc0/hctosys').read_text().strip()=='1'
print(json.dumps({'boot_id':pathlib.Path('/proc/sys/kernel/random/boot_id').read_text().strip(),'system_epoch':time.time(),'RTC_epoch':rtc.timestamp(),'RTC_UTC':rtc.isoformat(),'RTC_native_ints':v,'RTC_device':[247,0],'RTC_hctosys':'1','LocalRTC':'no','adjtime_absent':True}))
"""
start=time.time();mono=time.monotonic();q=subprocess.run(['ssh','-o','BatchMode=yes','-o','StrictHostKeyChecking=yes','-o','ConnectTimeout=5','root@'+host,'python3 -'],input=code,text=True,capture_output=True,timeout=10);end=time.time();elapsed=time.monotonic()-mono;d={'host':host,'controller_start_epoch':start,'controller_end_epoch':end,'elapsed_seconds':elapsed,'SSH_exit':q.returncode,'stderr':q.stderr,'repair_SHA':'949fa67206df93e0a980a3141b2513cbfd5fbbfb5f66b597b4cc99a33628a1c4','expected_new_boot_id':expected,'read_only':True}
try:
 assert q.returncode==0 and not q.stderr and elapsed<10 and abs(end-start-elapsed)<1;d['observed']=json.loads(q.stdout);o=d['observed'];assert o['boot_id']==expected and start-5<=o['system_epoch']<=end+5 and start-5<=o['RTC_epoch']<=end+5;d['PASS']=True
except Exception as e:d.update(PASS=False,failure=type(e).__name__+':'+str(e),stdout=q.stdout)
b=json.dumps(d,indent=2).encode();f=p/('manager-clock-persistence37-'+datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')+'.json');fd=os.open(f,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
with os.fdopen(fd,'wb') as stream:stream.write(b);stream.flush();os.fsync(stream.fileno())
fd=os.open(p,os.O_DIRECTORY);os.fsync(fd);os.close(fd);print(f,hashlib.sha256(b).hexdigest(),len(b),d['PASS'])
