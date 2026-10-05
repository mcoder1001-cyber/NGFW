#!/usr/bin/env python3
"""Run separate certificate-auth production packet proof without editing PSK fixture."""
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[4]

def replace_once(source, old, new):
    if source.count(old) != 1:
        raise SystemExit('fixture anchor changed; review required')
    return source.replace(old, new, 1)

def main():
    conf = Path('/run/vpp/startup.conf')
    info = conf.lstat()
    if (os.environ.get('NGFW_DISPOSABLE_VPP') != '1' or os.environ.get('NGFW_TEST_PREFIX') != 'w8'
        or not stat.S_ISREG(info.st_mode) or info.st_uid != 0
        or os.readlink('/proc/self/ns/mnt') == os.readlink('/proc/1/ns/mnt')
        or not re.search(r'^api-segment \{ prefix fulltest[0-9]+ \}$', conf.read_text(), re.M)):
        raise SystemExit('requires marked private disposable VPP and slot8')
    if not os.environ.get('NGFW_NATIVE_AGENT_BIN') or os.environ.get('NGFW_NATIVE_INITIATOR') != '1':
        raise SystemExit('requires production agent and native initiator')
    if os.environ.get('NGFW_NATIVE_PEER_LOSS') == '1':
        raise SystemExit('peer crash is not part of this certificate fixture')
    original = ROOT / 'apps/agent/internal/desired/ikev2_integration_test.go'
    shared_before = subprocess.check_output(["systemctl","show","vpp","-p","MainPID","-p","NRestarts"])
    startup_before = hashlib.sha256(Path("/etc/vpp/startup.conf").read_bytes()).hexdigest()
    source = original.read_text()
    source = replace_once(source, 'func TestIKEv2NativePackets(t *testing.T)', 'func TestIKEv2NativeCertificatePackets(t *testing.T)')
    source = replace_once(source, '"crypto/rand"', '"crypto/rand"\n"crypto/rsa"\n"crypto/x509"\n"crypto/x509/pkix"\n"encoding/pem"\n"math/big"')
    source = replace_once(source, '\tds := nativeDoc(t)', '''	values, peerCert, peerKey, caCert := certPeerMaterial(t)
	defer clear(peerKey)
	for _, value := range values { defer clear(value) }
	resolver = vpn.NewMapResolver(key, values["cert/local"], values["key/local"], values["cert/peer"])
	env.GlobalsOwner=true
	env.NativeRoot=filepath.Join(t.TempDir(),"native")
	env.Resolve=resolver.Resolve
	env.SecretRef=func(_ context.Context,ref string)(string,error){return key.Ref(values[ref]),nil}
	ds := nativeDoc(t)
	ds.Vpn.Pki=&ngfwv1.PkiConfig{Certificates:map[string]*ngfwv1.PkiCertificate{"local":{CertificateRef:proto.String("cert/local"),PrivateKeyRef:proto.String("key/local")},"peer":{CertificateRef:proto.String("cert/peer")}}}
	ds.Vpn.Ipsec.Tunnels["site"].Auth=&ngfwv1.IpsecAuth{Method:proto.String("cert"),Certificate:proto.String("local"),PeerCertificate:proto.String("peer")}
''')
    source = replace_once(source, '"NGFW_GLOBALS_OWNER=0"', '"NGFW_GLOBALS_OWNER=1"')
    source = replace_once(source, 'map[string][]byte{"psk/site": append([]byte(nil), material...)}', 'values')
    source = replace_once(source, '\tpt.Engine = proto.String("strongswan")', '\tpt.Engine = proto.String("strongswan")\n\tpt.Auth.Certificate=proto.String("peer")\n\tpt.Auth.RemoteCa=proto.String("ca")\n\tpt.Auth.PeerCertificate=nil')
    source = replace_once(source, '\tr := newPeerRenderer()', '''	for name,data := range map[string][]byte{filepath.Join(h.Paths("native-peer").CertDir("certs"),"peer.pem"):peerCert,filepath.Join(h.Paths("native-peer").CertDir("cacerts"),"ca.pem"):caCert} {
	 if err:=os.MkdirAll(filepath.Dir(name),0700);err!=nil {t.Fatal(err)}
	 if err:=os.WriteFile(name,data,0600);err!=nil {t.Fatal(err)}
	}
	r := newPeerRenderer()''')
    legacy = os.environ.get('NGFW_CERT_PEER_LEGACY') == '1'
    if legacy:
        source = replace_once(source, '\tif _, e = h.Start(ctx, "native-peer", peer, files[h.Paths("native-peer").StrongswanConf].Content); e != nil {', '\tpeerConf := files[h.Paths("native-peer").StrongswanConf]\n\tif bytes.Count(peerConf.Content, []byte("charon {")) != 1 {t.Fatal("private peer settings anchor changed")}\n\tpeerConf.Content = bytes.Replace(peerConf.Content, []byte("charon {"), []byte("charon {\\n\\tsignature_authentication = no"), 1)\n\tif _, e = h.Start(ctx, "native-peer", peer, peerConf.Content); e != nil {')
    print('CERT_PEER_SIGNATURE_MODE=' + ('legacy_RSA_SIG_SHA1' if legacy else 'stock_default_RFC7427'), flush=True)
    source = replace_once(source, '\tif e = r.Apply(ctx, files); e != nil {', '\tcertPeerLoadKey(t,ctx,h.Paths("native-peer").ViciSocket,peerKey)\n\tcertPeerLoadCertificate(t,ctx,h.Paths("native-peer").ViciSocket,caCert,"CA")\n\tcertPeerLoadCertificate(t,ctx,h.Paths("native-peer").ViciSocket,values["cert/local"],"NONE")\n\tdefer func(){ if raw,err:=os.ReadFile(h.Paths("native-peer").LogFile);err==nil {t.Logf("private certificate peer log: %s",raw)} }()\n\tif e = r.Apply(ctx, files); e != nil {')
    source = replace_once(source, '\t\tif bytes.Contains(raw, material) {', '\t\tfor _,value:=range values { if bytes.Contains(raw,value) {t.Fatal("production state echoed certificate material")} }\n\t\tif bytes.Contains(raw, material) {')
    helper = (Path(__file__).parent / 'helper.go').read_text().split('package desired\n', 1)[1]
    source += helper
    with tempfile.TemporaryDirectory(prefix='cert-peer-', dir=ROOT / '.scratch') as directory:
        staged = Path(directory) / 'cert_test.go'
        staged.write_text(source)
        subprocess.run(['gofmt', '-w', str(staged)], check=True)
        overlay = Path(directory) / 'overlay.json'
        overlay.write_text(json.dumps({'Replace': {str(original): str(staged)}}))
        print('CERT_FIXTURE_ORIGINAL_SHA256=' + hashlib.sha256(original.read_bytes()).hexdigest(), flush=True)
        print('CERT_FIXTURE_OVERLAY_SHA256=' + hashlib.sha256(staged.read_bytes()).hexdigest(), flush=True)
        result = subprocess.run(['go', 'test', '-overlay', str(overlay), './internal/desired', '-run', '^TestIKEv2NativeCertificatePackets$', '-v', '-count=1', '-timeout', '4m'], cwd=ROOT / 'apps/agent', stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
        if "-----BEGIN " in result.stdout:
            raise SystemExit("certificate material unexpectedly appeared in test output; output withheld")
        if shared_before != subprocess.check_output(["systemctl","show","vpp","-p","MainPID","-p","NRestarts"]) or startup_before != hashlib.sha256(Path("/etc/vpp/startup.conf").read_bytes()).hexdigest():
            raise SystemExit("shared VPP or startup file changed")
        print("SHARED_VPP_UNCHANGED=" + shared_before.decode().strip().replace("\n",","),flush=True)
        print(result.stdout, end='', flush=True)
        if result.returncode or '--- PASS: TestIKEv2NativeCertificatePackets' not in result.stdout or '--- SKIP:' in result.stdout:
            raise SystemExit(result.returncode or 1)
        print('PRODUCTION_CERTIFICATE_PEER_PACKETS=PASS', flush=True)

if __name__ == '__main__':
    main()
