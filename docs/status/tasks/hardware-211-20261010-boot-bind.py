#!/usr/bin/env python3
"""Operational .211 per-device binding candidate; MANAGER INSTALL/EXECUTION ONLY.

No vendor/global new_id, driverctl files, unsafe mode or shared IOMMU groups.
The manager must preserve original driver/override/link/bridge evidence and
review exact source before installing this script and the VPP dependency.
"""
import json,os,pathlib,re,stat,subprocess,time
MGMT='0000:04:00.0'
NICS=[('0000:01:00.0',16,'i40e'),('0000:01:00.1',17,'i40e'),('0000:01:00.2',18,'i40e'),('0000:01:00.3',19,'i40e'),('0000:02:00.0',20,'i40e'),('0000:02:00.1',21,'i40e'),('0000:02:00.2',22,'i40e'),('0000:02:00.3',23,'i40e'),('0000:03:00.0',24,'i40e'),('0000:03:00.1',25,'i40e'),('0000:03:00.2',26,'i40e'),('0000:03:00.3',27,'i40e'),('0000:05:00.0',29,'igc'),('0000:06:00.0',30,'igc'),('0000:07:00.0',31,'igc'),('0000:08:00.0',32,'igc'),('0000:09:00.0',33,'igc')]
def checked(args):
 p=subprocess.run(args,capture_output=True,text=True,timeout=30)
 if p.returncode:raise RuntimeError('command failed: '+repr(args)+' exit '+str(p.returncode))
 return p.stdout
def write(path,value):
 with open(path,'w') as f:f.write(value+'\n')
def group(p,want,pci):
 g=(p/'iommu_group').resolve(strict=True)
 assert g.name==str(want) and sorted(x.name for x in (g/'devices').iterdir())==[pci]
def management():
 d=pathlib.Path('/sys/class/net/enp4s0/device')
 assert d.resolve(strict=True).name==MGMT and (d/'driver').resolve(strict=True).name=='igc'
 group(d,28,MGMT)
 addresses=json.loads(checked(['ip','-j','addr','show','dev','enp4s0']))
 assert any(a.get('local')=='172.30.110.211' and a.get('prefixlen')==24 for n in addresses for a in n.get('addr_info',[]))
 routes=json.loads(checked(['ip','-j','-4','route','show','table','all']))
 assert any(r.get('dst')=='default' and r.get('dev')=='enp4s0' and r.get('gateway')=='172.30.110.1' for r in routes)
def unsafe_gate():
 p=pathlib.Path('/sys/module/vfio/parameters/enable_unsafe_noiommu_mode')
 assert not p.exists() or p.read_text().strip()=='N'
def module_options_gate():
 # Fail closed on configured install hooks or module options rather than
 # silently accepting vfio_pci.ids global registration at module load.
 config=checked(['modprobe','-c'])
 assert not any(re.match(r'^(options|install)\s+vfio(?:[-_]|\s|$)',x) for x in config.splitlines())
 assert not any('vfio' in x for x in pathlib.Path('/proc/cmdline').read_text().split())
def main():
 os.umask(0o077)
 assert os.geteuid()==0 and len(NICS)==17 and len({p for p,g,d in NICS})==17 and MGMT not in {p for p,g,d in NICS}
 assert checked(['systemctl','show','vpp.service','-p','ActiveState','--value']).strip()=='inactive','VPP must be stopped before changing data drivers'
 for attempt in range(30):
  try:management();break
  except (AssertionError,FileNotFoundError,RuntimeError):
   if attempt==29:raise
   time.sleep(1)
 unsafe_gate();module_options_gate()
 before_addr=json.loads(checked(['ip','-j','addr']))
 before_routes=json.loads(checked(['ip','-j','-4','route','show','table','all']))+json.loads(checked(['ip','-j','-6','route','show','table','all']))
 # Validate the complete scope before the first module/network/driver write.
 inventory=[]
 for pci,want,original in NICS:
  p=pathlib.Path('/sys/bus/pci/devices')/pci;group(p,want,pci)
  drv=(p/'driver').resolve(strict=True).name;assert drv in [original,'vfio-pci']
  assert (p/'driver_override').read_text().strip() in ['','(null)','vfio-pci']
  assert not os.path.lexists('/etc/driverctl.d/pci-'+pci),'global-ID driverctl persistence is excluded'
  net=p/'net';names=sorted(x.name for x in net.iterdir()) if net.exists() else []
  assert len(names)<=1 and (drv!='vfio-pci' or not names)
  for name in names:
   assert all(not x.get('addr_info') for x in before_addr if x['ifname']==name)
   assert all(r.get('dev')!=name for r in before_routes)
   master=pathlib.Path('/sys/class/net')/name/'master'
   if master.is_symlink():
    bridge=master.resolve(strict=True).name
    assert all(not x.get('addr_info') for x in before_addr if x['ifname']==bridge)
    assert all(r.get('dev')!=bridge for r in before_routes)
  inventory.append((pci,p,drv,names))
 checked(['modprobe','--ignore-install','vfio','enable_unsafe_noiommu_mode=0'])
 checked(['modprobe','--ignore-install','vfio-pci','ids=']);unsafe_gate()
 for pci,p,drv,names in inventory:
  management();unsafe_gate()
  if drv=='vfio-pci':continue
  for name in names:
   master=pathlib.Path('/sys/class/net')/name/'master'
   if master.is_symlink():checked(['ip','link','set','dev',name,'nomaster'])
   checked(['ip','link','set','dev',name,'down'])
  write(p/'driver_override','vfio-pci')
  write(p/'driver/unbind',pci)
  write(pathlib.Path('/sys/bus/pci/drivers_probe'),pci)
  assert (p/'driver').resolve(strict=True).name=='vfio-pci'
  management();group(p,next(g for x,g,d in NICS if x==pci),pci);unsafe_gate()
 for pci,want,original in NICS:
  p=pathlib.Path('/sys/bus/pci/devices')/pci
  assert (p/'driver').resolve(strict=True).name=='vfio-pci' and (p/'driver_override').read_text().strip()=='vfio-pci'
  assert stat.S_ISCHR(pathlib.Path('/dev/vfio/'+str(want)).stat().st_mode)
 management();unsafe_gate();print(json.dumps({'bound_count':17,'protected_management':True,'unsafe_noiommu':False,'global_new_id_or_driverctl_persistence':False}))
if __name__=='__main__':main()
