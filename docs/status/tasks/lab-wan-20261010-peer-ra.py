"""Genuine peer RA; restart only our dnsmasq when ppp0's real ifindex changes."""
import json, os, signal, subprocess, time
from pathlib import Path
assert os.geteuid()==0
expected=json.loads(os.environ['NGFW_WAN_ISP_NAMESPACE'])
info=Path('/proc/self/ns/net').stat()
assert [info.st_dev,info.st_ino]==expected
assert os.readlink('/proc/self/ns/net')!=os.readlink('/proc/1/ns/net')
stopping=False
child=None
identity=None

def stop_signal(*_):
 global stopping
 stopping=True
signal.signal(signal.SIGTERM,stop_signal)
signal.signal(signal.SIGINT,stop_signal)

def stop_child():
 global child
 if child is not None and child.poll() is None:
  child.terminate()
  try:child.wait(timeout=3)
  except subprocess.TimeoutExpired:
   if child.poll() is None:child.kill()
   child.wait(timeout=3)
 child=None

try:
 deadline=time.monotonic()+480
 while not stopping and time.monotonic()<deadline:
  result=subprocess.run(['ip','-j','link','show','ppp0'],capture_output=True,text=True,timeout=3)
  links=json.loads(result.stdout) if result.returncode==0 else []
  current=links[0]['ifindex'] if len(links)==1 else None
  if current!=identity:
   stop_child();identity=current
   if current is not None:
    subprocess.run(['ip','-6','addr','replace','2001:db8:20::1/64','dev','ppp0'],check=True,timeout=3)
    subprocess.run(['sysctl','-w','net.ipv6.conf.all.forwarding=1'],check=True,capture_output=True,timeout=3)
    child=subprocess.Popen(['dnsmasq','--no-daemon','--conf-file=/dev/null','--port=0','--interface=ppp0','--bind-interfaces','--enable-ra','--dhcp-range=2001:db8:20::,ra-only,64','--ra-param=ppp0,5,60'])
  if child is not None and child.poll() is not None:raise RuntimeError('owned peer RA daemon exited')
  time.sleep(.2)
finally:stop_child()
