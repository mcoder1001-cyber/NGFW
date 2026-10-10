"""Remaining genuine PPP acceptance; only original owned peer and real API."""
import json, os, pathlib, signal, subprocess, time

def guarded_peer(record):
 proc=pathlib.Path('/proc')/str(record['pid'])
 stat=(proc/'stat').read_text().split(') ',1)[1].split()
 assert stat[19]==record['start'] and os.getpgid(record['pid'])==record['pid']
 assert os.readlink(proc/'exe')==record['exe'] and pathlib.Path(record['exe']).name=='pppoe-server'
 assert [item.decode() for item in (proc/'cmdline').read_bytes().split(b'\0') if item]==record['argv']
 ns=(proc/'ns/net').stat();expected=pathlib.Path('/run/netns/ns-w20-carrier-isp').stat()
 assert [ns.st_dev,ns.st_ino]==record['namespace']==[expected.st_dev,expected.st_ino]
 return stat[0]

def lifecycle(api,runtime,config,password,state,wait_up,commit,command,Refused):
 record=json.loads(os.environ['NGFW_WAN_PEER_RECORD'])
 assert guarded_peer(record)!='Z'
 os.killpg(record['pid'],signal.SIGTERM)
 end=time.monotonic()+15
 while time.monotonic()<end and state(api).get('phase')=='up':time.sleep(.2)
 if state(api).get('phase')=='up':raise Refused('genuine peer loss did not withdraw PPP state')
 negative=subprocess.run(['ip','netns','exec','ns-w20-carrier-lan','ping','-4','-n','-c','3','-W','1','100.64.20.1'],capture_output=True,text=True,timeout=8)
 if negative.returncode==0:raise Refused('peer-loss negative packets succeeded')
 print('REAL_API_PEER_LOSS_STATE_AND_PACKET_WITHDRAWAL PASS',flush=True)
 log_path=runtime/'peer-restart.private.log'
 descriptor=os.open(log_path,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
 log=os.fdopen(descriptor,'wb')
 peer=subprocess.Popen(record['launch'],stdout=log,stderr=log,start_new_session=True)
 started=time.monotonic()
 proc=pathlib.Path('/proc')/str(peer.pid)
 started_identity=(proc/'stat').read_text().split(') ',1)[1].split()[19]
 new=None
 try:
  proc=pathlib.Path('/proc')/str(peer.pid)
  # ip netns exec becomes the genuine server; inspect after that transition.
  end=started+2
  while time.monotonic()<end:
   if os.readlink(proc/'exe')==record['exe']:break
   time.sleep(.02)
  peer_ns=(proc/'ns/net').stat()
  new={'pid':peer.pid,'start':(proc/'stat').read_text().split(') ',1)[1].split()[19],'argv':[item.decode() for item in (proc/'cmdline').read_bytes().split(b'\0') if item],'exe':os.readlink(proc/'exe'),'namespace':[peer_ns.st_dev,peer_ns.st_ino]}
  guarded_peer(new)
  wait_up(api,budget=11)
  elapsed=time.monotonic()-started
  if elapsed>11:raise Refused('real server restart exceeded holdoff1+10sec')
  print('REAL_API_SERVER_RESTART_HOLDOFF_1_PLUS_10 PASS elapsed='+str(round(elapsed,3)),flush=True)
  bad=password+'incorrect'
  reply=api.call('POST','/secrets?replace=true',{'kind':'password','name':'w20-carrier-bad','value':bad})
  assert reply.get('ref')=='password/w20-carrier-bad'
  wrong=json.loads(json.dumps(config['w20ppp']));wrong['pppoe']['passwordRef']='password/w20-carrier-bad';wrong['pppoe']['reconnect']['maxFail']=1
  api.call('PATCH','/config/interfaces',{'w20ppp':wrong});_,warnings=commit(api,('/interfaces/w20ppp/pppoe/passwordRef','/interfaces/w20ppp/pppoe/reconnect/maxFail'))
  end=time.monotonic()+35
  while time.monotonic()<end:
   current=state(api)
   if current.get('phase')!='up' and current.get('failCount',0)>0 and 'authentication' in current.get('lastError','').lower():break
   time.sleep(.3)
  else:raise Refused('wrong credential did not report clear real authentication error')
  print('REAL_API_WRONG_PASSWORD_AUTHENTICATION_ERROR PASS',flush=True)
  api.call('PATCH','/config/interfaces',{'w20ppp':config['w20ppp']});commit(api,('/interfaces/w20ppp/pppoe/passwordRef','/interfaces/w20ppp/pppoe/reconnect/maxFail'),warnings)
  wait_up(api)
  end=time.monotonic()+20
  while time.monotonic()<end:
   current=state(api)
   if current.get('phase')=='up' and current.get('failCount')==0 and current.get('lastError')=='':break
   time.sleep(.3)
  else:raise Refused('restored credential did not clear runtime authentication error')
  command('ip','-n','ns-w20-carrier-isp','route','replace','10.20.1.0/24','dev','ppp0')
  command('ip','-n','ns-w20-carrier-isp','-6','route','replace','2001:db8:21::/64','dev','ppp0')
  print('REAL_API_CORRECT_PASSWORD_RECOVERY_CLEARS_ERROR PASS',flush=True)
  for route in ('/config','/config/candidate','/config/diff','/audit?limit=200'):
   response=json.dumps(api.call('GET',route))
   if password in response or bad in response:raise Refused('plaintext credential in real config/audit response')
  for path in runtime.glob('*.private.log'):
   data=path.read_bytes()
   if password.encode() in data or bad.encode() in data:raise Refused('plaintext credential in real raw product/peer log')
  print('REAL_API_CORRECT_AND_WRONG_PASSWORD_LOG_CONFIG_AUDIT_ABSENCE PASS',flush=True)
 finally:
  if peer.poll() is None:
   assert (proc/'stat').read_text().split(') ',1)[1].split()[19]==started_identity and os.getpgid(peer.pid)==peer.pid
   if new is not None:guarded_peer(new)
   os.killpg(peer.pid,signal.SIGTERM)
   try:peer.wait(timeout=5)
   except subprocess.TimeoutExpired:
    if peer.poll() is None:
     assert (proc/'stat').read_text().split(') ',1)[1].split()[19]==started_identity and os.getpgid(peer.pid)==peer.pid
     if new is not None:guarded_peer(new)
     os.killpg(peer.pid,signal.SIGKILL)
    peer.wait(timeout=5)
  log.close()
