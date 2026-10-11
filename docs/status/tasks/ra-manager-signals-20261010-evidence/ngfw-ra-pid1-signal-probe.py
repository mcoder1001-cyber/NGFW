import socket,struct,time,json,os,stat
from pathlib import Path
socketpath=Path('/run/systemd/private')
for p in [socketpath,*socketpath.parents]:
 s=p.lstat()
 if s.st_uid!=0 or s.st_gid!=0 or s.st_mode&0o022 or stat.S_ISLNK(s.st_mode): raise SystemExit('protected socket path rejected')
pid1=Path('/proc/1/stat').read_text(); before=pid1[pid1.rindex(')')+1:].split()[19]
def align(a,n): a.extend(b'\0'*((-len(a))%n))
def string(a,v):
 align(a,4);b=v.encode();a.extend(struct.pack('<I',len(b))+b+b'\0')
def request(serial):
 h=bytearray()
 for key,sig,value in [(1,'o','/org/freedesktop/systemd1'),(2,'s','org.freedesktop.DBus.Properties'),(3,'s','Get'),(6,'s','org.freedesktop.systemd1'),(8,'g','ss')]:
  align(h,8); h.extend(bytes([key,1])+sig.encode()+b'\0')
  if sig=='g': h.extend(bytes([len(value)])+value.encode()+b'\0')
  else: string(h,value)
 length=len(h);align(h,8);b=bytearray();string(b,'org.freedesktop.systemd1.Manager');string(b,'Version')
 return struct.pack('<BBBBIII',108,1,2,1,len(b),serial,length)+h+b
out=[]
for attempt in range(5):
 s=socket.socket(socket.AF_UNIX,socket.SOCK_STREAM);s.settimeout(2);start=time.monotonic();s.connect(str(socketpath))
 peer=struct.unpack('3i',s.getsockopt(socket.SOL_SOCKET,socket.SO_PEERCRED,12))
 if peer!=(1,0,0): raise SystemExit('peer is not PID1 root')
 def raw(n):
  result=b''
  while len(result)<n:
   s.settimeout(max(.001,2-(time.monotonic()-start)))
   b,anc,flags,_=s.recvmsg(n-len(result),4096)
   if anc or flags & (socket.MSG_TRUNC|socket.MSG_CTRUNC) or not b: raise RuntimeError('unsafe reply')
   result+=b
  return result
 def line():
  b=b''
  while len(b)<256:
   b+=raw(1)
   if b.endswith(b'\r\n'): return b
  raise RuntimeError('auth bound')
 frames=[];total=0;error=None
 try:
  s.sendall(b'\0AUTH\r\n');r=line()
  if not r.startswith(b'REJECTED EXTERNAL'): raise RuntimeError('auth mechanisms rejected')
  s.sendall(b'AUTH EXTERNAL 30\r\n');r=line()
  if not r.startswith(b'OK ') or len(r)!=37: raise RuntimeError('UID0 authentication rejected')
  s.sendall(b'BEGIN\r\n')
  for serial in range(1,41):
   if time.monotonic()-start>1.9: break
   s.sendall(request(serial))
   while True:
    h=raw(16)
    order='<' if h[0]==108 else '>' if h[0]==66 else None
    if order is None or h[3]!=1: raise RuntimeError('header rejected')
    body,seq,fields=struct.unpack(order+'III',h[4:16]);size=16+((fields+7)//8)*8+body
    total+=size
    if size>16384 or total>16384: raise RuntimeError('reply byte bound')
    raw(size-16)
    frames.append({'type':h[1],'header':h.hex(),'size':size})
    if h[1] in (2,3): break
   time.sleep(.04)
 except (TimeoutError,RuntimeError) as e: error=str(e)
 finally: s.close()
 out.append({'attempt':attempt+1,'peer':peer,'duration':round(time.monotonic()-start,3),'frames':frames,'error':error})
pid1=Path('/proc/1/stat').read_text(); after=pid1[pid1.rindex(')')+1:].split()[19]
print(json.dumps({'pid1_start_unchanged':before==after,'fixed_method':'Properties.Get Manager.Version','subscribe_or_unit_changes':False,'connections':out},indent=2))
