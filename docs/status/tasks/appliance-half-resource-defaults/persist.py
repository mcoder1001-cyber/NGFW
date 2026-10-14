import json,pathlib,subprocess,re
w=pathlib.Path('/root/ngfw-half-resources-20261014');dp=json.loads((w/'plan.json').read_text())['dataplane']
assert any(x.name=='committed' for p in pathlib.Path('/var/lib/ngfw/startup-apply').iterdir() if p.is_dir() for x in p.iterdir())
conf=pathlib.Path('/etc/sysctl.d/80-vpp.conf');text=conf.read_text();text,n=re.subn(r'^vm.nr_hugepages\s*=.*$',f'vm.nr_hugepages={dp["hugepagesGb"]*512}',text,flags=re.M);assert n==1;conf.write_text(text)
config=pathlib.Path('/etc/ngfw/dataplane-performance.json');config.write_text(json.dumps({'cores':[dp['mainCore']]+dp['corelist']},indent=2));config.chmod(0o644)
lib=pathlib.Path('/usr/local/libexec');lib.mkdir(exist_ok=True)
p=lib/'ngfw-dataplane-performance';p.write_bytes((w/'performance.py').read_bytes());p.chmod(0o755)
u=pathlib.Path('/etc/systemd/system/ngfw-dataplane-performance.service')
u.write_text('[Unit]\nDescription=NGFW reviewed dataplane CPU frequency defaults\nAfter=systemd-sysctl.service\nBefore=vpp.service\n\n[Service]\nType=oneshot\nExecStart=/usr/local/libexec/ngfw-dataplane-performance\nRemainAfterExit=yes\n')
d=pathlib.Path('/etc/systemd/system/vpp.service.d');d.mkdir(exist_ok=True)
(d/'40-resource-defaults.conf').write_text('[Unit]\nRequires=ngfw-dataplane-performance.service\nAfter=ngfw-dataplane-performance.service\n\n[Service]\nLimitMEMLOCK=infinity\n')
if dp['managementPci']==['0000:04:00.0']:
 p=lib/'ngfw-hardware-211-bind';p.write_bytes((w/'bind-211.py').read_bytes());p.chmod(0o755)
 u=pathlib.Path('/etc/systemd/system/ngfw-hardware-211-bind.service')
 u.write_text('[Unit]\nDescription=NGFW reviewed per-device data NIC binding (host211)\nAfter=network.target\nBefore=vpp.service\n\n[Service]\nType=oneshot\nExecStart=/usr/local/libexec/ngfw-hardware-211-bind\nRemainAfterExit=yes\nTimeoutStartSec=180\nUMask=0077\n')
 (d/'20-hardware-211-bind.conf').write_text('[Unit]\nRequires=ngfw-hardware-211-bind.service\nAfter=ngfw-hardware-211-bind.service\n')
subprocess.run(['systemctl','daemon-reload'],check=True)
subprocess.run(['systemctl','start','ngfw-dataplane-performance.service'],check=True)
pid=subprocess.check_output(['systemctl','show','vpp','-p','MainPID','--value'],text=True).strip();assert int(pid)>0
subprocess.run(['prlimit','--pid',pid,'--memlock=unlimited:unlimited'],check=True)
print(json.dumps({'persistent_hugepages':dp['hugepagesGb']*512,'cpu_policies':len(dp['corelist'])+1,'memlock':'unlimited','bindings':'per-device boot service'}))
