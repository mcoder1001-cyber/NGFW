#!/usr/bin/env python3
"""Apply only CPU frequency policies selected by the reviewed dataplane budget."""
import json,os,pathlib
assert os.geteuid()==0
def cpus(text):
 result=set()
 for token in text.replace(',', ' ').split():
  parts=token.split('-'); result.update(range(int(parts[0]),int(parts[-1])+1))
 return result
p=json.loads(pathlib.Path('/etc/ngfw/dataplane-performance.json').read_text())
selected=set(p['cores'])
for cpu in sorted(selected):
 assert isinstance(cpu,int) and 0<=cpu<=1023
 d=pathlib.Path(f'/sys/devices/system/cpu/cpu{cpu}/cpufreq')
 if not d.exists():continue
 if not cpus((d/'related_cpus').read_text()).issubset(selected):
  print(f'Skipped CPU {cpu}: shared policy extends outside VPP budget');continue
 if 'performance' in (d/'scaling_available_governors').read_text().split():
  (d/'scaling_governor').write_text('performance\n')
print('Applied available performance governors to assigned VPP CPU policies')
