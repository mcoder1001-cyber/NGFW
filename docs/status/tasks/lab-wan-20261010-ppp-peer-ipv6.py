import os, subprocess, sys, tempfile, time, signal
from pathlib import Path
BASE=Path('/tmp/ngfw-lab-wan-20261010')
def run(*a,check=True):return subprocess.run(a,check=check,capture_output=True,text=True)
if len(sys.argv)==1:
 r=run('unshare','--net','--mount','--propagation','private',sys.executable,__file__,'child',check=False)
 print(r.stdout,end='');print(r.stderr,end='');sys.exit(r.returncode)
processes=[]
with tempfile.TemporaryDirectory(dir=BASE,prefix='ppp-') as directory:
 d=Path(directory); (d/'netns').mkdir();run('mount','--bind',str(d/'netns'),'/run/netns')
 secret=d/'secrets';secret.write_text('"w20" * "NGFW_TEST_PSK_w20" *\n');secret.chmod(0o600)
 for name in ('pap-secrets','chap-secrets'):run('mount','--bind',str(secret),'/etc/ppp/'+name)
 options=d/'options';options.write_text('auth\nrequire-pap\nipv6 ::1,::2\nlcp-echo-interval 1\nlcp-echo-failure 2\n')
 for ns in ('ns-w20-ppp-client','ns-w20-ppp-server'):
  run('ip','netns','add',ns);run('ip','-n',ns,'link','set','lo','up')
 run('ip','link','add','w20pc','type','veth','peer','name','w20ps')
 for dev,ns in [('w20pc','ns-w20-ppp-client'),('w20ps','ns-w20-ppp-server')]:
  run('ip','link','set',dev,'netns',ns);run('ip','-n',ns,'link','set',dev,'up')
 def start(args,label):
  log=(d/label).open('w');p=subprocess.Popen(args,stdout=log,stderr=log,start_new_session=True);processes.append((p,log));return p
 def stop(p):
  if p.poll() is None:
   os.killpg(p.pid,signal.SIGTERM)
   try:p.wait(timeout=5)
   except subprocess.TimeoutExpired:os.killpg(p.pid,signal.SIGKILL);p.wait()
 def up():return '100.64.20.10' in run('ip','-n','ns-w20-ppp-client','-4','address','show').stdout
 def wait(predicate,timeout=25):
  end=time.monotonic()+timeout
  while time.monotonic()<end:
   if predicate():return True
   time.sleep(.2)
  return False
 def client(password,label):return start(['ip','netns','exec','ns-w20-ppp-client','pppd','plugin','rp-pppoe.so','nic-w20pc','user','w20','password',password,'noauth','noipdefault','nodetach','ipv6','::2,::1','lcp-echo-interval','1','lcp-echo-failure','2'],label)
 try:
  server=start(['ip','netns','exec','ns-w20-ppp-server','pppoe-server','-F','-k','-I','w20ps','-L','100.64.20.1','-R','100.64.20.10','-N','1','-O',str(options)],'server-log')
  time.sleep(.5);p=client('NGFW_TEST_PSK_w20','client-log')
  assert wait(up), 'real PPPoE dial-up failed'
  print('REAL_RP_PPPOE_PAP_DIAL_UP PASS')
  time.sleep(.5)
  result=run('ip','netns','exec','ns-w20-ppp-client','ping','-n','-c','3','-W','2','100.64.20.1',check=False)
  print(result.stdout);print(result.stderr);print(run('ip','-n','ns-w20-ppp-client','route','show').stdout);print(run('ip','-n','ns-w20-ppp-server','route','show').stdout);print((d/'server-log').read_text());print((d/'client-log').read_text());assert result.returncode==0,'PPP peer ICMP failed';print('REAL_PPP_IPV4_PING PASS '+result.stdout.splitlines()[-2])
  run('ip','-n','ns-w20-ppp-client','-6','addr','add','2001:db8:20::2/64','dev','ppp0')
  run('ip','-n','ns-w20-ppp-server','-6','addr','add','2001:db8:20::1/64','dev','ppp0')
  time.sleep(1)
  result6=run('ip','netns','exec','ns-w20-ppp-client','ping','-6','-n','-c','3','-W','2','2001:db8:20::1',check=False)
  assert result6.returncode==0,'IPv6 real PPP peer ICMP failed';print('REAL_PPP_IPV6_PING PASS '+result6.stdout.splitlines()[-2])
  stop(server);assert wait(lambda:not up(),12),'PPP peer departure not detected';stop(p);print('REAL_PPP_PEER_LOSS_WITHDRAWAL PASS')
  server=start(['ip','netns','exec','ns-w20-ppp-server','pppoe-server','-F','-k','-I','w20ps','-L','100.64.20.1','-R','100.64.20.10','-N','1','-O',str(options)],'server-restart-log')
  time.sleep(.5);p=client('NGFW_TEST_PSK_w20_wrong','bad-log');assert wait(lambda:p.poll() is not None,25),'wrong-password client did not exit';assert not up(),'wrong password admitted';print('REAL_PPP_WRONG_PASSWORD_REJECTION PASS')
  time.sleep(1)
  p=client('NGFW_TEST_PSK_w20','reconnect-log');assert wait(up),'real PPP reconnect failed';print('REAL_PPP_RECONNECT PASS')
 finally:
  for p,log in reversed(processes):stop(p);log.close()
  for ns in ('ns-w20-ppp-client','ns-w20-ppp-server'):run('ip','netns','del',ns,check=False)
print('ISOLATED_NAMESPACE_CLEANUP PASS')
print('SCOPE real Linux PPP peer feasibility only; no NGFW carrier/VPP/API/PD acceptance')
