#!/usr/bin/env python3
"""Operational .37 per-device binding candidate; MANAGER INSTALL/EXECUTION ONLY.

No vendor/global new_id, driverctl files, unsafe mode or shared IOMMU groups.
The manager must preserve original driver/override/link/bridge evidence and
review exact source before installing this script and the VPP dependency.
"""
import json,os,pathlib,re,stat,subprocess,time
MGMT='0000:0c:00.0'
NICS=[('0000:0a:00.0', 56, 'igc'), ('0000:0b:00.0', 57, 'igc'), ('0000:0d:00.0', 59, 'igc'), ('0000:0e:00.0', 60, 'igc'), ('0000:0f:00.0', 61, 'igc'), ('0000:10:00.0', 62, 'igc'), ('0000:11:00.0', 63, 'igc')]
IDS_OBSERVATIONS=[]
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
 d=pathlib.Path('/sys/class/net/enp12s0/device')
 assert d.resolve(strict=True).name==MGMT and (d/'driver').resolve(strict=True).name=='igc'
 group(d,58,MGMT)
 addresses=json.loads(checked(['ip','-j','addr','show','dev','enp12s0']))
 assert any(a.get('local')=='172.30.126.37' and a.get('prefixlen')==24 for n in addresses for a in n.get('addr_info',[]))
 routes=json.loads(checked(['ip','-j','-4','route','show','table','all']))
 assert any(r.get('dst')=='default' and r.get('dev')=='enp12s0' and r.get('gateway')=='172.30.126.1' for r in routes)
def unsafe_gate():
 p=pathlib.Path('/sys/module/vfio/parameters/enable_unsafe_noiommu_mode')
 assert not p.exists() or p.read_text().strip()=='N'
def ids_gate(phase):
 p=pathlib.Path('/sys/module/vfio_pci/parameters/ids')
 if p.exists():
  value=p.read_text().strip();assert not value,'exposed global VFIO IDs must be empty'
  status='exposed_empty'
 else:
  # Linux can make this parameter unexposed. Its absence is not a measured
  # empty value; option/cmdline refusal and explicit empty load remain gates.
  status='loaded_parameter_unexposed' if p.parents[1].exists() else 'module_not_loaded'
 IDS_OBSERVATIONS.append({'phase':phase,'status':status,'empty_value_observed':status=='exposed_empty'})
def module_options_gate():
 # Fail closed on configured install hooks or module options rather than
 # silently accepting vfio_pci.ids global registration at module load.
 config=checked(['modprobe','-c'])
 assert not any(re.match(r'^(options|install)\s+vfio(?:[-_]|\s|$)',x) for x in config.splitlines())
 assert not any('vfio' in x for x in pathlib.Path('/proc/cmdline').read_text().split())
def main():
 os.umask(0o077)
 assert os.geteuid()==0 and len(NICS)==7 and len({p for p,g,d in NICS})==7 and MGMT not in {p for p,g,d in NICS}
 assert checked(['systemctl','show','vpp.service','-p','ActiveState','--value']).strip()=='inactive','VPP must be stopped before changing data drivers'
 for attempt in range(30):
  try:management();break
  except (AssertionError,FileNotFoundError,RuntimeError):
   if attempt==29:raise
   time.sleep(1)
 unsafe_gate();ids_gate('before_module_load');module_options_gate()
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
 checked(['modprobe','--ignore-install','vfio-pci','ids=']);unsafe_gate();ids_gate('after_module_load')
 for pci,p,drv,names in inventory:
  management();unsafe_gate();ids_gate('before_device_'+pci)
  if drv=='vfio-pci':ids_gate('after_device_'+pci);continue
  for name in names:
   master=pathlib.Path('/sys/class/net')/name/'master'
   if master.is_symlink():checked(['ip','link','set','dev',name,'nomaster'])
   checked(['ip','link','set','dev',name,'down'])
  write(p/'driver_override','vfio-pci')
  write(p/'driver/unbind',pci)
  write(pathlib.Path('/sys/bus/pci/drivers_probe'),pci)
  assert (p/'driver').resolve(strict=True).name=='vfio-pci'
  management();group(p,next(g for x,g,d in NICS if x==pci),pci);unsafe_gate();ids_gate('after_device_'+pci)
 for pci,want,original in NICS:
  p=pathlib.Path('/sys/bus/pci/devices')/pci
  assert (p/'driver').resolve(strict=True).name=='vfio-pci' and (p/'driver_override').read_text().strip()=='vfio-pci'
  assert stat.S_ISCHR(pathlib.Path('/dev/vfio/'+str(want)).stat().st_mode)
 management();unsafe_gate();ids_gate('final');print(json.dumps({'bound_count':7,'protected_management':True,'unsafe_noiommu':False,'global_new_id_or_driverctl_persistence':False,'ids_parameter_observations':IDS_OBSERVATIONS}))
if __name__=='__main__':main()
