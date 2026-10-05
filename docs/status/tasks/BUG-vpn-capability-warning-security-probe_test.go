package desired
import("testing";"strings")
func TestR2PKIGuardsPreserved(t *testing.T){
 ds:=wgState(t,`{"vpn":{"pki":{"hsm":{"enabled":true},"certificates":{"site":{"certificateRef":"cert/site","privateKeyRef":"key/site"}}}}}`)
 s:=&wgSink{};PKI(s,ds,map[string]bool{"vpn":true},PKIOptions{})
 got:=strings.Join(s.issues,"\n")
 for _,want:=range []string{"/vpn/pki/hsm","agent.secret-unavailable /vpn/pki/certificates/site/certificateRef","agent.secret-unavailable /vpn/pki/certificates/site/privateKeyRef"}{if !strings.Contains(got,want){t.Fatalf("guard missing: %s",want)}}
 if len(s.keys)!=0{t.Fatal("unavailable PKI material projected")}
}
