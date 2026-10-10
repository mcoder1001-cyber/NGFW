#!/usr/bin/env python3
"""Manager-only recovery of the observed exact empty legacy cache, no activation."""
import hashlib,json,os,re,stat,subprocess,sys

def run(*cmd):
    return subprocess.check_output(cmd,text=True).strip()
def sha(b):
    return hashlib.sha256(b).hexdigest()
def guard():
    assert run('cat','/proc/sys/kernel/random/boot_id')=='3a609803-be4e-46eb-bfc6-7dcfe385cf50'
    assert run('systemctl','is-active','vpp.service')=='active'
    assert run('systemctl','show','vpp.service','--value','-p','MainPID')=='33868'
    assert run('systemctl','show','vpp.service','--value','-p','NRestarts')=='0'
    for unit in ('ngfw-api.service','ngfw-agent.service'):
        assert run('systemctl','show',unit,'--value','-p','ActiveState')=='inactive'
        assert run('systemctl','show',unit,'--value','-p','MainPID')=='0'
    assert os.path.basename(os.readlink('/sys/bus/pci/devices/0000:04:00.0/driver'))=='igc'
    assert os.path.basename(os.readlink('/sys/bus/pci/devices/0000:04:00.0/iommu_group'))=='28'
    assert sha(open('/etc/vpp/startup.conf','rb').read())=='367ead293aefd84d4b3f85f882d3dac33834bda9129467c9a1578f7223a39184'
    assert sha(open('/usr/sbin/ngfw-agent','rb').read())==sys.argv[1]
    for line in open('/etc/ngfw/agent.env'):
        if line.startswith('NGFW_OWNER='):
            assert line.strip().split('=',1)[1].strip('"\'')=='ngfw'
        if line.startswith('NGFW_AGENT_STATE_DIR='):
            assert line.strip().split('=',1)[1].strip('"\'')=='/var/lib/ngfw/agent'

def snapshot():
    return [json.loads(run('ip','-j',*args)) for args in
            [('address','show'),('-4','route','show','table','all'),
             ('-6','route','show','table','all'),('-4','rule','show'),('-6','rule','show')]]

assert len(sys.argv)==2 and re.fullmatch('[0-9a-f]{64}',sys.argv[1])
guard()
network=snapshot()
d=os.open('/var/lib/ngfw/agent',os.O_DIRECTORY|os.O_NOFOLLOW)
f=os.open('auto-block.json',os.O_RDONLY|os.O_NOFOLLOW,dir_fd=d)
s=os.fstat(f); b=os.read(f,4096)
assert stat.S_ISREG(s.st_mode) and s.st_nlink==1 and s.st_uid==0 and s.st_gid==107
assert stat.S_IMODE(s.st_mode)==0o600 and s.st_size==2 and b==b'{}'
backup='/var/lib/ngfw-install-recovery/hardware-manager-20261010-plugin-211/auto-block.json.before'
q=os.open(backup,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
os.write(q,b);os.fsync(q);os.close(q)
parent=os.open(os.path.dirname(backup),os.O_DIRECTORY|os.O_NOFOLLOW)
os.fsync(parent);os.close(parent)
guard();assert snapshot()==network
assert os.stat('auto-block.json',dir_fd=d,follow_symlinks=False).st_ino==s.st_ino
new=b'{"owner":"ngfw"}'
t=os.open('.auto-block-owner-recovery',os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600,dir_fd=d)
os.fchown(t,s.st_uid,s.st_gid);os.write(t,new);os.fsync(t);os.close(t)
os.rename('.auto-block-owner-recovery','auto-block.json',src_dir_fd=d,dst_dir_fd=d);os.fsync(d)
os.close(f);os.close(d)
assert open('/var/lib/ngfw/agent/auto-block.json','rb').read()==new
z=os.stat('/var/lib/ngfw/agent/auto-block.json',follow_symlinks=False)
assert (z.st_uid,z.st_gid,stat.S_IMODE(z.st_mode))==(0,107,0o600)
guard();assert snapshot()==network
print(json.dumps({'normalized':True,'owner':'ngfw','entries':0,'backup_sha256':sha(b),'new_sha256':sha(new),'uid':0,'gid':107,'mode':'0600','network_equal':True,'services_started':False}))
