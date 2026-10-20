import re,sys
def parse(path):
    txt=open(path).read().split('\n')
    t0=float(txt[0]); tend=float(txt[-1] if txt[-1].strip() else txt[-2])
    sec=None; port=None; thread=None
    ifc={}; xs={}; rt={}
    for line in txt[1:-1]:
        if line.startswith('## IF'): sec='if'; continue
        if line.startswith('## XS'): sec='xs'; port=line.split()[2]; xs[port]={}; continue
        if line.startswith('## RUNTIME'): sec='rt'; continue
        if line.startswith('## END'): break
        if sec=='if':
            m=re.match(r'^(enp\S+)\s+\d+\s+up\s+\S+\s*(.*)$',line)
            if m: port=m.group(1); ifc[port]={}; rest=m.group(2).strip()
            else: rest=line.strip()
            m2=re.match(r'^([a-z\- ]+?)\s+(\d+)$',rest)
            if m2 and port: ifc[port][m2.group(1).strip()]=int(m2.group(2))
        elif sec=='xs':
            m=re.match(r'^\s+(\w+)\s+(\d+)$',line)
            if m: xs[port][m.group(1)]=int(m.group(2))
        elif sec=='rt':
            m=re.match(r'^Thread (\d+) (\S+)',line)
            if m: thread=m.group(2); rt[thread]={}; continue
            m=re.match(r'^(\S+)\s+(polling|active|any wait|event wait|done|time wait)\s+(\d+)\s+(\d+)\s+(\d+)\s+([\d.e+]+)\s+([\d.]+)',line)
            if m and thread: rt[thread][m.group(1)]=(int(m.group(3)),int(m.group(4)),float(m.group(6)),float(m.group(7)))
    return (t0+tend)/2, ifc, xs, rt
tA,ifA,xsA,rtA=parse(sys.argv[1]); tB,ifB,xsB,rtB=parse(sys.argv[2]); dt=tB-tA
print(f"interval {dt:.1f} s")
print(f"{'port':12} {'rx_unicast(offered)':>19} {'rx_good':>10} {'rx_missed':>10} {'miss%':>6} | {'vpp tx att':>10} {'tx_good':>10} {'tx-error':>10} {'txfail%':>7}  (Mpps)")
tot={}
for p in xsA:
    a,b=xsA[p],xsB[p]; d=lambda k:(b.get(k,0)-a.get(k,0))/dt/1e6
    if 'rx_unicast_packets' not in b and 'tx_good_packets' not in b: continue  # idle port, no xstats
    ia,ib=ifA.get(p,{}),ifB.get(p,{}); di=lambda k:(ib.get(k,0)-ia.get(k,0))/dt/1e6
    off=d('rx_unicast_packets'); miss=d('rx_missed_errors'); good=d('rx_good_packets')
    txatt=di('tx packets'); txg=d('tx_good_packets'); txe=di('tx-error')
    print(f"{p:12} {off:19.3f} {good:10.3f} {miss:10.3f} {100*miss/off if off else 0:6.1f} | {txatt:10.3f} {txg:10.3f} {txe:10.3f} {100*txe/txatt if txatt else 0:7.1f}")
    chip=p[:5]; c=tot.setdefault(chip,[0,0,0,0,0,0]); 
    for i,v in enumerate((off,good,miss,txatt,txg,txe)): c[i]+=v
for chip,c in tot.items():
    print(f"{chip+' total':12} {c[0]:19.3f} {c[1]:10.3f} {c[2]:10.3f} {100*c[2]/c[0] if c[0] else 0:6.1f} | {c[3]:10.3f} {c[4]:10.3f} {c[5]:10.3f} {100*c[5]/c[3] if c[3] else 0:7.1f}")
print()
print("per-thread node deltas (calls/s, vectors/s, vectors/call):")
nodes=['dpdk-input','ethernet-input','l2-input','l2-output']
for th in rtA:
    if th=='vpp_main': continue
    out=[]
    for n in nodes+[k for k in rtA[th] if k.endswith('-tx')]:
        if n in rtA[th] and n in rtB[th]:
            ca,va=rtA[th][n][0],rtA[th][n][1]; cb,vb=rtB[th][n][0],rtB[th][n][1]
            dc=(cb-ca)/dt; dv=(vb-va)/dt
            out.append(f"{n}: {dc/1e6:.2f}M calls/s {dv/1e6:.3f}M vec/s {dv/dc if dc else 0:.2f} v/c")
    print(th); [print("   ",o) for o in out]
# doorbells per port = sum over threads of tx calls
print()
print("TX node calls/s (doorbells) and vectors/call per port, summed over threads:")
for p in xsA:
    n=p+'-tx'; dc=dv=0
    for th in rtA:
        if n in rtA[th] and n in rtB[th]:
            dc+=(rtB[th][n][0]-rtA[th][n][0])/dt; dv+=(rtB[th][n][1]-rtA[th][n][1])/dt
    print(f"  {p}: {dc/1e6:.2f} M tx calls/s, {dv/1e6:.3f} M pkts/s to tx node, {dv/dc if dc else 0:.2f} pkts per call")
