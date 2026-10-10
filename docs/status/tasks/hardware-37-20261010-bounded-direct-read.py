import os,mmap,pathlib,subprocess,json,time,re
r={};counter=lambda:pathlib.Path('/sys/block/sda/device/ioerr_cnt').read_text().strip();d=subprocess.run(['/usr/bin/dd','--version'],capture_output=True,text=True);r['dd_implementation']={'exit':d.returncode,'stdout':d.stdout,'stderr':d.stderr}
r['before_ioerr']=counter();k1=subprocess.run(['dmesg'],capture_output=True,text=True).stdout.splitlines();r['bounded_direct_read']={'device':'/dev/sda2','open_flags':['O_RDONLY','O_DIRECT','O_CLOEXEC'],'method':'os.preadv into anonymous page-aligned mmap','block_bytes':1048576,'count':256,'bytes_read':0};start=time.monotonic()
try:
 fd=os.open('/dev/sda2',os.O_RDONLY|os.O_DIRECT|os.O_CLOEXEC)
 try:
  with mmap.mmap(-1,1048576) as buf:
   for i in range(256):
    n=os.preadv(fd,[buf],i*1048576)
    if n!=1048576:raise RuntimeError('short block read')
    r['bounded_direct_read']['bytes_read']+=n
 finally:os.close(fd)
 r['bounded_direct_read']['exit']=0
except OSError as e:r['bounded_direct_read'].update({'exit':1,'errno':e.errno,'error':e.strerror})
r['bounded_direct_read']['seconds']=time.monotonic()-start;r['after_ioerr']=counter();k2=subprocess.run(['dmesg'],capture_output=True,text=True).stdout.splitlines();r['new_storage_errors']=[l for l in k2 if l not in k1 and re.search(r'\bata[0-9]+\b|\bsda\b|\bscsi\b',l,re.I) and re.search(r'error|reset|uncorrect|\bUNC\b|failed|timeout',l,re.I)];print(json.dumps(r,indent=2))
