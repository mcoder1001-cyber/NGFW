import json, pathlib, sys
w=pathlib.Path('/root/ngfw-half-resources-20261014')
b=json.loads((w/'before.json').read_text());d=b['doc']['dataplane']
is211=sys.argv[1]=='211'
d.update(workers=9 if is211 else 21, mainCore=16 if is211 else 1,corelist=list(range(2,10))+[17] if is211 else list(range(2,12))+list(range(23,34)),hugepagesGb=15 if is211 else 7,buffersPerNuma=1048576 if is211 else 524288)
for pci,n in d['devices'].items():
 q=8 if is211 and int(pci[5:7],16)<=3 else 4
 n.update(rxQueues=q,txQueues=q,rxDesc=2048,txDesc=2048)
assert not set(d['devices']).intersection(d['managementPci'])
(w/'plan.json').write_text(json.dumps({'dataplane':d},indent=2));(w/'plan.json').chmod(0o600)
print(json.dumps({k:d[k] for k in ['workers','mainCore','corelist','hugepagesGb','buffersPerNuma']}))
